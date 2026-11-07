"""Object storage helper stubs — full implementations added in Task 4."""
from PIL import Image


def download_image(s3_client, bucket: str, key: str) -> tuple:
    raise NotImplementedError


def upload_result(s3_client, bucket: str, key: str, image: Image.Image, content_type: str) -> int:
    raise NotImplementedError
