"""Database helper stubs — full implementations added in Task 4."""


def get_job_status(conn, job_id: str) -> str:
    raise NotImplementedError


def set_job_processing(conn, job_id: str) -> None:
    raise NotImplementedError


def get_image_object_key(conn, image_id: str) -> str:
    raise NotImplementedError


def set_job_completed(conn, job_id: str, result_key: str, content_type: str, file_size: int) -> None:
    raise NotImplementedError


def set_job_failed(conn, job_id: str, error_message: str) -> None:
    raise NotImplementedError
