package api

import (
	"os"
	"path/filepath"
	"time"

	_ "lovebin/docs" // swagger docs

	"lovebin/modules/logger"

	"github.com/gofiber/fiber/v2"
	fiberSwagger "github.com/swaggo/fiber-swagger"
	"go.uber.org/zap"
)

var (
	frontendDir string = findFrontendDir()
)

// findFrontendDir tries to find the frontend directory
func findFrontendDir() string {
	paths := []string{}

	// Also try relative to working directory
	if wd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(wd, "frontend"))
		// Try going up one level if we're in cmd/lovebin
		if filepath.Base(wd) == "lovebin" {
			paths = append(paths, filepath.Join(filepath.Dir(wd), "frontend"))
		}
		// Try going up from cmd/lovebin
		if filepath.Base(wd) == "lovebin" && filepath.Base(filepath.Dir(wd)) == "cmd" {
			projectRoot := filepath.Dir(filepath.Dir(wd))
			paths = append(paths, filepath.Join(projectRoot, "frontend"))
		}
	}

	// Add common Docker/container paths
	paths = append(paths, "/app/frontend", "./frontend", "frontend")

	for _, path := range paths {
		// Clean the path and check if it exists
		cleanPath := filepath.Clean(path)
		if info, err := os.Stat(cleanPath); err == nil && info.IsDir() {
			// Return absolute path if possible
			if absPath, err := filepath.Abs(cleanPath); err == nil {
				return absPath
			}
			return cleanPath
		}
	}

	return "./frontend" // fallback
}

func SetupRoutes(app *fiber.App, handlers *Handlers, log logger.Logger) {
	log.Info("Setting up static files", zap.String("frontend_dir", frontendDir))

	// Verify the directory exists and contains logo.jpg
	logoPath := filepath.Join(frontendDir, "logo.jpg")
	// Clean and get absolute path for reliable checking
	logoPath = filepath.Clean(logoPath)
	if absLogoPath, err := filepath.Abs(logoPath); err == nil {
		logoPath = absLogoPath
	}

	if _, err := os.Stat(logoPath); err == nil {
		log.Info("Logo file found", zap.String("path", logoPath))
	} else {
		log.Warn("Logo file not found", zap.String("path", logoPath), zap.Error(err))
	}

	// Setup static files with explicit path handling
	app.Static("/static", frontendDir, fiber.Static{
		Index:         "",
		Browse:        false,
		Download:      false,
		CacheDuration: 24 * time.Hour,
	})

	// Also add explicit route for logo as fallback
	app.Get("/static/logo.jpg", func(c *fiber.Ctx) error {
		logoFile := filepath.Join(frontendDir, "logo.jpg")
		logoFile = filepath.Clean(logoFile)
		// Try to get absolute path
		if absLogoFile, err := filepath.Abs(logoFile); err == nil {
			logoFile = absLogoFile
		}

		if _, err := os.Stat(logoFile); err == nil {
			c.Set("Content-Type", "image/jpeg")
			return c.SendFile(logoFile)
		}
		// Try alternative paths
		altPaths := []string{
			filepath.Join(frontendDir, "logo.jpg"),
			"./frontend/logo.jpg",
			"frontend/logo.jpg",
		}
		for _, altPath := range altPaths {
			if absAltPath, err := filepath.Abs(altPath); err == nil {
				if _, err := os.Stat(absAltPath); err == nil {
					c.Set("Content-Type", "image/jpeg")
					return c.SendFile(absAltPath)
				}
			}
		}
		return c.Status(fiber.StatusNotFound).SendString("Logo not found")
	})

	// Main page
	app.Get("/", handlers.IndexPage)

	// Swagger routes
	app.Get("/swagger/*", fiberSwagger.WrapHandler)

	// Swagger JSON endpoint
	app.Get("/swagger.json", func(c *fiber.Ctx) error {
		c.Set("Content-Type", "application/json")
		return c.SendFile("./docs/swagger.json")
	})

	// API routes
	app.Get("/health", handlers.HealthCheck)
	app.Post("/upload", handlers.UploadMedia)
	app.Get("/media/:key", handlers.ViewMedia)                  // View page with preview
	app.Get("/media/:key/preview", handlers.PreviewMedia)       // Image preview (doesn't delete)
	app.Get("/media/:key/download", handlers.DownloadMediaFile) // Direct download
}
