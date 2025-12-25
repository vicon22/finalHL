package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metric struct {
	Timestamp int64   `json:"timestamp"`
	CPU       float64 `json:"cpu"`
	RPS       float64 `json:"rps"`
	DeviceID  string  `json:"device_id"`
}

type Analytics struct {
	mu              sync.RWMutex
	window          []Metric
	windowSize      int
	rollingAvg      float64
	mean            float64
	stdDev          float64
	anomalyCount    int64
	totalProcessed  int64
	anomalyThreshold float64
	metricChan      chan Metric
	stopChan        chan struct{}
}

func NewAnalytics(windowSize int, threshold float64) *Analytics {
	a := &Analytics{
		window:          make([]Metric, 0, windowSize),
		windowSize:      windowSize,
		anomalyThreshold: threshold,
		metricChan:      make(chan Metric, 100),
		stopChan:        make(chan struct{}),
	}
	go a.processMetrics()
	return a
}

func (a *Analytics) AddMetric(m Metric) {
	select {
	case a.metricChan <- m:
	default:
		log.Printf("Analytics channel full, metric dropped")
	}
}

func (a *Analytics) processMetrics() {
	for {
		select {
		case metric := <-a.metricChan:
			go a.computeStats(metric)
		case <-a.stopChan:
			return
		}
	}
}

func (a *Analytics) computeStats(m Metric) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.window) >= a.windowSize {
		a.window = a.window[1:]
	}
	a.window = append(a.window, m)

	a.totalProcessed++

	// Вычисляем rolling average для RPS (окно 50 событий)
	sum := 0.0
	for _, metric := range a.window {
		sum += metric.RPS
	}
	a.rollingAvg = sum / float64(len(a.window))

	// Вычисляем mean и stdDev для z-score (sliding window 50 событий)
	if len(a.window) >= 10 {
		// Вычисление среднего
		mean := 0.0
		for _, metric := range a.window {
			mean += metric.RPS
		}
		mean /= float64(len(a.window))
		a.mean = mean

		// Вычисление стандартного отклонения
		variance := 0.0
		for _, metric := range a.window {
			variance += (metric.RPS - mean) * (metric.RPS - mean)
		}
		variance /= float64(len(a.window))
		stdDev := 0.0
		if variance > 0 {
			stdDev = math.Sqrt(variance)
		}
		a.stdDev = stdDev

		// Детекция аномалий по z-score (threshold=2)
		if a.stdDev > 0 {
			zScore := (m.RPS - a.mean) / a.stdDev
			if zScore > a.anomalyThreshold || zScore < -a.anomalyThreshold {
				a.anomalyCount++
			}
		}
	}
}

func (a *Analytics) Stop() {
	close(a.stopChan)
}

func (a *Analytics) GetStats() (rollingAvg, mean, stdDev float64, anomalyCount, totalProcessed int64) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.rollingAvg, a.mean, a.stdDev, a.anomalyCount, a.totalProcessed
}

type Service struct {
	redis     *redis.Client
	analytics *Analytics
	metrics   *Metrics
}

type Metrics struct {
	requestsTotal    prometheus.Counter
	requestDuration  prometheus.Histogram
	anomaliesTotal   prometheus.Counter
	rpsGauge         prometheus.Gauge
	rollingAvgGauge  prometheus.Gauge
}

func NewMetrics() *Metrics {
	return &Metrics{
		requestsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		}),
		requestDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		anomaliesTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "anomalies_detected_total",
			Help: "Total number of anomalies detected",
		}),
		rpsGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "current_rps",
			Help: "Current requests per second",
		}),
		rollingAvgGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rolling_average_rps",
			Help: "Rolling average RPS over window",
		}),
	}
}

func (m *Metrics) Register() {
	prometheus.MustRegister(m.requestsTotal)
	prometheus.MustRegister(m.requestDuration)
	prometheus.MustRegister(m.anomaliesTotal)
	prometheus.MustRegister(m.rpsGauge)
	prometheus.MustRegister(m.rollingAvgGauge)
}

func NewService(redisAddr string) (*Service, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	analytics := NewAnalytics(50, 2.0) // window size 50, threshold 2σ
	metrics := NewMetrics()
	metrics.Register()

	return &Service{
		redis:     rdb,
		analytics: analytics,
		metrics:   metrics,
	}, nil
}

func (s *Service) handleMetrics(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.metrics.requestsTotal.Inc()
	defer func() {
		s.metrics.requestDuration.Observe(time.Since(start).Seconds())
	}()

	var metric Metric
	if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if metric.Timestamp == 0 {
		metric.Timestamp = time.Now().Unix()
	}

	ctx := context.Background()
	key := fmt.Sprintf("metric:%s:%d", metric.DeviceID, metric.Timestamp)
	data, _ := json.Marshal(metric)
	if err := s.redis.Set(ctx, key, data, 5*time.Minute).Err(); err != nil {
		log.Printf("Redis set error: %v", err)
	}

	s.analytics.AddMetric(metric)

	rollingAvg, _, _, _, _ := s.analytics.GetStats()
	s.metrics.rpsGauge.Set(metric.RPS)
	s.metrics.rollingAvgGauge.Set(rollingAvg)

	_, mean, stdDev, _, _ := s.analytics.GetStats()
	if stdDev > 0 {
		zScore := (metric.RPS - mean) / stdDev
		if zScore > 2.0 || zScore < -2.0 {
			s.metrics.anomaliesTotal.Inc()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"metric": metric,
	})
}

func (s *Service) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	rollingAvg, mean, stdDev, anomalyCount, totalProcessed := s.analytics.GetStats()

	anomalyRate := 0.0
	if totalProcessed > 0 {
		anomalyRate = float64(anomalyCount) / float64(totalProcessed) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rolling_average": rollingAvg,
		"mean":            mean,
		"std_dev":         stdDev,
		"anomaly_count":   anomalyCount,
		"total_processed": totalProcessed,
		"anomaly_rate":    anomalyRate,
		"window_size":     s.analytics.windowSize,
	})
}

func (s *Service) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := s.redis.Ping(ctx).Err(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func main() {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	service, err := NewService(redisAddr)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	r := mux.NewRouter()
	r.HandleFunc("/metrics", service.handleMetrics).Methods("POST")
	r.HandleFunc("/analyze", service.handleAnalyze).Methods("GET")
	r.HandleFunc("/health", service.handleHealth).Methods("GET")
	r.Handle("/prometheus", promhttp.Handler())

	r.PathPrefix("/debug/pprof/").Handler(http.DefaultServeMux)

	log.Printf("Starting server on port %s", port)
	log.Printf("pprof доступен на http://localhost:%s/debug/pprof/", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

