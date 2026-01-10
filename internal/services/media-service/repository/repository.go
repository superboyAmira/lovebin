package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MediaRepository wraps sqlc Queries and converts types
type MediaRepository struct {
	queries *Queries
}

// CreateMediaResourceInput represents input parameters for creating a media resource
type CreateMediaResourceInput struct {
	ResourceKey   string
	PasswordHash  *string
	ExpiresAt     *time.Time
	Salt          []byte
	Filename      *string
	FileExtension *string
	BlurEnabled   bool
}

// MediaResourceResult represents a media resource result
type MediaResourceResult struct {
	ID            string
	ResourceKey   string
	PasswordHash  *string
	ExpiresAt     *time.Time
	Viewed        bool
	CreatedAt     time.Time
	Salt          []byte
	Filename      *string
	FileExtension *string
	BlurEnabled   bool
}

func NewMediaRepository(db *pgxpool.Pool) *MediaRepository {
	return &MediaRepository{
		queries: New(db),
	}
}

func (r *MediaRepository) CreateMediaResource(ctx context.Context, arg CreateMediaResourceInput) (MediaResourceResult, error) {
	// Convert input types to sqlc types
	sqlcParams := CreateMediaResourceParams{
		ResourceKey: arg.ResourceKey,
		Salt:        arg.Salt,
	}

	// Convert password hash
	if arg.PasswordHash != nil {
		sqlcParams.PasswordHash = pgtype.Text{
			String: *arg.PasswordHash,
			Valid:  true,
		}
	}

	// Convert expires at
	if arg.ExpiresAt != nil {
		sqlcParams.ExpiresAt = pgtype.Timestamp{
			Time:  *arg.ExpiresAt,
			Valid: true,
		}
	}

	// Convert filename
	if arg.Filename != nil {
		sqlcParams.Filename = pgtype.Text{
			String: *arg.Filename,
			Valid:  true,
		}
	}

	// Convert file extension
	if arg.FileExtension != nil {
		sqlcParams.FileExtension = pgtype.Text{
			String: *arg.FileExtension,
			Valid:  true,
		}
	}

	// Convert blur enabled
	sqlcParams.BlurEnabled = pgtype.Bool{
		Bool:  arg.BlurEnabled,
		Valid: true,
	}

	dbResource, err := r.queries.CreateMediaResource(ctx, sqlcParams)
	if err != nil {
		return MediaResourceResult{}, err
	}

	return toMediaResourceResult(dbResource), nil
}

func (r *MediaRepository) GetMediaResourceByKey(ctx context.Context, resourceKey string) (MediaResourceResult, error) {
	dbResource, err := r.queries.GetMediaResourceByKey(ctx, resourceKey)
	if err != nil {
		return MediaResourceResult{}, err
	}

	return toMediaResourceResult(dbResource), nil
}

func (r *MediaRepository) GetMediaResourceByKeyAny(ctx context.Context, resourceKey string) (MediaResourceResult, error) {
	dbResource, err := r.queries.GetMediaResourceByKeyAny(ctx, resourceKey)
	if err != nil {
		return MediaResourceResult{}, err
	}

	return toMediaResourceResult(dbResource), nil
}

func (r *MediaRepository) MarkAsViewed(ctx context.Context, resourceKey string) error {
	return r.queries.MarkAsViewed(ctx, resourceKey)
}

func (r *MediaRepository) DeleteMediaResource(ctx context.Context, resourceKey string) error {
	return r.queries.DeleteMediaResource(ctx, resourceKey)
}

func (r *MediaRepository) GetMediaResourceForView(ctx context.Context, resourceKey string) (MediaResourceResult, error) {
	dbResource, err := r.queries.GetMediaResourceForView(ctx, resourceKey)
	if err != nil {
		return MediaResourceResult{}, err
	}

	return toMediaResourceResult(dbResource), nil
}

func (r *MediaRepository) GetExpiredResources(ctx context.Context) ([]string, error) {
	return r.queries.GetExpiredResources(ctx)
}

func (r *MediaRepository) DeleteExpiredResources(ctx context.Context) error {
	return r.queries.DeleteExpiredResources(ctx)
}

func toMediaResourceResult(db MediaResource) MediaResourceResult {
	result := MediaResourceResult{
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

	// Convert expires at
	if db.ExpiresAt.Valid {
		result.ExpiresAt = &db.ExpiresAt.Time
	}

	// Convert viewed
	if db.Viewed.Valid {
		result.Viewed = db.Viewed.Bool
	}

	// Convert created at
	if db.CreatedAt.Valid {
		result.CreatedAt = db.CreatedAt.Time
	}

	// Convert filename
	if db.Filename.Valid {
		result.Filename = &db.Filename.String
	}

	// Convert file extension
	if db.FileExtension.Valid {
		result.FileExtension = &db.FileExtension.String
	}

	// Convert blur enabled (default to false if not valid, but should always be valid since column has DEFAULT FALSE)
	result.BlurEnabled = db.BlurEnabled.Valid && db.BlurEnabled.Bool

	return result
}
