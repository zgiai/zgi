package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/bootstrap/fxapp"
	"github.com/zgiai/ginext/internal/migrationsseed"
	"github.com/zgiai/ginext/internal/migrationsv2"
	_ "github.com/zgiai/ginext/internal/modules/payment"
	"github.com/zgiai/ginext/pkg/database"
)

// @title ZGI-GinKit API
// @version 1.0
// @description A Gin-based enterprise-level web development kit
// @host localhost:2678
// @BasePath /v1
func main() {
	rootCmd := &cobra.Command{
		Use:   "server",
		Short: "ZGI API Server",
	}

	seedOpts := migrationsseed.SeedOptions{}

	// Add start command (contains original functionality)
	rootCmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return startServer()
		},
	})

	// Add migrate command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			return migrationsv2.Run()
		},
	})

	// Add migrate:rollback command
	rootCmd.AddCommand(&cobra.Command{
		Use:   "migrate:rollback",
		Short: "Rollback the last database migration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return migrationsv2.Rollback()
		},
	})

	seedCmd := &cobra.Command{
		Use:   "seed",
		Short: "Execute seed data",
		RunE: func(cmd *cobra.Command, args []string) error {
			if seedOpts.ListOnly {
				files, err := migrationsseed.ListSeedFiles(seedOpts.Env)
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

			if err := migrationsseed.RunSeed(context.Background(), seedOpts); err != nil {
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
