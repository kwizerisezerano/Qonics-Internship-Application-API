-- Create applicant_profiles table with PII encryption
CREATE TABLE IF NOT EXISTS applicant_profiles (
    id VARCHAR(50) PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    full_name_encrypted BYTEA NOT NULL,
    full_name_hash VARCHAR(128) NOT NULL,
    email_encrypted BYTEA NOT NULL,
    email_hash VARCHAR(128) NOT NULL,
    phone_encrypted BYTEA NOT NULL,
    phone_hash VARCHAR(128) NOT NULL,
    country VARCHAR(100),
    city VARCHAR(100),
    institution VARCHAR(255),
    field_of_study VARCHAR(255),
    qualification VARCHAR(255),
    academic_status VARCHAR(100),
    graduation_year INTEGER,
    skills TEXT[],
    bio TEXT,
    github_url VARCHAR(500),
    linkedin_url VARCHAR(500),
    portfolio_url VARCHAR(500),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX idx_applicant_profiles_user_id ON applicant_profiles(user_id);
CREATE INDEX idx_applicant_profiles_email_hash ON applicant_profiles(email_hash);
CREATE INDEX idx_applicant_profiles_phone_hash ON applicant_profiles(phone_hash);

-- Trigger to automatically update updated_at
CREATE TRIGGER update_applicant_profiles_updated_at BEFORE UPDATE ON applicant_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
