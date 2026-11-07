"""Dispatches an operation name to the corresponding transform function."""
from PIL import Image
from . import compress, convert, edge_detection, grayscale, resize

_REGISTRY = {
    "resize":         resize.process,
    "compress":       compress.process,
    "convert":        convert.process,
    "grayscale":      grayscale.process,
    "edge_detection": edge_detection.process,
}


def dispatch(operation: str, image: Image.Image, parameters: dict, source_content_type: str) -> tuple:
    """Route operation to its handler. Raises ValueError for unknown operations."""
    fn = _REGISTRY.get(operation)
    if fn is None:
        raise ValueError(f"unknown operation: {operation!r}; valid: {list(_REGISTRY)}")
    return fn(image, parameters, source_content_type)
