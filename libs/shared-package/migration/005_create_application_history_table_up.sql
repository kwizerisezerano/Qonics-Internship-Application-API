-- Create application_history table for status changes
CREATE TABLE IF NOT EXISTS application_history (
    id SERIAL PRIMARY KEY,
    application_id VARCHAR(50) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    status VARCHAR(30) NOT NULL,
    at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    note TEXT
);

-- Create index
CREATE INDEX idx_application_history_application_id ON application_history(application_id);
CREATE INDEX idx_application_history_at ON application_history(at);
