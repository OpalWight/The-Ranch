DROP INDEX IF EXISTS idx_files_directory_id;
ALTER TABLE files DROP COLUMN IF EXISTS directory_id;

DROP INDEX IF EXISTS idx_directories_parent_id;
DROP TABLE IF EXISTS directories;
