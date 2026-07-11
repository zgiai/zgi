package cache

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/zgiai/zgi/api/internal/cache/keys"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

const (
	dashboardModulePrefix = "system.dashboard"
	statsPart             = "stats"
	recentWorkPart        = "recent_work"

	entryTTL = 30 * time.Second
	// Dashboard requests remain correct without Redis. Keep cache operations short
	// enough that an unavailable cache does not materially affect request latency.
	redisOpTimeout = 5 * time.Millisecond
)

// DashboardCache stores short-lived dashboard response data. It does not cache
// authorization decisions; callers must authorize requests before using it.
type DashboardCache struct{}

func NewDashboardCache() *DashboardCache {
	return &DashboardCache{}
}

func (c *DashboardCache) GetStats(ctx context.Context, organizationID string) (*model.DashboardStatsResponse, bool) {
	var value model.DashboardStatsResponse
	if !getJSON(ctx, statsKey(organizationID), &value) {
		return nil, false
	}
	return &value, true
}

func (c *DashboardCache) SetStats(ctx context.Context, organizationID string, value *model.DashboardStatsResponse) {
	setJSON(ctx, statsKey(organizationID), value)
}

func (c *DashboardCache) GetRecentWork(ctx context.Context, organizationID, accountID string, limit int) (*model.RecentWorkResponse, bool) {
	var value model.RecentWorkResponse
	if !getJSON(ctx, recentWorkKey(organizationID, accountID, limit), &value) {
		return nil, false
	}
	return &value, true
}

func (c *DashboardCache) SetRecentWork(ctx context.Context, organizationID, accountID string, limit int, value *model.RecentWorkResponse) {
	setJSON(ctx, recentWorkKey(organizationID, accountID, limit), value)
}

func statsKey(organizationID string) string {
	// The current dashboard stats handler resolves only an organization. Add
	// workspace or other scope dimensions here only when the handler exposes
	// those scopes as part of its request semantics.
	return keys.DefaultBuilder().Build(dashboardModulePrefix, statsPart, organizationID)
}

func recentWorkKey(organizationID, accountID string, limit int) string {
	return keys.DefaultBuilder().Build(dashboardModulePrefix, recentWorkPart, organizationID, accountID, strconv.Itoa(limit))
}

func getJSON(ctx context.Context, key string, value any) bool {
	client := redisutil.GetClient()
	if client == nil || key == "" {
		return false
	}

	redisCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()

	payload, err := client.Get(redisCtx, key).Bytes()
	if err != nil {
		return false
	}
	return json.Unmarshal(payload, value) == nil
}

func setJSON(ctx context.Context, key string, value any) {
	client := redisutil.GetClient()
	if client == nil || key == "" || value == nil {
		return
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return
	}

	redisCtx, cancel := context.WithTimeout(ctx, redisOpTimeout)
	defer cancel()
	_ = client.SetEx(redisCtx, key, payload, entryTTL).Err()
}
