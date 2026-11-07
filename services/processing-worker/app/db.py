"""PostgreSQL helpers — update job status without HTTP calls to job-service."""
import logging
from datetime import datetime, timezone

log = logging.getLogger(__name__)


def get_job_status(conn, job_id: str) -> str:
    """Return current job status string, or 'UNKNOWN' if the job doesn't exist."""
    with conn.cursor() as cur:
        cur.execute("SELECT status FROM jobs WHERE id = %s", (job_id,))
        row = cur.fetchone()
        return row[0] if row else "UNKNOWN"


def get_image_object_key(conn, image_id: str) -> str:
    """Return the S3 object key for image_id. Raises ValueError if not found."""
    with conn.cursor() as cur:
        cur.execute("SELECT object_key FROM images WHERE id = %s", (image_id,))
        row = cur.fetchone()
        if not row:
            raise ValueError(f"image {image_id} not found")
        return row[0]


def set_job_processing(conn, job_id: str) -> None:
    """Set job to PROCESSING, record started_at, increment attempt_count."""
    now = datetime.now(timezone.utc)
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE jobs SET status = 'PROCESSING', started_at = %s, attempt_count = attempt_count + 1 WHERE id = %s",
            (now, job_id),
        )
    conn.commit()


def set_job_completed(conn, job_id: str, result_key: str, content_type: str, file_size: int) -> None:
    """Set job to COMPLETED and store result metadata."""
    now = datetime.now(timezone.utc)
    with conn.cursor() as cur:
        cur.execute(
            """UPDATE jobs
               SET status = 'COMPLETED', result_object_key = %s, result_content_type = %s,
                   result_file_size = %s, finished_at = %s
               WHERE id = %s""",
            (result_key, content_type, file_size, now, job_id),
        )
    conn.commit()


def set_job_failed(conn, job_id: str, error_message: str) -> None:
    """Set job to FAILED and record the error message."""
    now = datetime.now(timezone.utc)
    with conn.cursor() as cur:
        cur.execute(
            "UPDATE jobs SET status = 'FAILED', error_message = %s, finished_at = %s WHERE id = %s",
            (error_message, now, job_id),
        )
    conn.commit()
