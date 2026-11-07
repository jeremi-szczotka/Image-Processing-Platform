"""pytest configuration — sets required env vars before any test module imports."""
import os

# config.py requires these env vars at module level.
# Set defaults so the module is importable in unit tests without real services.
os.environ.setdefault("DATABASE_URL", "postgresql://test:test@localhost/test")
os.environ.setdefault("RABBITMQ_URL", "amqp://guest:guest@localhost/")
os.environ.setdefault("MINIO_ENDPOINT", "localhost:9000")
os.environ.setdefault("MINIO_ACCESS_KEY", "testkey")
os.environ.setdefault("MINIO_SECRET_KEY", "testsecret")
