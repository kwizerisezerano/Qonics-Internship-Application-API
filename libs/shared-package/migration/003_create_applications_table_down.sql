-- Drop trigger
DROP TRIGGER IF EXISTS update_applications_updated_at ON applications;

-- Drop indexes
DROP INDEX IF EXISTS idx_applications_applicant_id;
DROP INDEX IF EXISTS idx_applications_status;
DROP INDEX IF EXISTS idx_applications_department;
DROP INDEX IF EXISTS idx_applications_session;

-- Drop table
DROP TABLE IF EXISTS applications;
