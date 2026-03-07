package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "image/gif"

	"golang.org/x/image/draw"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/albertvo/the-ranch/internal/config"
	"github.com/albertvo/the-ranch/internal/metrics"
	"github.com/albertvo/the-ranch/internal/pubsub"
	"github.com/albertvo/the-ranch/internal/queue"
	"github.com/albertvo/the-ranch/internal/repository"
	"github.com/albertvo/the-ranch/internal/storage"
)

// main initializes the worker node and starts the task processing loop.
func main() {
	// Healthcheck flag for K8s exec probe on distroless/scratch
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		fmt.Println("ok")
		os.Exit(0)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg := config.Load()

	// Database
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

	// MinIO
	if cfg.MinIOEndpoint == "" {
		logger.Error("MINIO_ENDPOINT is required")
		os.Exit(1)
	}
	store, err := storage.NewMinIOStorage(
		cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey,
		cfg.MinIOBucket, cfg.MinIOUseSSL,
	)
	if err != nil {
		logger.Error("connecting to minio", "error", err)
		os.Exit(1)
	}
	logger.Info("connected to minio", "bucket", cfg.MinIOBucket)

	// Redis
	if cfg.RedisURL == "" {
		logger.Error("REDIS_URL is required for worker")
		os.Exit(1)
	}
	redisClient, err := pubsub.ConnectRedis(context.Background(), cfg.RedisURL)
	if err != nil {
		logger.Error("connecting to redis", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	logger.Info("connected to redis")

	ps := pubsub.NewRedisPubSub(redisClient)
	repo := repository.NewFileRepository(db)
	producer := queue.NewProducer(redisClient)

	hostname, _ := os.Hostname()
	consumer := queue.NewConsumer(redisClient, hostname)

	if err := consumer.EnsureGroup(context.Background()); err != nil {
		logger.Error("ensuring consumer group", "error", err)
		os.Exit(1)
	}
	logger.Info("consumer group ready", "consumer", hostname)

	// Metrics endpoint for Prometheus scraping
	go func() {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("GET /metrics", promhttp.Handler())
		metricsMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "ok")
		})
		metricsServer := &http.Server{Addr: ":9090", Handler: metricsMux}
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "error", err)
		}
	}()

	// Graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-done
		logger.Info("shutting down worker...")
		cancel()
	}()

	logger.Info("worker started, listening for tasks")

	for {
		if ctx.Err() != nil {
			break
		}

		tasks, err := consumer.Read(ctx, 1, 5*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			logger.Error("reading tasks", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for _, task := range tasks {
			logger.Info("processing task", "id", task.ID, "type", task.Type)

			start := time.Now()
			err := processTask(ctx, logger, repo, store, ps, producer, task)
			duration := time.Since(start).Seconds()

			taskType := string(task.Type)
			if err != nil {
				metrics.WorkerTasksProcessed.WithLabelValues(taskType, "error").Inc()
				logger.Error("task failed", "id", task.ID, "type", task.Type, "error", err)
			} else {
				metrics.WorkerTasksProcessed.WithLabelValues(taskType, "success").Inc()
				logger.Info("task completed", "id", task.ID, "type", task.Type)
			}
			metrics.WorkerTaskDuration.WithLabelValues(taskType).Observe(duration)

			if err := consumer.Ack(ctx, task.ID); err != nil {
				logger.Error("acking task", "id", task.ID, "error", err)
			}
		}
	}

	logger.Info("worker shutdown complete")
}

// processTask routes a task to its specific handler based on its type.
func processTask(ctx context.Context, logger *slog.Logger, repo *repository.FileRepository, store storage.Storage, pub pubsub.Publisher, producer *queue.Producer, task queue.Task) error {
	switch task.Type {
	case queue.TaskProcessUpload:
		return handleProcessUpload(ctx, logger, repo, store, pub, producer, task)
	case queue.TaskGenerateThumbnail:
		return handleGenerateThumbnail(ctx, logger, repo, store, task)
	case queue.TaskCleanupOrphans:
		return handleCleanupOrphans(ctx, logger, repo, store)
	default:
		return fmt.Errorf("unknown task type: %s", task.Type)
	}
}

// handleProcessUpload verifies a file's integrity and updates its status in the database.
func handleProcessUpload(ctx context.Context, logger *slog.Logger, repo *repository.FileRepository, store storage.Storage, pub pubsub.Publisher, producer *queue.Producer, task queue.Task) error {
	fileID := task.Payload["file_id"]
	storageKey := task.Payload["storage_key"]
	expectedChecksum := task.Payload["checksum"]

	if err := repo.UpdateStatus(ctx, fileID, "processing"); err != nil {
		return fmt.Errorf("updating status to processing: %w", err)
	}

	// Download and re-verify checksum
	obj, err := store.Download(ctx, storageKey)
	if err != nil {
		return fmt.Errorf("downloading for verification: %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, obj); err != nil {
		obj.Close()
		return fmt.Errorf("hashing file: %w", err)
	}
	obj.Close()

	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksum != expectedChecksum {
		_ = repo.UpdateStatus(ctx, fileID, "checksum_mismatch")
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	// Detect MIME type from the file record
	file, err := repo.GetByID(ctx, fileID)
	if err != nil {
		return fmt.Errorf("getting file record: %w", err)
	}
	if file == nil {
		return fmt.Errorf("file %s not found", fileID)
	}

	if err := repo.MarkProcessed(ctx, fileID); err != nil {
		return fmt.Errorf("marking processed: %w", err)
	}

	// Publish event
	evt, _ := json.Marshal(map[string]string{
		"event":     "file_processed",
		"file_id":   fileID,
		"name":      file.Name,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	if err := pub.Publish(ctx, "filesync:events", string(evt)); err != nil {
		logger.Error("publishing file_processed event", "error", err)
	}

	// If it's an image, enqueue thumbnail generation
	if strings.HasPrefix(file.MimeType, "image/") {
		logger.Info("enqueuing thumbnail generation", "file_id", fileID)
		_, err := producer.Enqueue(ctx, queue.TaskGenerateThumbnail, map[string]string{
			"file_id":     fileID,
			"storage_key": storageKey,
		})
		if err != nil {
			logger.Error("failed to enqueue thumbnail task", "error", err, "file_id", fileID)
		}
	}

	return nil
}

// handleGenerateThumbnail creates a resized preview for supported image formats.
func handleGenerateThumbnail(ctx context.Context, logger *slog.Logger, repo *repository.FileRepository, store storage.Storage, task queue.Task) error {
	fileID := task.Payload["file_id"]
	storageKey := task.Payload["storage_key"]

	obj, err := store.Download(ctx, storageKey)
	if err != nil {
		return fmt.Errorf("downloading image: %w", err)
	}
	defer obj.Close()

	src, format, err := image.Decode(obj)
	if err != nil {
		return fmt.Errorf("decoding image: %w", err)
	}

	// Resize to max 256px on longest side
	bounds := src.Bounds()
	maxDim := 256
	w, h := bounds.Dx(), bounds.Dy()
	if w > h {
		h = h * maxDim / w
		w = maxDim
	} else {
		w = w * maxDim / h
		h = maxDim
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	var buf bytes.Buffer
	contentType := "image/jpeg"
	switch format {
	case "png":
		contentType = "image/png"
		if err := png.Encode(&buf, dst); err != nil {
			return fmt.Errorf("encoding png thumbnail: %w", err)
		}
	default:
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
			return fmt.Errorf("encoding jpeg thumbnail: %w", err)
		}
	}

	thumbnailKey := strings.Replace(storageKey, "uploads/", "thumbnails/", 1)
	if err := store.Upload(ctx, thumbnailKey, &buf, int64(buf.Len()), contentType); err != nil {
		return fmt.Errorf("uploading thumbnail: %w", err)
	}

	if err := repo.SetThumbnailKey(ctx, fileID, thumbnailKey); err != nil {
		return fmt.Errorf("setting thumbnail key: %w", err)
	}

	logger.Info("thumbnail generated", "file_id", fileID, "thumbnail_key", thumbnailKey)
	return nil
}

// handleCleanupOrphans deletes objects from storage that are no longer referenced in the database.
func handleCleanupOrphans(ctx context.Context, logger *slog.Logger, repo *repository.FileRepository, store storage.Storage) error {
	dbKeys, err := repo.ListStorageKeys(ctx)
	if err != nil {
		return fmt.Errorf("listing db keys: %w", err)
	}
	dbKeySet := make(map[string]struct{}, len(dbKeys))
	for _, k := range dbKeys {
		dbKeySet[k] = struct{}{}
	}

	storageKeys, err := store.ListKeys(ctx, "uploads/")
	if err != nil {
		return fmt.Errorf("listing storage keys: %w", err)
	}

	var deleted int
	for _, key := range storageKeys {
		if _, exists := dbKeySet[key]; !exists {
			if err := store.Delete(ctx, key); err != nil {
				logger.Error("deleting orphan", "key", key, "error", err)
				continue
			}
			deleted++
		}
	}

	logger.Info("orphan cleanup complete", "checked", len(storageKeys), "deleted", deleted)
	return nil
}
