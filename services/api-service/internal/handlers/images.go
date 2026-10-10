package handlers

import (
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp"

	"image-platform/api-service/internal/config"
	"image-platform/api-service/internal/domain"
	"image-platform/api-service/internal/repository"
	"image-platform/api-service/internal/storage"
)

var allowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

// ImageHandler handles HTTP requests for image upload and retrieval.
type ImageHandler struct {
	repo repository.ImageRepository
	stor storage.Storage
	cfg  *config.Config
	log  *slog.Logger
}

func NewImageHandler(
	repo repository.ImageRepository,
	stor storage.Storage,
	cfg *config.Config,
	log *slog.Logger,
) *ImageHandler {
	return &ImageHandler{repo: repo, stor: stor, cfg: cfg, log: log}
}

// Upload handles POST /api/v1/images
func (h *ImageHandler) Upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes)

	if err := r.ParseMultipartForm(h.cfg.MaxUploadBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			respondError(w, http.StatusRequestEntityTooLarge, "file exceeds maximum allowed size")
			return
		}
		respondError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		respondError(w, http.StatusBadRequest, "missing required field: image")
		return
	}
	defer file.Close()

	// Detect content type from file bytes, not filename or declared MIME.
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		h.log.Error("read file header", "err", err)
		respondError(w, http.StatusInternalServerError, "failed to read file")
		return
	}
	contentType := http.DetectContentType(buf[:n])

	if !allowedContentTypes[contentType] {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("unsupported image format: %s", contentType))
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to process file")
		return
	}

	// Read image dimensions without decoding pixel data (pixel-bomb protection).
	imgCfg, _, err := image.DecodeConfig(file)
	if err != nil {
		respondError(w, http.StatusBadRequest, "cannot read image dimensions: file may be corrupt")
		return
	}
	if int64(imgCfg.Width)*int64(imgCfg.Height) > h.cfg.MaxMegapixels {
		respondError(w, http.StatusBadRequest, "image exceeds maximum allowed dimensions")
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to process file")
		return
	}

	imageID := uuid.New()
	objectKey := fmt.Sprintf("%s/original%s", imageID, extensionFor(contentType))

	if err := h.stor.Upload(r.Context(), storage.BucketOriginal, objectKey, file, header.Size, contentType); err != nil {
		h.log.Error("upload to storage", "err", err, "imageID", imageID)
		respondError(w, http.StatusInternalServerError, "failed to store image")
		return
	}

	img := &domain.Image{
		ID:               imageID,
		OriginalFilename: header.Filename,
		ObjectKey:        objectKey,
		ContentType:      contentType,
		FileSize:         header.Size,
		Width:            imgCfg.Width,
		Height:           imgCfg.Height,
	}
	if err := h.repo.Create(r.Context(), img); err != nil {
		h.log.Error("save image metadata", "err", err, "imageID", imageID)
		respondError(w, http.StatusInternalServerError, "failed to save image metadata")
		return
	}

	respondJSON(w, http.StatusCreated, img)
}

// GetByID handles GET /api/v1/images/{id}
func (h *ImageHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid image id")
		return
	}

	img, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		respondError(w, http.StatusNotFound, "image not found")
		return
	}
	if err != nil {
		h.log.Error("get image", "err", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to retrieve image")
		return
	}

	respondJSON(w, http.StatusOK, img)
}

// Download handles GET /api/v1/images/{id}/download
// Redirects the client to a short-lived presigned MinIO URL.
func (h *ImageHandler) Download(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid image id")
		return
	}

	img, err := h.repo.GetByID(r.Context(), id)
	if errors.Is(err, domain.ErrNotFound) {
		respondError(w, http.StatusNotFound, "image not found")
		return
	}
	if err != nil {
		h.log.Error("get image for download", "err", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to retrieve image")
		return
	}

	presignedURL, err := h.stor.PresignedDownloadURL(r.Context(), storage.BucketOriginal, img.ObjectKey, h.cfg.PresignedURLTTL)
	if err != nil {
		h.log.Error("generate presigned URL", "err", err, "id", id)
		respondError(w, http.StatusInternalServerError, "failed to generate download URL")
		return
	}

	http.Redirect(w, r, presignedURL, http.StatusFound)
}

func extensionFor(contentType string) string {
	switch contentType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
