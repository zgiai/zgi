package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestDashboardCacheStatsExpiresAndIsOrganizationScoped(t *testing.T) {
	server := withRedis(t)
	ctx := context.Background()
	cache := NewDashboardCache()
	value := &model.DashboardStatsResponse{Resources: model.ResourceStats{Agents: 3}}

	cache.SetStats(ctx, "organization-1", "account-1", "scope-1", value)

	got, ok := cache.GetStats(ctx, "organization-1", "account-1", "scope-1")
	if !ok || got.Resources.Agents != 3 {
		t.Fatalf("GetStats() = (%+v, %v), want cached organization stats", got, ok)
	}
	if got, ok := cache.GetStats(ctx, "organization-2", "account-1", "scope-1"); ok || got != nil {
		t.Fatalf("GetStats() for another organization = (%+v, %v), want nil false", got, ok)
	}

	server.FastForward(entryTTL + time.Second)
	if got, ok := cache.GetStats(ctx, "organization-1", "account-1", "scope-1"); ok || got != nil {
		t.Fatalf("GetStats() after TTL = (%+v, %v), want nil false", got, ok)
	}
}

func TestDashboardCacheRecentWorkIsAccountAndLimitScoped(t *testing.T) {
	withRedis(t)
	ctx := context.Background()
	cache := NewDashboardCache()
	value := &model.RecentWorkResponse{Items: []model.RecentWorkItem{{ID: "agent-1"}}}

	cache.SetRecentWork(ctx, "organization-1", "account-1", 10, "scope-1", value)

	if got, ok := cache.GetRecentWork(ctx, "organization-1", "account-1", 10, "scope-1"); !ok || len(got.Items) != 1 {
		t.Fatalf("GetRecentWork() = (%+v, %v), want cached recent work", got, ok)
	}
	if got, ok := cache.GetRecentWork(ctx, "organization-1", "account-2", 10, "scope-1"); ok || got != nil {
		t.Fatalf("GetRecentWork() for another account = (%+v, %v), want nil false", got, ok)
	}
	if got, ok := cache.GetRecentWork(ctx, "organization-1", "account-1", 20, "scope-1"); ok || got != nil {
		t.Fatalf("GetRecentWork() for another limit = (%+v, %v), want nil false", got, ok)
	}
}

func TestDashboardCacheMissesWithoutRedis(t *testing.T) {
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(nil)
	t.Cleanup(func() { redisutil.SetClient(previousRedis) })

	cache := NewDashboardCache()
	if got, ok := cache.GetStats(context.Background(), "organization-1", "account-1", "scope-1"); ok || got != nil {
		t.Fatalf("GetStats() without Redis = (%+v, %v), want nil false", got, ok)
	}
}

func withRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	previousRedis := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() { redisutil.SetClient(previousRedis) })
	return server
}
