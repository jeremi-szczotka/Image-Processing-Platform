"""Unit tests for process_message — all external deps mocked."""
import io
import uuid
import pytest
from PIL import Image
from unittest.mock import MagicMock, patch, call

from app import worker


def _make_rgb_image() -> Image.Image:
    return Image.new("RGB", (100, 80), color=(10, 20, 30))


def _make_payload(**overrides) -> dict:
    base = {
        "job_id": str(uuid.uuid4()),
        "image_id": str(uuid.uuid4()),
        "operation": "resize",
        "parameters": {"width": 50},
    }
    return {**base, **overrides}


class MockDB:
    """Minimal in-memory stub for db module functions."""
    def __init__(self, job_status="PENDING", object_key="some-uuid/original.jpg"):
        self.job_status = job_status
        self.object_key = object_key
        self.processing_called = False
        self.completed_args = None
        self.failed_args = None

    def get_job_status(self, conn, job_id): return self.job_status
    def get_image_object_key(self, conn, image_id): return self.object_key
    def set_job_processing(self, conn, job_id): self.processing_called = True
    def set_job_completed(self, conn, job_id, result_key, content_type, file_size):
        self.completed_args = (job_id, result_key, content_type, file_size)
    def set_job_failed(self, conn, job_id, error_message):
        self.failed_args = (job_id, error_message)


class MockStorage:
    """Minimal in-memory stub for storage module functions."""
    def __init__(self, image=None, content_type="image/jpeg"):
        self._image = image or Image.new("RGB", (100, 80))
        self._content_type = content_type
        self.uploaded_key = None
        self.download_fail = False

    def download_image(self, client, bucket, key):
        if self.download_fail:
            raise RuntimeError("S3 unavailable")
        return self._image, self._content_type

    def upload_result(self, client, bucket, key, image, content_type):
        self.uploaded_key = key
        return 12345


# ── Happy path ────────────────────────────────────────────────────────────────

def test_process_message_happy_path_resize(monkeypatch):
    mock_db = MockDB()
    mock_storage = MockStorage()
    monkeypatch.setattr(worker, "db", mock_db)
    monkeypatch.setattr(worker, "storage", mock_storage)

    payload = _make_payload(operation="resize", parameters={"width": 50})
    worker.process_message(payload, conn=object(), s3_client=object())

    assert mock_db.processing_called
    assert mock_db.completed_args is not None
    job_id, result_key, ct, size = mock_db.completed_args
    assert job_id == payload["job_id"]
    assert result_key.startswith(payload["job_id"])
    assert result_key.endswith(".jpg")
    assert size == 12345


def test_process_message_sets_result_key_in_processed_bucket(monkeypatch):
    mock_db = MockDB()
    mock_storage = MockStorage()
    monkeypatch.setattr(worker, "db", mock_db)
    monkeypatch.setattr(worker, "storage", mock_storage)

    payload = _make_payload(operation="grayscale", parameters={})
    worker.process_message(payload, conn=object(), s3_client=object())

    assert mock_storage.uploaded_key == f"{payload['job_id']}/result.jpg"


# ── Idempotency ───────────────────────────────────────────────────────────────

def test_process_message_skips_completed_job(monkeypatch):
    mock_db = MockDB(job_status="COMPLETED")
    mock_storage = MockStorage()
    monkeypatch.setattr(worker, "db", mock_db)
    monkeypatch.setattr(worker, "storage", mock_storage)

    worker.process_message(_make_payload(), conn=object(), s3_client=object())

    assert not mock_db.processing_called
    assert mock_db.completed_args is None


def test_process_message_skips_failed_job(monkeypatch):
    mock_db = MockDB(job_status="FAILED")
    mock_storage = MockStorage()
    monkeypatch.setattr(worker, "db", mock_db)
    monkeypatch.setattr(worker, "storage", mock_storage)

    worker.process_message(_make_payload(), conn=object(), s3_client=object())

    assert not mock_db.processing_called


# ── Error handling ────────────────────────────────────────────────────────────

def test_process_message_sets_failed_on_storage_error(monkeypatch):
    mock_db = MockDB()
    mock_storage = MockStorage()
    mock_storage.download_fail = True
    monkeypatch.setattr(worker, "db", mock_db)
    monkeypatch.setattr(worker, "storage", mock_storage)

    with pytest.raises(RuntimeError):
        worker.process_message(_make_payload(), conn=object(), s3_client=object())

    assert mock_db.failed_args is not None
    assert "S3 unavailable" in mock_db.failed_args[1]


def test_process_message_sets_failed_on_unknown_operation(monkeypatch):
    mock_db = MockDB()
    mock_storage = MockStorage()
    monkeypatch.setattr(worker, "db", mock_db)
    monkeypatch.setattr(worker, "storage", mock_storage)

    with pytest.raises(ValueError, match="unknown operation"):
        worker.process_message(_make_payload(operation="explode"), conn=object(), s3_client=object())

    assert mock_db.failed_args is not None
    assert mock_db.completed_args is None


def test_process_message_does_not_set_completed_on_failure(monkeypatch):
    mock_db = MockDB()
    mock_storage = MockStorage()
    mock_storage.download_fail = True
    monkeypatch.setattr(worker, "db", mock_db)
    monkeypatch.setattr(worker, "storage", mock_storage)

    with pytest.raises(RuntimeError):
        worker.process_message(_make_payload(), conn=object(), s3_client=object())

    assert mock_db.completed_args is None
