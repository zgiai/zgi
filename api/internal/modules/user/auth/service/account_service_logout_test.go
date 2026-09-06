package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	authmodel "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/util"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestAccountServiceLogoutRevokesOnlyPresentedSession(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	oldClient := redisutil.GetClient()
	redisutil.SetClient(client)
	jwtpkg.Init(&config.Config{JWT: config.JWTConfig{Secret: "local-logout-test-only", JWTExpire: time.Hour}})
	t.Cleanup(func() { redisutil.SetClient(oldClient); jwtpkg.Init(nil); _ = client.Close() })
	tm := util.NewTokenManager()
	svc := &AccountService{tokenMgr: tm}
	ctx := context.Background()
	account := &authmodel.Account{ID: "logout-account", Email: "logout@example.invalid"}
	first, err := tm.GenerateToken(ctx, "refresh", account, nil, nil)
	require.NoError(t, err)
	other, err := tm.GenerateToken(ctx, "refresh", account, nil, nil)
	require.NoError(t, err)
	access, err := jwtpkg.GenerateTokenFixed(account.ID, "")
	require.NoError(t, err)
	require.NoError(t, client.Set(ctx, "refresh_token:"+first, account.ID, time.Hour).Err())
	require.NoError(t, client.Set(ctx, "refresh_token:"+account.ID, other, time.Hour).Err())
	for range 2 {
		require.NoError(t, svc.Logout(ctx, access, first))
	}
	_, err = jwtpkg.GetUserIDFromToken(access)
	require.ErrorIs(t, err, jwtpkg.ErrTokenRevoked)
	_, err = tm.GetTokenData(first, "refresh")
	require.Error(t, err)
	_, err = tm.GetTokenData(other, "refresh")
	require.NoError(t, err)
	require.False(t, server.Exists("refresh_token:"+first))
	for _, key := range []string{"refresh_token:" + account.ID, "current_token:refresh:" + account.ID} {
		value, err := server.Get(key)
		require.NoError(t, err)
		require.Equal(t, other, value)
	}

	outsider, err := jwtpkg.GenerateTokenFixed("other-account", "")
	require.NoError(t, err)
	require.Error(t, svc.Logout(ctx, outsider, other), "cross-account refresh revocation must fail")
	_, err = tm.GetTokenData(other, "refresh")
	require.NoError(t, err, "mismatched logout must leave the victim's refresh token usable")
	accessOnly, err := jwtpkg.GenerateTokenFixed(account.ID, "")
	require.NoError(t, err)
	require.NoError(t, svc.Logout(ctx, accessOnly, ""), "old access-only clients remain supported")
	_, err = jwtpkg.GetUserIDFromToken(accessOnly)
	require.ErrorIs(t, err, jwtpkg.ErrTokenRevoked)
}
