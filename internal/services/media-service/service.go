package mediaservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"fmt"
	mediarepo "lovebin/internal/services/media-service/repository"
	"lovebin/modules/encryption"
	"lovebin/modules/logger"
	"lovebin/modules/postgres"
	"lovebin/modules/s3"
	"lovebin/modules/timeparser"

	"go.uber.org/zap"
)

// Convert repository types to service types
func repoToServiceMediaResource(repo mediarepo.MediaResourceResult) MediaResource {
	result := MediaResource{
		ID:            repo.ID,
		ResourceKey:   repo.ResourceKey,
		PasswordHash:  repo.PasswordHash,
		Viewed:        repo.Viewed,
		Salt:          repo.Salt,
		Filename:      repo.Filename,
		FileExtension: repo.FileExtension,
		BlurEnabled:   repo.BlurEnabled,
	}

	// Convert ExpiresAt
	if repo.ExpiresAt != nil {
		result.ExpiresAt = timeparser.NewUniversalTime(*repo.ExpiresAt)
	}

	// Convert CreatedAt
	if !repo.CreatedAt.IsZero() {
		result.CreatedAt = timeparser.NewUniversalTime(repo.CreatedAt)
	}

	return result
}

func serviceToRepoCreateParams(arg CreateMediaResourceParams) mediarepo.CreateMediaResourceInput {
	return mediarepo.CreateMediaResourceInput{
		ResourceKey:   arg.ResourceKey,
		PasswordHash:  arg.PasswordHash,
		ExpiresAt:     arg.ExpiresAt,
		Salt:          arg.Salt,
		Filename:      arg.Filename,
		FileExtension: arg.FileExtension,
		BlurEnabled:   arg.BlurEnabled,
	}
}

type Service struct {
	logger     logger.Logger
	postgres   postgres.Postgres
	s3         s3.S3
	encryption encryption.Encryption
	repo       Repository
}

type Repository interface {
	CreateMediaResource(ctx context.Context, arg mediarepo.CreateMediaResourceInput) (mediarepo.MediaResourceResult, error)
	GetMediaResourceByKey(ctx context.Context, resourceKey string) (mediarepo.MediaResourceResult, error)
	GetMediaResourceByKeyAny(ctx context.Context, resourceKey string) (mediarepo.MediaResourceResult, error)
	MarkAsViewed(ctx context.Context, resourceKey string) error
	DeleteMediaResource(ctx context.Context, resourceKey string) error
	GetMediaResourceForView(ctx context.Context, resourceKey string) (mediarepo.MediaResourceResult, error)
	GetExpiredResources(ctx context.Context) ([]string, error)
	DeleteExpiredResources(ctx context.Context) error
}

type CreateMediaResourceParams struct {
	ResourceKey   string
	PasswordHash  *string
	ExpiresAt     *time.Time
	Salt          []byte
	Filename      *string
	FileExtension *string
	BlurEnabled   bool
}

type MediaResource struct {
	ID            string
	ResourceKey   string
	PasswordHash  *string
	ExpiresAt     timeparser.UniversalTime
	Viewed        bool
	CreatedAt     timeparser.UniversalTime
	Salt          []byte
	Filename      *string
	FileExtension *string
	BlurEnabled   bool
}

func NewService(
	logger logger.Logger,
	postgres postgres.Postgres,
	s3 s3.S3,
	encryption encryption.Encryption,
	repo Repository,
) *Service {
	return &Service{
		logger:     logger,
		postgres:   postgres,
		s3:         s3,
		encryption: encryption,
		repo:       repo,
	}
}

type UploadRequest struct {
	Data        io.Reader
	Password    string
	ExpiresAt   timeparser.UniversalTime // zero time means never expires
	Filename    string                   // original filename
	BlurEnabled bool                     // enable blur effect on preview
}

type UploadResponse struct {
	ResourceKey string
	URL         string
}

func (s *Service) UploadMedia(ctx context.Context, req UploadRequest) (*UploadResponse, error) {
	// Generate resource key (this will be part of URL)
	resourceKey, err := encryption.GenerateURLKey()
	if err != nil {
		return nil, err
	}

	// Generate encryption key (this will be part of URL, not stored in DB)
	encKey, err := s.encryption.GenerateKey()
	if err != nil {
		return nil, err
	}
	encKeyBase64 := base64.RawURLEncoding.EncodeToString(encKey)

	// Read all data
	data, err := io.ReadAll(req.Data)
	if err != nil {
		return nil, err
	}

	// Encrypt data using encryption key
	// If password is provided, we use it as additional layer, otherwise use encKey
	encryptionPassword := string(encKey)
	if req.Password != "" {
		// Combine encryption key with password for stronger security
		encryptionPassword = req.Password + string(encKey)
	}

	encryptedData, salt, err := s.encryption.Encrypt(data, encryptionPassword)
	if err != nil {
		return nil, err
	}

	// Upload to S3
	s3Key := "media/" + resourceKey
	_, err = s.s3.Upload(ctx, "", s3Key, bytes.NewReader(encryptedData))
	if err != nil {
		return nil, err
	}

	// Hash password if provided (for access control)
	var passwordHash *string
	if req.Password != "" {
		hash, err := hashPassword(req.Password)
		if err != nil {
			return nil, err
		}
		passwordHash = &hash
	}

	// Convert UniversalTime to *time.Time for database (nil if zero)
	var expiresAt *time.Time
	if !req.ExpiresAt.IsZero() {
		expiresAt = &req.ExpiresAt.Time
	}

	// Extract filename and extension
	var filename *string
	var fileExtension *string
	if req.Filename != "" {
		// Extract extension from filename
		extWithDot := filepath.Ext(req.Filename)
		if extWithDot != "" {
			// Remove leading dot from extension for storage
			ext := strings.TrimPrefix(extWithDot, ".")
			fileExtension = &ext
			// Get base filename without extension
			baseName := strings.TrimSuffix(req.Filename, extWithDot)
			if baseName == "" {
				baseName = req.Filename
			}
			filename = &baseName
		} else {
			// No extension, use full filename
			filename = &req.Filename
		}
	}

	// Store in database (salt is needed for decryption)
	_, err = s.repo.CreateMediaResource(ctx, serviceToRepoCreateParams(CreateMediaResourceParams{
		ResourceKey:   resourceKey,
		PasswordHash:  passwordHash,
		ExpiresAt:     expiresAt,
		Salt:          salt,
		Filename:      filename,
		FileExtension: fileExtension,
		BlurEnabled:   req.BlurEnabled,
	}))
	if err != nil {
		// Cleanup S3 on error
		_ = s.s3.Delete(ctx, "", s3Key)
		return nil, err
	}

	// Return URL with encryption key as fragment (not sent to server)
	// Format: /media/{resourceKey}#{encKey}
	return &UploadResponse{
		ResourceKey: resourceKey + "#" + encKeyBase64,
		URL:         "/media/" + resourceKey + "#" + encKeyBase64,
	}, nil
}

type DownloadRequest struct {
	ResourceKey  string
	Password     string
	EncKeyBase64 string
}

type MediaInfo struct {
	Filename      *string
	FileExtension *string
	IsImage       bool
	BlurEnabled   bool
}

// GetMediaInfo gets media file information without downloading
func (s *Service) GetMediaInfo(ctx context.Context, resourceKey string) (*MediaInfo, error) {
	// Get resource from database (any, including viewed)
	repoResource, err := s.repo.GetMediaResourceByKeyAny(ctx, resourceKey)
	if err != nil {
		return nil, ErrNotFound
	}
	resource := repoToServiceMediaResource(repoResource)

	// Check if it's an image
	isImage := false
	if resource.FileExtension != nil {
		ext := strings.ToLower(*resource.FileExtension)
		isImage = slices.Contains([]string{"jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "ico"}, ext)
	}

	return &MediaInfo{
		Filename:      resource.Filename,
		FileExtension: resource.FileExtension,
		IsImage:       isImage,
		BlurEnabled:   resource.BlurEnabled,
	}, nil
}

// GetMediaPreview gets media file for preview (doesn't mark as viewed or delete)
func (s *Service) GetMediaPreview(ctx context.Context, req *DownloadRequest) (*DownloadResponse, error) {
	if req.EncKeyBase64 == "" {
		return nil, ErrMissingEncryptionKey
	}

	// Decode encryption key
	encKey, err := base64.RawURLEncoding.DecodeString(req.EncKeyBase64)
	if err != nil {
		return nil, ErrInvalidEncryptionKey
	}

	// Get resource from database (without lock, don't mark as viewed)
	repoResource, err := s.repo.GetMediaResourceByKey(ctx, req.ResourceKey)
	if err != nil {
		return nil, ErrNotFound
	}
	resource := repoToServiceMediaResource(repoResource)

	// Check expiration
	if !resource.ExpiresAt.IsZero() && resource.ExpiresAt.Time.Before(time.Now().UTC()) {
		return nil, ErrExpired
	}

	// Check if already viewed
	if resource.Viewed {
		return nil, ErrAlreadyViewed
	}

	// Verify password if required
	if resource.PasswordHash != nil {
		if !verifyPassword(req.Password, *resource.PasswordHash) {
			return nil, ErrInvalidPassword
		}
	}

	// Download from S3
	s3Key := "media/" + req.ResourceKey
	data, err := s.s3.Download(ctx, "", s3Key)
	if err != nil {
		return nil, ErrNotFound
	}

	// Decrypt data
	encryptedData, err := io.ReadAll(data)
	if err != nil {
		return nil, err
	}
	err = data.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close data: %w", err)
	}

	// Reconstruct encryption password
	encryptionPassword := string(encKey)
	if req.Password != "" {
		encryptionPassword = req.Password + string(encKey)
	}

	decryptedData, err := s.encryption.Decrypt(encryptedData, resource.Salt, encryptionPassword)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	// Return preview (don't delete or mark as viewed)
	return &DownloadResponse{
		Data:          io.NopCloser(bytes.NewReader(decryptedData)),
		Filename:      resource.Filename,
		FileExtension: resource.FileExtension,
	}, nil
}

type DownloadResponse struct {
	Data          io.ReadCloser
	Filename      *string
	FileExtension *string
}

func (s *Service) DownloadMedia(ctx context.Context, req *DownloadRequest) (*DownloadResponse, error) {
	if req.EncKeyBase64 == "" {
		return nil, ErrMissingEncryptionKey
	}

	// Decode encryption key
	encKey, err := base64.RawURLEncoding.DecodeString(req.EncKeyBase64)
	if err != nil {
		return nil, ErrInvalidEncryptionKey
	}

	// Get resource from database with lock
	repoResource, err := s.repo.GetMediaResourceForView(ctx, req.ResourceKey)
	if err != nil {
		return nil, ErrNotFound
	}
	resource := repoToServiceMediaResource(repoResource)

	// Check expiration
	if !resource.ExpiresAt.IsZero() && resource.ExpiresAt.Time.Before(time.Now().UTC()) {
		return nil, ErrExpired
	}

	// Check if already viewed
	if resource.Viewed {
		return nil, ErrAlreadyViewed
	}

	// Verify password if required
	if resource.PasswordHash != nil {
		if !verifyPassword(req.Password, *resource.PasswordHash) {
			return nil, ErrInvalidPassword
		}
	}

	// Download from S3
	s3Key := "media/" + req.ResourceKey
	data, err := s.s3.Download(ctx, "", s3Key)
	if err != nil {
		return nil, ErrNotFound
	}

	// Decrypt data first
	encryptedData, err := io.ReadAll(data)
	data.Close()
	if err != nil {
		return nil, err
	}

	// Reconstruct encryption password
	encryptionPassword := string(encKey)
	if req.Password != "" {
		encryptionPassword = req.Password + string(encKey)
	}

	decryptedData, err := s.encryption.Decrypt(encryptedData, resource.Salt, encryptionPassword)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	// Mark as viewed (don't delete, just mark as viewed)
	err = s.repo.MarkAsViewed(ctx, req.ResourceKey)
	if err != nil {
		// Log error but don't fail the download
		s.logger.Warn("failed to mark resource as viewed", zap.Error(err), zap.String("resource_key", req.ResourceKey))
	}

	return &DownloadResponse{
		Data:          io.NopCloser(bytes.NewReader(decryptedData)),
		Filename:      resource.Filename,
		FileExtension: resource.FileExtension,
	}, nil
}

// CleanupExpiredResources removes expired resources from database and S3
func (s *Service) CleanupExpiredResources(ctx context.Context) error {
	// Get list of expired resource keys before deletion
	expiredKeys, err := s.repo.GetExpiredResources(ctx)
	if err != nil {
		s.logger.Error("failed to get expired resources", zap.Error(err))
		return err
	}

	// Delete from S3
	for _, resourceKey := range expiredKeys {
		s3Key := "media/" + resourceKey
		if err := s.s3.Delete(ctx, "", s3Key); err != nil {
			// Log error but continue with other deletions
			s.logger.Warn("failed to delete expired resource from S3", zap.String("resource_key", resourceKey), zap.Error(err))
		} else {
			s.logger.Info("deleted expired resource from S3", zap.String("resource_key", resourceKey))
		}
	}

	// Delete from database
	if err := s.repo.DeleteExpiredResources(ctx); err != nil {
		s.logger.Error("failed to delete expired resources from database", zap.Error(err))
		return err
	}

	s.logger.Info("cleanup completed", zap.Int("deleted_count", len(expiredKeys)))
	return nil
}

// Helper functions
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

var (
	ErrAlreadyViewed        = errors.New("resource already viewed")
	ErrInvalidPassword      = errors.New("invalid password")
	ErrExpired              = errors.New("resource expired")
	ErrNotFound             = errors.New("resource not found")
	ErrMissingEncryptionKey = errors.New("encryption key missing from URL")
	ErrInvalidEncryptionKey = errors.New("invalid encryption key")
	ErrDecryptionFailed     = errors.New("decryption failed")
)
