package handler

import (
	"log/slog"
	"net/http"

	"github.com/albertvo/the-ranch/internal/repository"
)

type StorageHandler struct {
	repo   *repository.FileRepository
	logger *slog.Logger
}

func NewStorageHandler(repo *repository.FileRepository, logger *slog.Logger) *StorageHandler {
	return &StorageHandler{repo: repo, logger: logger}
}

func (h *StorageHandler) Stats(w http.ResponseWriter, r *http.Request) {
	fileCount, totalBytes, err := h.repo.StorageStats(r.Context())
	if err != nil {
		h.logger.Error("querying storage stats", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"file_count":  fileCount,
		"total_bytes": totalBytes,
	})
}
