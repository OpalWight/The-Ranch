package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/albertvo/the-ranch/internal/config"
	"github.com/albertvo/the-ranch/internal/handler"
	"github.com/albertvo/the-ranch/internal/middleware"
	"github.com/albertvo/the-ranch/internal/pubsub"
	"github.com/albertvo/the-ranch/internal/queue"
	"github.com/albertvo/the-ranch/internal/repository"
	"github.com/albertvo/the-ranch/internal/storage"

	_ "github.com/albertvo/the-ranch/internal/metrics" // register Prometheus metrics
)

// main initializes dependencies and starts the HTTP API server.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg := config.Load()

	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required")
		os.Exit(1)
	}

	db, err := repository.ConnectPostgres(cfg.DatabaseURL)
	if err != nil {
		logger.Error("connecting to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("connected to database")

	// MinIO storage
	if cfg.MinIOEndpoint == "" {
		logger.Error("MINIO_ENDPOINT is required")
		os.Exit(1)
	}

	store, err := storage.NewMinIOStorage(
		cfg.MinIOEndpoint,
		cfg.MinIOAccessKey,
		cfg.MinIOSecretKey,
		cfg.MinIOBucket,
		cfg.MinIOUseSSL,
	)
	if err != nil {
		logger.Error("connecting to minio", "error", err)
		os.Exit(1)
	}

	if err := store.EnsureBucket(context.Background()); err != nil {
		logger.Error("ensuring minio bucket", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to minio", "bucket", cfg.MinIOBucket)

	// Redis (optional — gracefully degrade if not configured)
	var ps pubsub.PubSub
	var taskProducer *queue.Producer
	if cfg.RedisURL != "" {
		redisClient, err := pubsub.ConnectRedis(context.Background(), cfg.RedisURL)
		if err != nil {
			logger.Error("connecting to redis", "error", err)
			os.Exit(1)
		}
		defer redisClient.Close()
		ps = pubsub.NewRedisPubSub(redisClient)
		taskProducer = queue.NewProducer(redisClient)
		logger.Info("connected to redis")
	} else {
		logger.Warn("REDIS_URL not set, real-time events and task queue disabled")
	}

	fileRepo := repository.NewFileRepository(db)
	dirRepo := repository.NewDirectoryRepository(db)
	healthHandler := handler.NewHealthHandler(db)
	fileHandler := handler.NewFileHandler(fileRepo, store, logger)
	dirHandler := handler.NewDirectoryHandler(dirRepo, fileRepo, logger)
	storageHandler := handler.NewStorageHandler(fileRepo, logger)
	if ps != nil {
		fileHandler.SetPublisher(ps)
	}
	if taskProducer != nil {
		fileHandler.SetProducer(taskProducer)
	}

	mux := http.NewServeMux()

	// Health probes
	mux.HandleFunc("GET /healthz", healthHandler.Liveness)
	mux.HandleFunc("GET /readyz", healthHandler.Readiness)

	// File CRUD (metadata)
	mux.HandleFunc("POST /api/v1/files", fileHandler.Create)
	mux.HandleFunc("GET /api/v1/files", fileHandler.List)
	mux.HandleFunc("GET /api/v1/files/{id}", fileHandler.GetByID)
	mux.HandleFunc("PATCH /api/v1/files/{id}", fileHandler.Update)
	mux.HandleFunc("DELETE /api/v1/files/{id}", fileHandler.Delete)
	mux.HandleFunc("DELETE /api/v1/files/bulk", fileHandler.BulkDelete)

	// File upload/download (binary)
	mux.HandleFunc("POST /api/v1/files/upload", fileHandler.Upload)
	mux.HandleFunc("GET /api/v1/files/{id}/download", fileHandler.Download)
	mux.HandleFunc("GET /api/v1/files/{id}/thumbnail", fileHandler.Thumbnail)

	// Directory CRUD
	mux.HandleFunc("POST /api/v1/directories", dirHandler.Create)
	mux.HandleFunc("GET /api/v1/directories", dirHandler.List)
	mux.HandleFunc("GET /api/v1/directories/{id}", dirHandler.GetByID)
	mux.HandleFunc("GET /api/v1/directories/{id}/contents", dirHandler.Contents)
	mux.HandleFunc("PATCH /api/v1/directories/{id}", dirHandler.Update)
	mux.HandleFunc("DELETE /api/v1/directories/{id}", dirHandler.Delete)
	mux.HandleFunc("DELETE /api/v1/directories/bulk", dirHandler.BulkDelete)

	// Storage stats
	mux.HandleFunc("GET /api/v1/storage/stats", storageHandler.Stats)

	// SSE real-time events
	if ps != nil {
		eventHandler := handler.NewEventHandler(ps, logger)
		mux.HandleFunc("GET /api/v1/events/stream", eventHandler.Stream)
	}

	// Prometheus metrics
	mux.Handle("GET /metrics", promhttp.Handler())

	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: middleware.Metrics(middleware.Logging(logger)(mux)),
	}

	// Graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("The Ranch is up", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	logger.Info("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

	logger.Info("shutdown complete")
}
