package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/albertvo/the-ranch/internal/metrics"
	"github.com/albertvo/the-ranch/internal/model"
	"github.com/albertvo/the-ranch/internal/pubsub"
	"github.com/albertvo/the-ranch/internal/queue"
	"github.com/albertvo/the-ranch/internal/repository"
	"github.com/albertvo/the-ranch/internal/storage"
)

type FileHandler struct {
	repo      *repository.FileRepository
	storage   storage.Storage
	publisher pubsub.Publisher
	producer  *queue.Producer
	logger    *slog.Logger
}

func NewFileHandler(repo *repository.FileRepository, store storage.Storage, logger *slog.Logger) *FileHandler {
	return &FileHandler{repo: repo, storage: store, logger: logger}
}

// SetPublisher sets the optional event publisher for file change notifications.
func (h *FileHandler) SetPublisher(pub pubsub.Publisher) {
	h.publisher = pub
}

// SetProducer sets the optional task queue producer for background processing.
func (h *FileHandler) SetProducer(p *queue.Producer) {
	h.producer = p
}

// fileEvent represents a file change event published to Redis Pub/Sub.
type fileEvent struct {
	Event     string `json:"event"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	Timestamp string `json:"timestamp"`
}

// publishEvent publishes a file change event. It's a no-op if publisher is nil.
func (h *FileHandler) publishEvent(ctx context.Context, event string, fileID string, name string) {
	if h.publisher == nil {
		return
	}

	evt := fileEvent{
		Event:     event,
		FileID:    fileID,
		Name:      name,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(evt)
	if err != nil {
		h.logger.Error("marshaling file event", "error", err)
		return
	}

	if err := h.publisher.Publish(ctx, EventsChannel, string(data)); err != nil {
		h.logger.Error("publishing file event", "error", err, "event", event, "file_id", fileID)
	}
}

// Create handles metadata-only file record creation (no binary upload).
func (h *FileHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.Checksum == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and checksum are required"})
		return
	}

	file, err := h.repo.Create(r.Context(), req)
	if err != nil {
		h.logger.Error("creating file", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.publishEvent(r.Context(), "file_created", file.ID, file.Name)

	writeJSON(w, http.StatusCreated, file)
}

// Upload handles multipart file upload — streams to MinIO, then stores metadata in Postgres.
func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
	// 512MB max
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file field is required"})
		return
	}
	defer file.Close()

	// Hash the file content while streaming to MinIO
	hasher := sha256.New()
	reader := io.TeeReader(file, hasher)

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	storageKey := fmt.Sprintf("uploads/%s", header.Filename)

	// Stream directly to MinIO — no full buffering in memory
	if err := h.storage.Upload(r.Context(), storageKey, reader, header.Size, contentType); err != nil {
		h.logger.Error("uploading to storage", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage upload failed"})
		return
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Store metadata in Postgres
	req := model.CreateFileRequest{
		Name:       header.Filename,
		SizeBytes:  header.Size,
		MimeType:   contentType,
		Checksum:   checksum,
		StorageKey: &storageKey,
	}

	record, err := h.repo.Create(r.Context(), req)
	if err != nil {
		h.logger.Error("creating file record", "error", err)
		// Best-effort cleanup of the uploaded object
		_ = h.storage.Delete(r.Context(), storageKey)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	metrics.UploadBytesTotal.Add(float64(header.Size))

	h.publishEvent(r.Context(), "file_uploaded", record.ID, record.Name)

	// Enqueue background processing task
	if h.producer != nil {
		_, err := h.producer.Enqueue(r.Context(), queue.TaskProcessUpload, map[string]string{
			"file_id":     record.ID,
			"storage_key": storageKey,
			"checksum":    checksum,
		})
		if err != nil {
			h.logger.Error("enqueuing process_upload task", "error", err, "file_id", record.ID)
		}
	}

	writeJSON(w, http.StatusCreated, record)
}

// Download proxies file content from MinIO to the client.
func (h *FileHandler) Download(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	record, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("getting file for download", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if record == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}
	if record.StorageKey == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file has no stored content"})
		return
	}

	obj, err := h.storage.Download(r.Context(), *record.StorageKey)
	if err != nil {
		h.logger.Error("downloading from storage", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage download failed"})
		return
	}
	defer obj.Close()

	w.Header().Set("Content-Type", record.MimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, record.Name))
	io.Copy(w, obj)
}

func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	files, err := h.repo.List(r.Context())
	if err != nil {
		h.logger.Error("listing files", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if files == nil {
		files = []model.File{}
	}
	writeJSON(w, http.StatusOK, files)
}

func (h *FileHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	file, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("getting file", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if file == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	writeJSON(w, http.StatusOK, file)
}

// Delete removes from both MinIO and Postgres.
func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Look up the record first to get the storage key
	record, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("getting file for delete", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if record == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	// Delete from MinIO first
	if record.StorageKey != nil {
		if err := h.storage.Delete(r.Context(), *record.StorageKey); err != nil {
			h.logger.Error("deleting from storage", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage delete failed"})
			return
		}
	}

	// Then delete from Postgres
	if err := h.repo.Delete(r.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
			return
		}
		h.logger.Error("deleting file record", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.publishEvent(r.Context(), "file_deleted", record.ID, record.Name)

	w.WriteHeader(http.StatusNoContent)
}
