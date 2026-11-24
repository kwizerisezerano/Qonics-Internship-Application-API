-- Internship price per course (specialization) and period (duration in months),
-- managed by admins. A combination with no row uses the default of
-- 200000 RWF per month (see utils.LookupPrice).
CREATE TABLE IF NOT EXISTS prices (
    id SERIAL PRIMARY KEY,
    department VARCHAR(50) NOT NULL CHECK (department IN ('software', 'iot')),
    specialization VARCHAR(50) NOT NULL,
    duration_months INTEGER NOT NULL CHECK (duration_months >= 1),
    price NUMERIC(12,2) NOT NULL CHECK (price >= 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'RWF',
    updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (specialization, duration_months)
);

CREATE TRIGGER update_prices_updated_at BEFORE UPDATE ON prices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
