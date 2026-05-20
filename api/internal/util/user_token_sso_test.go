package util

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestTokenManagerIncrementTokenUsage(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	redisUtil.SetClient(client)

	tm := NewTokenManager()
	ctx := context.Background()

	count, err := tm.IncrementTokenUsage(ctx, "ticket-1", "sso_login_ticket", 3*time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)

	count, err = tm.IncrementTokenUsage(ctx, "ticket-1", "sso_login_ticket", time.Minute)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)

	ttl := server.TTL(tm.getTokenUsageKey("ticket-1", "sso_login_ticket"))
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, time.Minute)
}

func TestTokenManagerDecrementTokenUsageDeletesKeyAtZero(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	redisUtil.SetClient(client)

	tm := NewTokenManager()
	ctx := context.Background()

	_, err := tm.IncrementTokenUsage(ctx, "ticket-1", "sso_login_ticket", time.Minute)
	require.NoError(t, err)

	count, err := tm.DecrementTokenUsage(ctx, "ticket-1", "sso_login_ticket")
	require.NoError(t, err)
	require.Zero(t, count)
	require.False(t, server.Exists(tm.getTokenUsageKey("ticket-1", "sso_login_ticket")))
}
