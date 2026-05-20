package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/zgiai/zgi/api/internal/migrationsseed"
	"github.com/zgiai/zgi/api/internal/migrationsv2"
	"github.com/zgiai/zgi/api/pkg/logger"
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
		// Execute database migrations (upgrade)
		if err := migrationsv2.Run(); err != nil {
			logger.Fatal("Migration failed: %v", err)
		}
		logger.Info("Migration completed successfully")

	case "down":
		// Rollback last database migration
		if err := migrationsv2.Rollback(); err != nil {
			logger.Fatal("Rollback failed: %v", err)
		}
		logger.Info("Rollback completed successfully")

	case "seed":
		// Execute seed command
		runSeedCommand()

	default:
		logger.Fatal("Unknown command: %s", command)
		printUsage()
		os.Exit(1)
	}
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
		files, err := migrationsseed.ListSeedFiles(*env)
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

	if err := migrationsseed.RunSeed(context.Background(), migrationsseed.SeedOptions{
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
  down            Rollback the last migration
  seed            Execute seed data

Seed Options:
  -env string     Environment (development, staging, production) (default: development)
  -file string    Execute specific seed file
  -list           List all available seed files
  -db string      Database connection string (priority: -db flag > DATABASE_URL env > config file)

Examples:
  # Migration
  go run cmd/migrate/main.go up
  go run cmd/migrate/main.go down

  # Seed
  go run cmd/migrate/main.go seed                    # Execute development environment seeds
  go run cmd/migrate/main.go seed -env=production    # Execute production environment seeds
  go run cmd/migrate/main.go seed -list              # List all seeds
  go run cmd/migrate/main.go seed -force             # Re-run built-in workflow seeds

Make Commands:
  make migrate     # Execute migrations
`)
}
