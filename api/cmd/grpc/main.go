package main

import (
	"log"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/infra/grpc"
	"github.com/zgiai/zgi/api/pkg/database"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := database.InitDB(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Create and start gRPC server
	server := grpc.NewServer(db)

	port := 50051 // Default gRPC port
	if err := server.Start(port); err != nil {
		log.Fatalf("gRPC server failed: %v", err)
	}
}
