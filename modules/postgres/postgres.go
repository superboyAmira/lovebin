package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres interface for dependency injection
type Postgres interface {
	GetPool() *pgxpool.Pool
	Close()
}

type postgresImpl struct {
	pool *pgxpool.Pool
}

func (p *postgresImpl) GetPool() *pgxpool.Pool {
	return p.pool
}

func (p *postgresImpl) Close() {
	if p.pool != nil {
		p.pool.Close()
	}
}

// Config holds PostgreSQL configuration
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// Init initializes the PostgreSQL module
func Init(ctx context.Context, cfg Config) (Postgres, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &postgresImpl{pool: pool}, nil
}
