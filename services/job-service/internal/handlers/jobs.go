// services/job-service/internal/handlers/jobs.go
package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"image-platform/job-service/internal/domain"
	"image-platform/job-service/internal/repository"
)

// JobHandler handles HTTP requests for job creation and retrieval.
type JobHandler struct {
	repo repository.JobRepository
	log  *slog.Logger
}

// NewJobHandler creates a JobHandler backed by the supplied repository and logger.
func NewJobHandler(repo repository.JobRepository, log *slog.Logger) *JobHandler {
	return &JobHandler{repo: repo, log: log}
}

// createJobRequest is the JSON body expected by POST /api/v1/jobs.
type createJobRequest struct {
	ImageID    uuid.UUID      `json:"image_id"`
	Operation  string         `json:"operation"`
	Parameters map[string]any `json:"parameters"`
}

// Create handles POST /api/v1/jobs. It validates the request body, persists the job
// together with its outbox event in a single transaction, and returns 201 with the
// new job. Returns 400 when image_id is missing, the operation is unknown, or the
// referenced image does not exist.
func (h *JobHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.ImageID == uuid.Nil {
		respondError(w, http.StatusBadRequest, "image_id is required")
		return
	}
	if !domain.ValidOperations[req.Operation] {
		respondError(w, http.StatusBadRequest,
			"invalid operation; valid values: resize, compress, convert, grayscale, edge_detection")
		return
	}
	if req.Parameters == nil {
		req.Parameters = map[string]any{}
	}

	job := &domain.Job{
		ID:         uuid.New(),
		ImageID:    req.ImageID,
		Operation:  req.Operation,
		Parameters: req.Parameters,
	}

	payload, err := json.Marshal(map[string]any{
		"job_id":     job.ID,
		"image_id":   job.ImageID,
		"operation":  job.Operation,
		"parameters": job.Parameters,
	})
	if err != nil {
		h.log.Error("marshal outbox payload", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to create job")
		return
	}

	if err := h.repo.CreateWithOutbox(r.Context(), job, payload); err != nil {
		if errors.Is(err, repository.ErrImageNotFound) {
			respondError(w, http.StatusBadRequest, "image not found")
			return
		}
		h.log.Error("create job", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to create job")
		return
	}

	respondJSON(w, http.StatusCreated, job)
}

// GetByID handles GET /api/v1/jobs/{id}. It returns 404 when no job matches
// the given UUID and 400 when the UUID cannot be parsed.
func (h *JobHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	job, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		respondError(w, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		h.log.Error("get job", "err", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to retrieve job")
		return
	}

	respondJSON(w, http.StatusOK, job)
}
