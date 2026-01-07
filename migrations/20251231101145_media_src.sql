-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS media_resources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_key VARCHAR(255) UNIQUE NOT NULL, -- S3 key
    password_hash VARCHAR(255), -- bcrypt hash of password (optional)
    expires_at TIMESTAMP, -- expiration time (NULL means never expires)
    viewed BOOLEAN DEFAULT FALSE, -- whether resource was viewed
    created_at TIMESTAMP DEFAULT NOW(),
    salt BYTEA -- salt for encryption (encryption key is in URL, not stored)
);
CREATE INDEX IF NOT EXISTS idx_media_resources_expires_at ON media_resources(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_media_resources_resource_key ON media_resources(resource_key);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS media_resources;
-- +goose StatementEnd
