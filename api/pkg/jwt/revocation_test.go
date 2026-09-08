package jwt

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	jwtlib "github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestAccessTokenRevocation(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	oldClient, oldConfig := redisutil.GetClient(), cfg
	redisutil.SetClient(client)
	Init(&config.Config{JWT: config.JWTConfig{Secret: "local-jwt-test-only", JWTExpire: time.Hour}})
	t.Cleanup(func() { redisutil.SetClient(oldClient); Init(oldConfig); _ = client.Close() })
	first, err := GenerateTokenFixed("account-a", "")
	require.NoError(t, err)
	second, err := GenerateTokenFixed("account-a", "")
	require.NoError(t, err)
	require.NotEqual(t, first, second, "two logins in one second must not share a revoked JWT")
	_, err = GetUserIDFromToken(first)
	require.NoError(t, err)
	for range 2 {
		accountID, err := RevokeToken(context.Background(), first)
		require.NoError(t, err)
		require.Equal(t, "account-a", accountID)
	}
	_, err = GetUserIDFromToken(first)
	require.ErrorIs(t, err, ErrTokenRevoked)
	_, err = GetUserIDFromToken(second)
	require.NoError(t, err, "another session must not be revoked")
	require.Positive(t, server.TTL(revocationKey(first)))
	require.LessOrEqual(t, server.TTL(revocationKey(first)), time.Hour)
	for _, key := range server.Keys() {
		require.False(t, strings.Contains(key, first), "stored keys must not disclose the JWT")
	}

	missingExpiry, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"user_id": "account-a",
	}).SignedString([]byte("local-jwt-test-only"))
	require.NoError(t, err)
	_, err = RevokeToken(context.Background(), missingExpiry)
	require.Error(t, err)
	_, err = RevokeToken(context.Background(), "not-a-jwt")
	require.Error(t, err)
	forged, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"user_id": "account-a", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("wrong-signing-key"))
	require.NoError(t, err)
	_, err = RevokeToken(context.Background(), forged)
	require.Error(t, err, "logout must verify the signature even when expiry validation is relaxed")
	expired, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{
		"user_id": "account-a", "exp": time.Now().Add(-time.Minute).Unix(),
	}).SignedString([]byte("local-jwt-test-only"))
	require.NoError(t, err)
	_, err = GetUserIDFromToken(expired)
	require.Error(t, err, "expired token must remain invalid for authentication")
	owner, err := RevokeToken(context.Background(), expired)
	require.NoError(t, err, "expired access token may still identify its own refresh token for logout")
	require.Equal(t, "account-a", owner)
	_, err = GetUserIDFromToken(expired)
	require.Error(t, err)

	server.SetError("synthetic storage failure")
	_, err = GetUserIDFromToken(second)
	require.ErrorIs(t, err, ErrRevocationStoreUnavailable)
	_, err = RevokeToken(context.Background(), second)
	require.Error(t, err, "logout cannot report server success without storing the revocation")
	server.SetError("")
	server.FastForward(2 * time.Hour)
	require.False(t, server.Exists(revocationKey(first)), "revocation records must expire")
	redisutil.SetClient(nil)
	_, err = GetUserIDFromToken(second)
	require.ErrorIs(t, err, ErrRevocationStoreUnavailable)
}
