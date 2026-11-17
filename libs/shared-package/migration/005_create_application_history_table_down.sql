-- Drop indexes
DROP INDEX IF EXISTS idx_application_history_application_id;
DROP INDEX IF EXISTS idx_application_history_at;

-- Drop table
DROP TABLE IF EXISTS application_history;
