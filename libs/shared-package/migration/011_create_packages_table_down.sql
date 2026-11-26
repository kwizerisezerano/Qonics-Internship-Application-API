DROP TRIGGER IF EXISTS update_packages_updated_at ON packages;
DROP TABLE IF EXISTS packages;

-- Remove package_id and skills from applications table
ALTER TABLE applications DROP COLUMN IF EXISTS package_id;
ALTER TABLE applications DROP COLUMN IF EXISTS skills;
