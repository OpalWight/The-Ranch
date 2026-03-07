package repository

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"

	"github.com/albertvo/the-ranch/internal/model"
)

// FileRepository handles database operations for file metadata.
type FileRepository struct {
	db *sql.DB
}

// NewFileRepository creates a new instance of FileRepository.
func NewFileRepository(db *sql.DB) *FileRepository {
	return &FileRepository{db: db}
}

// ConnectPostgres establishes a connection to the PostgreSQL database.
func ConnectPostgres(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return db, nil
}

const fileCols = `id, name, size_bytes, mime_type, checksum, storage_key, directory_id, status, thumbnail_key, processed_at, created_at, updated_at`

// scanFile is a helper to scan a database row into a File model.
func scanFile(scanner interface{ Scan(...any) error }) (*model.File, error) {
	var f model.File
	err := scanner.Scan(
		&f.ID, &f.Name, &f.SizeBytes, &f.MimeType, &f.Checksum,
		&f.StorageKey, &f.DirectoryID, &f.Status, &f.ThumbnailKey, &f.ProcessedAt,
		&f.CreatedAt, &f.UpdatedAt,
	)
	return &f, err
}

// Create inserts a new file record into the database.
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

// List retrieves all file records from the database, newest first.
func (r *FileRepository) List(ctx context.Context) ([]model.File, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+fileCols+` FROM files ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing files: %w", err)
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

// GetByID retrieves a single file record by its unique identifier.
func (r *FileRepository) GetByID(ctx context.Context, id string) (*model.File, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+fileCols+` FROM files WHERE id = $1`, id,
	)
	f, err := scanFile(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting file: %w", err)
	}
	return f, nil
}

// Delete removes a file record from the database by its ID.
func (r *FileRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM files WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting file: %w", err)
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

func (r *FileRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE files SET status = $1, updated_at = NOW() WHERE id = $2`, status, id); err != nil {
		return fmt.Errorf("updating status: %w", err)
	}
	return nil
}

func (r *FileRepository) SetThumbnailKey(ctx context.Context, id string, thumbnailKey string) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE files SET thumbnail_key = $1, updated_at = NOW() WHERE id = $2`, thumbnailKey, id); err != nil {
		return fmt.Errorf("setting thumbnail key: %w", err)
	}
	return nil
}

func (r *FileRepository) MarkProcessed(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE files SET status = 'processed', processed_at = NOW(), updated_at = NOW() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("marking processed: %w", err)
	}
	return nil
}

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

// StorageStats returns the total number of files and total bytes used.
func (r *FileRepository) StorageStats(ctx context.Context) (fileCount int64, totalBytes int64, err error) {
	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(size_bytes), 0) FROM files`,
	).Scan(&fileCount, &totalBytes)
	if err != nil {
		return 0, 0, fmt.Errorf("querying storage stats: %w", err)
	}
	return fileCount, totalBytes, nil
}

// ListStorageKeys retrieves all unique storage keys currently in use.
func (r *FileRepository) ListStorageKeys(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT storage_key FROM files WHERE storage_key IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("listing storage keys: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scanning storage key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}
