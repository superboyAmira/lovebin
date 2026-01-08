-- name: CreateMediaResource :one
INSERT INTO media_resources (
    resource_key,
    password_hash,
    expires_at,
    salt,
    filename,
    file_extension
) VALUES (
    $1, $2, $3, $4, $5, $6
) RETURNING id, resource_key, password_hash, expires_at, viewed, created_at, salt, filename, file_extension;

-- name: GetMediaResourceByKey :one
SELECT id, resource_key, password_hash, expires_at, viewed, created_at, salt, filename, file_extension
FROM media_resources
WHERE resource_key = $1
AND (expires_at IS NULL OR expires_at > NOW())
AND viewed = FALSE;

-- name: MarkAsViewed :exec
UPDATE media_resources
SET viewed = TRUE
WHERE resource_key = $1;

-- name: DeleteMediaResource :exec
DELETE FROM media_resources
WHERE resource_key = $1;

-- name: DeleteExpiredResources :exec
DELETE FROM media_resources
WHERE expires_at IS NOT NULL
AND expires_at <= NOW();

-- name: GetMediaResourceForView :one
SELECT id, resource_key, password_hash, expires_at, viewed, created_at, salt, filename, file_extension
FROM media_resources
WHERE resource_key = $1
AND (expires_at IS NULL OR expires_at > NOW())
FOR UPDATE;

