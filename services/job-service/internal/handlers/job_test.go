package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"image-platform/job-service/internal/domain"
	"image-platform/job-service/internal/handlers"
	"image-platform/job-service/internal/repository"
)

// ── Mock types ────────────────────────────────────────────────────────────────

type mockJobRepo struct {
	jobs      map[uuid.UUID]*domain.Job
	createErr error
}

func newMockRepo() *mockJobRepo {
	return &mockJobRepo{jobs: make(map[uuid.UUID]*domain.Job)}
}

func (m *mockJobRepo) CreateWithOutbox(_ context.Context, job *domain.Job, _ []byte) error {
	if m.createErr != nil {
		return m.createErr
	}
	job.Status = "PENDING"
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()
	cp := *job
	m.jobs[job.ID] = &cp
	return nil
}

func (m *mockJobRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Job, error) {
	if job, ok := m.jobs[id]; ok {
		return job, nil
	}
	return nil, domain.ErrNotFound
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func chiRouter(method, pattern string, h http.HandlerFunc) *chi.Mux {
	r := chi.NewRouter()
	r.Method(method, pattern, h)
	return r
}

func postJSON(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestCreate_ValidRequest_Returns201WithPendingStatus(t *testing.T) {
	h := handlers.NewJobHandler(newMockRepo(), discardLogger())
	imageID := uuid.New().String()
	rr := postJSON(t, h.Create, `{"image_id":"`+imageID+`","operation":"resize","parameters":{"width":800}}`)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "PENDING", resp["status"])
	assert.NotEmpty(t, resp["id"])
	assert.Equal(t, imageID, resp["image_id"])
	assert.Equal(t, "resize", resp["operation"])
}

func TestCreate_NilParameters_DefaultsToEmptyObject(t *testing.T) {
	h := handlers.NewJobHandler(newMockRepo(), discardLogger())
	rr := postJSON(t, h.Create, `{"image_id":"`+uuid.New().String()+`","operation":"grayscale"}`)

	require.Equal(t, http.StatusCreated, rr.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, map[string]any{}, resp["parameters"])
}

func TestCreate_MissingImageID_Returns400(t *testing.T) {
	h := handlers.NewJobHandler(newMockRepo(), discardLogger())
	rr := postJSON(t, h.Create, `{"operation":"resize"}`)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "image_id")
}

func TestCreate_InvalidOperation_Returns400(t *testing.T) {
	h := handlers.NewJobHandler(newMockRepo(), discardLogger())
	rr := postJSON(t, h.Create, `{"image_id":"`+uuid.New().String()+`","operation":"explode"}`)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "operation")
}

func TestCreate_ImageNotFound_Returns400(t *testing.T) {
	repo := newMockRepo()
	repo.createErr = repository.ErrImageNotFound
	h := handlers.NewJobHandler(repo, discardLogger())
	rr := postJSON(t, h.Create, `{"image_id":"`+uuid.New().String()+`","operation":"resize"}`)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "image not found")
}

func TestCreate_RepoError_Returns500(t *testing.T) {
	repo := newMockRepo()
	repo.createErr = errors.New("db down")
	h := handlers.NewJobHandler(repo, discardLogger())
	rr := postJSON(t, h.Create, `{"image_id":"`+uuid.New().String()+`","operation":"resize"}`)
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	h := handlers.NewJobHandler(newMockRepo(), discardLogger())
	rr := postJSON(t, h.Create, `not json`)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

// ── GetByID tests ─────────────────────────────────────────────────────────────

func TestGetByID_ExistingJob_Returns200(t *testing.T) {
	repo := newMockRepo()
	job := &domain.Job{
		ID:         uuid.New(),
		ImageID:    uuid.New(),
		Operation:  "resize",
		Status:     "PENDING",
		Parameters: map[string]any{},
	}
	repo.jobs[job.ID] = job
	h := handlers.NewJobHandler(repo, discardLogger())

	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}", h.GetByID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+job.ID.String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, job.ID.String(), resp["id"])
	assert.Equal(t, "PENDING", resp["status"])
}

func TestGetByID_NonexistentJob_Returns404(t *testing.T) {
	h := handlers.NewJobHandler(newMockRepo(), discardLogger())
	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}", h.GetByID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String(), nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	h := handlers.NewJobHandler(newMockRepo(), discardLogger())
	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}", h.GetByID)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/not-a-uuid", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
