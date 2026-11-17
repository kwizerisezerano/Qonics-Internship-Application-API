-- Drop trigger
DROP TRIGGER IF EXISTS update_applicant_profiles_updated_at ON applicant_profiles;

-- Drop indexes
DROP INDEX IF EXISTS idx_applicant_profiles_user_id;
DROP INDEX IF EXISTS idx_applicant_profiles_email_hash;
DROP INDEX IF EXISTS idx_applicant_profiles_phone_hash;

-- Drop table
DROP TABLE IF EXISTS applicant_profiles;
