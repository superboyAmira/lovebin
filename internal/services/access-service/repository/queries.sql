-- name: VerifyPassword :one
SELECT password_hash FROM media_resources
WHERE resource_key = $1;

-- name: CheckResourceAccess :one
SELECT 
    id,
    resource_key,
    password_hash,
    expires_at,
    viewed,
    salt
FROM media_resources
WHERE resource_key = $1
AND (expires_at IS NULL OR expires_at > NOW());

