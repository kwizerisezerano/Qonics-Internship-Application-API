-- Seed default admin user for development
-- This script should be run after migrations to create a default admin account
-- Default credentials: kwizerisezerano@gmail.com / admin123
INSERT INTO users (
    full_name_encrypted, full_name_hash,
    email_encrypted, email_hash,
    phone_encrypted, phone_hash,
    password_hash, role
) VALUES (
    pgp_sym_encrypt('Admin User', 'secret'),
    encode(digest('Admin User', 'sha256'), 'hex'),
    pgp_sym_encrypt('kwizerisezerano@gmail.com', 'secret'),
    encode(digest('kwizerisezerano@gmail.com', 'sha256'), 'hex'),
    pgp_sym_encrypt('+250788123456', 'secret'),
    encode(digest('+250788123456', 'sha256'), 'hex'),
    '$2a$10$lcd8ySJRuqV9VggXA5fH/O4Wsp5lXF5kax2icraih2eaYTGmrK8Yq', -- bcrypt hash of 'admin123'
    'admin'
) ON CONFLICT (email_hash) DO NOTHING;
