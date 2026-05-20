package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/seeders"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

func main() {
	logger.Init()

	// Parse command line arguments
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "up":
		runCmd := flag.NewFlagSet("up", flag.ExitOnError)
		dryRun := runCmd.Bool("pretend", false, "Show migration status without applying changes")
		noLock := runCmd.Bool("no-lock", false, "Run without the PostgreSQL migration advisory lock")
		if err := runCmd.Parse(os.Args[2:]); err != nil {
			logger.Fatal("Failed to parse arguments: %v", err)
		}

		if *dryRun {
			if err := migrations.PrintStatus(); err != nil {
				logger.Fatal("Migration status failed: %v", err)
			}
			return
		}

		if *noLock {
			cfgDB, err := openConfiguredDB()
			if err != nil {
				logger.Fatal("Database initialization failed: %v", err)
			}
			if err := migrations.RunWithOptions(cfgDB, migrations.RunOptions{NoLock: true}); err != nil {
				logger.Fatal("Migration failed: %v", err)
			}
			logger.Info("Migration completed successfully")
			return
		}

		if err := migrations.Run(); err != nil {
			logger.Fatal("Migration failed: %v", err)
		}
		logger.Info("Migration completed successfully")

	case "down":
		runRollbackCommand()

	case "rollback":
		runRollbackCommand()

	case "status":
		if err := migrations.PrintStatus(); err != nil {
			logger.Fatal("Migration status failed: %v", err)
		}

	case "check":
		runCheckCommand()

	case "make":
		runMakeCommand()

	case "seed":
		// Execute seed command
		runSeedCommand()

	default:
		logger.Fatal("Unknown command: %s", command)
		printUsage()
		os.Exit(1)
	}
}

func runRollbackCommand() {
	rollbackCmd := flag.NewFlagSet("rollback", flag.ExitOnError)
	confirmID := rollbackCmd.String("confirm", "", "Confirm the exact latest applied migration ID to roll back")
	noLock := rollbackCmd.Bool("no-lock", false, "Run without the PostgreSQL migration advisory lock; requires ZGI_UNSAFE_NO_MIGRATION_LOCK=1")
	if err := rollbackCmd.Parse(os.Args[2:]); err != nil {
		logger.Fatal("Failed to parse arguments: %v", err)
	}

	db, err := openConfiguredDB()
	if err != nil {
		logger.Fatal("Database initialization failed: %v", err)
	}
	if err := migrations.RollbackWithOptions(db, migrations.RollbackOptions{
		ConfirmID: *confirmID,
		NoLock:    *noLock,
	}); err != nil {
		logger.Fatal("Rollback failed: %v", err)
	}
	logger.Info("Rollback completed successfully")
}

func runMakeCommand() {
	makeCmd := flag.NewFlagSet("make", flag.ExitOnError)
	if err := makeCmd.Parse(os.Args[2:]); err != nil {
		logger.Fatal("Failed to parse arguments: %v", err)
	}
	if makeCmd.NArg() != 1 {
		logger.Fatal("Usage: go run ./cmd/migrate make <migration_slug>")
	}
	file, err := createMigrationFile(makeCmd.Arg(0))
	if err != nil {
		logger.Fatal("Failed to create migration: %v", err)
	}
	fmt.Printf("Created migration: %s\n", file)
}

func runCheckCommand() {
	checkCmd := flag.NewFlagSet("check", flag.ExitOnError)
	dbURL := checkCmd.String("db", "", "Fresh PostgreSQL database DSN used to execute all migrations")
	if err := checkCmd.Parse(os.Args[2:]); err != nil {
		logger.Fatal("Failed to parse arguments: %v", err)
	}

	dsn := strings.TrimSpace(*dbURL)
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("ZGI_MIGRATION_CHECK_DSN"))
	}

	result, err := migrations.Check(migrations.CheckOptions{PostgresDSN: dsn})
	if err != nil {
		logger.Fatal("Migration check failed: %v", err)
	}

	fmt.Printf("Migration check passed: %d migrations, %d migration files checked\n", result.MigrationCount, len(result.CheckedFiles))
	if result.PostgresCheckRan {
		fmt.Println("Fresh PostgreSQL execution: passed")
	} else if result.PostgresCheckSkipped {
		fmt.Println("Fresh PostgreSQL execution: skipped; pass -db or set ZGI_MIGRATION_CHECK_DSN to enable")
	}
}

func createMigrationFile(slug string) (string, error) {
	slug = strings.TrimSpace(strings.ToLower(strings.ReplaceAll(slug, "-", "_")))
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(slug) {
		return "", fmt.Errorf("migration slug must be lower_snake_case")
	}

	timestamp := nextMigrationTimestamp(time.Now().UTC())
	id := timestamp + "_" + slug
	name := "migration" + timestamp
	path := filepath.Join("internal", "migrations", id+".go")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("migration already exists: %s", path)
	}

	content := fmt.Sprintf(`package migrations

import (
	"fmt"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const %sID = "%s"

func init() {
	registerSchemaMigration(%sID, up%s, down%s)
}

func up%s(schema *mschema.Builder) error {
	return fmt.Errorf("migration %s is not implemented")
}

func down%s(schema *mschema.Builder) error {
	return fmt.Errorf("rollback for migration %s is not implemented")
}
`, name, id, name, timestamp, timestamp, timestamp, id, timestamp, id)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func nextMigrationTimestamp(now time.Time) string {
	for i := 0; i < 100; i++ {
		candidate := now.Add(time.Duration(i) * time.Second).Format("20060102150405")
		matches, err := filepath.Glob(filepath.Join("internal", "migrations", candidate+"*.go"))
		if err == nil && len(matches) == 0 {
			return candidate
		}
	}
	return now.Format("20060102150405") + fmt.Sprintf("%02d", now.Nanosecond()/1e7)
}

func openConfiguredDB() (*gorm.DB, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return database.InitDB(cfg.Database)
}

// runSeedCommand executes the seed command
func runSeedCommand() {
	// Define seed subcommand flags
	seedCmd := flag.NewFlagSet("seed", flag.ExitOnError)
	env := seedCmd.String("env", "development", "Environment: development, staging, production")
	file := seedCmd.String("file", "", "Execute specific seed file")
	listOnly := seedCmd.Bool("list", false, "List available seed files only")
	force := seedCmd.Bool("force", false, "Execute all seeds even if the initial seed marker already exists")
	dbURL := seedCmd.String("db", "", "Database connection string")

	// Parse seed command arguments (skip "seed" itself)
	if err := seedCmd.Parse(os.Args[2:]); err != nil {
		logger.Fatal("Failed to parse arguments: %v", err)
	}

	// List files (no database connection needed)
	if *listOnly {
		files, err := seeders.ListSeedFiles(*env)
		if err != nil {
			logger.Fatal("Failed to list seed files: %v", err)
		}

		fmt.Println("\nAvailable Seed Files:")
		fmt.Println("=====================================")
		for _, f := range files {
			fmt.Printf("  • %s\n", f)
		}
		fmt.Printf("\nTotal: %d files\n", len(files))
		return
	}

	if err := seeders.RunSeed(context.Background(), seeders.SeedOptions{
		Env:         *env,
		File:        *file,
		ListOnly:    *listOnly,
		Force:       *force,
		DatabaseURL: *dbURL,
	}); err != nil {
		logger.Fatal("Failed to execute seeds: %v", err)
	}

	if *file != "" {
		fmt.Println("\n✅ Seed executed successfully")
		return
	}

	fmt.Println("\n✅ All seeds executed successfully")
}

// printUsage prints usage information
func printUsage() {
	fmt.Print(`
Database Management Tool

Usage:
  go run cmd/migrate/main.go <command> [options]

Commands:
  up              Execute all pending migrations (upgrade)
  rollback        Rollback the last migration with explicit confirmation
  down            Alias of rollback; requires explicit confirmation
  status          Show migration status
  check           Validate migration IDs, filenames, safety rules, and optional fresh PostgreSQL execution
  make <slug>     Create a timestamped migration file
  seed            Execute seed data

Migration Options:
  -pretend        Show migration status without applying changes
  -no-lock        Unsafe: run without the PostgreSQL advisory migration lock; requires ZGI_UNSAFE_NO_MIGRATION_LOCK=1

Seed Options:
  -env string     Environment (development, staging, production) (default: development)
  -file string    Execute specific seed file
  -list           List all available seed files
  -db string      Database connection string (priority: -db flag > DATABASE_URL env > config file)

Examples:
  # Migration
  go run cmd/migrate/main.go up
  go run cmd/migrate/main.go up -pretend
  go run cmd/migrate/main.go status
  go run cmd/migrate/main.go check
  go run cmd/migrate/main.go check -db "host=localhost user=postgres password=postgres dbname=zgi_check port=5432 sslmode=disable"
  go run cmd/migrate/main.go make create_audit_events
  go run cmd/migrate/main.go rollback -confirm 20260601090000_create_audit_events

  # Seed
  go run cmd/migrate/main.go seed                    # Execute development environment seeds
  go run cmd/migrate/main.go seed -env=production    # Execute production environment seeds
  go run cmd/migrate/main.go seed -list              # List all seeds
  go run cmd/migrate/main.go seed -force             # Re-run built-in workflow seeds

Make Commands:
  make migrate     # Execute migrations
`)
}
