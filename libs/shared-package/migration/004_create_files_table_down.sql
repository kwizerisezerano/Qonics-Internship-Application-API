-- Drop indexes
DROP INDEX IF EXISTS idx_files_source;
DROP INDEX IF EXISTS idx_files_owner_id;
DROP INDEX IF EXISTS idx_files_file_type;
DROP INDEX IF EXISTS idx_files_deleted_at;

-- Drop trigger + table
DROP TRIGGER IF EXISTS update_files_updated_at ON files;
DROP TABLE IF EXISTS files;
