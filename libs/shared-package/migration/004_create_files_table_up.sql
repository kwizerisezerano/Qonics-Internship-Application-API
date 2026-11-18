-- Create generic files table (leazi-style polymorphic owner)
-- A file is created in a "pending" state (deleted_at in the future) by the
-- presigned-url endpoint and becomes permanent once confirm-upload clears
-- deleted_at. Ownership is modelled generically through source_table +
-- source_table_id (+ extra), so any entity can own files without the files
-- table knowing about it.
CREATE TABLE IF NOT EXISTS files (
    id VARCHAR(50) PRIMARY KEY,
    owner_id VARCHAR(50),
    owner_type VARCHAR(50),
    name VARCHAR(255) NOT NULL,
    file_path VARCHAR(500) NOT NULL,
    file_type VARCHAR(50) NOT NULL CHECK (file_type IN ('cv', 'transcript', 'diploma', 'certificates', 'cover_letter')),
    file_size BIGINT NOT NULL,
    source_table VARCHAR(100),
    source_table_id VARCHAR(50),
    description TEXT,
    extra JSONB DEFAULT '{}'::jsonb,
    uploaded_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

-- Create indexes
CREATE INDEX idx_files_source ON files(source_table, source_table_id);
CREATE INDEX idx_files_owner_id ON files(owner_id);
CREATE INDEX idx_files_file_type ON files(file_type);
CREATE INDEX idx_files_deleted_at ON files(deleted_at);

-- Trigger to automatically update updated_at
CREATE TRIGGER update_files_updated_at BEFORE UPDATE ON files
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
