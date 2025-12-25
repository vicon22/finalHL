# Высоконагруженный сервис с ИИ-оптимизацией на Go в Kubernetes

Проект представляет собой высоконагруженный сервис на языке Go для обработки потоковых данных от IoT-устройств с интегрированной аналитикой, кэшированием в Redis и мониторингом через Prometheus.

## Архитектура

- **Go сервис**: HTTP API для приема метрик, аналитика на основе rolling average и z-score
- **Redis**: Кэширование метрик
- **Prometheus**: Сбор метрик для мониторинга
- **Grafana**: Визуализация метрик (опционально)
- **Kubernetes**: Оркестрация с HPA для автоматического масштабирования

## Функциональность

### Эндпоинты

- `POST /metrics` - Прием метрик от IoT устройств
- `GET /analyze` - Получение аналитики (rolling average, аномалии)
- `GET /health` - Проверка здоровья сервиса
- `GET /prometheus` - Метрики Prometheus

### Аналитика

1. **Rolling Average**: Сглаживание нагрузки по окну из 50 последних событий
2. **Z-Score детекция аномалий**: Обнаружение аномалий при отклонении > 2σ от среднего

## Требования

- Go 1.22+
- Docker
- Kubernetes (Minikube или Kind)
- kubectl

## Быстрый старт

### Развертывание в Kubernetes

#### 1. Запуск Minikube

```bash
minikube start --cpus=2 --memory=4g
```

#### 2. Развертывание

```bash
# 1. Запуск Minikube
minikube start --cpus=2 --memory=4g

eval $(minikube docker-env)
docker build -t go-service:latest .

kubectl apply -f k8s
```

#### 3. Доступ к сервису

```bash
minikube tunnel

minikube addons enable ingress

kubectl port-forward svc/ingress-nginx-controller 8080:80 -n ingress-nginx

# В другом терминале
curl http://localhost:8080/health
```

Ожидаемый ответ:
```json
{"status":"healthy"}
```

### Ручная отправка метрики
```bash
curl -X POST http://localhost:8080/metrics \
  -H "Content-Type: application/json" \
  -d '{
    "timestamp": 1234567890,
    "cpu": 50.5,
    "rps": 100.0,
    "device_id": "device_1"
  }'
```

Ожидаемый ответ:
```json
{"status":"ok","metric":{...}}
```

### Получение аналитики
```bash
curl http://localhost:8080/analyze
```

Ожидаемый ответ:
```json
{
  "rolling_average": 100.0,
  "mean": 100.0,
  "std_dev": 20.0,
  "anomaly_count": 5,
  "total_processed": 1000,
  "anomaly_rate": 0.5,
  "window_size": 50
}
```

## Нагрузочное тестирование

### Использование Locust

**Интерактивный режим (с веб-интерфейсом):**
```bash
# Установка Locust
pip3 install -r requirements.txt

# Запуск в интерактивном режиме
locust -f locustfile.py --host=http://localhost:8080

# Откройте в браузере: http://localhost:8089
```

### Prometheus
```bash

# Port-forward
kubectl port-forward service/prometheus 9090:9090

# Открыть в браузере
open http://localhost:9090
```

### Grafana
```bash
# Port-forward
kubectl port-forward service/grafana 3000:3000

# Открыть в браузере
open http://localhost:3000
# Логин: admin / admin
```
### Alertmanager
```bash
# Port-forward
kubectl port-forward service/grafana 9093:9093

# Открыть в браузере
open http://localhost:9093
```

## Мониторинг HPA

```bash
# Просмотр текущего состояния HPA
kubectl get hpa go-service-hpa -w

# Просмотр подов
kubectl get pods -w

# Метрики подов
kubectl top pods
```

## Метрики Prometheus

Сервис экспортирует следующие метрики:

- `http_requests_total` - Общее количество HTTP запросов
- `http_request_duration_seconds` - Длительность запросов
- `anomalies_detected_total` - Количество обнаруженных аномалий
- `current_rps` - Текущий RPS
- `rolling_average_rps` - Rolling average RPS

## Структура проекта

```
.
├── main.go                    # Основной код сервиса (аналитика, Redis, Prometheus)
├── go.mod                     # Go зависимости
├── go.sum                     # Checksums зависимостей
├── Dockerfile                 # Docker образ (multi-stage build)
├── docker-compose.yml         # Локальное развертывание (Go + Redis)
├── locustfile.py              # Locust скрипт для нагрузочного тестирования
├── requirements.txt           # Python зависимости (Locust)
├── .gitignore                # Игнорируемые файлы
│
├── k8s/                       # Kubernetes манифесты
│   ├── configmap.yaml        # Конфигурация Go сервиса (Redis адрес, порт)
│   ├── redis-deployment.yaml # Redis deployment и service
│   ├── go-service-deployment.yaml # Go сервис deployment и service
│   ├── hpa.yaml              # Horizontal Pod Autoscaler (2-5 реплик)
│   ├── ingress.yaml          # Ingress для доступа к сервисам
│   │
│   ├── prometheus-deployment.yaml # Prometheus deployment, service, config
│   ├── prometheus-alerts.yaml     # Правила алертов Prometheus
│   │
│   ├── alertmanager-config.yaml   # Alertmanager config и deployment
│   ├── alertmanager-smtp-secret.yaml # Secret для SMTP пароля
│   │
│   ├── grafana-deployment.yaml    # Grafana deployment и service
│   ├── grafana-datasource.yaml    # Автоматическая настройка Prometheus datasource
│   └── grafana-dashboards.yaml    # Автоматическая настройка дашбордов
│
├── scripts/                   # Вспомогательные скрипты
│   └── load_test.py          # Python скрипт для нагрузочного тестирования
│
└── README.md                  # Документация проекта
```

### Описание компонентов

**Основной код:**
- `main.go` - HTTP сервис с аналитикой (rolling average, z-score), интеграцией Redis и Prometheus метриками

**Kubernetes манифесты:**
- **Go Service**: Deployment с 2 репликами, Service, ConfigMap, HPA для авто-масштабирования
- **Redis**: Deployment для кэширования метрик
- **Prometheus**: Deployment с RBAC, Service, ConfigMap с правилами алертов
- **Alertmanager**: Deployment с ConfigMap для отправки алертов на email (пароль из Secret)
- **Grafana**: Deployment с автоматической настройкой datasource и дашбордов
- **Ingress**: Настройка доступа к сервисам через единую точку входа

**Тестирование:**
- `locustfile.py` - Нагрузочное тестирование с разными типами пользователей
- `scripts/load_test.py` - Простой Python скрипт для нагрузочного тестирования

## Производительность

- **Целевой RPS**: 1000+ запросов в секунду
- **Латентность**: < 50 мс (P95)
- **Точность детекции аномалий**: > 70% на синтетических данных
- **False positive rate**: < 10%

## Оптимизация

### Профилирование Go

```bash
# Включение pprof в коде
import _ "net/http/pprof"

# Сбор профиля
go tool pprof http://localhost:8080/debug/pprof/profile
```
