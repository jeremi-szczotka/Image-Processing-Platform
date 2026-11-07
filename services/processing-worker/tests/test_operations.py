"""Unit tests for image operations — no external services required."""
import io
import pytest
from PIL import Image

from app.operations import dispatch
from app.operations import resize, compress, convert, grayscale, edge_detection


def make_image(width=200, height=150, mode="RGB", fmt="JPEG") -> Image.Image:
    img = Image.new(mode, (width, height), color=(180, 80, 40))
    buf = io.BytesIO()
    save_fmt = "JPEG" if fmt == "JPEG" else fmt
    img.save(buf, format=save_fmt)
    buf.seek(0)
    result = Image.open(buf)
    result.load()
    return result


# ── resize ────────────────────────────────────────────────────────────────────

def test_resize_width_only_scales_proportionally():
    img = make_image(200, 150)
    result, ct, ext = resize.process(img, {"width": 100}, "image/jpeg")
    assert result.width == 100
    assert result.height == 75
    assert ct == "image/jpeg"
    assert ext == ".jpg"


def test_resize_height_only_scales_proportionally():
    img = make_image(200, 150)
    result, ct, ext = resize.process(img, {"height": 75}, "image/jpeg")
    assert result.height == 75
    assert result.width == 100


def test_resize_both_dimensions_exact():
    img = make_image(200, 150)
    result, ct, ext = resize.process(img, {"width": 300, "height": 200}, "image/jpeg")
    assert result.size == (300, 200)


def test_resize_preserves_png_format():
    img = make_image(200, 150, fmt="PNG")
    result, ct, ext = resize.process(img, {"width": 100}, "image/png")
    assert ct == "image/png"
    assert ext == ".png"


def test_resize_no_params_returns_original_size():
    img = make_image(200, 150)
    result, ct, ext = resize.process(img, {}, "image/jpeg")
    assert result.size == (200, 150)


# ── compress ──────────────────────────────────────────────────────────────────

def test_compress_outputs_jpeg():
    img = make_image(200, 150)
    result, ct, ext = compress.process(img, {}, "image/jpeg")
    assert ct == "image/jpeg"
    assert ext == ".jpg"


def test_compress_png_input_outputs_jpeg():
    img = make_image(200, 150, fmt="PNG")
    result, ct, ext = compress.process(img, {"quality": 60}, "image/png")
    assert ct == "image/jpeg"
    assert ext == ".jpg"


def test_compress_result_is_valid_image():
    img = make_image(200, 150)
    result, ct, ext = compress.process(img, {"quality": 50}, "image/jpeg")
    assert result.size == (200, 150)


# ── convert ───────────────────────────────────────────────────────────────────

def test_convert_jpeg_to_png():
    img = make_image(200, 150)
    result, ct, ext = convert.process(img, {"format": "PNG"}, "image/jpeg")
    assert ct == "image/png"
    assert ext == ".png"


def test_convert_jpeg_to_webp():
    img = make_image(200, 150)
    result, ct, ext = convert.process(img, {"format": "WEBP"}, "image/jpeg")
    assert ct == "image/webp"
    assert ext == ".webp"


def test_convert_unknown_format_raises():
    img = make_image(200, 150)
    with pytest.raises(ValueError, match="format"):
        convert.process(img, {"format": "BMP"}, "image/jpeg")


def test_convert_missing_format_raises():
    img = make_image(200, 150)
    with pytest.raises(ValueError, match="format"):
        convert.process(img, {}, "image/jpeg")


# ── grayscale ─────────────────────────────────────────────────────────────────

def test_grayscale_produces_single_channel():
    img = make_image(200, 150)
    result, ct, ext = grayscale.process(img, {}, "image/jpeg")
    assert result.mode == "L"
    assert ct == "image/jpeg"
    assert ext == ".jpg"


def test_grayscale_preserves_dimensions():
    img = make_image(200, 150)
    result, ct, ext = grayscale.process(img, {}, "image/jpeg")
    assert result.size == (200, 150)


def test_grayscale_preserves_png_format():
    img = make_image(200, 150, fmt="PNG")
    result, ct, ext = grayscale.process(img, {}, "image/png")
    assert ct == "image/png"
    assert ext == ".png"


# ── edge_detection ────────────────────────────────────────────────────────────

def test_edge_detection_preserves_dimensions():
    img = make_image(200, 150)
    result, ct, ext = edge_detection.process(img, {}, "image/jpeg")
    assert result.size == (200, 150)
    assert ct == "image/jpeg"
    assert ext == ".jpg"


def test_edge_detection_produces_rgb_image():
    img = make_image(200, 150)
    result, ct, ext = edge_detection.process(img, {}, "image/jpeg")
    assert result.mode == "RGB"


# ── dispatch ──────────────────────────────────────────────────────────────────

def test_dispatch_resize():
    img = make_image(200, 150)
    result, ct, ext = dispatch("resize", img, {"width": 100}, "image/jpeg")
    assert result.width == 100


def test_dispatch_unknown_operation_raises():
    img = make_image(200, 150)
    with pytest.raises(ValueError, match="unknown operation"):
        dispatch("explode", img, {}, "image/jpeg")
