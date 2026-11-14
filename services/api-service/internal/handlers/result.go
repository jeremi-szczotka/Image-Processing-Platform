package handlers

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"image-platform/api-service/internal/config"
	"image-platform/api-service/internal/domain"
	"image-platform/api-service/internal/repository"
	"image-platform/api-service/internal/storage"
)

// ResultHandler serves processed-image results, streaming bytes directly from object storage.
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
// Streams the processed image bytes from object storage with the correct Content-Type.
// When ?download=1 is present, adds Content-Disposition: attachment so the browser
// saves the file rather than displaying it.
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

	if jr.ObjectKey == nil {
		h.log.Error("completed job has no result_object_key", "id", id)
		respondError(w, http.StatusInternalServerError, "result not available")
		return
	}

	reader, contentType, err := h.stor.GetObject(r.Context(), storage.BucketProcessed, *jr.ObjectKey)
	if err != nil {
		h.log.Error("get result object", "err", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to retrieve result")
		return
	}
	defer reader.Close()

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)

	if r.URL.Query().Get("download") == "1" {
		ext := extensionForContentType(contentType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="result%s"`, ext))
	}

	if _, err := io.Copy(w, reader); err != nil {
		h.log.Error("stream result", "err", err, "id", id)
	}
}

func extensionForContentType(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
