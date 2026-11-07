"""Resize operation — scales image to target dimensions."""
from PIL import Image

_FORMAT_MAP = {"image/jpeg": ("JPEG", ".jpg"), "image/png": ("PNG", ".png"), "image/webp": ("WEBP", ".webp")}


def process(image: Image.Image, parameters: dict, source_content_type: str) -> tuple:
    """Scale image. Provide width, height, or both. Omitting a dimension preserves aspect ratio."""
    width = parameters.get("width")
    height = parameters.get("height")
    if width and height:
        image = image.resize((int(width), int(height)), Image.LANCZOS)
    elif width:
        ratio = int(width) / image.width
        image = image.resize((int(width), round(image.height * ratio)), Image.LANCZOS)
    elif height:
        ratio = int(height) / image.height
        image = image.resize((round(image.width * ratio), int(height)), Image.LANCZOS)
    fmt, ext = _FORMAT_MAP.get(source_content_type, ("JPEG", ".jpg"))
    return image, source_content_type, ext
