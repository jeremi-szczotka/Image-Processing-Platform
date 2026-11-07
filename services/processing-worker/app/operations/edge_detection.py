"""Edge detection operation — highlights edges using PIL's FIND_EDGES filter."""
from PIL import Image, ImageFilter

_FORMAT_MAP = {"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}


def process(image: Image.Image, parameters: dict, source_content_type: str) -> tuple:
    """Apply FIND_EDGES filter. Returns RGB image; preserves source format."""
    if image.mode != "RGB":
        image = image.convert("RGB")
    image = image.filter(ImageFilter.FIND_EDGES)
    ext = _FORMAT_MAP.get(source_content_type, ".jpg")
    return image, source_content_type, ext
