"""Configuration loaded from environment variables."""
import os

DATABASE_URL: str = os.environ["DATABASE_URL"]
RABBITMQ_URL: str = os.environ["RABBITMQ_URL"]
MINIO_ENDPOINT: str = os.environ["MINIO_ENDPOINT"]
MINIO_ACCESS_KEY: str = os.environ["MINIO_ACCESS_KEY"]
MINIO_SECRET_KEY: str = os.environ["MINIO_SECRET_KEY"]
MINIO_USE_SSL: bool = os.environ.get("MINIO_USE_SSL", "false").lower() == "true"
LOG_LEVEL: str = os.environ.get("WORKER_LOG_LEVEL", "INFO").upper()

BUCKET_ORIGINAL = "images-original"
BUCKET_PROCESSED = "images-processed"
QUEUE_NAME = "jobs"
