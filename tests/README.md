# API Endpoint Tests

This directory contains integration tests for the Qonics Appy API backend.

## Prerequisites

- All services must be running (docker-compose up)
- Gateway must be accessible at http://localhost:8080

## Running Tests

### Run all tests:
```bash
cd tests
go test -v
```

### Run specific test file:
```bash
cd tests
go test -v auth_test.go
go test -v applicant_test.go
go test -v application_test.go
```

## Test Coverage

### auth_test.go
- Register new user
- Login with credentials
- Forgot password
- Reset password
- Logout

### applicant_test.go
- Get applicant profile
- Save/update profile
- List notifications
- Mark notifications as read
- Get settings

### application_test.go
- List my applications
- List all applications (admin)
- Create new application
- Get availability/sessions

## Frontend API Contract

The backend endpoints match the frontend service calls:

### Auth Service (authService.ts)
- POST /auth/login
- POST /auth/register
- POST /auth/logout
- POST /auth/forgot-password
- POST /auth/reset-password
- POST /auth/change-password

### Applicant Service (applicantService.ts)
- GET /applicant/profile
- PUT /applicant/profile
- GET /applicants (admin)
- GET /notifications
- POST /notifications/read
- GET /settings
- PUT /settings

### Application Service (applicationService.ts)
- GET /applications/mine
- GET /applications (admin)
- POST /applications
- GET /applications/{id}
- PATCH /applications/{id}/status
- POST /applications/{id}/notes
- GET /sessions/bookings

## Notes

- Tests use the KrakenD gateway at http://localhost:8080
- Authentication tokens are obtained via login
- Some endpoints require admin role and may return 403 for regular users
- The database is not reset between tests, so you may need to clean up manually
