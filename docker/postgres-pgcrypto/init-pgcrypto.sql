-- Enable pgcrypto extension
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Set encryption key for current session
DO $$
BEGIN
    PERFORM set_config('app.encryption_key', 'secret', false);
EXCEPTION WHEN undefined_object THEN
    RAISE NOTICE 'Could not set app.encryption_key, will use default in functions';
END $$;

-- Grant necessary permissions
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON ALL TABLES IN SCHEMA public TO postgres;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO postgres;
GRANT ALL ON ALL FUNCTIONS IN SCHEMA public TO postgres;

