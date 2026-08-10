package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var invocationContentCleanupDatabases sync.Map

// startInvocationContentCleanup is independent from content capture. This is
// intentional: disabling capture after it has been used must not leave expired
// sensitive rows behind. There is one lightweight worker per SQL connection
// pool, even when the application constructs several Gateway services.
func startInvocationContentCleanup(db *gorm.DB) {
	if db == nil {
		return
	}
	// A self-hosted process may be started before its migrations are applied.
	// Treat the optional table as absent instead of producing recurring errors.
	if !db.Migrator().HasTable((invocationContentRow{}).TableName()) {
		return
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		return
	}
	if _, loaded := invocationContentCleanupDatabases.LoadOrStore(sqlDB, struct{}{}); loaded {
		return
	}
	go runInvocationContentCleanup(db)
}

func runInvocationContentCleanup(db *gorm.DB) {
	cleanup := func() {
		if err := cleanupExpiredInvocationContent(context.Background(), db, time.Now().UTC()); err != nil {
			logger.Warn("failed to clean expired llm invocation content", zap.Error(err))
		}
	}
	cleanup()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		cleanup()
	}
}

func cleanupExpiredInvocationContent(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&invocationContentRow{}).Error
}
