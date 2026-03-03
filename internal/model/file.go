package model

import "time"

type File struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	SizeBytes    int64      `json:"size_bytes"`
	MimeType     string     `json:"mime_type"`
	Checksum     string     `json:"checksum"`
	StorageKey   *string    `json:"storage_key,omitempty"`
	Status       string     `json:"status"`
	ThumbnailKey *string    `json:"thumbnail_key,omitempty"`
	ProcessedAt  *time.Time `json:"processed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type CreateFileRequest struct {
	Name       string  `json:"name"`
	SizeBytes  int64   `json:"size_bytes"`
	MimeType   string  `json:"mime_type"`
	Checksum   string  `json:"checksum"`
	StorageKey *string `json:"storage_key,omitempty"`
}
