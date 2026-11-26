APP_BINARY = app-release
POSTGRES_IMAGE = qonics-postgres-pgcrypto

.PHONY: up up_build build down stop auth applicant application notification file shared-package recreate_postgres_test test_db test

## up: starts all containers in the background without forcing build
up:
	@echo "Starting Docker images..."
	docker compose up -d
	@echo "Docker images started"

## up_build: stops docker compose (if running), builds all projects with tests, and starts docker compose
up_build: test_db shared-package auth applicant application notification file
	@echo "Stopping docker images (if running...)"
	docker compose down
	@echo "Building (when required) and starting docker images..."
	docker compose up --build -d
	@echo "Docker images built and started!"

## Build single service and pass the service by using the argument s. Ex: make build s=auth
build: $(s)
	@echo "Building service: $(s)"
	docker compose build $(s)
	@echo "$(s) built!"

## down: destroy docker compose
down:
	@echo "destroy docker compose..."
	docker compose down
	@echo "Done!"

## down: stop docker compose
stop:
	@echo "Stopping docker compose..."
	docker compose stop
	@echo "Done!"

## auth: builds the auth service binary and runs tests
auth:
	@echo "Building auth service binary..."
	cd ./services/auth && go mod tidy
	cd ./services/auth && go test -v ./...
	cd ./services/auth && env GOOS=linux CGO_ENABLED=0 go build -o ${APP_BINARY} -buildvcs=false
	@echo "Done!"

## applicant: builds the applicant service binary and runs tests
applicant:
	@echo "Building applicant service binary..."
	cd ./services/applicant && go mod tidy
	cd ./services/applicant && go test -v ./...
	cd ./services/applicant && env GOOS=linux CGO_ENABLED=0 go build -o ${APP_BINARY} -buildvcs=false
	@echo "Done!"

## application: builds the application service binary and runs tests
application:
	@echo "Building application service binary..."
	cd ./services/application && go mod tidy
	cd ./services/application && go test -v ./...
	cd ./services/application && env GOOS=linux CGO_ENABLED=0 go build -o ${APP_BINARY} -buildvcs=false
	@echo "Done!"

## notification: builds the notification worker binary and runs tests
notification:
	@echo "Building notification worker binary..."
	cd ./services/notification && go mod tidy
	cd ./services/notification && go test -v
	cd ./services/notification && env GOOS=linux CGO_ENABLED=0 go build -o ${APP_BINARY} -buildvcs=false
	@echo "Done!"

## file: builds the file service binary and runs tests
file:
	@echo "Building file service binary..."
	cd ./services/file && go mod tidy
	cd ./services/file && go test -v ./...
	cd ./services/file && env GOOS=linux CGO_ENABLED=0 go build -o ${APP_BINARY} -buildvcs=false
	@echo "Done!"

## shared-package: test and verify the shared package
shared-package:
	@echo "Testing shared-package..."
	cd ./libs/shared-package && go mod tidy
	cd ./libs/shared-package && go test -v ./...
	@echo "Done!"

## recreate_postgres_test: Delete and recreate Docker container for PostgreSQL test
recreate_postgres_test:
	@echo "Stopping and removing existing PostgreSQL test container..."
	docker stop qonics-postgres-test || true
	docker rm qonics-postgres-test || true
	@echo "Creating new PostgreSQL container for testing..."
	docker run --name qonics-postgres-test \
		-e POSTGRES_USER=postgres \
		-e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_DB=qonics_test_db \
		--restart unless-stopped \
		-p 5434:5432 \
		-d postgres:17-alpine
	@echo "Waiting for PostgreSQL test container to be ready..."
	@# The image restarts itself once after initialising, so wait for it to accept
	@# connections twice in a row instead of sleeping a fixed time.
	@for i in $$(seq 1 60); do \
		if docker exec qonics-postgres-test pg_isready -U postgres -d qonics_test_db >/dev/null 2>&1; then \
			sleep 2; \
			if docker exec qonics-postgres-test pg_isready -U postgres -d qonics_test_db >/dev/null 2>&1; then break; fi; \
		fi; \
		sleep 1; \
	done
	@echo "PostgreSQL test container recreated!"

## redis: Start Redis (needed by the auth, applicant and application tests)
redis:
	docker compose up -d --wait redis

## test_db: Run migrations on test database
test_db: recreate_postgres_test
	@echo "Running migrations on test database..."
	@for f in ./libs/shared-package/migration/*_up.sql; do \
		docker exec -i qonics-postgres-test psql -U postgres -d qonics_test_db < $$f; \
	done
	@echo "Applying admin seed to test database..."
	docker exec -i qonics-postgres-test psql -U postgres -d qonics_test_db < ./libs/shared-package/migration/seed_admin.sql
	@echo "Test database migrations completed!"

## test: Run all service tests against test database
test: test_db redis
	@echo "Running all service tests..."
	cd ./libs/shared-package && go test -v ./...
	cd ./services/auth && go test -v ./...
	cd ./services/applicant && go test -v ./...
	cd ./services/application && go test -v ./...
	cd ./services/notification && go test -v ./...
	cd ./services/file && go test -v ./...
	@echo "All tests completed!"
