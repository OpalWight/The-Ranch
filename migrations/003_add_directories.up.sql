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
