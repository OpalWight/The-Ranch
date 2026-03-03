package repository

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"

	"github.com/albertvo/the-ranch/internal/model"
)

type FileRepository struct {
	db *sql.DB
}

func NewFileRepository(db *sql.DB) *FileRepository {
	return &FileRepository{db: db}
}

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

const fileCols = `id, name, size_bytes, mime_type, checksum, storage_key, status, thumbnail_key, processed_at, created_at, updated_at`

func scanFile(scanner interface{ Scan(...any) error }) (*model.File, error) {
	var f model.File
	err := scanner.Scan(
		&f.ID, &f.Name, &f.SizeBytes, &f.MimeType, &f.Checksum,
		&f.StorageKey, &f.Status, &f.ThumbnailKey, &f.ProcessedAt,
		&f.CreatedAt, &f.UpdatedAt,
	)
	return &f, err
}

func (r *FileRepository) Create(ctx context.Context, req model.CreateFileRequest) (*model.File, error) {
	row := r.db.QueryRowContext(ctx,
		`INSERT INTO files (name, size_bytes, mime_type, checksum, storage_key)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+fileCols,
		req.Name, req.SizeBytes, req.MimeType, req.Checksum, req.StorageKey,
	)
	f, err := scanFile(row)
	if err != nil {
		return nil, fmt.Errorf("inserting file: %w", err)
	}
	return f, nil
}

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
	_, err := r.db.ExecContext(ctx,
		`UPDATE files SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("updating status: %w", err)
	}
	return nil
}

func (r *FileRepository) SetThumbnailKey(ctx context.Context, id string, thumbnailKey string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE files SET thumbnail_key = $1, updated_at = NOW() WHERE id = $2`, thumbnailKey, id)
	if err != nil {
		return fmt.Errorf("setting thumbnail key: %w", err)
	}
	return nil
}

func (r *FileRepository) MarkProcessed(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE files SET status = 'processed', processed_at = NOW(), updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("marking processed: %w", err)
	}
	return nil
}

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
