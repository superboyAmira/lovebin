package accessservice

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	accessrepo "lovebin/internal/services/access-service/repository"
	"lovebin/modules/logger"
	"lovebin/modules/postgres"
)

// Convert repository types to service types
func repoToServiceResourceAccess(repo accessrepo.ResourceAccess) ResourceAccess {
	return ResourceAccess{
		ID:           repo.ID,
		ResourceKey:  repo.ResourceKey,
		PasswordHash: repo.PasswordHash,
		ExpiresAt:    repo.ExpiresAt,
		Viewed:       repo.Viewed,
		Salt:         repo.Salt,
	}
}

type Service struct {
	logger   logger.Logger
	postgres postgres.Postgres
	repo     Repository
}

type Repository interface {
	VerifyPassword(ctx context.Context, resourceKey string) (string, error)
	CheckResourceAccess(ctx context.Context, resourceKey string) (accessrepo.ResourceAccess, error)
}

type ResourceAccess struct {
	ID           string
	ResourceKey  string
	PasswordHash *string
	ExpiresAt    *time.Time
	Viewed       bool
	Salt         []byte
}

func NewService(
	logger logger.Logger,
	postgres postgres.Postgres,
	repo Repository,
) *Service {
	return &Service{
		logger:   logger,
		postgres: postgres,
		repo:     repo,
	}
}

func (s *Service) VerifyAccess(ctx context.Context, resourceKey, password string) error {
	repoAccess, err := s.repo.CheckResourceAccess(ctx, resourceKey)
	if err != nil {
		return ErrNotFound
	}
	access := repoToServiceResourceAccess(repoAccess)

	// Check expiration
	if access.ExpiresAt != nil && access.ExpiresAt.Before(time.Now()) {
		return ErrExpired
	}

	// Check if already viewed
	if access.Viewed {
		return ErrAlreadyViewed
	}

	// Verify password if required
	if access.PasswordHash != nil {
		if password == "" {
			return ErrPasswordRequired
		}
		if err := bcrypt.CompareHashAndPassword([]byte(*access.PasswordHash), []byte(password)); err != nil {
			return ErrInvalidPassword
		}
	}

	return nil
}

func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

var (
	ErrNotFound         = errors.New("resource not found")
	ErrExpired          = errors.New("resource expired")
	ErrAlreadyViewed    = errors.New("resource already viewed")
	ErrPasswordRequired = errors.New("password required")
	ErrInvalidPassword  = errors.New("invalid password")
)
