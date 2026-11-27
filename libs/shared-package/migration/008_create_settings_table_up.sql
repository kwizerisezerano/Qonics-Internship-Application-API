-- Create settings table (single row for app-wide settings)
CREATE TABLE IF NOT EXISTS settings (
    id SERIAL PRIMARY KEY,
    applications_open BOOLEAN DEFAULT TRUE,
    close_on DATE,
    closure_message TEXT,
    morning_seats INTEGER DEFAULT 3 CHECK (morning_seats >= 0),
    afternoon_seats INTEGER DEFAULT 3 CHECK (afternoon_seats >= 0),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Insert default settings
INSERT INTO settings (applications_open, close_on, closure_message, morning_seats, afternoon_seats)
VALUES (TRUE, NULL, '', 3, 3)
ON CONFLICT DO NOTHING;

-- Trigger to automatically update updated_at
CREATE TRIGGER update_settings_updated_at BEFORE UPDATE ON settings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
