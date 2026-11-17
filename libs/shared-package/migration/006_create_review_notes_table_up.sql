-- Create review_notes table
CREATE TABLE IF NOT EXISTS review_notes (
    id VARCHAR(50) PRIMARY KEY,
    application_id VARCHAR(50) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    author VARCHAR(255) NOT NULL,
    text TEXT NOT NULL,
    at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create index
CREATE INDEX idx_review_notes_application_id ON review_notes(application_id);
CREATE INDEX idx_review_notes_at ON review_notes(at);
