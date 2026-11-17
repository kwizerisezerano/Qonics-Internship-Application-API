-- Create settings table (single row for app-wide settings)
CREATE TABLE IF NOT EXISTS settings (
    id SERIAL PRIMARY KEY,
    applications_open BOOLEAN DEFAULT TRUE,
    close_on DATE,
    closure_message TEXT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Insert default settings
INSERT INTO settings (applications_open, close_on, closure_message)
VALUES (TRUE, NULL, '')
ON CONFLICT DO NOTHING;

-- Trigger to automatically update updated_at
CREATE TRIGGER update_settings_updated_at BEFORE UPDATE ON settings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
