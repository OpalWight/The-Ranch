CREATE TABLE IF NOT EXISTS files (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL,
    mime_type   TEXT NOT NULL DEFAULT 'application/octet-stream',
    checksum    TEXT NOT NULL,
    storage_key TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_files_name ON files(name);
CREATE INDEX IF NOT EXISTS idx_files_created_at ON files(created_at);
