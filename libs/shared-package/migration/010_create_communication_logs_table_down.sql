-- Drop communication_logs table
DROP TRIGGER IF EXISTS update_communication_logs_updated_at ON communication_logs;
DROP TABLE IF EXISTS communication_logs;
