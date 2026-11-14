package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"image-platform/api-service/internal/domain"
	"image-platform/api-service/internal/handlers"
	"image-platform/api-service/internal/repository"
)

type mockJobResultRepo struct {
	result *repository.JobResult
	err    error
}

func (m *mockJobResultRepo) GetResultByJobID(_ context.Context, _ uuid.UUID) (*repository.JobResult, error) {
	return m.result, m.err
}

func newResultHandler(repo repository.JobResultRepository, stor *mockStorage) *handlers.ResultHandler {
	return handlers.NewResultHandler(repo, stor, testCfg(), discardLogger())
}

func TestDownloadResult_CompletedJob_Returns302(t *testing.T) {
	key := "abc123/result.jpg"
	repo := &mockJobResultRepo{result: &repository.JobResult{Status: "COMPLETED", ObjectKey: &key}}
	stor := &mockStorage{presignURL: "https://s3.example.com/result.jpg"}
	h := newResultHandler(repo, stor)

	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}/result", h.DownloadResult)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String()+"/result", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "https://s3.example.com/result.jpg", w.Header().Get("Location"))
}

func TestDownloadResult_CompletedJob_PresignsFromProcessedBucket(t *testing.T) {
	key := "abc123/result.jpg"
	capturedBucket := ""
	repo := &mockJobResultRepo{result: &repository.JobResult{Status: "COMPLETED", ObjectKey: &key}}
	stor := &mockStorage{
		presignURL:  "https://s3.example.com/result.jpg",
		presignFunc: func(bucket, _ string) { capturedBucket = bucket },
	}
	h := newResultHandler(repo, stor)

	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}/result", h.DownloadResult)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String()+"/result", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "images-processed", capturedBucket)
}

func TestDownloadResult_PendingJob_Returns409(t *testing.T) {
	repo := &mockJobResultRepo{result: &repository.JobResult{Status: "PENDING"}}
	h := newResultHandler(repo, &mockStorage{})

	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}/result", h.DownloadResult)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String()+"/result", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestDownloadResult_ProcessingJob_Returns409(t *testing.T) {
	repo := &mockJobResultRepo{result: &repository.JobResult{Status: "PROCESSING"}}
	h := newResultHandler(repo, &mockStorage{})

	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}/result", h.DownloadResult)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String()+"/result", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestDownloadResult_UnknownJob_Returns404(t *testing.T) {
	repo := &mockJobResultRepo{err: domain.ErrNotFound}
	h := newResultHandler(repo, &mockStorage{})

	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}/result", h.DownloadResult)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String()+"/result", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDownloadResult_InvalidUUID_Returns400(t *testing.T) {
	h := newResultHandler(&mockJobResultRepo{}, &mockStorage{})

	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}/result", h.DownloadResult)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/not-a-uuid/result", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDownloadResult_PresignError_Returns500(t *testing.T) {
	key := "abc123/result.jpg"
	repo := &mockJobResultRepo{result: &repository.JobResult{Status: "COMPLETED", ObjectKey: &key}}
	stor := &mockStorage{presignErr: errors.New("s3 unavailable")}
	h := newResultHandler(repo, stor)

	r := chiRouter(http.MethodGet, "/api/v1/jobs/{id}/result", h.DownloadResult)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+uuid.New().String()+"/result", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
