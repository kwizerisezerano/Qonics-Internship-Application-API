-- Drop trigger
DROP TRIGGER IF EXISTS update_activity_logs_updated_at ON activity_logs;

-- Drop indexes
DROP INDEX IF EXISTS idx_activity_logs_user_id;
DROP INDEX IF EXISTS idx_activity_logs_activity_type;
DROP INDEX IF EXISTS idx_activity_logs_status;
DROP INDEX IF EXISTS idx_activity_logs_created_at;

-- Drop table
DROP TABLE IF EXISTS activity_logs;
