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

func (h *FileHandler) SetPublisher(pub pubsub.Publisher) {
	h.publisher = pub
}

func (h *FileHandler) SetProducer(p *queue.Producer) {
	h.producer = p
}

type fileEvent struct {
	Event     string `json:"event"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	Timestamp string `json:"timestamp"`
}

func (h *FileHandler) publishEvent(ctx context.Context, event, fileID, name string) {
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

func (h *FileHandler) Upload(w http.ResponseWriter, r *http.Request) {
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

	hasher := sha256.New()
	reader := io.TeeReader(file, hasher)

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	storageKey := fmt.Sprintf("uploads/%s", header.Filename)

	if err := h.storage.Upload(r.Context(), storageKey, reader, header.Size, contentType); err != nil {
		h.logger.Error("uploading to storage", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage upload failed"})
		return
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))

	directoryID := r.FormValue("directory_id")
	var dirPtr *string
	if directoryID != "" {
		dirPtr = &directoryID
	}

	req := model.CreateFileRequest{
		Name:        header.Filename,
		SizeBytes:   header.Size,
		MimeType:    contentType,
		Checksum:    checksum,
		StorageKey:  &storageKey,
		DirectoryID: dirPtr,
	}

	record, err := h.repo.Create(r.Context(), req)
	if err != nil {
		h.logger.Error("creating file record", "error", err)
		_ = h.storage.Delete(r.Context(), storageKey)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	metrics.UploadBytesTotal.Add(float64(header.Size))

	h.publishEvent(r.Context(), "file_uploaded", record.ID, record.Name)

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

func (h *FileHandler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	record, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("getting file for thumbnail", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if record == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}
	if record.ThumbnailKey == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "thumbnail not found"})
		return
	}

	obj, err := h.storage.Download(r.Context(), *record.ThumbnailKey)
	if err != nil {
		h.logger.Error("downloading thumbnail from storage", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage download failed"})
		return
	}
	defer obj.Close()

	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Cache-Control", "public, max-age=31536000")
	io.Copy(w, obj)
}

func (h *FileHandler) List(w http.ResponseWriter, r *http.Request) {
	directoryID := r.URL.Query().Get("directory_id")

	var files []model.File
	var err error
	if directoryID != "" {
		files, err = h.repo.ListByDirectory(r.Context(), &directoryID)
	} else if r.URL.Query().Has("directory_id") {
		// Explicit ?directory_id= (empty) means root
		files, err = h.repo.ListByDirectory(r.Context(), nil)
	} else {
		// No param = list all files
		files, err = h.repo.List(r.Context())
	}
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

func (h *FileHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var data map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	// Sanitize data: only allow directory_id and name for now
	allowed := map[string]bool{"directory_id": true, "name": true}
	sanitized := make(map[string]interface{})
	for k, v := range data {
		if allowed[k] {
			sanitized[k] = v
		}
	}

	record, err := h.repo.Update(r.Context(), id, sanitized)
	if err != nil {
		h.logger.Error("updating file", "error", err, "id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	h.publishEvent(r.Context(), "file_updated", record.ID, record.Name)
	writeJSON(w, http.StatusOK, record)
}

func (h *FileHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	for _, id := range req.IDs {
		record, err := h.repo.GetByID(r.Context(), id)
		if err != nil || record == nil {
			continue
		}
		if record.StorageKey != nil {
			_ = h.storage.Delete(r.Context(), *record.StorageKey)
		}
		h.publishEvent(r.Context(), "file_deleted", record.ID, record.Name)
	}

	if err := h.repo.BulkDelete(r.Context(), req.IDs); err != nil {
		h.logger.Error("bulk deleting files", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *FileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

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

	if record.StorageKey != nil {
		if err := h.storage.Delete(r.Context(), *record.StorageKey); err != nil {
			h.logger.Error("deleting from storage", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage delete failed"})
			return
		}
	}

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
