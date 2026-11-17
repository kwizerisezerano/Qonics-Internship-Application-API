#!/bin/bash
# Run migrations in order

set -e

echo "Running database migrations..."

# List of migration files in order
migrations=(
    "000_init_pgcrypto_up.sql"
    "001_create_users_table_up.sql"
    "002_create_applicant_profiles_table_up.sql"
    "003_create_applications_table_up.sql"
    "004_create_application_documents_table_up.sql"
    "005_create_application_history_table_up.sql"
    "006_create_review_notes_table_up.sql"
    "007_create_notifications_table_up.sql"
    "008_create_settings_table_up.sql"
    "009_create_activity_logs_table_up.sql"
    "010_create_communication_logs_table_up.sql"
)

for migration in "${migrations[@]}"; do
    echo "Running $migration..."
    psql -U postgres -d qonics_db -f "/migrations/$migration"
done

echo "Seeding default admin user..."
psql -U postgres -d qonics_db -f "/migrations/seed_admin.sql"

echo "Migrations completed successfully!"
