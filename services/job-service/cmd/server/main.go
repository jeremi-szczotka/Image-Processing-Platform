// services/job-service/cmd/server/main.go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"image-platform/job-service/internal/config"
	"image-platform/job-service/internal/handlers"
	"image-platform/job-service/internal/outbox"
	"image-platform/job-service/internal/publisher"
	"image-platform/job-service/internal/repository"
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

	pub, err := publisher.NewRabbitMQPublisher(cfg.RabbitMQURL)
	if err != nil {
		log.Error("connect rabbitmq", "err", err)
		os.Exit(1)
	}
	defer pub.Close()

	jobRepo := repository.NewPostgresJobRepository(db)
	outboxRepo := repository.NewPostgresOutboxRepository(db)

	relay := outbox.NewRelay(outboxRepo, pub, cfg.OutboxPollInterval, log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go relay.Run(ctx)

	jobHandler := handlers.NewJobHandler(jobRepo, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/jobs", jobHandler.Create)
		r.Get("/jobs/{id}", jobHandler.GetByID)
	})
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	log.Info("job-service starting", "port", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
