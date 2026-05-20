package catalogsync

import (
	"context"
	"strings"
	"sync"

	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const defaultCatalogGRPCAddr = "localhost:50051"

var (
	synchronizerBootstrapOnce sync.Once
	startSynchronizerLoop     = runSynchronizerLoop
)

func EnsureSynchronizerStarted(ctx context.Context, db *gorm.DB, edition string, grpcAddr string) {
	if db == nil {
		return
	}
	if strings.ToUpper(strings.TrimSpace(edition)) != "CLOUD" {
		return
	}

	addr := strings.TrimSpace(grpcAddr)
	if addr == "" {
		addr = defaultCatalogGRPCAddr
	}
	if ctx == nil {
		ctx = context.Background()
	}

	synchronizerBootstrapOnce.Do(func() {
		go startSynchronizerLoop(ctx, db, addr)
	})
}

func runSynchronizerLoop(ctx context.Context, db *gorm.DB, grpcAddr string) {
	for {
		syncer := NewSynchronizer(db, grpcAddr)
		if err := syncer.Start(ctx); err == nil {
			return
		} else {
			logger.WarnContext(ctx, "Catalog synchronizer start failed",
				zap.String("grpc_addr", grpcAddr),
				zap.Error(err),
			)
		}

		if !sleepContext(ctx, defaultReconnectDelay) {
			return
		}
	}
}

func resetSynchronizerBootstrapForTest() {
	synchronizerBootstrapOnce = sync.Once{}
	startSynchronizerLoop = runSynchronizerLoop
}
