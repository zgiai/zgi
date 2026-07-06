package modelmeta

import (
	"context"
	"strings"
	"sync"

	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

var localBootstrapOnce sync.Once

// EnsureLocalBootstrapStarted runs a one-shot best-effort ModelMeta sync for
// non-cloud deployments. Failures are logged and do not block service startup.
func EnsureLocalBootstrapStarted(ctx context.Context, db *gorm.DB, edition string) {
	if db == nil || strings.EqualFold(strings.TrimSpace(edition), "CLOUD") {
		return
	}

	localBootstrapOnce.Do(func() {
		go func() {
			svc := NewService(db)
			if !svc.HasConfiguredAPIBaseURL() {
				logger.Info("Skipping local model metadata bootstrap sync: MODELMETA_API_URL is not configured")
				return
			}

			if _, err := svc.SyncProviders(ctx); err != nil {
				logger.Warn("Local provider bootstrap sync failed: %v", err)
				return
			}

			if _, err := svc.SyncAllProviders(ctx); err != nil {
				logger.Warn("Local model bootstrap sync failed: %v", err)
			}
		}()
	})
}
