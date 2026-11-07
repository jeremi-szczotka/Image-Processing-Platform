"""Grayscale operation — converts to single-channel luminance."""
from PIL import Image

_FORMAT_MAP = {"image/jpeg": ("JPEG", ".jpg"), "image/png": ("PNG", ".png"), "image/webp": ("WEBP", ".webp")}


def process(image: Image.Image, parameters: dict, source_content_type: str) -> tuple:
    """Convert image to grayscale (mode L). Preserves source format."""
    image = image.convert("L")
    _, ext = _FORMAT_MAP.get(source_content_type, ("JPEG", ".jpg"))
    return image, source_content_type, ext
