ALTER TABLE files ADD COLUMN status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE files ADD COLUMN thumbnail_key TEXT;
ALTER TABLE files ADD COLUMN processed_at TIMESTAMPTZ;

CREATE INDEX idx_files_status ON files(status);
