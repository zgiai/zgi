package database

import (
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/observability"
	pkglogger "github.com/zgiai/ginext/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(cfg config.DatabaseConfig) (*gorm.DB, error) {
	driver := strings.TrimSpace(cfg.Driver)
	if driver == "" {
		driver = "postgres"
	}
	if !strings.EqualFold(driver, "postgres") {
		return nil, fmt.Errorf("unsupported database driver %q", cfg.Driver)
	}

	logLevel := gormlogger.Warn
	if cfg.DebugSQL {
		logLevel = gormlogger.Info
	}

	newLogger := gormlogger.New(
		pkglogger.NewStdLogger("app"),
		gormlogger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
			ParameterizedQueries:      true,
		},
	)

	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s timezone=%s",
		cfg.Host,
		cfg.Username,
		cfg.Password,
		cfg.DBName,
		cfg.Port,
		cfg.SSLMode,
		cfg.Timezone,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: newLogger, // Use custom logger with Info level for debugging
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if observability.DBEnabled() {
		if err := observability.InstrumentGORM(db, driver, cfg.DBName); err != nil {
			pkglogger.Warn("failed to register opentelemetry database tracing", "error", err)
		} else {
			pkglogger.Info("opentelemetry database tracing registered successfully")
		}
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Set connection pool
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	// Check if we can connect to the database
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Register Sentry plugin for error reporting only when DSN is configured.
	if config.Current().Sentry.DSN != "" {
		sentryPlugin := &SentryPlugin{
			SlowQueryThreshold: 1000 * time.Millisecond, // 1 second
		}
		if err := db.Use(sentryPlugin); err != nil {
			pkglogger.Warn("failed to register sentry plugin", "error", err)
		} else {
			pkglogger.Info("sentry database plugin registered successfully")
		}
	}

	DB = db
	return db, nil
}

// GetDB returns the database connection instance
func GetDB() *gorm.DB {
	return DB
}

// SetDB replaces the package database instance.
func SetDB(db *gorm.DB) {
	DB = db
}
