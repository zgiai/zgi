package dataplane

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"plugin_runner/internal/config"
)

func initDatabase(cfg *config.Config, log *zap.Logger) (*gorm.DB, *sql.DB, error) {
	if err := ensureDatabaseExists(cfg); err != nil {
		return nil, nil, err
	}

	dialector, err := buildDialector(cfg)
	if err != nil {
		return nil, nil, err
	}

	ormDB, err := gorm.Open(dialector, &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := ormDB.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("extract database handle: %w", err)
	}
	configurePool(cfg, sqlDB)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("ping database: %w", err)
	}

	log.Info("data-plane database connected",
		zap.String("driver", strings.ToLower(cfg.DBDriver)),
		zap.String("host", cfg.DBHost),
		zap.Int("port", cfg.DBPort),
		zap.String("database", cfg.DBName),
		zap.Int("max_open_conns", cfg.DBMaxOpenConns),
		zap.Int("max_idle_conns", cfg.DBMaxIdleConns),
	)
	return ormDB, sqlDB, nil
}

func ensureDatabaseExists(cfg *config.Config) error {
	switch strings.ToLower(cfg.DBDriver) {
	case "postgres", "postgresql":
		return ensurePostgresDatabaseExists(cfg)
	default:
		return nil
	}
}

func buildDialector(cfg *config.Config) (gorm.Dialector, error) {
	driver := strings.ToLower(cfg.DBDriver)
	switch driver {
	case "postgres", "postgresql":
		return postgres.Open(postgresDSN(cfg)), nil
	case "mysql":
		return mysql.Open(mysqlDSN(cfg)), nil
	default:
		return nil, fmt.Errorf("unsupported db driver %q", cfg.DBDriver)
	}
}

func postgresDSN(cfg *config.Config) string {
	return postgresDSNWithDB(cfg, cfg.DBName)
}

func postgresDSNWithDB(cfg *config.Config, dbName string) string {
	base := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, dbName, cfg.DBSSLMode,
	)
	if opts := strings.TrimSpace(cfg.DBOptions); opts != "" {
		base = fmt.Sprintf("%s %s", base, opts)
	}
	return base
}

func ensurePostgresDatabaseExists(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	adminDB, err := sql.Open("pgx", postgresDSNWithDB(cfg, "postgres"))
	if err != nil {
		return fmt.Errorf("connect maintenance database: %w", err)
	}
	defer adminDB.Close()

	if err := adminDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping maintenance database: %w", err)
	}

	var exists bool
	if err := adminDB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", cfg.DBName).Scan(&exists); err != nil {
		return fmt.Errorf("check database existence: %w", err)
	}
	if exists {
		return nil
	}

	createQuery := fmt.Sprintf(`CREATE DATABASE "%s"`, strings.ReplaceAll(cfg.DBName, `"`, `""`))
	if _, err := adminDB.ExecContext(ctx, createQuery); err != nil {
		return fmt.Errorf("create database %q: %w", cfg.DBName, err)
	}
	return nil
}

func mysqlDSN(cfg *config.Config) string {
	base := fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=Local",
		cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName,
	)
	if opts := strings.TrimSpace(cfg.DBOptions); opts != "" {
		opts = strings.TrimPrefix(opts, "?")
		if strings.HasPrefix(opts, "&") {
			base += opts
		} else {
			base += "&" + opts
		}
	}
	return base
}

func configurePool(cfg *config.Config, sqlDB *sql.DB) {
	if cfg.DBMaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	}
	if cfg.DBMaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	}
	if cfg.DBConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	}
	if cfg.DBConnMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)
	}
}
