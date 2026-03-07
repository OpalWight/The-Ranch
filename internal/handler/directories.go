package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/albertvo/the-ranch/internal/model"
	"github.com/albertvo/the-ranch/internal/repository"
)

// DirectoryHandler handles HTTP requests for directory CRUD operations.
type DirectoryHandler struct {
	dirRepo  *repository.DirectoryRepository
	fileRepo *repository.FileRepository
	logger   *slog.Logger
}

// NewDirectoryHandler creates a new instance of DirectoryHandler.
func NewDirectoryHandler(dirRepo *repository.DirectoryRepository, fileRepo *repository.FileRepository, logger *slog.Logger) *DirectoryHandler {
	return &DirectoryHandler{dirRepo: dirRepo, fileRepo: fileRepo, logger: logger}
}

// Create handles POST /api/v1/directories.
func (h *DirectoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateDirectoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	if req.ParentID != nil {
		parent, err := h.dirRepo.GetByID(r.Context(), *req.ParentID)
		if err != nil {
			h.logger.Error("getting parent directory", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		if parent == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent directory not found"})
			return
		}
	}

	dir, err := h.dirRepo.Create(r.Context(), req)
	if err != nil {
		h.logger.Error("creating directory", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusCreated, dir)
}

// List handles GET /api/v1/directories.
func (h *DirectoryHandler) List(w http.ResponseWriter, r *http.Request) {
	parentID := r.URL.Query().Get("parent_id")
	var parentPtr *string
	if parentID != "" {
		parentPtr = &parentID
	}

	dirs, err := h.dirRepo.ListByParent(r.Context(), parentPtr)
	if err != nil {
		h.logger.Error("listing directories", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if dirs == nil {
		dirs = []model.Directory{}
	}
	writeJSON(w, http.StatusOK, dirs)
}

// GetByID handles GET /api/v1/directories/{id}.
func (h *DirectoryHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	dir, err := h.dirRepo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("getting directory", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if dir == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "directory not found"})
		return
	}
	writeJSON(w, http.StatusOK, dir)
}

// Contents handles GET /api/v1/directories/{id}/contents.
func (h *DirectoryHandler) Contents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dir, err := h.dirRepo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("getting directory", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if dir == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "directory not found"})
		return
	}

	subdirs, err := h.dirRepo.ListByParent(r.Context(), &id)
	if err != nil {
		h.logger.Error("listing subdirectories", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	files, err := h.fileRepo.ListByDirectory(r.Context(), &id)
	if err != nil {
		h.logger.Error("listing files in directory", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	breadcrumb, err := h.dirRepo.GetBreadcrumb(r.Context(), id)
	if err != nil {
		h.logger.Error("getting breadcrumb", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if subdirs == nil {
		subdirs = []model.Directory{}
	}
	if files == nil {
		files = []model.File{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"directory":   dir,
		"directories": subdirs,
		"files":       files,
		"breadcrumb":  breadcrumb,
	})
}

func (h *DirectoryHandler) BulkDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	var deletable []string
	for _, id := range req.IDs {
		hasChildren, err := h.dirRepo.HasChildren(r.Context(), id)
		if err != nil || hasChildren {
			continue
		}
		deletable = append(deletable, id)
	}

	if err := h.dirRepo.BulkDelete(r.Context(), deletable); err != nil {
		h.logger.Error("bulk deleting directories", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Delete handles DELETE /api/v1/directories/{id}.
func (h *DirectoryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dir, err := h.dirRepo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("getting directory for delete", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if dir == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "directory not found"})
		return
	}

	hasChildren, err := h.dirRepo.HasChildren(r.Context(), id)
	if err != nil {
		h.logger.Error("checking directory children", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if hasChildren {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "directory is not empty"})
		return
	}

	if err := h.dirRepo.Delete(r.Context(), id); err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "directory not found"})
			return
		}
		h.logger.Error("deleting directory", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Update handles PATCH /api/v1/directories/{id}.
func (h *DirectoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	dir, err := h.dirRepo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("getting directory for update", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	if dir == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "directory not found"})
		return
	}

	var req model.UpdateDirectoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	updated, err := h.dirRepo.Update(r.Context(), id, req)
	if err != nil {
		h.logger.Error("updating directory", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, updated)
}
