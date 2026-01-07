package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "lovebin/docs" // swagger docs

	"lovebin/internal/app"
	"lovebin/modules/encryption"
	"lovebin/modules/logger"
	"lovebin/modules/postgres"
	"lovebin/modules/s3"
)

// @title           LoveBin API
// @version         1.0
// @description     Сервис обмена фотографиями и видео с шифрованием на стороне клиента
// @termsOfService  http://swagger.io/terms/

// @host      localhost:8080
// @BasePath  /

// @schemes   http https
func main() {
	ctx := context.Background()

	// Load configuration from environment
	cfg := app.Config{
		Logger: logger.Config{
			Level: getEnv("LOG_LEVEL", "info"),
		},
		Postgres: postgres.Config{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			User:     getEnv("POSTGRES_USER", "postgres"),
			Password: getEnv("POSTGRES_PASSWORD", "postgres"),
			DBName:   getEnv("POSTGRES_DB", "lovebin"),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		S3: s3.Config{
			Region:          getEnv("S3_REGION", "us-east-1"),
			Bucket:          getEnv("S3_BUCKET", "lovebin-media"),
			Endpoint:        getEnv("S3_ENDPOINT", ""),
			AccessKeyID:     getEnv("S3_ACCESS_KEY_ID", ""),
			SecretAccessKey: getEnv("S3_SECRET_ACCESS_KEY", ""),
		},
		Encryption: encryption.Config{
			Iterations: 100000,
		},
		Server: app.ServerConfig{
			Port: getEnv("SERVER_PORT", "8080"),
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
		},
	}

	// Initialize application
	application, err := app.New(ctx, cfg)
	if err != nil {
		panic(err)
	}

	// Start server in goroutine
	serverAddr := cfg.Server.Host + ":" + cfg.Server.Port
	go func() {
		if err := application.Start(serverAddr); err != nil {
			panic(err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		panic(err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
