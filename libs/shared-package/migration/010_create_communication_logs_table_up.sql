-- Create communication_logs table for tracking email and in-app notifications
CREATE TABLE IF NOT EXISTS communication_logs (
    id SERIAL PRIMARY KEY,
    channel VARCHAR(20) NOT NULL CHECK (channel IN ('EMAIL', 'IN_APP', 'SMS')),
    recipient_id VARCHAR(255) NOT NULL,
    recipient_type VARCHAR(20) NOT NULL CHECK (recipient_type IN ('USER', 'EMAIL')),
    subject VARCHAR(500) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX idx_communication_logs_channel ON communication_logs(channel);
CREATE INDEX idx_communication_logs_status ON communication_logs(status);
CREATE INDEX idx_communication_logs_recipient ON communication_logs(recipient_id);
CREATE INDEX idx_communication_logs_created_at ON communication_logs(created_at);

-- Trigger to automatically update updated_at
CREATE TRIGGER update_communication_logs_updated_at BEFORE UPDATE ON communication_logs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
