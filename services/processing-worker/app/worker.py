"""Core message handler — stateless, injectable for testing."""
import logging
from app import db, storage
from app import config
from app.operations import dispatch

log = logging.getLogger(__name__)


def process_message(payload: dict, conn, s3_client) -> None:
    """Process a single job message end-to-end.

    Downloads source image, applies the requested operation, uploads the result,
    and updates job status directly in PostgreSQL. Raises on failure after marking
    the job FAILED — caller is responsible for nacking the RabbitMQ message.
    """
    job_id = str(payload["job_id"])
    image_id = str(payload["image_id"])
    operation = payload["operation"]
    parameters = payload.get("parameters") or {}

    current_status = db.get_job_status(conn, job_id)
    if current_status in ("COMPLETED", "FAILED"):
        log.info("job %s already %s — skipping (idempotent re-delivery)", job_id, current_status)
        return

    db.set_job_processing(conn, job_id)

    try:
        object_key = db.get_image_object_key(conn, image_id)
        source_image, source_ct = storage.download_image(s3_client, config.BUCKET_ORIGINAL, object_key)
        result_image, result_ct, result_ext = dispatch(operation, source_image, parameters, source_ct)
        result_key = f"{job_id}/result{result_ext}"
        file_size = storage.upload_result(s3_client, config.BUCKET_PROCESSED, result_key, result_image, result_ct)
        db.set_job_completed(conn, job_id, result_key, result_ct, file_size)
        log.info("job %s completed — result_key=%s size=%d", job_id, result_key, file_size)
    except Exception as exc:
        log.error("job %s failed: %s", job_id, exc)
        db.set_job_failed(conn, job_id, str(exc))
        raise
