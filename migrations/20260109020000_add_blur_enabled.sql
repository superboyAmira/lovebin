-- +goose Up
-- +goose StatementBegin
ALTER TABLE media_resources
ADD COLUMN IF NOT EXISTS blur_enabled BOOLEAN DEFAULT FALSE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE media_resources
DROP COLUMN IF EXISTS blur_enabled;
-- +goose StatementEnd
