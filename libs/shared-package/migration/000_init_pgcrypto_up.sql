-- Enable pgcrypto extension
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Helper function to encrypt data
CREATE OR REPLACE FUNCTION encrypt_data(data text)
RETURNS bytea AS $$
BEGIN
    RETURN pgp_sym_encrypt(data, 'secret');
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Helper function to decrypt data
CREATE OR REPLACE FUNCTION decrypt_data(data bytea)
RETURNS text AS $$
BEGIN
    RETURN pgp_sym_decrypt(data, 'secret');
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Helper function to encrypt and hash
CREATE OR REPLACE FUNCTION encrypt_and_hash(data text, OUT encrypted bytea, OUT hash_val varchar)
AS $$
BEGIN
    encrypted := encrypt_data(data);
    hash_val := digest(data, 'sha256');
END;
$$ LANGUAGE plpgsql;

-- Helper function to encrypt data
CREATE OR REPLACE FUNCTION encrypt_data(data text)
RETURNS bytea AS $$
BEGIN
    RETURN pgp_sym_encrypt(data, current_setting('app.encryption_key', true));
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Helper function to decrypt data
CREATE OR REPLACE FUNCTION decrypt_data(data bytea)
RETURNS text AS $$
BEGIN
    RETURN pgp_sym_decrypt(data, current_setting('app.encryption_key', true));
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;

-- Helper function to encrypt and hash
CREATE OR REPLACE FUNCTION encrypt_and_hash(data text, OUT encrypted bytea, OUT hash_val varchar)
AS $$
BEGIN
    encrypted := encrypt_data(data);
    hash_val := digest(data, 'sha256');
END;
$$ LANGUAGE plpgsql;
