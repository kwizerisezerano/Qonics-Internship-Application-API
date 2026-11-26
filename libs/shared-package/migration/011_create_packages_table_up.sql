-- Training packages with pricing, skills, and billing period
-- managed by admins. Each package includes a set of skills at a fixed price.
-- Period indicates billing frequency (monthly/yearly), duration is tracked in applications.
CREATE TABLE IF NOT EXISTS packages (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description TEXT,
    price NUMERIC(12,2) NOT NULL CHECK (price >= 0),
    skills TEXT[] NOT NULL,
    period VARCHAR(20) NOT NULL DEFAULT 'monthly' CHECK (period IN ('monthly', 'yearly', 'quarterly')),
    currency VARCHAR(3) NOT NULL DEFAULT 'RWF',
    is_active BOOLEAN DEFAULT true,
    updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER update_packages_updated_at BEFORE UPDATE ON packages
FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add package_id to applications table
ALTER TABLE applications ADD COLUMN IF NOT EXISTS package_id INTEGER REFERENCES packages(id) ON DELETE SET NULL;

-- Add skills array to applications table for record-keeping
ALTER TABLE applications ADD COLUMN IF NOT EXISTS skills TEXT[];

-- Seed packages with the three pricing tiers (prices are monthly by default)
INSERT INTO packages (name, description, price, skills, period, currency, is_active) VALUES
('Basic Training Package', 'Coding and Design (Figma)', 150000, ARRAY['coding', 'design'], 'monthly', 'RWF', true),
('Standard Training Package', 'Coding, Design (Figma), 3D Printing, and CNC', 250000, ARRAY['coding', 'design', '3d-printing', 'cnc'], 'monthly', 'RWF', true),
('Premium Training Package', 'Coding, Design (Figma), 3D Printing, CNC, and IoT (Embedded System)', 300000, ARRAY['coding', 'design', '3d-printing', 'cnc', 'iot'], 'monthly', 'RWF', true);
