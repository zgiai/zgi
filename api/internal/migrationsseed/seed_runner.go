package migrationsseed

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// SeedOptions controls how seed execution is performed.
type SeedOptions struct {
	Env         string
	File        string
	ListOnly    bool
	Force       bool
	DatabaseURL string
}

// Normalize fills in defaults for omitted seed options.
func (o SeedOptions) Normalize() SeedOptions {
	if o.Env == "" {
		o.Env = "development"
	}
	return o
}

// ListSeedFiles returns the available embedded seed files for the given environment.
func ListSeedFiles(env string) ([]string, error) {
	seeder := NewSeeder(nil, SeedOptions{Env: env}.Normalize().Env)
	return seeder.List()
}

// RunSeed executes seed files according to the provided options.
func RunSeed(ctx context.Context, opts SeedOptions) error {
	opts = opts.Normalize()

	if opts.ListOnly {
		return nil
	}

	databaseURL := resolveSeedDatabaseURL(opts.DatabaseURL)
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("Database connection successful")

	seeder := NewSeeder(db, opts.Env)
	if opts.File != "" {
		logger.Info("Executing specific seed file", "file", opts.File)
		return seeder.RunSpecific(ctx, opts.File)
	}

	logger.Info("Executing all seeds", "env", opts.Env)
	return seeder.RunAll(ctx, opts.Force)
}

func resolveSeedDatabaseURL(override string) string {
	if override != "" {
		return override
	}

	dbCfg := config.Current().Database
	if dbCfg.URL != "" {
		return dbCfg.URL
	}

	logger.Info("Using database config from .env", "host", dbCfg.Host, "dbname", dbCfg.DBName)
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		dbCfg.Host, dbCfg.Port, dbCfg.Username, dbCfg.Password, dbCfg.DBName, dbCfg.SSLMode, dbCfg.Timezone,
	)
}
