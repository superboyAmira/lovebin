.PHONY: migrate migrate-down migrate-up sqlc swag generate

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
