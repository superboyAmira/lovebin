package mediaservice

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	mediarepo "lovebin/internal/services/media-service/repository"
	"lovebin/modules/encryption"
	"lovebin/modules/logger"
	"lovebin/modules/postgres"
	"lovebin/modules/s3"
	"lovebin/modules/timeparser"
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
	MarkAsViewed(ctx context.Context, resourceKey string) error
	DeleteMediaResource(ctx context.Context, resourceKey string) error
	GetMediaResourceForView(ctx context.Context, resourceKey string) (mediarepo.MediaResourceResult, error)
}

type CreateMediaResourceParams struct {
	ResourceKey   string
	PasswordHash  *string
	ExpiresAt     *time.Time
	Salt          []byte
	Filename      *string
	FileExtension *string
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
	Data      io.Reader
	Password  string
	ExpiresAt timeparser.UniversalTime // zero time means never expires
	Filename  string                   // original filename
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
	ResourceKey string
	Password    string
}

type DownloadResponse struct {
	Data          io.ReadCloser
	Filename      *string
	FileExtension *string
}

func (s *Service) DownloadMedia(ctx context.Context, req DownloadRequest) (*DownloadResponse, error) {
	// Parse resource key and encryption key from URL
	// Format: resourceKey#encKey or just resourceKey
	parts := strings.Split(req.ResourceKey, "#")
	resourceKey := parts[0]
	var encKeyBase64 string
	if len(parts) > 1 {
		encKeyBase64 = parts[1]
	}

	if encKeyBase64 == "" {
		return nil, ErrMissingEncryptionKey
	}

	// Decode encryption key
	encKey, err := base64.RawURLEncoding.DecodeString(encKeyBase64)
	if err != nil {
		return nil, ErrInvalidEncryptionKey
	}

	// Get resource from database with lock
	repoResource, err := s.repo.GetMediaResourceForView(ctx, resourceKey)
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
	s3Key := "media/" + resourceKey
	data, err := s.s3.Download(ctx, "", s3Key)
	if err != nil {
		return nil, ErrNotFound
	}

	// Mark as viewed (this will trigger deletion)
	err = s.repo.MarkAsViewed(ctx, resourceKey)
	if err != nil {
		data.Close()
		return nil, err
	}

	// Decrypt data
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

	// Delete from S3
	_ = s.s3.Delete(ctx, "", s3Key)

	// Delete from database
	_ = s.repo.DeleteMediaResource(ctx, resourceKey)

	return &DownloadResponse{
		Data:          io.NopCloser(bytes.NewReader(decryptedData)),
		Filename:      resource.Filename,
		FileExtension: resource.FileExtension,
	}, nil
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
