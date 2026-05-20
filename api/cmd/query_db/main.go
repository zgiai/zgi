package main

import (
	"fmt"
	"log"
	"os"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"github.com/zgiai/ginext/pkg/database"
)

func main() {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Init DB
	dbInstance, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to init DB: %v", err)
	}

	var dataset model.Dataset
	// Find one dataset with graph flow enabled
	if err := dbInstance.Where("enable_graph_flow = ?", true).First(&dataset).Error; err != nil {
		fmt.Printf("No dataset found with enable_graph_flow=true: %v\n", err)

		// Fallback: find any dataset
		var anyDataset model.Dataset
		if err := dbInstance.First(&anyDataset).Error; err != nil {
			fmt.Printf("No datasets found at all: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Found dataset (GraphFlow disabled): %s (%s)\n", anyDataset.ID, anyDataset.Name)
	} else {
		fmt.Printf("Found dataset (GraphFlow enabled): %s (%s)\n", dataset.ID, dataset.Name)
	}
}
