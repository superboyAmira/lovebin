package api

import (
	"io"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	accessservice "lovebin/internal/services/access-service"
	mediaservice "lovebin/internal/services/media-service"
	"lovebin/modules/logger"
)

type Handlers struct {
	logger         logger.Logger
	mediaService   *mediaservice.Service
	accessService  *accessservice.Service
}

func NewHandlers(
	logger logger.Logger,
	mediaService *mediaservice.Service,
	accessService *accessservice.Service,
) *Handlers {
	return &Handlers{
		logger:        logger,
		mediaService:  mediaService,
		accessService: accessService,
	}
}

type UploadRequest struct {
	Password  string        `json:"password,omitempty"`
	ExpiresIn *time.Duration `json:"expires_in,omitempty"` // e.g., "1h", "24h", "7d"
}

type UploadResponse struct {
	ResourceKey string `json:"resource_key"`
	URL         string `json:"url"`
}

// UploadMedia handles media upload
func (h *Handlers) UploadMedia(c *fiber.Ctx) error {
	var req UploadRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}
	// Get file from multipart form
	file, err := c.FormFile("file")
	if err != nil {
		// Try to get file from body if not multipart
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "file is required in multipart form",
		})
	}

	// Open file
	src, err := file.Open()
	if err != nil {
		h.logger.Error("failed to open file", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to process file",
		})
	}
	defer src.Close()

	// Upload media
	uploadReq := mediaservice.UploadRequest{
		Data:      src,
		Password:  req.Password,
		ExpiresIn: req.ExpiresIn,
	}

	resp, err := h.mediaService.UploadMedia(c.Context(), uploadReq)
	if err != nil {
		h.logger.Error("failed to upload media", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to upload media",
		})
	}

	return c.JSON(UploadResponse{
		ResourceKey: resp.ResourceKey,
		URL:         resp.URL,
	})
}

type DownloadRequest struct {
	Password string `json:"password,omitempty"`
}

// DownloadMedia handles media download (one-time view)
func (h *Handlers) DownloadMedia(c *fiber.Ctx) error {
	resourceKey := c.Params("key")
	if resourceKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "resource key is required",
		})
	}

	// Get password from query parameter or body
	var req DownloadRequest
	_ = c.BodyParser(&req) // Ignore error, try query param if body parsing fails
	
	// If password not in body, try query param
	if req.Password == "" {
		req.Password = c.Query("password", "")
	}

	// Extract resource key (without encryption key part for access check)
	resourceKeyParts := strings.Split(resourceKey, "#")
	resourceKeyForCheck := resourceKeyParts[0]

	// Verify access (using only resource key, not encryption key)
	err := h.accessService.VerifyAccess(c.Context(), resourceKeyForCheck, req.Password)
	if err != nil {
		switch err {
		case accessservice.ErrNotFound:
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "resource not found",
			})
		case accessservice.ErrExpired:
			return c.Status(fiber.StatusGone).JSON(fiber.Map{
				"error": "resource expired",
			})
		case accessservice.ErrAlreadyViewed:
			return c.Status(fiber.StatusGone).JSON(fiber.Map{
				"error": "resource already viewed",
			})
		case accessservice.ErrPasswordRequired, accessservice.ErrInvalidPassword:
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or missing password",
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "failed to verify access",
			})
		}
	}

	// Download media (this will mark as viewed and delete)
	downloadReq := mediaservice.DownloadRequest{
		ResourceKey: resourceKey,
		Password:    req.Password,
	}

	resp, err := h.mediaService.DownloadMedia(c.Context(), downloadReq)
	if err != nil {
		h.logger.Error("failed to download media", zap.Error(err))
		
		// Handle specific errors
		switch err {
		case mediaservice.ErrNotFound:
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "resource not found",
			})
		case mediaservice.ErrAlreadyViewed:
			return c.Status(fiber.StatusGone).JSON(fiber.Map{
				"error": "resource already viewed",
			})
		case mediaservice.ErrMissingEncryptionKey, mediaservice.ErrInvalidEncryptionKey:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid or missing encryption key in URL",
			})
		case mediaservice.ErrDecryptionFailed:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "decryption failed - invalid password or corrupted data",
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "failed to download media",
			})
		}
	}
	defer resp.Data.Close()

	// Stream response
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", "attachment")

	_, err = io.Copy(c.Response().BodyWriter(), resp.Data)
	if err != nil {
		h.logger.Error("failed to stream response", zap.Error(err))
		return err
	}

	return nil
}

// HealthCheck handles health check endpoint
func (h *Handlers) HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}

