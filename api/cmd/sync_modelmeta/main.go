package main

import (
	"context"
	"log"

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
		log.Println("dry run enabled, skipping provider sync")
		return
	}

	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	result, err := modelmeta.NewService(db).SyncProviders(context.Background())
	if err != nil {
		log.Fatalf("failed to sync providers: %v", err)
	}

	log.Printf(
		"providers total=%d created=%d updated=%d errors=%d duration_ms=%d",
		result.TotalProviders,
		result.CreatedProviders,
		result.UpdatedProviders,
		len(result.Errors),
		result.DurationMs,
	)
}
