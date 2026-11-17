# Qonics Appy API

Backend API for the Qonics internship application system, following the Leazi microservices pattern.

## Architecture

This is a Go-based microservices platform with the following components:

- **KrakenD Gateway** (port 8080) - API gateway routing
- **Auth Service** (port 9001) - JWT authentication, user registration, password management
- **Applicant Service** (port 9002) - Profile management, notifications, settings
- **Application Service** (port 9003) - Application CRUD, status management, document handling
- **PostgreSQL** (port 5432) - Primary database with pgcrypto for PII encryption
- **Redis** (port 6379) - Caching and session management

## Services

### Auth Service
- POST `/api/v1/auth/register` - User registration
- POST `/api/v1/auth/login` - User login
- POST `/api/v1/auth/logout` - User logout
- POST `/api/v1/auth/forgot-password` - Password reset request
- POST `/api/v1/auth/reset-password` - Password reset
- POST `/api/v1/auth/change-password` - Change password (authenticated)

### Applicant Service
- GET `/api/v1/applicant/profile` - Get current user's profile
- PUT `/api/v1/applicant/profile` - Update profile
- GET `/api/v1/applicant/notifications` - Get notifications
- POST `/api/v1/applicant/notifications/read` - Mark notifications as read
- GET `/api/v1/applicants` - List all applicants (admin)
- GET `/api/v1/settings` - Get application settings
- PUT `/api/v1/settings` - Update settings (admin)

### Application Service
- GET `/api/v1/applications/mine` - Get current user's applications
- GET `/api/v1/applications` - Get all applications (admin)
- POST `/api/v1/applications` - Create new application
- GET `/api/v1/applications/{id}` - Get application by ID
- PATCH `/api/v1/applications/{id}/status` - Update application status
- POST `/api/v1/applications/{id}/notes` - Add review note
- GET `/api/v1/sessions/bookings` - Get session availability

## Setup

### Prerequisites
- Docker and Docker Compose
- Go 1.24+ (for local development)
- Make

### Quick Start

1. Clone the repository
```bash
cd /home/kwizera/Qonics/qonics-appy-api
```

2. Configure environment variables
```bash
cp .env.example .env
# Edit .env with your configuration
```

3. Start all services
```bash
make up_build
```

This will:
- Build all service binaries
- Start PostgreSQL, Redis, and all microservices
- Run database migrations
- Start the KrakenD gateway

4. Access the API
- Gateway: http://localhost:8080
- Auth Service: http://localhost:9001
- Applicant Service: http://localhost:9002
- Application Service: http://localhost:9003
- PostgreSQL: localhost:5432
- Redis: localhost:6379

## Frontend Integration

To connect the frontend (qonics-appy-web) to this API:

1. In the frontend's `.env` file, set:
```bash
NUXT_PUBLIC_API_BASE=http://localhost:8080
```

2. The frontend will now make all API calls through the KrakenD gateway at port 8080

3. All API endpoints match the frontend service calls:
- `/api/v1/auth/*` - Authentication
- `/api/v1/applicant/*` - Applicant profiles
- `/api/v1/applications/*` - Application management
- `/api/v1/settings` - Application settings
- `/api/v1/sessions/bookings` - Session availability

### Development

Build a single service:
```bash
make build s=auth
make build s=applicant
make build s=application
```

Start without rebuilding:
```bash
make up
```

Stop all services:
```bash
make down
```

## Database Migrations

Migrations are located in `libs/shared-package/migration/` and follow the pattern:
- `XXX_table_name_up.sql` - Create table
- `XXX_table_name_down.sql` - Drop table

Migrations run automatically on PostgreSQL startup via docker-compose initdb.

## PII Encryption

Following the Leazi pattern, all PII (names, emails, phones) is encrypted using PostgreSQL's pgcrypto extension:
- Data is stored encrypted using `pgp_sym_encrypt`
- Searchable hash columns use `digest(..., 'sha256')`
- Never query encrypted columns directly - use hash columns
- Encryption key is set via `ENCRYPTION_KEY` environment variable (CHANGE IN PRODUCTION)

The encryption implementation includes:
- Custom PostgreSQL image with pgcrypto extension
- Migration script to initialize pgcrypto and helper functions
- Shared utility functions for encryption/decryption in Go
- All services use the encryption utilities for PII operations

## Configuration

Each service has a `config.yml.example` file. Copy to `config.yml` and configure:

```yaml
port: "9000"
database:
  host: localhost
  port: "5432"
  user: postgres
  password: postgres
  dbname: qonics_db
  sslmode: disable
redis:
  host: localhost
  port: "6379"
  password: ""
  db: 0
jwt:
  secret: your-super-secret-jwt-key-change-in-production
```

## Project Structure

```
qonics-appy-api/
├── services/
│   ├── auth/          # Authentication service
│   ├── applicant/     # Applicant profile service
│   └── application/   # Application management service
├── libs/
│   └── shared-package/  # Shared utilities, logger, middleware
├── docker/            # Docker configurations
├── k8s/              # Kubernetes manifests
├── krakend.json      # API gateway configuration
├── docker-compose.yml
├── Makefile
└── go.mod
```

## Testing

Run tests for shared package:
```bash
make shared-package
```

## Notes

- All services use JWT for authentication
- Admin endpoints require `role: admin` in JWT claims
- The gateway routes all requests through KrakenD
- Database migrations are versioned and reversible
# Qonics-Internship-Application-API
