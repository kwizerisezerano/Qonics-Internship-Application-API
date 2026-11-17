-- Drop helper functions
DROP FUNCTION IF EXISTS encrypt_and_hash(text);
DROP FUNCTION IF EXISTS decrypt_data(bytea);
DROP FUNCTION IF EXISTS encrypt_data(text);

-- Drop pgcrypto extension
DROP EXTENSION IF EXISTS pgcrypto;
