-- name: CreateMediaResource :one
INSERT INTO media_resources (
    resource_key,
    password_hash,
    expires_at,
    salt,
    filename,
    file_extension,
    blur_enabled
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING id, resource_key, password_hash, expires_at, viewed, created_at, salt, filename, file_extension, blur_enabled;

-- name: GetMediaResourceByKey :one
SELECT id, resource_key, password_hash, expires_at, viewed, created_at, salt, filename, file_extension, blur_enabled
FROM media_resources
WHERE resource_key = $1
AND (expires_at IS NULL OR expires_at > NOW())
AND viewed = FALSE;

-- name: GetMediaResourceByKeyAny :one
SELECT id, resource_key, password_hash, expires_at, viewed, created_at, salt, filename, file_extension, blur_enabled
FROM media_resources
WHERE resource_key = $1
AND (expires_at IS NULL OR expires_at > NOW());

-- name: MarkAsViewed :exec
UPDATE media_resources
SET viewed = TRUE
WHERE resource_key = $1;

-- name: DeleteMediaResource :exec
DELETE FROM media_resources
WHERE resource_key = $1;

-- name: GetExpiredResources :many
SELECT resource_key
FROM media_resources
WHERE expires_at IS NOT NULL
AND expires_at <= NOW();

-- name: DeleteExpiredResources :exec
DELETE FROM media_resources
WHERE expires_at IS NOT NULL
AND expires_at <= NOW();

-- name: GetMediaResourceForView :one
SELECT id, resource_key, password_hash, expires_at, viewed, created_at, salt, filename, file_extension, blur_enabled
FROM media_resources
WHERE resource_key = $1
AND (expires_at IS NULL OR expires_at > NOW())
FOR UPDATE;

