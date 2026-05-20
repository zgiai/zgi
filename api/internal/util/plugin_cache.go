package util

import (
	"context"

	"github.com/zgiai/zgi/api/pkg/logger"
)

// ClearTenantCache Clear all cache entries for a specific tenant
func ClearTenantCache(ctx context.Context, tenantID string) {
	// Clear all cache entries with the tenant prefix (more efficient approach)
	tenantCachePrefix := "tenant:" + tenantID + ":"
	if cacheErr := DeleteCacheByPrefix(ctx, tenantCachePrefix); cacheErr != nil {
		logger.Warn("Failed to clear tenant cache prefix for tenant %s: %v", tenantID, cacheErr)
	}
}
