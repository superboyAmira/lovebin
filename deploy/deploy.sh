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

# Pull latest image
echo "Pulling latest Docker image: ${DOCKER_IMAGE}:${DOCKER_TAG:-latest}"
docker pull "${DOCKER_IMAGE}:${DOCKER_TAG:-latest}"

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

echo ""
echo "=== Deployment Complete ==="
echo "Application is running at http://localhost"
echo "Check logs with: cd deploy && docker-compose -f docker-compose.prod.yml logs -f"
echo ""
