package main

import (
	"context"
	"log"
	"os"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/llm/modelmeta"
	"github.com/zgiai/ginext/pkg/database"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if cfg.Tooling.DryRun {
		log.Println("dry run enabled, skipping model sync")
		return
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	svc := modelmeta.NewService(db)
	ctx := context.Background()

	if len(os.Args) > 1 && os.Args[1] != "" {
		result, err := svc.SyncProviderModels(ctx, os.Args[1], nil)
		if err != nil {
			log.Fatalf("failed to sync provider %s: %v", os.Args[1], err)
		}
		log.Printf(
			"provider=%s total=%d new=%d updated=%d skipped=%d errors=%d duration_ms=%d",
			result.Provider,
			result.TotalModels,
			result.NewModels,
			result.UpdatedModels,
			result.SkippedModels,
			len(result.Errors),
			result.DurationMs,
		)
		return
	}

	results, err := svc.SyncAllProviders(ctx)
	if err != nil {
		log.Fatalf("failed to sync models: %v", err)
	}

	for provider, result := range results {
		log.Printf(
			"provider=%s total=%d new=%d updated=%d skipped=%d errors=%d duration_ms=%d",
			provider,
			result.TotalModels,
			result.NewModels,
			result.UpdatedModels,
			result.SkippedModels,
			len(result.Errors),
			result.DurationMs,
		)
	}
}
