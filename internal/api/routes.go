package api

import (
	"github.com/gofiber/fiber/v2"
)

func SetupRoutes(app *fiber.App, handlers *Handlers) {
	app.Get("/health", handlers.HealthCheck)
	app.Post("/upload", handlers.UploadMedia)
	app.Get("/media/:key", handlers.DownloadMedia)
	app.Post("/media/:key", handlers.DownloadMedia) // Support POST for password
}

