.PHONY: migrate sqlc swag generate

# Run migrations
migrate:
	cd migrations/ && goose postgres "postgres://postgres:postgres@localhost:5432/lovebin" status

# Generate sqlc code
sqlc:
	cd internal/services/media-service/repository && sqlc generate
	cd internal/services/access-service/repository && sqlc generate

# Generate swagger documentation
swag:
	swag init -g cmd/lovebin/main.go -o docs

# Generate all code (sqlc + swagger)
generate: sqlc swag
