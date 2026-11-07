"""S3/MinIO helpers — download source images and upload results."""
import io
import logging
from PIL import Image
from minio import Minio

log = logging.getLogger(__name__)

_CT_TO_FORMAT = {"image/jpeg": "JPEG", "image/png": "PNG", "image/webp": "WEBP"}
_FORMAT_TO_CT = {"JPEG": "image/jpeg", "PNG": "image/png", "WEBP": "image/webp"}


def get_minio_client(endpoint: str, access_key: str, secret_key: str, secure: bool) -> Minio:
    """Create a MinIO client. Works against any S3-compatible endpoint."""
    return Minio(endpoint, access_key=access_key, secret_key=secret_key, secure=secure)


def download_image(client: Minio, bucket: str, object_key: str) -> tuple:
    """Download object_key from bucket and open it as a PIL Image.

    Returns (image, content_type). Loads pixel data eagerly so the connection
    can be released before processing begins.
    """
    response = client.get_object(bucket, object_key)
    try:
        data = response.read()
    finally:
        response.close()
        response.release_conn()

    image = Image.open(io.BytesIO(data))
    image.load()
    content_type = _FORMAT_TO_CT.get(image.format or "", "image/jpeg")
    return image, content_type


def upload_result(client: Minio, bucket: str, object_key: str, image: Image.Image, content_type: str) -> int:
    """Encode image and upload to bucket/object_key. Returns file size in bytes."""
    fmt = _CT_TO_FORMAT.get(content_type, "JPEG")
    buf = io.BytesIO()
    save_kwargs: dict = {}
    if fmt == "JPEG":
        # Ensure JPEG-compatible mode; include quality setting
        if image.mode not in ("RGB", "L"):
            image = image.convert("RGB")
        save_kwargs["quality"] = 90
    image.save(buf, format=fmt, **save_kwargs)
    size = buf.tell()
    buf.seek(0)
    client.put_object(bucket, object_key, buf, size, content_type=content_type)
    log.debug("uploaded %s/%s (%d bytes)", bucket, object_key, size)
    return size
