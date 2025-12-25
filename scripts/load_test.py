#!/usr/bin/env python3
"""
Скрипт для нагрузочного тестирования сервиса
Использует requests для отправки метрик
"""
import requests
import time
import random
import json
import sys
from concurrent.futures import ThreadPoolExecutor, as_completed
from statistics import mean, stdev

BASE_URL = "http://localhost:8080"
if len(sys.argv) > 1:
    BASE_URL = sys.argv[1]

RPS_TARGET = 1000  # Целевое количество запросов в секунду
DURATION = 300  # 5 минут
THREADS = 50  # Количество параллельных потоков

def generate_metric(device_id=None):
    """Генерирует случайную метрику"""
    if device_id is None:
        device_id = f"device_{random.randint(1, 100)}"
    
    # Базовое значение RPS с небольшими флуктуациями
    base_rps = 100.0
    rps = base_rps + random.gauss(0, 20)
    
    # Периодически добавляем аномалии (5% случаев)
    if random.random() < 0.05:
        rps = base_rps + random.gauss(0, 100) * 3
    
    return {
        "timestamp": int(time.time()),
        "cpu": random.uniform(0, 100),
        "rps": max(0, rps),
        "device_id": device_id
    }

def send_metric():
    """Отправляет одну метрику"""
    try:
        metric = generate_metric()
        response = requests.post(
            f"{BASE_URL}/metrics",
            json=metric,
            timeout=5
        )
        return response.status_code == 200, response.elapsed.total_seconds()
    except Exception as e:
        return False, 0

def run_load_test():
    """Запускает нагрузочный тест"""
    print(f"Начало нагрузочного теста:")
    print(f"  URL: {BASE_URL}")
    print(f"  Целевой RPS: {RPS_TARGET}")
    print(f"  Длительность: {DURATION} секунд")
    print(f"  Потоков: {THREADS}")
    print()
    
    start_time = time.time()
    end_time = start_time + DURATION
    
    total_requests = 0
    successful_requests = 0
    failed_requests = 0
    latencies = []
    
    # Вычисляем интервал между запросами для достижения целевого RPS
    request_interval = 1.0 / (RPS_TARGET / THREADS)
    
    def worker():
        nonlocal total_requests, successful_requests, failed_requests
        local_latencies = []
        
        while time.time() < end_time:
            success, latency = send_metric()
            total_requests += 1
            if success:
                successful_requests += 1
                local_latencies.append(latency * 1000)  # в миллисекундах
            else:
                failed_requests += 1
            
            time.sleep(request_interval)
        
        return local_latencies
    
    # Запускаем потоки
    with ThreadPoolExecutor(max_workers=THREADS) as executor:
        futures = [executor.submit(worker) for _ in range(THREADS)]
        for future in as_completed(futures):
            latencies.extend(future.result())
    
    elapsed = time.time() - start_time
    actual_rps = total_requests / elapsed
    
    print(f"\nРезультаты теста:")
    print(f"  Всего запросов: {total_requests}")
    print(f"  Успешных: {successful_requests}")
    print(f"  Неудачных: {failed_requests}")
    print(f"  Время выполнения: {elapsed:.2f} секунд")
    print(f"  Фактический RPS: {actual_rps:.2f}")
    print(f"  Успешность: {(successful_requests/total_requests*100):.2f}%")
    
    if latencies:
        print(f"\nЛатентность:")
        print(f"  Средняя: {mean(latencies):.2f} мс")
        print(f"  Медиана: {sorted(latencies)[len(latencies)//2]:.2f} мс")
        print(f"  P95: {sorted(latencies)[int(len(latencies)*0.95)]:.2f} мс")
        print(f"  P99: {sorted(latencies)[int(len(latencies)*0.99)]:.2f} мс")
        if len(latencies) > 1:
            print(f"  Стандартное отклонение: {stdev(latencies):.2f} мс")
    
    # Проверяем аналитику
    try:
        response = requests.get(f"{BASE_URL}/analyze", timeout=5)
        if response.status_code == 200:
            analytics = response.json()
            print(f"\nАналитика:")
            print(f"  Rolling Average: {analytics.get('rolling_average', 0):.2f}")
            print(f"  Mean: {analytics.get('mean', 0):.2f}")
            print(f"  Std Dev: {analytics.get('std_dev', 0):.2f}")
            print(f"  Аномалий обнаружено: {analytics.get('anomaly_count', 0)}")
            print(f"  Всего обработано: {analytics.get('total_processed', 0)}")
            print(f"  Процент аномалий: {analytics.get('anomaly_rate', 0):.2f}%")
    except Exception as e:
        print(f"\nНе удалось получить аналитику: {e}")

if __name__ == "__main__":
    run_load_test()

