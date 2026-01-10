.PHONY: migrate migrate-down migrate-up sqlc swag generate docker-build docker-push

# Variables
GITHUB_USER ?= $(shell git config user.name | tr '[:upper:]' '[:lower:]' | tr ' ' '-')
IMAGE_NAME ?= ghcr.io/$(GITHUB_USER)/lovebin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")

# Run migrations
migrate:
	cd migrations/ && goose postgres "postgres://postgres:postgres@localhost:5432/lovebin" status

migrate-up:
	cd migrations/ && goose postgres "postgres://postgres:postgres@localhost:5432/lovebin" up

migrate-down:
	cd migrations/ && goose postgres "postgres://postgres:postgres@localhost:5432/lovebin" down

# Generate sqlc code
sqlc:
	cd internal/services/media-service/repository && sqlc generate
	cd internal/services/access-service/repository && sqlc generate

# Generate swagger documentation
swag:
	swag init -g cmd/lovebin/main.go -o docs --parseDependency --parseInternal

# Generate all code (sqlc + swagger)
generate: sqlc swag

# Docker build
docker-build:
	docker build -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest -f deploy/Dockerfile .

# Docker push to GitHub Container Registry
docker-push: docker-build
	@echo "Pushing $(IMAGE_NAME):$(VERSION) to GitHub Container Registry..."
	docker push $(IMAGE_NAME):$(VERSION)
	docker push $(IMAGE_NAME):latest
	@echo "Successfully pushed $(IMAGE_NAME):$(VERSION) and $(IMAGE_NAME):latest"
