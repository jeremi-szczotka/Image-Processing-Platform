"""Convert operation — changes image format."""
from PIL import Image

_ALLOWED = {"JPEG": ("image/jpeg", ".jpg"), "PNG": ("image/png", ".png"), "WEBP": ("image/webp", ".webp")}


def process(image: Image.Image, parameters: dict, source_content_type: str) -> tuple:
    """Convert to target format. Raises ValueError when format is unknown or missing."""
    fmt = str(parameters.get("format", "")).upper()
    if fmt not in _ALLOWED:
        raise ValueError(f"convert requires format in {list(_ALLOWED)}; got {fmt!r}")
    if fmt == "JPEG" and image.mode not in ("RGB", "L"):
        image = image.convert("RGB")
    ct, ext = _ALLOWED[fmt]
    return image, ct, ext
