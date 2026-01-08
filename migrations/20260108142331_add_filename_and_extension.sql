-- +goose Up
-- +goose StatementBegin
ALTER TABLE media_resources
ADD COLUMN IF NOT EXISTS filename VARCHAR(255),
ADD COLUMN IF NOT EXISTS file_extension VARCHAR(50);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE media_resources
DROP COLUMN IF EXISTS filename,
DROP COLUMN IF EXISTS file_extension;
-- +goose StatementEnd
