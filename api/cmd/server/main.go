package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/bootstrap/fxapp"
	"github.com/zgiai/zgi/api/internal/migrations"
	_ "github.com/zgiai/zgi/api/internal/modules/payment"
	"github.com/zgiai/zgi/api/internal/seeders"
	"github.com/zgiai/zgi/api/pkg/database"
)

// @title ZGI-GinKit API
// @version 1.0
// @description A Gin-based enterprise-level web development kit
// @host localhost:2679
// @BasePath /v1
func main() {
	rootCmd := &cobra.Command{
		Use:   "server",
		Short: "ZGI API Server",
	}

	seedOpts := seeders.SeedOptions{}
	migrateOpts := migrations.RunOptions{}
	rollbackOpts := migrations.RollbackOptions{}

	// Add start command (contains original functionality)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return startServer()
		},
	})

	// Add migrate command
	migrateCmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			if migrateOpts.DryRun {
				return migrations.PrintStatus()
			}
			if migrateOpts.NoLock {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				db, err := database.InitDB(cfg.Database)
				if err != nil {
					return err
				}
				return migrations.RunWithOptions(db, migrateOpts)
			}
			return migrations.Run()
		},
	}
	migrateCmd.Flags().BoolVar(&migrateOpts.DryRun, "pretend", false, "Show migration status without applying changes")
	migrateCmd.Flags().BoolVar(&migrateOpts.NoLock, "no-lock", false, "Run without the PostgreSQL migration advisory lock")
	rootCmd.AddCommand(migrateCmd)

	rootCmd.AddCommand(&cobra.Command{
		Use:   "migrate:status",
		Short: "Show database migration status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return migrations.PrintStatus()
		},
	})

	rollbackCmd := &cobra.Command{
		Use:   "migrate:rollback",
		Short: "Rollback the last database migration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			db, err := database.InitDB(cfg.Database)
			if err != nil {
				return err
			}
			return migrations.RollbackWithOptions(db, rollbackOpts)
		},
	}
	rollbackCmd.Flags().StringVar(&rollbackOpts.ConfirmID, "confirm", "", "Confirm the exact latest applied migration ID to roll back")
	rollbackCmd.Flags().BoolVar(&rollbackOpts.NoLock, "no-lock", false, "Run without the PostgreSQL migration advisory lock; requires ZGI_UNSAFE_NO_MIGRATION_LOCK=1")
	rootCmd.AddCommand(rollbackCmd)

	seedCmd := &cobra.Command{
		Use:   "seed",
		Short: "Execute seed data",
		RunE: func(cmd *cobra.Command, args []string) error {
			if seedOpts.ListOnly {
				files, err := seeders.ListSeedFiles(seedOpts.Env)
				if err != nil {
					return err
				}

				fmt.Println("\nAvailable Seed Files:")
				fmt.Println("=====================================")
				for _, f := range files {
					fmt.Printf("  • %s\n", f)
				}
				fmt.Printf("\nTotal: %d files\n", len(files))
				return nil
			}

			if err := seeders.RunSeed(context.Background(), seedOpts); err != nil {
				return err
			}

			if seedOpts.File != "" {
				fmt.Println("\n✅ Seed executed successfully")
				return nil
			}

			fmt.Println("\n✅ All seeds executed successfully")
			return nil
		},
	}
	seedCmd.Flags().StringVar(&seedOpts.Env, "env", "development", "Environment: development, staging, production")
	seedCmd.Flags().StringVar(&seedOpts.File, "file", "", "Execute specific seed file")
	seedCmd.Flags().BoolVar(&seedOpts.ListOnly, "list", false, "List available seed files only")
	seedCmd.Flags().BoolVar(&seedOpts.Force, "force", false, "Execute all seeds even if the initial seed marker already exists")
	seedCmd.Flags().StringVar(&seedOpts.DatabaseURL, "db", "", "Database connection string")
	rootCmd.AddCommand(seedCmd)

	// Add db:check-connection command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "db:check-connection",
		Short: "Check database connection",
		RunE: func(cmd *cobra.Command, args []string) error {
			return checkDBConnection()
		},
	})

	if len(os.Args) == 1 {
		rootCmd.SetArgs([]string{"start"})
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func startServer() error {
	app := fxapp.NewApp()
	if err := app.Err(); err != nil {
		return err
	}

	app.Run()
	return nil
}

// checkDBConnection verifies database connectivity
func checkDBConnection() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		return fmt.Errorf("init db: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql db: %w", err)
	}
	defer sqlDB.Close()

	return nil
}
