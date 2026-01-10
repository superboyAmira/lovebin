#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== LoveBin Production Deployment ==="

# Check if .env exists
if [ ! -f "$PROJECT_ROOT/config/.env" ]; then
    echo "Error: config/.env file not found!"
    echo "Please copy config/.env.example to config/.env and update with your values"
    exit 1
fi

# Load environment variables
set -a
source "$PROJECT_ROOT/config/.env"
set +a

# Check if DOCKER_IMAGE is set
if [ -z "$DOCKER_IMAGE" ]; then
    echo "Error: DOCKER_IMAGE is not set in config/.env"
    exit 1
fi

# Set default values if not set
if [ -z "$DOCKER_TAG" ]; then
    DOCKER_TAG="latest"
fi
if [ -z "$LOG_LEVEL" ]; then
    LOG_LEVEL="info"
fi
if [ -z "$POSTGRES_SSLMODE" ]; then
    POSTGRES_SSLMODE="disable"
fi
if [ -z "$S3_REGION" ]; then
    S3_REGION="us-east-1"
fi

# Export for docker-compose
export DOCKER_IMAGE
export DOCKER_TAG
export LOG_LEVEL
export POSTGRES_SSLMODE
export S3_REGION

# Pull latest image
echo "Pulling latest Docker image: ${DOCKER_IMAGE}:${DOCKER_TAG}"
docker pull "${DOCKER_IMAGE}:${DOCKER_TAG}"

# Stop existing containers
echo "Stopping existing containers..."
cd "$PROJECT_ROOT/deploy"
docker-compose -f docker-compose.prod.yml down

# Start containers
echo "Starting containers..."
docker-compose -f docker-compose.prod.yml up -d

# Wait for services to be healthy
echo "Waiting for services to be healthy..."
sleep 10

# Check health
echo "Checking service health..."
if docker exec lovebin-app-prod wget --no-verbose --tries=1 --spider http://localhost:8080/health; then
    echo "✓ Application is healthy"
else
    echo "✗ Application health check failed"
    exit 1
fi

# Adding bucket to minio
echo "Creating MinIO bucket if it doesn't exist..."
docker exec lovebin-minio-prod mc alias set myminio http://localhost:9000 "${MINIO_ROOT_USER}" "${MINIO_ROOT_PASSWORD}" || true
docker exec lovebin-minio-prod mc mb "myminio/${S3_BUCKET}" || {
    echo "Bucket ${S3_BUCKET} might already exist, continuing..."
}

# Migrating database
echo "Migrating database..."
# Build connection string from .env variables
POSTGRES_CONN_STRING="postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5432/${POSTGRES_DB}?sslmode=${POSTGRES_SSLMODE}"

# Install goose if not present
if ! command -v goose >/dev/null 2>&1; then
    echo "Installing goose..."
    curl -fsSL https://raw.githubusercontent.com/pressly/goose/master/install.sh | sh
fi

# Run migrations
echo "Running database migrations..."
if [ -d "$PROJECT_ROOT/migrations" ]; then
    cd "$PROJECT_ROOT/migrations"
    if command -v goose >/dev/null 2>&1; then
        # Check if postgres port is accessible
        if nc -z localhost 5432 2>/dev/null; then
            # Port is exposed, connect directly
            goose postgres "$POSTGRES_CONN_STRING" up || {
                echo "Error: Failed to run migrations"
                exit 1
            }
        else
            # Port not exposed, provide instructions
            echo "Warning: PostgreSQL port 5432 is not exposed to host"
            echo "To run migrations, you can either:"
            echo "1. Temporarily expose postgres port in docker-compose.prod.yml:"
            echo "   Add 'ports: - \"5432:5432\"' to postgres service, then run:"
            echo "   cd $PROJECT_ROOT/migrations"
            echo "   goose postgres \"$POSTGRES_CONN_STRING\" up"
            echo "2. Or run migrations manually after exposing the port"
            echo ""
            echo "Skipping automatic migrations..."
        fi
    else
        echo "Error: goose installation failed"
        exit 1
    fi
else
    echo "Warning: migrations directory not found at $PROJECT_ROOT/migrations"
fi

echo ""
echo "=== Deployment Complete ==="
echo "Application is running at http://localhost"
echo "Check logs with: cd deploy && docker-compose -f docker-compose.prod.yml logs -f"
echo ""
