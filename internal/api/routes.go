package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/swaggo/fiber-swagger"
	_ "lovebin/docs" // swagger docs
)

func SetupRoutes(app *fiber.App, handlers *Handlers) {
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
	app.Get("/media/:key", handlers.DownloadMedia)
}
