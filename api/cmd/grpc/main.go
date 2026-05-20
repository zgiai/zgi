package main

import (
	"log"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/infra/grpc"
	"github.com/zgiai/zgi/api/internal/infra/platform"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"github.com/zgiai/zgi/api/internal/util"
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
