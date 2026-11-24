-- Internship price per month, managed by admins (single row)
CREATE TABLE IF NOT EXISTS prices (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    price_per_month NUMERIC(12,2) NOT NULL DEFAULT 200000 CHECK (price_per_month >= 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'RWF',
    updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO prices (id, price_per_month, currency)
VALUES (1, 200000, 'RWF')
ON CONFLICT (id) DO NOTHING;

CREATE TRIGGER update_prices_updated_at BEFORE UPDATE ON prices
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
