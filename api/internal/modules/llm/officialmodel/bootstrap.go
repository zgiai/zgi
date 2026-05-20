package officialmodel

import (
	"context"
	"strings"
	"sync"

	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const defaultOfficialModelGRPCAddr = "localhost:50051"

var (
	synchronizerBootstrapOnce sync.Once
	startSynchronizerLoop     = runSynchronizerLoop
)

// EnsureSynchronizerStarted starts the official model synchronizer once per process in CLOUD mode.
// It retries the initial connect in the background until the console gRPC endpoint becomes available.
func EnsureSynchronizerStarted(ctx context.Context, db *gorm.DB, edition string, grpcAddr string) {
	if db == nil {
		return
	}
	if strings.ToUpper(strings.TrimSpace(edition)) != "CLOUD" {
		return
	}
	addr := strings.TrimSpace(grpcAddr)
	if addr == "" {
		addr = defaultOfficialModelGRPCAddr
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
			logger.WarnContext(ctx, "Official model synchronizer start failed",
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
