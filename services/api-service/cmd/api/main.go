// Package main is the entry point for the api-service HTTP server.
// It wires together configuration, database, object storage, and HTTP routing,
// then starts listening for requests.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"image-platform/api-service/internal/config"
	"image-platform/api-service/internal/handlers"
	"image-platform/api-service/internal/repository"
	"image-platform/api-service/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		log.Error("config", "err", err)
		os.Exit(1)
	}

	db, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Error("connect postgres", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	if err := db.Ping(context.Background()); err != nil {
		log.Error("ping postgres", "err", err)
		os.Exit(1)
	}

	minioStorage, err := storage.NewMinioStorage(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey, cfg.MinioUseSSL)
	if err != nil {
		log.Error("connect minio", "err", err)
		os.Exit(1)
	}
	if err := minioStorage.EnsureBuckets(context.Background()); err != nil {
		log.Error("ensure buckets", "err", err)
		os.Exit(1)
	}

	imageRepo := repository.NewPostgresImageRepository(db)
	imageHandler := handlers.NewImageHandler(imageRepo, minioStorage, cfg, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/images", imageHandler.Upload)
		r.Get("/images/{id}", imageHandler.GetByID)
		r.Get("/images/{id}/download", imageHandler.Download)
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	log.Info("api-service starting", "port", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
