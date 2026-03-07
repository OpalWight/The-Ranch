# Plan: Svelte Frontend + Directories + Tailscale

## Decisions
- Directories: Separate `directories` table, self-referencing parent_id, reject delete if non-empty
- Frontend: Svelte SPA in `web/`, nginx container with reverse proxy to API
- Namespace: All app components in `default`
- Remote access: Tailscale K8s Operator with MagicDNS domain
- Ingress: Split routing — `/` -> web, `/api/*` -> API, annotated for Tailscale

---

## Step 1: Migration 003_add_directories

### New file: `migrations/003_add_directories.up.sql`
```sql
CREATE TABLE IF NOT EXISTS directories (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    parent_id  UUID REFERENCES directories(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (parent_id, name)
);

CREATE INDEX idx_directories_parent_id ON directories(parent_id);

ALTER TABLE files ADD COLUMN directory_id UUID REFERENCES directories(id);
CREATE INDEX idx_files_directory_id ON files(directory_id);
```

### New file: `migrations/003_add_directories.down.sql`
```sql
DROP INDEX IF EXISTS idx_files_directory_id;
ALTER TABLE files DROP COLUMN IF EXISTS directory_id;

DROP INDEX IF EXISTS idx_directories_parent_id;
DROP TABLE IF EXISTS directories;
```

### Modified file: `deploy/k8s/base/configmap.yaml`
Add to the `filesync-migrations` ConfigMap data:
```yaml
  003_add_directories.up.sql: |
    CREATE TABLE IF NOT EXISTS directories (
        id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        name       TEXT NOT NULL,
        parent_id  UUID REFERENCES directories(id),
        created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
        UNIQUE (parent_id, name)
    );

    CREATE INDEX idx_directories_parent_id ON directories(parent_id);

    ALTER TABLE files ADD COLUMN directory_id UUID REFERENCES directories(id);
    CREATE INDEX idx_files_directory_id ON files(directory_id);
  003_add_directories.down.sql: |
    DROP INDEX IF EXISTS idx_files_directory_id;
    ALTER TABLE files DROP COLUMN IF EXISTS directory_id;

    DROP INDEX IF EXISTS idx_directories_parent_id;
    DROP TABLE IF EXISTS directories;
```

---

## Step 2: Directory model

### New file: `internal/model/directory.go`
```go
package model

import "time"

// Directory represents a folder that can contain files and subdirectories.
type Directory struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ParentID  *string    `json:"parent_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// CreateDirectoryRequest defines the payload for creating a new directory.
type CreateDirectoryRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id,omitempty"`
}

// UpdateDirectoryRequest defines the payload for renaming or moving a directory.
type UpdateDirectoryRequest struct {
	Name     *string `json:"name,omitempty"`
	ParentID *string `json:"parent_id,omitempty"`
}

// DirectoryContents holds the files and subdirectories within a directory.
type DirectoryContents struct {
	Directory     *Directory  `json:"directory,omitempty"`
	Directories   []Directory `json:"directories"`
	Files         []File      `json:"files"`
}
```

---

## Step 3: Directory repository

### New file: `internal/repository/directory.go`
```go
package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/albertvo/the-ranch/internal/model"
)

type DirectoryRepository struct {
	db *sql.DB
}

func NewDirectoryRepository(db *sql.DB) *DirectoryRepository {
	return &DirectoryRepository{db: db}
}

const dirCols = `id, name, parent_id, created_at, updated_at`

func scanDirectory(scanner interface{ Scan(...any) error }) (*model.Directory, error) {
	var d model.Directory
	err := scanner.Scan(&d.ID, &d.Name, &d.ParentID, &d.CreatedAt, &d.UpdatedAt)
	return &d, err
}

func (r *DirectoryRepository) Create(ctx context.Context, req model.CreateDirectoryRequest) (*model.Directory, error) {
	row := r.db.QueryRowContext(ctx,
		`INSERT INTO directories (name, parent_id) VALUES ($1, $2) RETURNING `+dirCols,
		req.Name, req.ParentID,
	)
	d, err := scanDirectory(row)
	if err != nil {
		return nil, fmt.Errorf("inserting directory: %w", err)
	}
	return d, nil
}

func (r *DirectoryRepository) GetByID(ctx context.Context, id string) (*model.Directory, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+dirCols+` FROM directories WHERE id = $1`, id,
	)
	d, err := scanDirectory(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting directory: %w", err)
	}
	return d, nil
}

// ListByParent lists subdirectories of a given parent (NULL = root).
func (r *DirectoryRepository) ListByParent(ctx context.Context, parentID *string) ([]model.Directory, error) {
	var rows *sql.Rows
	var err error
	if parentID == nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+dirCols+` FROM directories WHERE parent_id IS NULL ORDER BY name`)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+dirCols+` FROM directories WHERE parent_id = $1 ORDER BY name`, *parentID)
	}
	if err != nil {
		return nil, fmt.Errorf("listing directories: %w", err)
	}
	defer rows.Close()

	var dirs []model.Directory
	for rows.Next() {
		d, err := scanDirectory(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning directory: %w", err)
		}
		dirs = append(dirs, *d)
	}
	return dirs, rows.Err()
}

// HasChildren returns true if the directory contains any files or subdirectories.
func (r *DirectoryRepository) HasChildren(ctx context.Context, id string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM directories WHERE parent_id = $1) +
		        (SELECT COUNT(*) FROM files WHERE directory_id = $1)`, id,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking children: %w", err)
	}
	return count > 0, nil
}

func (r *DirectoryRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM directories WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting directory: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *DirectoryRepository) Update(ctx context.Context, id string, req model.UpdateDirectoryRequest) (*model.Directory, error) {
	// Build dynamic update
	if req.Name != nil {
		if _, err := r.db.ExecContext(ctx,
			`UPDATE directories SET name = $1, updated_at = NOW() WHERE id = $2`,
			*req.Name, id); err != nil {
			return nil, fmt.Errorf("updating directory name: %w", err)
		}
	}
	if req.ParentID != nil {
		if _, err := r.db.ExecContext(ctx,
			`UPDATE directories SET parent_id = $1, updated_at = NOW() WHERE id = $2`,
			req.ParentID, id); err != nil {
			return nil, fmt.Errorf("updating directory parent: %w", err)
		}
	}
	return r.GetByID(ctx, id)
}

// GetBreadcrumb returns the directory chain from root to the given directory.
func (r *DirectoryRepository) GetBreadcrumb(ctx context.Context, id string) ([]model.Directory, error) {
	rows, err := r.db.QueryContext(ctx,
		`WITH RECURSIVE ancestors AS (
			SELECT `+dirCols+` FROM directories WHERE id = $1
			UNION ALL
			SELECT d.id, d.name, d.parent_id, d.created_at, d.updated_at
			FROM directories d
			JOIN ancestors a ON d.id = a.parent_id
		)
		SELECT `+dirCols+` FROM ancestors ORDER BY created_at`, id)
	if err != nil {
		return nil, fmt.Errorf("getting breadcrumb: %w", err)
	}
	defer rows.Close()

	var dirs []model.Directory
	for rows.Next() {
		d, err := scanDirectory(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning ancestor: %w", err)
		}
		dirs = append(dirs, *d)
	}
	return dirs, rows.Err()
}
```

---

## Step 4: Directory handler + routes

### New file: `internal/handler/directories.go`
```go
package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/albertvo/the-ranch/internal/model"
	"github.com/albertvo/the-ranch/internal/pubsub"
	"github.com/albertvo/the-ranch/internal/repository"
)

type DirectoryHandler struct {
	dirRepo  *repository.DirectoryRepository
	fileRepo *repository.FileRepository
	publisher pubsub.Publisher
	logger   *slog.Logger
}

func NewDirectoryHandler(dirRepo *repository.DirectoryRepository, fileRepo *repository.FileRepository, logger *slog.Logger) *DirectoryHandler {
	return &DirectoryHandler{dirRepo: dirRepo, fileRepo: fileRepo, logger: logger}
}

func (h *DirectoryHandler) SetPublisher(pub pubsub.Publisher) {
	h.publisher = pub
}

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

	// Validate parent exists if specified
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

// Contents returns the directory, its subdirectories, files, and breadcrumb.
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
```

### Modified file: `cmd/api/main.go`
Add after file handler setup (around line 93):
```go
	dirRepo := repository.NewDirectoryRepository(db)
	dirHandler := handler.NewDirectoryHandler(dirRepo, fileRepo, logger)
	if ps != nil {
		dirHandler.SetPublisher(ps)
	}
```

Add route registrations (after line 114):
```go
	// Directory CRUD
	mux.HandleFunc("POST /api/v1/directories", dirHandler.Create)
	mux.HandleFunc("GET /api/v1/directories", dirHandler.List)
	mux.HandleFunc("GET /api/v1/directories/{id}", dirHandler.GetByID)
	mux.HandleFunc("GET /api/v1/directories/{id}/contents", dirHandler.Contents)
	mux.HandleFunc("PATCH /api/v1/directories/{id}", dirHandler.Update)
	mux.HandleFunc("DELETE /api/v1/directories/{id}", dirHandler.Delete)
```

---

## Step 5: Update file model/repo/handler for directory_id

### Modified file: `internal/model/file.go`
Add `DirectoryID` field to File struct:
```go
type File struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	SizeBytes    int64      `json:"size_bytes"`
	MimeType     string     `json:"mime_type"`
	Checksum     string     `json:"checksum"`
	StorageKey   *string    `json:"storage_key,omitempty"`
	DirectoryID  *string    `json:"directory_id,omitempty"`
	Status       string     `json:"status"`
	ThumbnailKey *string    `json:"thumbnail_key,omitempty"`
	ProcessedAt  *time.Time `json:"processed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type CreateFileRequest struct {
	Name        string  `json:"name"`
	SizeBytes   int64   `json:"size_bytes"`
	MimeType    string  `json:"mime_type"`
	Checksum    string  `json:"checksum"`
	StorageKey  *string `json:"storage_key,omitempty"`
	DirectoryID *string `json:"directory_id,omitempty"`
}
```

### Modified file: `internal/repository/postgres.go`
Update `fileCols`:
```go
const fileCols = `id, name, size_bytes, mime_type, checksum, storage_key, directory_id, status, thumbnail_key, processed_at, created_at, updated_at`
```

Update `scanFile`:
```go
func scanFile(scanner interface{ Scan(...any) error }) (*model.File, error) {
	var f model.File
	err := scanner.Scan(
		&f.ID, &f.Name, &f.SizeBytes, &f.MimeType, &f.Checksum,
		&f.StorageKey, &f.DirectoryID, &f.Status, &f.ThumbnailKey, &f.ProcessedAt,
		&f.CreatedAt, &f.UpdatedAt,
	)
	return &f, err
}
```

Update `Create` INSERT:
```go
func (r *FileRepository) Create(ctx context.Context, req model.CreateFileRequest) (*model.File, error) {
	row := r.db.QueryRowContext(ctx,
		`INSERT INTO files (name, size_bytes, mime_type, checksum, storage_key, directory_id)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+fileCols,
		req.Name, req.SizeBytes, req.MimeType, req.Checksum, req.StorageKey, req.DirectoryID,
	)
	f, err := scanFile(row)
	if err != nil {
		return nil, fmt.Errorf("inserting file: %w", err)
	}
	return f, nil
}
```

Add new `ListByDirectory` method:
```go
// ListByDirectory lists files in a given directory (NULL = root).
func (r *FileRepository) ListByDirectory(ctx context.Context, directoryID *string) ([]model.File, error) {
	var rows *sql.Rows
	var err error
	if directoryID == nil {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+fileCols+` FROM files WHERE directory_id IS NULL ORDER BY created_at DESC`)
	} else {
		rows, err = r.db.QueryContext(ctx,
			`SELECT `+fileCols+` FROM files WHERE directory_id = $1 ORDER BY created_at DESC`, *directoryID)
	}
	if err != nil {
		return nil, fmt.Errorf("listing files by directory: %w", err)
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		f, err := scanFile(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning file: %w", err)
		}
		files = append(files, *f)
	}
	return files, rows.Err()
}
```

### Modified file: `internal/handler/files.go`
Update `Upload` to accept `directory_id` from the multipart form:
```go
// In the Upload function, after getting the file from the form, add:
	directoryID := r.FormValue("directory_id")
	var dirPtr *string
	if directoryID != "" {
		dirPtr = &directoryID
	}

// Update the CreateFileRequest to include DirectoryID:
	req := model.CreateFileRequest{
		Name:        header.Filename,
		SizeBytes:   header.Size,
		MimeType:    contentType,
		Checksum:    checksum,
		StorageKey:  &storageKey,
		DirectoryID: dirPtr,
	}
```

Update the `List` handler to accept a `directory_id` query param:
```go
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
```

---

## Step 6: Scaffold Svelte app

### Directory structure:
```
web/
├── package.json
├── svelte.config.js
├── vite.config.js
├── tsconfig.json
├── src/
│   ├── app.html
│   ├── app.css
│   ├── lib/
│   │   ├── api.ts              # API client
│   │   ├── sse.ts              # SSE event client
│   │   ├── types.ts            # TypeScript interfaces matching Go models
│   │   └── utils.ts            # Format helpers (file size, dates)
│   └── routes/
│       ├── +layout.svelte      # App shell / nav wrapper
│       └── +page.svelte        # Main file browser
├── static/
│   └── favicon.png
├── Dockerfile
└── nginx.conf
```

### Key file: `web/src/lib/types.ts`
```typescript
export interface File {
  id: string;
  name: string;
  size_bytes: number;
  mime_type: string;
  checksum: string;
  storage_key?: string;
  directory_id?: string;
  status: string;
  thumbnail_key?: string;
  processed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface Directory {
  id: string;
  name: string;
  parent_id?: string;
  created_at: string;
  updated_at: string;
}

export interface DirectoryContents {
  directory?: Directory;
  directories: Directory[];
  files: File[];
  breadcrumb: Directory[];
}

export interface FileEvent {
  event: string;
  file_id: string;
  name: string;
  timestamp: string;
}
```

### Key file: `web/src/lib/api.ts`
```typescript
const BASE = '/api/v1';

async function request<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, opts);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

// Files
export const listFiles = (directoryId?: string | null) => {
  const params = new URLSearchParams();
  if (directoryId !== undefined) {
    params.set('directory_id', directoryId ?? '');
  }
  return request<File[]>(`/files?${params}`);
};

export const uploadFile = (file: globalThis.File, directoryId?: string) => {
  const form = new FormData();
  form.append('file', file);
  if (directoryId) form.append('directory_id', directoryId);
  return request<File>('/files/upload', { method: 'POST', body: form });
};

export const deleteFile = (id: string) =>
  request<void>(`/files/${id}`, { method: 'DELETE' });

export const downloadUrl = (id: string) => `${BASE}/files/${id}/download`;

// Directories
export const listDirectories = (parentId?: string | null) => {
  const params = new URLSearchParams();
  if (parentId !== undefined) {
    params.set('parent_id', parentId ?? '');
  }
  return request<Directory[]>(`/directories?${params}`);
};

export const createDirectory = (name: string, parentId?: string) =>
  request<Directory>('/directories', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, parent_id: parentId || undefined }),
  });

export const getDirectoryContents = (id: string) =>
  request<DirectoryContents>(`/directories/${id}/contents`);

export const deleteDirectory = (id: string) =>
  request<void>(`/directories/${id}`, { method: 'DELETE' });

export const updateDirectory = (id: string, data: { name?: string; parent_id?: string }) =>
  request<Directory>(`/directories/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
```

### Key file: `web/src/lib/sse.ts`
```typescript
import type { FileEvent } from './types';

export function connectSSE(onEvent: (event: FileEvent) => void): EventSource {
  const source = new EventSource('/api/v1/events/stream');

  source.addEventListener('file_changed', (e: MessageEvent) => {
    try {
      const data: FileEvent = JSON.parse(e.data);
      onEvent(data);
    } catch (err) {
      console.error('Failed to parse SSE event:', err);
    }
  });

  source.onerror = () => {
    console.warn('SSE connection lost, will auto-reconnect...');
  };

  return source;
}
```

---

## Step 7: File browser UI

### Key file: `web/src/routes/+page.svelte`
Main features:
- **Breadcrumb bar** at top: Root > Folder > Subfolder (clickable to navigate)
- **Directory grid/list** showing folders first, then files
- **Folder icons** for directories, file-type icons for files
- **"New Folder" button** — inline text input or modal
- **Upload zone** — drag-and-drop area + click-to-browse, uploads to current directory
- **File actions** — download (click), delete (button with confirm)
- **Activity feed sidebar** — SSE-powered live feed of file events
- **Empty state** — friendly message when a directory is empty
- **File size formatting** — human-readable (KB, MB, GB)
- **Responsive layout** — works on mobile (Tailscale means phones on your tailnet too)

The page state is driven by a `currentDirectoryId` variable:
- `null` = root (list root dirs + root files)
- `string` = inside a directory (use `/directories/{id}/contents`)

Navigation updates `currentDirectoryId` and refetches. SSE events trigger a refetch of the current view.

---

## Step 8: Web Dockerfile + nginx config

### New file: `web/Dockerfile`
```dockerfile
# Build stage
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# Runtime stage
FROM nginx:alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/build /usr/share/nginx/html
EXPOSE 80
```

### New file: `web/nginx.conf`
```nginx
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    # SPA fallback — serve index.html for all non-file routes
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Reverse proxy API calls to the backend service
    location /api/ {
        proxy_pass http://filesync-api.default.svc.cluster.local/api/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE support
        proxy_set_header Connection '';
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 86400s;
    }

    # Health check for k8s probes
    location /nginx-health {
        return 200 'ok';
        add_header Content-Type text/plain;
    }
}
```

---

## Step 9: K8s manifests (web deployment, service, updated ingress)

### New file: `deploy/k8s/base/web-deployment.yaml`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: filesync-web
spec:
  replicas: 1
  selector:
    matchLabels:
      app: filesync
      component: web
  template:
    metadata:
      labels:
        app: filesync
        component: web
    spec:
      containers:
        - name: web
          image: ghcr.io/albertvo/homelab/web:latest
          ports:
            - containerPort: 80
              name: http
          livenessProbe:
            httpGet:
              path: /nginx-health
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /nginx-health
              port: http
            initialDelaySeconds: 3
            periodSeconds: 5
          resources:
            requests:
              cpu: 10m
              memory: 32Mi
            limits:
              cpu: 100m
              memory: 64Mi
      imagePullSecrets:
        - name: ghcr-auth
```

### New file: `deploy/k8s/base/web-service.yaml`
```yaml
apiVersion: v1
kind: Service
metadata:
  name: filesync-web
spec:
  selector:
    app: filesync
    component: web
  ports:
    - port: 80
      targetPort: http
      protocol: TCP
      name: http
  type: ClusterIP
```

### Modified file: `deploy/k8s/base/ingress.yaml`
Replace the current single-backend ingress with split routing + Tailscale annotation:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: filesync-ingress
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
    tailscale.com/expose: "true"
    tailscale.com/hostname: "filesync"
spec:
  ingressClassName: tailscale  # Changed from traefik — Tailscale operator takes over
  tls:
    - hosts:
        - filesync.homelab.local
      secretName: filesync-tls
  rules:
    - host: filesync.homelab.local
      http:
        paths:
          - path: /api
            pathType: Prefix
            backend:
              service:
                name: filesync-api
                port:
                  name: http
          - path: /metrics
            pathType: Prefix
            backend:
              service:
                name: filesync-api
                port:
                  name: http
          - path: /healthz
            pathType: Prefix
            backend:
              service:
                name: filesync-api
                port:
                  name: http
          - path: /readyz
            pathType: Prefix
            backend:
              service:
                name: filesync-api
                port:
                  name: http
          - path: /
            pathType: Prefix
            backend:
              service:
                name: filesync-web
                port:
                  name: http
```

**Note:** Since the web frontend's nginx already proxies `/api/*` to the backend, an alternative (simpler) approach is to point ALL ingress traffic to `filesync-web` and let nginx handle the routing. This avoids duplicating path rules in the ingress. The tradeoff is the extra hop through nginx for API calls. For a homelab this is negligible. Either approach works — I'll implement whichever you prefer.

---

## Step 10: Tailscale operator manifests

### Modified file: `deploy/k8s/base/helmrepository.yaml`
Add Tailscale repo:
```yaml
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: tailscale
  namespace: flux-system
spec:
  interval: 1h
  url: https://pkgs.tailscale.com/helmcharts
```

### New file: `deploy/k8s/base/helmrelease-tailscale.yaml`
```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: tailscale-operator
  namespace: flux-system
spec:
  interval: 30m
  targetNamespace: tailscale-system
  install:
    createNamespace: true
  chart:
    spec:
      chart: tailscale-operator
      sourceRef:
        kind: HelmRepository
        name: tailscale
        namespace: flux-system
  values:
    oauth:
      clientId: "${TAILSCALE_CLIENT_ID}"
      clientSecret: "${TAILSCALE_CLIENT_SECRET}"
```

**Manual prerequisite:** You need to:
1. Go to Tailscale admin console > Settings > OAuth clients
2. Create an OAuth client with `auth_keys` and `devices:write` scopes
3. Create a SealedSecret (or plain Secret) with the client ID and secret
4. Reference it in the HelmRelease values (or use Flux variable substitution)

### New file: `deploy/k8s/base/sealed-secret-tailscale.yaml`
```yaml
# Placeholder — you need to seal this with kubeseal after creating the OAuth client
apiVersion: v1
kind: Secret
metadata:
  name: tailscale-operator-oauth
  namespace: tailscale-system
type: Opaque
stringData:
  client_id: "REPLACE_ME"
  client_secret: "REPLACE_ME"
```

---

## Step 11: CI pipeline update

### Modified file: `.github/workflows/ci.yaml`
Add a `build-web` job alongside the existing `build` job:
```yaml
  build-web:
    needs: test
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    permissions:
      packages: write
      contents: read
    steps:
      - uses: actions/checkout@v4

      - uses: docker/setup-buildx-action@v3

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/build-push-action@v6
        with:
          context: ./web
          push: true
          tags: |
            ghcr.io/${{ github.repository }}/web:${{ github.sha }}
            ghcr.io/${{ github.repository }}/web:latest
          cache-from: type=gha,scope=web
          cache-to: type=gha,mode=max,scope=web
```

---

## Step 12: Kustomization wiring

### Modified file: `deploy/k8s/base/kustomization.yaml`
Add:
```yaml
  - web-deployment.yaml
  - web-service.yaml
  - helmrelease-tailscale.yaml
  - sealed-secret-tailscale.yaml
```

---

## Summary of ALL files changed/created

### New files (10):
1. `migrations/003_add_directories.up.sql`
2. `migrations/003_add_directories.down.sql`
3. `internal/model/directory.go`
4. `internal/repository/directory.go`
5. `internal/handler/directories.go`
6. `web/` (entire Svelte app — ~10 files)
7. `web/Dockerfile`
8. `web/nginx.conf`
9. `deploy/k8s/base/web-deployment.yaml`
10. `deploy/k8s/base/web-service.yaml`
11. `deploy/k8s/base/helmrelease-tailscale.yaml`
12. `deploy/k8s/base/sealed-secret-tailscale.yaml`

### Modified files (8):
1. `internal/model/file.go` — add DirectoryID field
2. `internal/repository/postgres.go` — update cols, scan, add ListByDirectory
3. `internal/handler/files.go` — directory_id in upload + list filtering
4. `cmd/api/main.go` — register directory routes
5. `deploy/k8s/base/configmap.yaml` — add migration 003
6. `deploy/k8s/base/ingress.yaml` — split routing + Tailscale annotation
7. `deploy/k8s/base/helmrepository.yaml` — add Tailscale repo
8. `deploy/k8s/base/kustomization.yaml` — add new resources
9. `.github/workflows/ci.yaml` — add web build job
