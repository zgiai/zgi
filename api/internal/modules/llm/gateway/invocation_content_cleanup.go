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

const (
	invocationContentCleanupBatchSize  = 500
	invocationContentCleanupMaxBatches = 20
)

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
		if _, err := cleanupExpiredInvocationContent(context.Background(), db, time.Now().UTC()); err != nil {
			logger.Warn("failed to clean expired llm invocation content", zap.Error(err))
		}
	}
	cleanup()
	// Each pass is capped at 10,000 rows. Running every minute keeps up with
	// high-volume tenants while preserving short, bounded delete locks.
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cleanup()
	}
}

// cleanupExpiredInvocationContent deletes in bounded batches so a large audit
// volume cannot turn the retention job into one long database lock.
func cleanupExpiredInvocationContent(ctx context.Context, db *gorm.DB, now time.Time) (int64, error) {
	var total int64
	for range invocationContentCleanupMaxBatches {
		var requestIDs []string
		if err := db.WithContext(ctx).Model(&invocationContentRow{}).
			Where("expires_at <= ?", now).Order("expires_at ASC").Limit(invocationContentCleanupBatchSize).
			Pluck("request_id", &requestIDs).Error; err != nil {
			return total, err
		}
		if len(requestIDs) == 0 {
			return total, nil
		}
		result := db.WithContext(ctx).Where("request_id IN ?", requestIDs).Delete(&invocationContentRow{})
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if len(requestIDs) < invocationContentCleanupBatchSize {
			return total, nil
		}
	}
	return total, nil
}
