package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/lib/pq"
	"github.com/zgiai/ginext/pkg/logger"
)

//go:embed seeds
var seedFS embed.FS

// Seeder manages the execution of seed data
type Seeder struct {
	db  *sql.DB
	env string // development, staging, production
}

const (
	initialSeedExecutionName    = "initial_bootstrap"
	initialSeedExecutionVersion = "v1"
)

// NewSeeder creates a new Seeder instance
func NewSeeder(db *sql.DB, env string) *Seeder {
	return &Seeder{
		db:  db,
		env: env,
	}
}

// RunAll executes all seeds applicable to the current environment
func (s *Seeder) RunAll(ctx context.Context, force bool) error {
	logger.Info("Starting to execute seed data", "env", s.env)

	if !force {
		executed, err := s.hasExecutionMarker(ctx, initialSeedExecutionName, initialSeedExecutionVersion)
		if err != nil {
			return fmt.Errorf("check seed execution marker: %w", err)
		}
		if executed {
			if err := SeedBuiltInWorkflows(ctx, s.db); err != nil {
				return fmt.Errorf("ensure built-in workflow seeds: %w", err)
			}
			logger.Info(
				"Initial seed already executed, ensured built-in workflow seeds and skipped remaining seeds",
				"name", initialSeedExecutionName,
				"version", initialSeedExecutionVersion,
			)
			return nil
		}

		historicalSeeded, err := s.hasHistoricalInitialSeedData(ctx)
		if err != nil {
			return fmt.Errorf("check historical seed data: %w", err)
		}
		if historicalSeeded {
			if err := s.recordExecutionMarker(ctx, initialSeedExecutionName, initialSeedExecutionVersion, "backfill"); err != nil {
				return fmt.Errorf("backfill seed execution marker: %w", err)
			}
			if err := SeedBuiltInWorkflows(ctx, s.db); err != nil {
				return fmt.Errorf("ensure built-in workflow seeds: %w", err)
			}
			logger.Info(
				"Historical initial seed data detected, backfilled execution marker, ensured built-in workflow seeds and skipped remaining seeds",
				"name", initialSeedExecutionName,
				"version", initialSeedExecutionVersion,
			)
			return nil
		}
	}

	// Execute base data (all environments)
	if err := s.runDirectory(ctx, "seeds/00_base"); err != nil {
		return fmt.Errorf("failed to execute base seeds: %w", err)
	}

	if err := SeedBuiltInWorkflows(ctx, s.db); err != nil {
		return fmt.Errorf("failed to seed built-in workflows: %w", err)
	}

	// Execute environment-specific data
	switch strings.ToLower(s.env) {
	case "development", "dev":
		if err := s.runDirectory(ctx, "seeds/01_development"); err != nil {
			return fmt.Errorf("failed to execute development seeds: %w", err)
		}
	case "production", "prod":
		if err := s.runDirectory(ctx, "seeds/02_production"); err != nil {
			// Production environment may not have additional seeds, ignore directory not found errors
			if !strings.Contains(err.Error(), "file does not exist") {
				return fmt.Errorf("failed to execute production seeds: %w", err)
			}
		}
	}

	if err := s.recordExecutionMarker(ctx, initialSeedExecutionName, initialSeedExecutionVersion, "manual"); err != nil {
		return fmt.Errorf("record seed execution marker: %w", err)
	}

	logger.Info("Seed data execution completed")
	return nil
}

// RunSpecific executes a specific seed file
func (s *Seeder) RunSpecific(ctx context.Context, filename string) error {
	content, err := seedFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read seed file: %w", err)
	}

	return s.executeSQLFile(ctx, filename, string(content))
}

// runDirectory executes all SQL files in the specified directory
func (s *Seeder) runDirectory(ctx context.Context, dir string) error {
	entries, err := seedFS.ReadDir(dir)
	if err != nil {
		// Directory not existing is not an error
		if strings.Contains(err.Error(), "file does not exist") {
			logger.Info("Seed directory does not exist, skipping", "dir", dir)
			return nil
		}
		return err
	}

	// Sort by filename to ensure execution order
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			// Use path.Join instead of filepath.Join for embed.FS (always uses forward slashes)
			files = append(files, path.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)

	// Execute each SQL file
	for _, file := range files {
		content, err := seedFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}

		if err := s.executeSQLFile(ctx, file, string(content)); err != nil {
			return err
		}
	}

	return nil
}

// executeSQLFile executes a single SQL file
func (s *Seeder) executeSQLFile(ctx context.Context, filename, content string) error {
	logger.Info("Executing seed file", "file", filename)

	// Use transaction to ensure atomicity
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute SQL
	if _, err := tx.ExecContext(ctx, content); err != nil {
		return fmt.Errorf("failed to execute %s: %w", filename, err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Info("✓ Seed file executed successfully", "file", filename)
	return nil
}

// List returns all available seed files
func (s *Seeder) List() ([]string, error) {
	var files []string

	dirs := []string{"seeds/00_base", "seeds/01_development", "seeds/02_production"}
	for _, dir := range dirs {
		entries, err := seedFS.ReadDir(dir)
		if err != nil {
			// Skip if directory does not exist
			if strings.Contains(err.Error(), "file does not exist") {
				continue
			}
			return nil, err
		}

		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				// Use path.Join instead of filepath.Join for embed.FS (always uses forward slashes)
				files = append(files, path.Join(dir, entry.Name()))
			}
		}
	}

	sort.Strings(files)
	return files, nil
}

// GetEnv returns the current environment
func (s *Seeder) GetEnv() string {
	return s.env
}

func (s *Seeder) hasExecutionMarker(ctx context.Context, name, version string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("seed database is not initialized")
	}

	var tableExists bool
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name = 'seed_executions'
		)`,
	).Scan(&tableExists); err != nil {
		return false, err
	}
	if !tableExists {
		return false, nil
	}

	var markerExists bool
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM seed_executions
			WHERE name = $1
			  AND version = $2
		)`,
		name,
		version,
	).Scan(&markerExists); err != nil {
		return false, err
	}

	return markerExists, nil
}

func (s *Seeder) recordExecutionMarker(ctx context.Context, name, version, executedBy string) error {
	if s.db == nil {
		return fmt.Errorf("seed database is not initialized")
	}

	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO seed_executions (name, version, executed_by, status)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (name, version) DO NOTHING`,
		name,
		version,
		executedBy,
		"success",
	)
	return err
}

func (s *Seeder) hasHistoricalInitialSeedData(ctx context.Context) (bool, error) {
	agentsSeeded, err := s.hasBuiltInAgentData(ctx)
	if err != nil {
		return false, err
	}
	if !agentsSeeded {
		return false, nil
	}

	return s.hasBuiltInWorkflowData(ctx)
}

func (s *Seeder) hasBuiltInAgentData(ctx context.Context) (bool, error) {
	exists, err := s.tableExists(ctx, "agents")
	if err != nil || !exists {
		return false, err
	}

	var matchedCount int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(DISTINCT id)
		 FROM agents
		 WHERE tenant_id::text = $1
		   AND id::text = ANY($2)`,
		BuiltInTenantID,
		pq.Array(BuiltInWorkflowSeedAgentIDs()),
	).Scan(&matchedCount); err != nil {
		return false, err
	}

	return matchedCount == len(BuiltInWorkflowSeedScenarios()), nil
}

func (s *Seeder) hasBuiltInWorkflowData(ctx context.Context) (bool, error) {
	exists, err := s.tableExists(ctx, "workflows")
	if err != nil || !exists {
		return false, err
	}

	var matchedCount int
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(DISTINCT agent_id)
		 FROM workflows
		 WHERE tenant_id::text = $1
		   AND agent_id::text = ANY($2)`,
		BuiltInTenantID,
		pq.Array(BuiltInWorkflowSeedAgentIDs()),
	).Scan(&matchedCount); err != nil {
		return false, err
	}

	return matchedCount == len(BuiltInWorkflowSeedScenarios()), nil
}

func (s *Seeder) tableExists(ctx context.Context, tableName string) (bool, error) {
	var tableExists bool
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public'
			  AND table_name = $1
		)`,
		tableName,
	).Scan(&tableExists); err != nil {
		return false, err
	}

	return tableExists, nil
}
