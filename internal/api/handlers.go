package api

import (
	"html/template"
	"io"
	"net/url"
	"os"
	"path/filepath"
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
	Password    string                   `json:"password,omitempty" form:"password"`
	ExpiresIn   timeparser.UniversalTime `json:"expires_in" form:"expires_in"`
	BlurEnabled bool                     `json:"blur_enabled" form:"blur_enabled"`
}

type UploadResponse struct {
	ResourceKey string                   `json:"resource_key"`
	URL         string                   `json:"url"`
	ExpiresIn   timeparser.UniversalTime `json:"expires_in"`
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
		return h.renderError(c, "Файл не указан в форме")
	}

	// Parse form data
	var req UploadRequest
	req.Password = c.FormValue("password")
	expiresInStr := c.FormValue("expires_in")
	blurEnabledStr := c.FormValue("blur_enabled")
	req.BlurEnabled = blurEnabledStr == "true"

	// Parse expires_in using universal time parser
	if expiresInStr != "" {
		if err := req.ExpiresIn.UnmarshalText([]byte(expiresInStr)); err != nil {
			// Return HTML error for HTMX
			if c.Get("HX-Request") == "true" {
				return h.renderResult(c, false, "", "Неверный формат времени: "+err.Error(), timeparser.UniversalTime{})
			}
			return h.renderError(c, "Неверный формат времени: "+err.Error())
		}

		// Проверяем, что время в будущем
		if !req.ExpiresIn.IsZero() && req.ExpiresIn.Time.Before(time.Now().UTC()) {
			// Return HTML error for HTMX
			if c.Get("HX-Request") == "true" {
				return h.renderResult(c, false, "", "Время истечения должно быть в будущем", timeparser.UniversalTime{})
			}
			return h.renderError(c, "Время истечения должно быть в будущем")
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
		Data:        src,
		Password:    req.Password,
		ExpiresAt:   req.ExpiresIn,
		Filename:    file.Filename,
		BlurEnabled: req.BlurEnabled,
	}

	resp, err := h.mediaService.UploadMedia(c.Context(), uploadReq)
	if err != nil {
		h.logger.Error("failed to upload media", zap.Error(err))
		// Return HTML error for HTMX
		if c.Get("HX-Request") == "true" {
			return h.renderResult(c, false, "", "Не удалось загрузить файл. Попробуйте еще раз.", timeparser.UniversalTime{})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to upload media",
		})
	}

	// Check if request is from HTMX
	if c.Get("HX-Request") == "true" {
		// Build full URL using request headers (works with reverse proxies and local dev)
		// Prefer X-Forwarded headers (from reverse proxy), fallback to Host header
		proto := c.Get("X-Forwarded-Proto")
		if proto == "" {
			// Check if request was HTTPS
			if c.Protocol() == "https" || c.Get("X-Forwarded-Ssl") == "on" {
				proto = "https"
			} else {
				proto = "http"
			}
		}

		// Use X-Forwarded-Host if available (from reverse proxy)
		// Otherwise use Host header which already contains correct host:port
		host := c.Get("X-Forwarded-Host")
		if host == "" {
			// Host header already contains the correct host:port combination
			// that the client used to connect (works with any port, proxy, tunnel, etc.)
			host = c.Get("Host")
			if host == "" {
				// Fallback to hostname if Host header is missing
				host = c.Hostname()
			}
		}

		fullURL := proto + "://" + host + resp.URL

		return h.renderResult(c, true, fullURL, "", req.ExpiresIn)
	}

	return c.JSON(UploadResponse{
		ResourceKey: resp.ResourceKey,
		URL:         resp.URL,
		ExpiresIn:   req.ExpiresIn,
	})
}

type DownloadRequest struct {
	Password string `json:"password,omitempty"`
}

// getResourceKeyAndEncryptionKey extracts resource key and encryption key from request
// Supports formats:
// - /media/resourceKey#encKey
// - /media/resourceKey?password=xxx#encKey (fragment from full URL)
// - /media/resourceKey?enc_key=xxx (fallback if fragment not available)
func getResourceKeyAndEncryptionKey(c *fiber.Ctx) (resourceKey string, encKeyBase64 string, err error) {
	resourceKeyEncoded := c.Params("key")
	if resourceKeyEncoded == "" {
		return "", "", fiber.NewError(fiber.StatusBadRequest, "Resource key is required")
	}

	// Decode URL-encoded key
	resourceKey, err = url.PathUnescape(resourceKeyEncoded)
	if err != nil {
		resourceKey, err = url.QueryUnescape(resourceKeyEncoded)
		if err != nil {
			return "", "", fiber.NewError(fiber.StatusBadRequest, "Invalid resource key encoding")
		}
	}

	// Try to get encryption key from multiple sources:
	// 1. From fragment in the resourceKey itself (format: resourceKey#encKey)
	parts := strings.Split(resourceKey, "#")
	if len(parts) > 1 {
		resourceKey = parts[0]
		encKeyBase64 = parts[1]
		return resourceKey, encKeyBase64, nil
	}

	// 2. From query parameter (if fragment was passed as query param by JavaScript)
	encKeyBase64 = c.Query("enc_key", "")
	if encKeyBase64 != "" {
		return resourceKey, encKeyBase64, nil
	}

	// 3. Try to extract from Referer header if available (for browser requests with fragment)
	referer := c.Get("Referer")
	if referer != "" {
		if parsedURL, parseErr := url.Parse(referer); parseErr == nil {
			if parsedURL.Fragment != "" {
				encKeyBase64 = parsedURL.Fragment
				return resourceKey, encKeyBase64, nil
			}
		}
	}

	// 4. Try to get from full URL if available (for direct requests)
	fullURL := c.OriginalURL()
	if fullURL != "" {
		if parsedURL, parseErr := url.Parse(fullURL); parseErr == nil {
			if parsedURL.Fragment != "" {
				encKeyBase64 = parsedURL.Fragment
				return resourceKey, encKeyBase64, nil
			}
		}
	}

	// Return resourceKey without encryption key (will be handled by service layer)
	return resourceKey, encKeyBase64, nil
}

// ViewMedia handles media view page (HTML with preview and download button)
func (h *Handlers) ViewMedia(c *fiber.Ctx) error {
	resourceKey, encKeyBase64, err := getResourceKeyAndEncryptionKey(c)
	if err != nil {
		return err
	}

	// Get password from query
	password := c.Query("password", "")

	// Check if password is required (without verifying it yet)
	accessInfo, err := h.accessService.CheckResourceAccess(c.Context(), resourceKey)
	if err != nil {
		switch err {
		case accessservice.ErrNotFound:
			return h.renderError(c, "Ресурс не найден")
		case accessservice.ErrExpired:
			return h.renderError(c, "Ресурс истек")
		case accessservice.ErrAlreadyViewed:
			return h.renderAlreadyViewed(c)
		default:
			return h.renderError(c, "Ошибка при проверке доступа")
		}
	}

	// If password is required but not provided, show password modal
	passwordRequired := accessInfo.PasswordHash != nil && *accessInfo.PasswordHash != ""
	if passwordRequired && password == "" {
		// Show page with password modal
		return h.renderViewPageWithPasswordModal(c, resourceKey, resourceKey)
	}

	// Verify access with password
	err = h.accessService.VerifyAccess(c.Context(), resourceKey, password)
	if err != nil {
		switch err {
		case accessservice.ErrNotFound:
			return h.renderError(c, "Ресурс не найден")
		case accessservice.ErrExpired:
			return h.renderError(c, "Ресурс истек")
		case accessservice.ErrAlreadyViewed:
			return h.renderAlreadyViewed(c)
		case accessservice.ErrPasswordRequired, accessservice.ErrInvalidPassword:
			// Show page with password modal and error
			return h.renderViewPageWithPasswordModal(c, resourceKey, resourceKey, "Неверный пароль")
		default:
			return h.renderError(c, "Ошибка при проверке доступа")
		}
	}

	// Get media info
	mediaInfo, err := h.mediaService.GetMediaInfo(c.Context(), resourceKey)
	if err != nil {
		if err == mediaservice.ErrNotFound {
			return h.renderError(c, "Ресурс не найден")
		}
		return h.renderError(c, "Ошибка при получении информации о ресурсе")
	}

	// Log blur enabled for debugging
	h.logger.Info("Media info retrieved", zap.Bool("blur_enabled", mediaInfo.BlurEnabled), zap.String("resource_key", resourceKey))

	// Build download URL with encryption key as query param
	downloadURL := "/media/" + url.QueryEscape(resourceKey) + "/download"
	queryParams := []string{}
	if password != "" {
		queryParams = append(queryParams, "password="+url.QueryEscape(password))
	}
	if encKeyBase64 != "" {
		queryParams = append(queryParams, "enc_key="+url.QueryEscape(encKeyBase64))
	}
	if len(queryParams) > 0 {
		downloadURL += "?" + strings.Join(queryParams, "&")
	}

	// Build preview URL for images (with enc_key as query param, not fragment)
	previewURL := ""
	if mediaInfo.IsImage {
		previewURL = "/media/" + url.QueryEscape(resourceKey) + "/preview"
		queryParams := []string{}
		if password != "" {
			queryParams = append(queryParams, "password="+url.QueryEscape(password))
		}
		if encKeyBase64 != "" {
			queryParams = append(queryParams, "enc_key="+url.QueryEscape(encKeyBase64))
		}
		if len(queryParams) > 0 {
			previewURL += "?" + strings.Join(queryParams, "&")
		}
	}

	// Build filename for display
	displayFilename := "file"
	if mediaInfo.Filename != nil && *mediaInfo.Filename != "" {
		displayFilename = *mediaInfo.Filename
		if mediaInfo.FileExtension != nil && *mediaInfo.FileExtension != "" {
			displayFilename += "." + *mediaInfo.FileExtension
		}
	} else if mediaInfo.FileExtension != nil && *mediaInfo.FileExtension != "" {
		displayFilename = "file." + *mediaInfo.FileExtension
	}

	// Render view page
	return h.renderViewPage(c, mediaInfo, displayFilename, downloadURL, previewURL, false, "")
}

// renderViewPageWithPasswordModal renders the view page with password modal
func (h *Handlers) renderViewPageWithPasswordModal(c *fiber.Ctx, resourceKey, resourceKeyForCheck string, errorMsg ...string) error {
	// Get media info (without password check, just to get file info)
	// Note: GetMediaInfo uses GetMediaResourceByKey which checks viewed=false, so it might fail
	// We'll try to get basic info, but if it fails, we'll still show the modal
	mediaInfo, err := h.mediaService.GetMediaInfo(c.Context(), resourceKeyForCheck)

	// Build filename for display (use default if we can't get info)
	displayFilename := "file"
	if err == nil && mediaInfo != nil {
		if mediaInfo.Filename != nil && *mediaInfo.Filename != "" {
			displayFilename = *mediaInfo.Filename
			if mediaInfo.FileExtension != nil && *mediaInfo.FileExtension != "" {
				displayFilename += "." + *mediaInfo.FileExtension
			}
		} else if mediaInfo.FileExtension != nil && *mediaInfo.FileExtension != "" {
			displayFilename = "file." + *mediaInfo.FileExtension
		}
	}

	errorMessage := ""
	if len(errorMsg) > 0 && errorMsg[0] != "" {
		errorMessage = errorMsg[0]
	}

	// Create a minimal MediaInfo if we couldn't get it
	if mediaInfo == nil {
		mediaInfo = &mediaservice.MediaInfo{
			Filename:      nil,
			FileExtension: nil,
			IsImage:       false,
		}
	}

	// Render view page with password modal
	return h.renderViewPage(c, mediaInfo, displayFilename, "", "", true, errorMessage)
}

// DownloadMediaFile handles media download (one-time view) - direct file download
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
// @Router       /media/{key}/download [get]
func (h *Handlers) DownloadMediaFile(c *fiber.Ctx) error {
	resourceKey, encKeyBase64, err := getResourceKeyAndEncryptionKey(c)
	if err != nil {
		return err
	}

	var req DownloadRequest
	req.Password = c.Query("password", "")

	// Verify access (using only resource key, not encryption key)
	err = h.accessService.VerifyAccess(c.Context(), resourceKey, req.Password)
	if err != nil {
		switch err {
		case accessservice.ErrNotFound:
			return h.renderError(c, "Ресурс не найден")
		case accessservice.ErrExpired:
			return h.renderError(c, "Ресурс истек")
		case accessservice.ErrAlreadyViewed:
			return h.renderAlreadyViewed(c)
		case accessservice.ErrPasswordRequired, accessservice.ErrInvalidPassword:
			return h.renderError(c, "Неверный или отсутствующий пароль")
		default:
			return h.renderError(c, "Ошибка при проверке доступа")
		}
	}

	// Download media (this will mark as viewed and delete)
	downloadReq := mediaservice.DownloadRequest{
		ResourceKey:  resourceKey,
		Password:     req.Password,
		EncKeyBase64: encKeyBase64,
	}

	// Log for debugging
	if encKeyBase64 == "" {
		h.logger.Warn("encryption key is empty for download", zap.String("resource_key", resourceKey))
	}

	resp, err := h.mediaService.DownloadMedia(c.Context(), &downloadReq)
	if err != nil {
		h.logger.Error("failed to download media", zap.Error(err))

		// Handle specific errors
		switch err {
		case mediaservice.ErrNotFound:
			return h.renderError(c, "Ресурс не найден")
		case mediaservice.ErrAlreadyViewed:
			return h.renderAlreadyViewed(c)
		case mediaservice.ErrMissingEncryptionKey, mediaservice.ErrInvalidEncryptionKey:
			return h.renderError(c, "Неверный или отсутствующий ключ шифрования в URL")
		case mediaservice.ErrDecryptionFailed:
			return h.renderError(c, "Ошибка расшифровки - неверный пароль или поврежденные данные")
		default:
			return h.renderError(c, "Ошибка при загрузке медиа")
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
		downloadFilename = resourceKey
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to stream response",
		})
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

// IndexPage handles the main page
func (h *Handlers) IndexPage(c *fiber.Ctx) error {

	if _, err := os.Stat(filepath.Join(frontendDir, "index.html")); err == nil {
		return c.SendFile(filepath.Join(frontendDir, "index.html"))
	}

	return c.Status(fiber.StatusInternalServerError).SendString("Frontend file not found. Please ensure you're running from the project root.")
}

// renderError renders the error template
func (h *Handlers) renderError(c *fiber.Ctx, errorMsg string) error {
	var tmpl *template.Template
	var err error

	if _, statErr := os.Stat(filepath.Join(frontendDir, "error.html")); statErr == nil {
		tmpl, err = template.ParseFiles(filepath.Join(frontendDir, "error.html"))
		if err != nil {
			h.logger.Error("failed to parse error template", zap.Error(err))
			return c.Status(fiber.StatusBadRequest).SendString("Template execution error")
		}
	}

	if tmpl == nil {
		h.logger.Error("failed to parse error template", zap.Error(err), zap.String("frontend_dir", frontendDir))
		return c.Status(fiber.StatusBadRequest).SendString("Template error")
	}

	data := struct {
		Error string
	}{
		Error: errorMsg,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		h.logger.Error("failed to execute error template", zap.Error(err))
		return c.Status(fiber.StatusBadRequest).SendString("Template execution error")
	}

	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.Status(fiber.StatusBadRequest).SendString(buf.String())
}

// renderAlreadyViewed renders the already viewed template
func (h *Handlers) renderAlreadyViewed(c *fiber.Ctx) error {
	templatePath := filepath.Join(frontendDir, "already-viewed.html")

	var tmpl *template.Template
	var err error

	if _, statErr := os.Stat(templatePath); statErr == nil {
		tmpl, err = template.ParseFiles(templatePath)
		if err != nil {
			h.logger.Error("failed to parse already-viewed template", zap.Error(err), zap.String("path", templatePath))
			return c.Status(fiber.StatusGone).SendString("Template execution error")
		}
	} else {
		h.logger.Error("already-viewed template file not found", zap.Error(statErr), zap.String("path", templatePath), zap.String("frontend_dir", frontendDir))
		return c.Status(fiber.StatusGone).SendString("Template file not found")
	}

	if tmpl == nil {
		h.logger.Error("failed to parse already-viewed template", zap.Error(err), zap.String("frontend_dir", frontendDir), zap.String("path", templatePath))
		return c.Status(fiber.StatusGone).SendString("Template error")
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, nil); err != nil {
		h.logger.Error("failed to execute already-viewed template", zap.Error(err))
		return c.Status(fiber.StatusGone).SendString("Template execution error")
	}

	c.Set("Content-Type", "text/html; charset=utf-8")
	return c.Status(fiber.StatusGone).SendString(buf.String())
}

// renderResult renders the result template for HTMX
func (h *Handlers) renderResult(c *fiber.Ctx, success bool, url, errorMsg string, expiresIn timeparser.UniversalTime) error {
	var tmpl *template.Template
	var err error

	if _, statErr := os.Stat(filepath.Join(frontendDir, "result.html")); statErr == nil {
		tmpl, err = template.ParseFiles(filepath.Join(frontendDir, "result.html"))
		if err != nil {
			h.logger.Error("failed to parse result template", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).SendString("Template execution error")
		}
	}

	data := struct {
		Success   bool
		URL       string
		Error     string
		ExpiresIn timeparser.UniversalTime
	}{
		Success:   success,
		URL:       url,
		Error:     errorMsg,
		ExpiresIn: expiresIn,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		h.logger.Error("failed to execute result template", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).SendString("Template execution error")
	}

	c.Set("Content-Type", "text/html")
	return c.SendString(buf.String())
}

// PreviewMedia handles media preview (for images, doesn't delete file)
func (h *Handlers) PreviewMedia(c *fiber.Ctx) error {
	resourceKey, encKeyBase64, err := getResourceKeyAndEncryptionKey(c)
	if err != nil {
		return err
	}

	var req DownloadRequest
	req.Password = c.Query("password", "")

	// Verify access
	err = h.accessService.VerifyAccess(c.Context(), resourceKey, req.Password)
	if err != nil {
		switch err {
		case accessservice.ErrNotFound:
			return h.renderError(c, "Ресурс не найден")
		case accessservice.ErrExpired:
			return h.renderError(c, "Ресурс истек")
		case accessservice.ErrAlreadyViewed:
			return h.renderAlreadyViewed(c)
		case accessservice.ErrPasswordRequired, accessservice.ErrInvalidPassword:
			return h.renderError(c, "Неверный или отсутствующий пароль")
		default:
			return h.renderError(c, "Ошибка при проверке доступа")
		}
	}

	// Get preview (doesn't mark as viewed or delete)
	previewReq := mediaservice.DownloadRequest{
		ResourceKey:  resourceKey,
		Password:     req.Password,
		EncKeyBase64: encKeyBase64,
	}

	// Log for debugging
	if encKeyBase64 == "" {
		h.logger.Warn("encryption key is empty for preview", zap.String("resource_key", resourceKey))
	}

	resp, err := h.mediaService.GetMediaPreview(c.Context(), &previewReq)
	if err != nil {
		h.logger.Error("failed to get media preview", zap.Error(err))
		switch err {
		case mediaservice.ErrNotFound:
			return h.renderError(c, "Ресурс не найден")
		case mediaservice.ErrAlreadyViewed:
			return h.renderAlreadyViewed(c)
		case mediaservice.ErrMissingEncryptionKey, mediaservice.ErrInvalidEncryptionKey:
			return h.renderError(c, "Неверный или отсутствующий ключ шифрования в URL")
		case mediaservice.ErrDecryptionFailed:
			return h.renderError(c, "Ошибка расшифровки - неверный пароль или поврежденные данные")
		default:
			return h.renderError(c, "Ошибка при получении превью")
		}
	}
	defer resp.Data.Close()

	// Determine content type based on extension
	contentType := "application/octet-stream"
	if resp.FileExtension != nil {
		ext := strings.ToLower(*resp.FileExtension)
		switch ext {
		case "jpg", "jpeg":
			contentType = "image/jpeg"
		case "png":
			contentType = "image/png"
		case "gif":
			contentType = "image/gif"
		case "webp":
			contentType = "image/webp"
		case "bmp":
			contentType = "image/bmp"
		case "svg":
			contentType = "image/svg+xml"
		case "ico":
			contentType = "image/x-icon"
		}
	}

	c.Set("Content-Type", contentType)
	c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Set("Pragma", "no-cache")
	c.Set("Expires", "0")

	_, err = io.Copy(c.Response().BodyWriter(), resp.Data)
	if err != nil {
		h.logger.Error("failed to stream preview", zap.Error(err))
		return err
	}

	return nil
}

// renderViewPage renders the view page template
func (h *Handlers) renderViewPage(c *fiber.Ctx, mediaInfo *mediaservice.MediaInfo, displayFilename, downloadURL, previewURL string, showPasswordModal bool, passwordError string) error {

	var tmpl *template.Template
	var err error
	if _, err := os.Stat(filepath.Join(frontendDir, "view.html")); err == nil {
		tmpl, err = template.ParseFiles(filepath.Join(frontendDir, "view.html"))
		if err != nil {
			h.logger.Error("failed to parse view template", zap.Error(err))
			return c.Status(fiber.StatusInternalServerError).SendString("Template execution error")
		}
	}

	if tmpl == nil {
		h.logger.Error("failed to parse view template", zap.Error(err), zap.String("frontend_dir", frontendDir))
		return c.Status(fiber.StatusInternalServerError).SendString("Template error")
	}

	data := struct {
		Filename          string
		IsImage           bool
		DownloadURL       string
		PreviewURL        string
		ShowPasswordModal bool
		PasswordError     string
		ResourceKey       string
		BlurEnabled       bool
	}{
		Filename:          displayFilename,
		IsImage:           mediaInfo.IsImage,
		DownloadURL:       downloadURL,
		PreviewURL:        previewURL,
		ShowPasswordModal: showPasswordModal,
		PasswordError:     passwordError,
		ResourceKey:       c.Params("key"),
		BlurEnabled:       mediaInfo.BlurEnabled,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		h.logger.Error("failed to execute view template", zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).SendString("Template execution error")
	}

	c.Set("Content-Type", "text/html")
	return c.SendString(buf.String())
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
