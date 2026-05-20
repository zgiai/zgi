package driver

import (
	"context"
	"fmt"
	"net/url"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// Config holds connection settings required to initialise the shared pgx pool.
type Config struct {
	DBHost string `mapstructure:"db_host"`
	DBPort string `mapstructure:"db_port"`
	DBUser string `mapstructure:"db_user"`
	DBPass string `mapstructure:"db_password"`
	DBName string `mapstructure:"db_name"`
}

// Pool wraps pgxpool.Pool so upper layers can depend on a small surface area.
type Pool struct {
	raw *pgxpool.Pool
}

// NewPool builds a new connection pool. Call Close when the application shuts down.
func NewPool(ctx context.Context, cfg Config) (*Pool, error) {
	// URL encode username and password to handle special characters
	encodedUser := url.QueryEscape(cfg.DBUser)
	encodedPass := url.QueryEscape(cfg.DBPass)

	dsn := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s",
		encodedUser, encodedPass, cfg.DBHost, cfg.DBPort, cfg.DBName)

	//poolCfg, err := pgxpool.ParseConfig(dsn)
	//if err != nil {
	//	return nil, fmt.Errorf("sqlmeta: parse dsn: %w", err)
	//}
	//
	//raw, err := pgxpool.NewWithConfig(ctx, poolCfg)
	//if err != nil {
	//	return nil, fmt.Errorf("sqlmeta: new pool: %w", err)
	//}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	logger.InfoContext(ctx, "sqlmeta PostgreSQL connected")

	return &Pool{raw: pool}, nil
}

// Close frees underlying resources. Safe to call multiple times.
func (p *Pool) Close() {
	if p == nil || p.raw == nil {
		return
	}
	p.raw.Close()
}

// Raw exposes the underlying pgx pool for lower-level operations when needed.
func (p *Pool) Raw() *pgxpool.Pool {
	if p == nil {
		return nil
	}
	return p.raw
}
