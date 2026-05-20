package main

import (
	"log"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/infra/grpc"
	"github.com/zgiai/ginext/internal/infra/platform"
	workspace_repo "github.com/zgiai/ginext/internal/modules/workspace/repository"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/database"
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

	// Initialize dependencies (similar to cmd/server/main.go)
	tokenMgr := util.NewTokenManager()

	workspaceRepo := workspace_repo.NewWorkspaceRepository(db)
	workspaceMemberRepo := workspace_repo.NewWorkspaceMemberRepository(db)

	platformContainer, err := platform.NewContainer(db)
	if err != nil {
		log.Fatalf("Failed to initialize platform container: %v", err)
	}

	serviceContainer := container.NewServiceContainer(
		db,
		tokenMgr,
		cfg,
		workspaceRepo,
		workspaceMemberRepo,
		platformContainer,
	)

	serviceContainer.InitializeDependencies()

	// Create and start gRPC server
	server := grpc.NewServer(serviceContainer)

	port := 50051 // Default gRPC port
	if err := server.Start(port); err != nil {
		log.Fatalf("gRPC server failed: %v", err)
	}
}
