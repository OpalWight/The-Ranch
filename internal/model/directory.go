package model

import "time"

// Directory represents a folder that can contain files and subdirectories.
type Directory struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	ParentID  *string   `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	Directory   *Directory  `json:"directory,omitempty"`
	Directories []Directory `json:"directories"`
	Files       []File      `json:"files"`
}
