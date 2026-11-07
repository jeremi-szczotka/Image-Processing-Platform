"""Compress operation — re-encodes as JPEG at reduced quality."""
from PIL import Image


def process(image: Image.Image, parameters: dict, source_content_type: str) -> tuple:
    """Re-encode as JPEG at given quality (default 85). Always outputs JPEG."""
    quality = int(parameters.get("quality", 85))
    if image.mode not in ("RGB", "L"):
        image = image.convert("RGB")
    return image, "image/jpeg", ".jpg"
