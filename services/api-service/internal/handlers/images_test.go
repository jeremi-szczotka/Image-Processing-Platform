package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"image-platform/api-service/internal/config"
	"image-platform/api-service/internal/domain"
	"image-platform/api-service/internal/handlers"
)

// ── Mock implementations ─────────────────────────────────────────────────────

type mockStorage struct {
	uploadErr  error
	presignURL string
	presignErr error
}

func (m *mockStorage) Upload(_ context.Context, _, _ string, _ io.Reader, _ int64, _ string) error {
	return m.uploadErr
}

func (m *mockStorage) PresignedDownloadURL(_ context.Context, _, _ string, _ time.Duration) (string, error) {
	return m.presignURL, m.presignErr
}

type mockRepo struct {
	images    map[uuid.UUID]*domain.Image
	createErr error
}

func newMockRepo() *mockRepo {
	return &mockRepo{images: make(map[uuid.UUID]*domain.Image)}
}

func (m *mockRepo) Create(_ context.Context, img *domain.Image) error {
	if m.createErr != nil {
		return m.createErr
	}
	if img.ID == uuid.Nil {
		img.ID = uuid.New()
	}
	img.CreatedAt = time.Now()
	cp := *img
	m.images[img.ID] = &cp
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Image, error) {
	if img, ok := m.images[id]; ok {
		return img, nil
	}
	return nil, domain.ErrNotFound
}

// ── Test helpers ─────────────────────────────────────────────────────────────

func testCfg() *config.Config {
	return &config.Config{
		Port:            "8080",
		MaxUploadBytes:  10 * 1024 * 1024,
		MaxMegapixels:   50_000_000,
		PresignedURLTTL: time.Hour,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newHandler(repo *mockRepo, stor *mockStorage) *handlers.ImageHandler {
	return handlers.NewImageHandler(repo, stor, testCfg(), discardLogger())
}

func makeTestJPEG(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	buf := &bytes.Buffer{}
	_ = jpeg.Encode(buf, img, &jpeg.Options{Quality: 75})
	return buf.Bytes()
}

func buildMultipartReq(t *testing.T, fieldName, filename string, content []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	part, err := w.CreateFormFile(fieldName, filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/images", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func chiRouter(method, pattern string, h http.HandlerFunc) *chi.Mux {
	r := chi.NewRouter()
	r.Method(method, pattern, h)
	return r
}

// ── Upload tests ─────────────────────────────────────────────────────────────

func TestUpload_ValidJPEG_Returns201WithMetadata(t *testing.T) {
	repo := newMockRepo()
	h := newHandler(repo, &mockStorage{presignURL: "http://minio/presigned"})

	req := buildMultipartReq(t, "image", "photo.jpg", makeTestJPEG(100, 80))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["id"])
	assert.Equal(t, "image/jpeg", resp["content_type"])
	assert.Equal(t, float64(100), resp["width"])
	assert.Equal(t, float64(80), resp["height"])
}

func TestUpload_TextFileMasqueradingAsJPEG_Returns400(t *testing.T) {
	h := newHandler(newMockRepo(), &mockStorage{})

	req := buildMultipartReq(t, "image", "evil.jpg", []byte("this is plain text, definitely not a JPEG"))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "unsupported image format")
}

func TestUpload_MissingImageField_Returns400(t *testing.T) {
	h := newHandler(newMockRepo(), &mockStorage{})

	// Send multipart form but with wrong field name
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("wrong_field", "value")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/images", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestUpload_FileTooLarge_Returns413(t *testing.T) {
	cfg := testCfg()
	cfg.MaxUploadBytes = 100 // tiny limit to trigger the error with a small test body
	h := handlers.NewImageHandler(newMockRepo(), &mockStorage{}, cfg, discardLogger())

	req := buildMultipartReq(t, "image", "big.jpg", bytes.Repeat([]byte("x"), 500))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
}

func TestUpload_ImageExceedsMaxPixels_Returns400(t *testing.T) {
	cfg := testCfg()
	cfg.MaxMegapixels = 1000 // 1000 px max — 100x100 = 10 000 > 1000 → rejected
	h := handlers.NewImageHandler(newMockRepo(), &mockStorage{}, cfg, discardLogger())

	req := buildMultipartReq(t, "image", "huge.jpg", makeTestJPEG(100, 100))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "dimensions")
}

func TestUpload_StorageFailure_Returns500(t *testing.T) {
	stor := &mockStorage{uploadErr: errors.New("minio unavailable")}
	h := newHandler(newMockRepo(), stor)

	req := buildMultipartReq(t, "image", "photo.jpg", makeTestJPEG(50, 50))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestUpload_RepoFailure_Returns500(t *testing.T) {
	repo := &mockRepo{
		images:    make(map[uuid.UUID]*domain.Image),
		createErr: errors.New("db down"),
	}
	h := newHandler(repo, &mockStorage{})

	req := buildMultipartReq(t, "image", "photo.jpg", makeTestJPEG(50, 50))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// ── GetByID tests ─────────────────────────────────────────────────────────────

func TestGetByID_ExistingImage_Returns200(t *testing.T) {
	repo := newMockRepo()
	img := &domain.Image{ID: uuid.New(), ContentType: "image/jpeg", Width: 100, Height: 80}
	repo.images[img.ID] = img

	r := chiRouter(http.MethodGet, "/api/v1/images/{id}", newHandler(repo, &mockStorage{}).GetByID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/"+img.ID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, img.ID.String(), resp["id"])
}

func TestGetByID_NonexistentImage_Returns404(t *testing.T) {
	r := chiRouter(http.MethodGet, "/api/v1/images/{id}", newHandler(newMockRepo(), &mockStorage{}).GetByID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := chiRouter(http.MethodGet, "/api/v1/images/{id}", newHandler(newMockRepo(), &mockStorage{}).GetByID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/not-a-uuid", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ── Download tests ────────────────────────────────────────────────────────────

func TestDownload_ExistingImage_Redirects302ToPresignedURL(t *testing.T) {
	repo := newMockRepo()
	stor := &mockStorage{presignURL: "http://minio:9000/presigned-link"}
	img := &domain.Image{ID: uuid.New(), ObjectKey: "abc/original.jpg"}
	repo.images[img.ID] = img

	r := chiRouter(http.MethodGet, "/api/v1/images/{id}/download", newHandler(repo, stor).Download)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/"+img.ID.String()+"/download", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "http://minio:9000/presigned-link", rr.Header().Get("Location"))
}

func TestDownload_NonexistentImage_Returns404(t *testing.T) {
	r := chiRouter(http.MethodGet, "/api/v1/images/{id}/download", newHandler(newMockRepo(), &mockStorage{}).Download)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/"+uuid.New().String()+"/download", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}
