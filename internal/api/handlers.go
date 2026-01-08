package api

import (
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	accessservice "lovebin/internal/services/access-service"
	mediaservice "lovebin/internal/services/media-service"
	"lovebin/modules/logger"
	"lovebin/modules/timeparser"
)

type Handlers struct {
	logger        logger.Logger
	mediaService  *mediaservice.Service
	accessService *accessservice.Service
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
	Password  string                   `json:"password,omitempty" form:"password"`
	ExpiresIn timeparser.UniversalTime `json:"expires_in,omitempty" form:"expires_in"` // e.g., "1h", "24h", "7d", "2024-12-31T23:59:59Z"
}

type UploadResponse struct {
	ResourceKey string `json:"resource_key"`
	URL         string `json:"url"`
}

// UploadMedia handles media upload
// @Summary      Upload media file
// @Description  Upload a media file (photo or video) with optional password protection and expiration time. ExpiresIn supports: duration (1h, 24h, 7d, 2w, 1y) or absolute time (RFC3339, ISO8601, Unix timestamp)
// @Tags         media
// @Accept       multipart/form-data
// @Produce      json
// @Param        file        formData  file    true   "Media file to upload"
// @Param        password    formData  string  false  "Optional password for access protection"
// @Param        expires_in  formData  string  false  "Expiration time: duration (1h, 24h, 7d, 2w) or absolute (RFC3339, ISO8601, Unix timestamp). Leave empty for no expiration"
// @Success      200  {object}  UploadResponse
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /upload [post]
func (h *Handlers) UploadMedia(c *fiber.Ctx) error {
	// Get file from multipart form first
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "file is required in multipart form",
		})
	}

	// Parse form data
	var req UploadRequest
	req.Password = c.FormValue("password")
	expiresInStr := c.FormValue("expires_in")

	// Parse expires_in using universal time parser
	if expiresInStr != "" {
		if err := req.ExpiresIn.UnmarshalText([]byte(expiresInStr)); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid expires_in format: " + err.Error(),
			})
		}

		// Проверяем, что время в будущем
		if !req.ExpiresIn.IsZero() && req.ExpiresIn.Time.Before(time.Now().UTC()) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "expires_in must be in the future",
			})
		}
	} else {
		req.ExpiresIn = timeparser.NewUniversalTime(time.Now().Add(24 * time.Hour))
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
		ExpiresAt: req.ExpiresIn,
		Filename:  file.Filename,
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
// @Summary      Download media file
// @Description  Download a media file. The file will be deleted after first successful download. Requires encryption key in URL fragment.
// @Tags         media
// @Accept       json
// @Produce      application/octet-stream
// @Param        key       path      string  true   "Resource key with encryption key (format: resourceKey#encryptionKey)"
// @Param        password  query     string  false  "Password if resource is password protected"
// @Success      200       {file}    binary
// @Failure      400       {object}  map[string]string
// @Failure      401       {object}  map[string]string
// @Failure      404       {object}  map[string]string
// @Failure      410       {object}  map[string]string
// @Failure      500       {object}  map[string]string
// @Router       /media/{key} [get]
func (h *Handlers) DownloadMedia(c *fiber.Ctx) error {
	resourceKeyEncoded := c.Params("key")
	if resourceKeyEncoded == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "resource key is required",
		})
	}

	// Decode URL-encoded key (handles %3D, %23, etc.)
	resourceKey, err := url.PathUnescape(resourceKeyEncoded)
	if err != nil {
		// If PathUnescape fails, try QueryUnescape as fallback
		resourceKey, err = url.QueryUnescape(resourceKeyEncoded)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "invalid resource key encoding",
			})
		}
	}

	var req DownloadRequest
	req.Password = c.Query("password", "")

	// Extract resource key (without encryption key part for access check)
	resourceKeyParts := strings.Split(resourceKey, "#")
	resourceKeyForCheck := resourceKeyParts[0]

	// Verify access (using only resource key, not encryption key)
	err = h.accessService.VerifyAccess(c.Context(), resourceKeyForCheck, req.Password)
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

	// Build filename from saved name and extension
	var downloadFilename string
	if resp.Filename != nil && *resp.Filename != "" {
		downloadFilename = *resp.Filename
		if resp.FileExtension != nil && *resp.FileExtension != "" {
			downloadFilename += "." + *resp.FileExtension
		}
	} else {
		// Fallback to resourceKey if filename not saved
		downloadFilename = resourceKeyForCheck
	}

	// Stream response
	c.Set("Content-Type", "application/octet-stream")
	// Set Content-Disposition with filename
	// Use RFC 5987 format for UTF-8 support (filename* parameter)
	// This ensures proper encoding for non-ASCII characters (e.g., Russian, Chinese, etc.)
	disposition := buildContentDisposition(downloadFilename)
	c.Set("Content-Disposition", disposition)

	_, err = io.Copy(c.Response().BodyWriter(), resp.Data)
	if err != nil {
		h.logger.Error("failed to stream response", zap.Error(err))
		return err
	}

	return nil
}

// buildContentDisposition builds Content-Disposition header with proper UTF-8 encoding
// Uses RFC 5987 format: attachment; filename="fallback"; filename*=UTF-8”encoded
// This ensures proper display of non-ASCII characters (Russian, Chinese, etc.) in filenames
func buildContentDisposition(filename string) string {
	// Escape filename for ASCII fallback (basic escaping for quotes and backslashes)
	escapedASCII := strings.ReplaceAll(filename, `\`, `\\`)
	escapedASCII = strings.ReplaceAll(escapedASCII, `"`, `\"`)

	// For UTF-8 encoding, use RFC 5987 format
	// Format: filename*=UTF-8''percent-encoded-filename
	// Percent-encode the filename using URL encoding
	// QueryEscape handles UTF-8 properly, but we need to replace + with %20 for RFC 5987
	encodedUTF8 := url.QueryEscape(filename)
	encodedUTF8 = strings.ReplaceAll(encodedUTF8, "+", "%20")

	// Build the header value with both ASCII fallback and UTF-8 encoded version
	// Modern browsers will use filename* if available, older ones will use filename
	return `attachment; filename="` + escapedASCII + `"; filename*=UTF-8''` + encodedUTF8
}

// HealthCheck handles health check endpoint
// @Summary      Health check
// @Description  Check if the service is running
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /health [get]
func (h *Handlers) HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "ok",
	})
}
