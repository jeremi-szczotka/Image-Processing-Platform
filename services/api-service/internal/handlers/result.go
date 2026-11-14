package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"image-platform/api-service/internal/config"
	"image-platform/api-service/internal/domain"
	"image-platform/api-service/internal/repository"
	"image-platform/api-service/internal/storage"
)

// ResultHandler serves processed-image downloads via a presigned S3 redirect.
type ResultHandler struct {
	repo repository.JobResultRepository
	stor storage.Storage
	cfg  *config.Config
	log  *slog.Logger
}

// NewResultHandler creates a ResultHandler wired with the given dependencies.
func NewResultHandler(
	repo repository.JobResultRepository,
	stor storage.Storage,
	cfg *config.Config,
	log *slog.Logger,
) *ResultHandler {
	return &ResultHandler{repo: repo, stor: stor, cfg: cfg, log: log}
}

// DownloadResult handles GET /api/v1/jobs/{id}/result.
// Returns 302 to a presigned S3 URL for the processed image when the job is COMPLETED.
// Returns 409 when the job exists but is not yet COMPLETED. Returns 404 when not found.
func (h *ResultHandler) DownloadResult(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	jr, err := h.repo.GetResultByJobID(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		h.log.Error("get job result", "err", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to retrieve job")
		return
	}

	if jr.Status != "COMPLETED" {
		respondError(w, http.StatusConflict, "job result not ready: status is "+jr.Status)
		return
	}

	presignedURL, err := h.stor.PresignedDownloadURL(r.Context(), storage.BucketProcessed, *jr.ObjectKey, h.cfg.PresignedURLTTL)
	if err != nil {
		h.log.Error("presign result url", "err", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to generate result URL")
		return
	}

	http.Redirect(w, r, presignedURL, http.StatusFound)
}
