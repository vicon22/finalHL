"""
Locust файл для нагрузочного тестирования Go сервиса
Использование: locust -f locustfile.py --host=http://localhost:8080
"""

from locust import HttpUser, task, between
import random
import time


class MetricsUser(HttpUser):
    """
    Отправка не аномальной метрики от IoT устройств
    """
    wait_time = between(0.1, 0.5)  # Задержка между запросами 0.1-0.5 секунды
    
    def on_start(self):
        """Выполняется один раз при старте пользователя"""
        self.device_id = f"device_{random.randint(1, 1000)}"
        self.base_rps = random.uniform(80, 120)  # Базовое значение RPS для устройства
        
    @task(10)
    def send_metric(self):
        """
        Основная задача - отправка метрики
        Вес 10 означает, что эта задача выполняется чаще других
        """
        # Генерируем нормальную метрику с небольшими флуктуациями
        metric = {
            "timestamp": int(time.time()),
            "cpu": random.uniform(0, 100),
            "rps": max(0, self.base_rps + random.gauss(0, 15)),  # Нормальное распределение
            "device_id": self.device_id
        }
        
        with self.client.post(
            "/metrics",
            json=metric,
            catch_response=True,
            name="POST /metrics"
        ) as response:
            if response.status_code == 200:
                response.success()
            else:
                response.failure(f"Got status code {response.status_code}")
    
    @task(2)
    def send_anomalous_metric(self):
        """
        Отправка аномальной метрики (для тестирования детекции аномалий)
        Вес 2 означает, что это происходит реже обычных метрик
        """
        # Генерируем аномалию - резкий скачок или падение RPS
        anomaly_type = random.choice(["spike", "drop"])
        
        if anomaly_type == "spike":
            rps = self.base_rps + random.uniform(150, 300)  # Резкий скачок
        else:
            rps = max(0, self.base_rps - random.uniform(100, 200))  # Резкое падение
        
        metric = {
            "timestamp": int(time.time()),
            "cpu": random.uniform(0, 100),
            "rps": rps,
            "device_id": self.device_id
        }
        
        with self.client.post(
            "/metrics",
            json=metric,
            catch_response=True,
            name="POST /metrics (anomaly)"
        ) as response:
            if response.status_code == 200:
                response.success()
            else:
                response.failure(f"Got status code {response.status_code}")
