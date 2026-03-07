package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/albertvo/the-ranch/internal/model"
)

// DirectoryRepository handles database operations for directory metadata.
type DirectoryRepository struct {
	db *sql.DB
}

// NewDirectoryRepository creates a new instance of DirectoryRepository.
func NewDirectoryRepository(db *sql.DB) *DirectoryRepository {
	return &DirectoryRepository{db: db}
}

const dirCols = `id, name, parent_id, created_at, updated_at`

func scanDirectory(scanner interface{ Scan(...any) error }) (*model.Directory, error) {
	var d model.Directory
	err := scanner.Scan(&d.ID, &d.Name, &d.ParentID, &d.CreatedAt, &d.UpdatedAt)
	return &d, err
}

// Create inserts a new directory record into the database.
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

// GetByID retrieves a single directory by its unique identifier.
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

func (r *DirectoryRepository) BulkDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	query := "DELETE FROM directories WHERE id IN ("
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query += ")"

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// Delete removes a directory record from the database by its ID.
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

// Update applies partial updates to a directory (name and/or parent).
func (r *DirectoryRepository) Update(ctx context.Context, id string, req model.UpdateDirectoryRequest) (*model.Directory, error) {
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
