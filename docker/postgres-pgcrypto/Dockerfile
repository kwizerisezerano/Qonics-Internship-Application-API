FROM postgres:17-alpine

# Install pgcrypto extension
RUN apk add --no-cache postgresql-dev

# Copy init script to create pgcrypto extension
COPY init-pgcrypto.sql /docker-entrypoint-initdb.d/01-init-pgcrypto.sql
