-- Create application_documents table
CREATE TABLE IF NOT EXISTS application_documents (
    id VARCHAR(50) PRIMARY KEY,
    application_id VARCHAR(50) REFERENCES applications(id) ON DELETE CASCADE,
    document_type VARCHAR(50) NOT NULL CHECK (document_type IN ('cv', 'transcript', 'diploma', 'certificates', 'cover_letter')),
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    file_path VARCHAR(500) NOT NULL,
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX idx_application_documents_application_id ON application_documents(application_id);
CREATE INDEX idx_application_documents_type ON application_documents(document_type);
