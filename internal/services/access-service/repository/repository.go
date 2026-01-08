package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"lovebin/modules/timeparser"
)

// ResourceAccess represents resource access information
type ResourceAccess struct {
	ID           string
	ResourceKey  string
	PasswordHash *string
	ExpiresAt    timeparser.UniversalTime
	Viewed       bool
	Salt         []byte
}

// AccessRepository wraps sqlc Queries and converts types
type AccessRepository struct {
	queries *Queries
}

func NewAccessRepository(db *pgxpool.Pool) *AccessRepository {
	return &AccessRepository{
		queries: New(db),
	}
}

func (r *AccessRepository) VerifyPassword(ctx context.Context, resourceKey string) (string, error) {
	result, err := r.queries.VerifyPassword(ctx, resourceKey)
	if err != nil {
		return "", err
	}
	if !result.Valid {
		return "", nil
	}
	return result.String, nil
}

func (r *AccessRepository) CheckResourceAccess(ctx context.Context, resourceKey string) (ResourceAccess, error) {
	dbAccess, err := r.queries.CheckResourceAccess(ctx, resourceKey)
	if err != nil {
		return ResourceAccess{}, err
	}

	return toResourceAccess(dbAccess), nil
}

func toResourceAccess(db CheckResourceAccessRow) ResourceAccess {
	result := ResourceAccess{
		ResourceKey: db.ResourceKey,
		Salt:        db.Salt,
	}

	// Convert ID
	if db.ID.Valid {
		result.ID = uuid.UUID(db.ID.Bytes).String()
	}

	// Convert password hash
	if db.PasswordHash.Valid {
		result.PasswordHash = &db.PasswordHash.String
	}

	// Convert expires at to UniversalTime (UTC)
	if db.ExpiresAt.Valid {
		result.ExpiresAt = timeparser.NewUniversalTime(db.ExpiresAt.Time)
	}

	// Convert viewed
	if db.Viewed.Valid {
		result.Viewed = db.Viewed.Bool
	}

	return result
}
