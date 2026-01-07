.PHONY: migrate sqlc clean test docker-build docker-up docker-down docker-logs docker-restart docker-run

# Run the application locally
run:
	go run cmd/lovebin/main.go

# Build the application
build:
	go build -o bin/lovebin cmd/lovebin/main.go

# Run migrations
migrate:
	goose postgres "postgres://postgres:postgres@localhost:5432/lovebin" status

# Generate sqlc code
sqlc:
	cd internal/services/media-service/repository && sqlc generate
	cd internal/services/access-service/repository && sqlc generate

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test ./...

# Install dependencies
deps:
	go mod download
	go mod tidy

# Docker commands
docker-build:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f app

docker-restart:
	docker-compose restart app

# Build and run with docker-compose
docker-run: docker-build docker-up
	@echo "Application is running at http://localhost:8080"
	@echo "MinIO console at http://localhost:9001 (minioadmin/minioadmin)"
	@echo "PostgreSQL at localhost:5432 (postgres/postgres)"

# Stop and remove all containers, volumes
docker-clean:
	docker-compose down -v
