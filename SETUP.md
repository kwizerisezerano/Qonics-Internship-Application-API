# Qonics Internship Application API - Setup Guide

## Overview

This is the backend API for the Qonics Internship Application system, built following the Leazi architecture pattern.

## Architecture

- **Backend**: Go with Fiber v2, PostgreSQL with pgcrypto, Redis, KrakenD API Gateway
- **Services**:
  - Auth Service (port 9001)
  - Applicant Service (port 9002)
  - Application Service (port 9003)
  - Notification Worker
- **Database**: PostgreSQL with pgcrypto for PII encryption
- **Message Queue**: Redis for notification processing

## Local Development

### Prerequisites

- Docker and Docker Compose
- Go 1.26+
- Make

### Quick Start

```bash
# Build and start all services
make up_build

# Or start without rebuilding
make up

# Stop all services
make down
```

### Database Setup

The project uses two databases:

1. **Production Database** (port 5432): `qonics_db` - Used by Docker Compose services
2. **Test Database** (port 5434): `qonics_test_db` - Used for running tests

#### Running Migrations

Migrations run automatically when PostgreSQL starts. The seed admin account is created automatically.

**Default Admin Credentials:**
- Email: `admin@qonics.com`
- Password: `admin123`

### Running Tests

```bash
# Run all tests against test database
make test

# This will:
# 1. Create/recreate postgres-test container on port 5434
# 2. Run all migrations on test database
# 3. Run service tests (auth, applicant, application, notification)
```

### Service-Specific Commands

```bash
# Build and test specific service
make auth
make applicant
make application
make notification

# Build only (no tests)
make build s=auth
```

### Environment Configuration

### Email Setup (SMTP)

The notification worker uses SMTP to send emails. For Gmail, you must use an **App Password**:

1. Go to: https://myaccount.google.com/apppasswords
2. Sign in with your Gmail account
3. Select "Mail" and "Other (custom name)"
4. Copy the generated 16-character password
5. Add to `.env` file:

```bash
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-16-char-app-password
SMTP_FROM=noreply@qonics.com
```

**⚠️ IMPORTANT:**
- Never use your regular Gmail password
- The `.env` file is already in `.gitignore` and won't be committed
- For production, use a secure secret management system

### Docker Compose Email Setup

For Docker Compose, create `.env.docker` with SMTP credentials:

```bash
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USER=your-email@gmail.com
SMTP_PASSWORD=your-16-char-app-password
SMTP_FROM=noreply@qonics.com
```

The notification worker's Dockerfile copies `.env.docker` to the container, and the main.go loads it using `godotenv`.

### Service Configuration

Each service has a `config.yml.example` file. Copy it to `config.yml` and adjust as needed:

```bash
cp services/auth/config.yml.example services/auth/config.yml
cp services/applicant/config.yml.example services/applicant/config.yml
cp services/application/config.yml.example services/application/config.yml
cp services/notification/config.yml.example services/notification/config.yml
```

The notification worker's `config.yml` supports environment variable substitution for email credentials.

## API Endpoints

### Auth Service (http://localhost:9001)

- `POST /api/v1/auth/register` - User registration
- `POST /api/v1/auth/login` - User login
- `POST /api/v1/auth/logout` - User logout
- `POST /api/v1/auth/forgot-password` - Forgot password
- `POST /api/v1/auth/reset-password` - Reset password
- `POST /api/v1/auth/change-password` - Change password (authenticated)

### Applicant Service (http://localhost:9002)

- `GET /api/v1/applicant/profile` - Get applicant profile
- `PUT /api/v1/applicant/profile` - Update applicant profile
- `GET /api/v1/applicant/notifications` - Get notifications
- `PUT /api/v1/applicant/notifications/read` - Mark notifications as read
- `GET /api/v1/applicant/settings` - Get settings
- `PUT /api/v1/applicant/settings` - Update settings

### Application Service (http://localhost:9003)

- `GET /api/v1/applications/mine` - Get current user's applications
- `GET /api/v1/applications` - Get all applications (admin)
- `GET /api/v1/applications/:id` - Get single application
- `POST /api/v1/applications` - Create new application
- `PUT /api/v1/applications/:id/status` - Update application status
- `POST /api/v1/applications/:id/notes` - Add review note
- `GET /api/v1/applications/:id/availability` - Get availability

### KrakenD Gateway (http://localhost:8080)

All services are accessible through the KrakenD gateway at port 8080.

## PII Encryption

All sensitive data (names, emails, phones) is encrypted at rest using PostgreSQL pgcrypto:

- Encryption key: `secret` (development only - change in production)
- Hash columns for searching
- Decryption happens server-side

## Error Handling

- **Known errors** (4xx): No logging, user-friendly messages
- **Unknown errors** (500): Critical logging with context

## Activity Logging

Successful operations (registration, login, logout) are logged to the `activity_logs` table.

## Notifications

- Email notifications: Queued in Redis, processed by notification worker
- In-app notifications: Stored in `notifications` table
- Communication logs: Tracked in `communication_logs` table

## Testing

Tests are organized per service in `controller/*_test.go` files:

- `services/auth/controller/auth_test.go`
- `services/applicant/controller/applicant_test.go`
- `services/application/controller/application_test.go`
- `services/notification/main_test.go`

Tests use the test database on port 5434 and require the test container to be running.
