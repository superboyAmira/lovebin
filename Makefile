.PHONY: migrate migrate-down migrate-up sqlc swag generate docker-build docker-build-amd64 docker-buildx docker-push docker-push-amd64 deploy-copy

# Variables
DOCKER_USER ?= aamira
IMAGE_NAME ?= $(DOCKER_USER)/lovebin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "latest")
USERNAME ?= root
SSH_PASSWORD ?=

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

# Docker build (local platform)
docker-build:
	docker build -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest -f deploy/Dockerfile .

# Docker build for linux/amd64 (for production servers)
docker-build-amd64:
	docker buildx build --platform linux/amd64 -t $(IMAGE_NAME):$(VERSION) -t $(IMAGE_NAME):latest -f deploy/Dockerfile . --load

# Docker push to Docker Hub (builds multi-platform: linux/amd64 + linux/arm64)
# Creates one image with support for both architectures
docker-push:
	@echo "Building and pushing $(IMAGE_NAME):$(VERSION) to Docker Hub (multi-platform: linux/amd64, linux/arm64)..."
	@if ! docker buildx ls | grep -q "docker-container"; then \
		echo "Creating buildx builder..."; \
		docker buildx create --name multiarch --use || true; \
	fi
	docker login -u aamira
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE_NAME):$(VERSION) \
		-t $(IMAGE_NAME):latest \
		-f deploy/Dockerfile . \
		--push
	@echo "Successfully pushed $(IMAGE_NAME):$(VERSION) and $(IMAGE_NAME):latest to Docker Hub"
	@echo "Image supports both linux/amd64 and linux/arm64 architectures"

# Deploy copy - copy config and deploy folders to remote server
# Usage: make deploy-copy IP=192.168.1.100 USERNAME=root
# Usage: make deploy-copy IP=192.168.1.100 SSH_PASSWORD=your_password
# Usage: make deploy-copy IP=192.168.1.100 (uses default username=root, requires SSH keys)
deploy-copy:
	@if [ -z "$(IP)" ]; then \
		echo "Error: IP is required. Usage: make deploy-copy IP=192.168.1.100 [USERNAME=root] [SSH_PASSWORD=password]"; \
		exit 1; \
	fi
	@if [ -n "$(SSH_PASSWORD)" ]; then \
		if ! command -v sshpass >/dev/null 2>&1; then \
			echo "Error: sshpass is required when using SSH_PASSWORD. Install it with: brew install hudochenkov/sshpass/sshpass (macOS) or apt-get install sshpass (Linux)"; \
			exit 1; \
		fi; \
		sshpass -p '$(SSH_PASSWORD)' ssh -o StrictHostKeyChecking=no $(USERNAME)@$(IP) "mkdir -p /opt/lovebin"; \
		rsync -avz --progress \
			--exclude='*.swp' \
			--exclude='*.swo' \
			--exclude='*~' \
			--exclude='.git' \
			-e "sshpass -p '$(SSH_PASSWORD)' ssh -o StrictHostKeyChecking=no" \
			config/ $(USERNAME)@$(IP):/opt/lovebin/config/; \
		rsync -avz --progress \
			--exclude='*.swp' \
			--exclude='*.swo' \
			--exclude='*~' \
			--exclude='.git' \
			-e "sshpass -p '$(SSH_PASSWORD)' ssh -o StrictHostKeyChecking=no" \
			deploy/ $(USERNAME)@$(IP):/opt/lovebin/deploy/; \
		rsync -avz --progress \
			--exclude='*.swp' \
			--exclude='*.swo' \
			--exclude='*~' \
			--exclude='.git' \
			-e "sshpass -p '$(SSH_PASSWORD)' ssh -o StrictHostKeyChecking=no" \
			frontend/ $(USERNAME)@$(IP):/opt/lovebin/frontend/; \
		rsync -avz --progress \
			--exclude='*.swp' \
			--exclude='*.swo' \
			--exclude='*~' \
			--exclude='.git' \
			-e "sshpass -p '$(SSH_PASSWORD)' ssh -o StrictHostKeyChecking=no" \
			migrations/ $(USERNAME)@$(IP):/opt/lovebin/migrations/; \
	else \
		ssh -o StrictHostKeyChecking=no $(USERNAME)@$(IP) "mkdir -p /opt/lovebin"; \
		rsync -avz --progress \
			--exclude='*.swp' \
			--exclude='*.swo' \
			--exclude='*~' \
			--exclude='.git' \
			-e "ssh -o StrictHostKeyChecking=no" \
			config/ $(USERNAME)@$(IP):/opt/lovebin/config/; \
		rsync -avz --progress \
			--exclude='*.swp' \
			--exclude='*.swo' \
			--exclude='*~' \
			--exclude='.git' \
			-e "ssh -o StrictHostKeyChecking=no" \
			deploy/ $(USERNAME)@$(IP):/opt/lovebin/deploy/; \
		rsync -avz --progress \
			--exclude='*.swp' \
			--exclude='*.swo' \
			--exclude='*~' \
			--exclude='.git' \
			-e "ssh -o StrictHostKeyChecking=no" \
			frontend/ $(USERNAME)@$(IP):/opt/lovebin/frontend/; \
		rsync -avz --progress \
			--exclude='*.swp' \
			--exclude='*.swo' \
			--exclude='*~' \
			--exclude='.git' \
			-e "ssh -o StrictHostKeyChecking=no" \
			migrations/ $(USERNAME)@$(IP):/opt/lovebin/migrations/; \
	fi
	@echo "Successfully copied config and deploy folders to $(USERNAME)@$(IP):/opt/lovebin/"
	@echo "Note: .env files were excluded from copying for security reasons"
	@echo "Please manually copy and update config/.env on the remote server"
