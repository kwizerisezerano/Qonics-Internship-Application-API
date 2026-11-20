-- Create users table with PII encryption
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    full_name_encrypted BYTEA NOT NULL,
    -- Not unique: two people can share a full name.
    full_name_hash VARCHAR(128) NOT NULL,
    email_encrypted BYTEA NOT NULL,
    email_hash VARCHAR(128) NOT NULL UNIQUE,
    phone_encrypted BYTEA,
    phone_hash VARCHAR(128),
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL DEFAULT 'applicant' CHECK (role IN ('applicant', 'admin')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create index on email_hash for faster lookups
CREATE INDEX idx_users_email_hash ON users(email_hash);
CREATE INDEX idx_users_phone_hash ON users(phone_hash);
CREATE INDEX idx_users_full_name_hash ON users(full_name_hash);

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Trigger to automatically update updated_at
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
