package app

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"

	"lovebin/internal/api"
	accessservice "lovebin/internal/services/access-service"
	accessrepo "lovebin/internal/services/access-service/repository"
	mediaservice "lovebin/internal/services/media-service"
	mediarepo "lovebin/internal/services/media-service/repository"
	"lovebin/modules/encryption"
	"lovebin/modules/logger"
	"lovebin/modules/postgres"
	"lovebin/modules/s3"
)

type Config struct {
	Logger     logger.Config
	Postgres   postgres.Config
	S3         s3.Config
	Encryption encryption.Config
	Server     ServerConfig
}

type ServerConfig struct {
	Port string
	Host string
}

type App struct {
	logger        logger.Logger
	postgres      postgres.Postgres
	s3            s3.S3
	encryption    encryption.Encryption
	mediaService  *mediaservice.Service
	accessService *accessservice.Service
	handlers      *api.Handlers
	server        *fiber.App
}

func New(ctx context.Context, cfg Config) (*App, error) {
	// Initialize logger
	log, err := logger.Init(logger.Config{Level: cfg.Logger.Level})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Initialize PostgreSQL
	pg, err := postgres.Init(ctx, cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize postgres: %w", err)
	}

	// Initialize S3
	s3Client, err := s3.Init(ctx, cfg.S3)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize s3: %w", err)
	}

	// Initialize encryption
	enc := encryption.Init(cfg.Encryption)

	// Initialize repositories
	mediaRepo := mediarepo.NewMediaRepository(pg.GetPool())
	accessRepo := accessrepo.NewAccessRepository(pg.GetPool())

	// Initialize services
	mediaSvc := mediaservice.NewService(log, pg, s3Client, enc, mediaRepo)
	accessSvc := accessservice.NewService(log, pg, accessRepo)

	// Initialize handlers
	handlers := api.NewHandlers(log, mediaSvc, accessSvc)

	// Initialize Fiber
	server := fiber.New(fiber.Config{
		AppName:      "LoveBin",
		BodyLimit:    100 * 1024 * 1024, // 100MB limit
		ReadTimeout:  time.Second * 30,
		WriteTimeout: time.Second * 30,
	})

	// Middleware
	server.Use(recover.New())
	server.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowCredentials: true,
		ExposeHeaders:    "Content-Length",
		MaxAge:           3600,
	}))
	server.Use(func(c *fiber.Ctx) error {
		statusCode := c.Response().StatusCode()
		log.Info("Request", zap.String("method", c.Method()), zap.String("path", c.Path()), zap.Int("status", statusCode))
		return c.Next()
	})

	// Setup routes
	api.SetupRoutes(server, handlers)

	return &App{
		logger:        log,
		postgres:      pg,
		s3:            s3Client,
		encryption:    enc,
		mediaService:  mediaSvc,
		accessService: accessSvc,
		handlers:      handlers,
		server:        server,
	}, nil
}

func (a *App) Start(addr string) error {
	return a.server.Listen(addr)
}

func (a *App) Shutdown(ctx context.Context) error {
	if err := a.server.Shutdown(); err != nil {
		return err
	}
	a.postgres.Close()
	a.logger.Sync()
	return nil
}
