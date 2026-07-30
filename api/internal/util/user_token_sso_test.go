package util

import (
	"context"
	"encoding/json"
	"sync"
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

func TestTokenManagerVerifyTokenCodeEnforcesAttemptLimitAtomically(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	redisUtil.SetClient(client)

	tm := NewTokenManager()
	token, err := tm.GenerateDataToken(t.Context(), "email_registration_challenge", map[string]interface{}{
		"registration_email": "user@example.com",
		"code":               "123456",
	})
	require.NoError(t, err)

	const requestCount = 20
	statuses := make(chan TokenCodeStatus, requestCount)
	errorsSeen := make(chan error, requestCount)
	var wg sync.WaitGroup
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, status, verifyErr := tm.VerifyTokenCode(
				t.Context(), token, "email_registration_challenge",
				"registration_email", "user@example.com", "000000", "",
				3, time.Minute, TokenCodeConsume,
			)
			statuses <- status
			errorsSeen <- verifyErr
		}()
	}
	wg.Wait()
	close(statuses)
	close(errorsSeen)

	counts := map[TokenCodeStatus]int{}
	for status := range statuses {
		counts[status]++
	}
	for verifyErr := range errorsSeen {
		require.NoError(t, verifyErr)
	}
	require.Equal(t, 2, counts[TokenCodeMismatch])
	require.Equal(t, 1, counts[TokenCodeRateLimited])
	require.Equal(t, requestCount-3, counts[TokenCodeInvalid])
}

func TestTokenManagerVerifyTokenCodeRejectsTokenWithoutPositiveTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	redisUtil.SetClient(client)

	tm := NewTokenManager()
	token := "persistent-token"
	raw, err := json.Marshal(map[string]interface{}{
		"token_type": "email_code_login",
		"email":      "user@example.com",
		"code":       "123456",
	})
	require.NoError(t, err)
	key := tm.getTokenKey(token, "email_code_login")
	require.NoError(t, client.Set(t.Context(), key, raw, 0).Err())

	data, status, err := tm.VerifyTokenCode(
		t.Context(), token, "email_code_login",
		"email", "user@example.com", "123456", "",
		5, time.Minute, TokenCodeReserve,
	)
	require.NoError(t, err)
	require.Nil(t, data)
	require.Equal(t, TokenCodeInvalid, status)
	require.False(t, server.Exists(key))
}

func TestTokenManagerReservationPreservesPositiveTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	redisUtil.SetClient(client)

	tm := NewTokenManager()
	token, err := tm.GenerateDataToken(t.Context(), "email_registration_challenge", map[string]interface{}{
		"registration_email": "user@example.com",
		"code":               "123456",
	})
	require.NoError(t, err)
	key := tm.getTokenKey(token, "email_registration_challenge")
	initialTTL := server.TTL(key)

	_, status, err := tm.VerifyTokenCode(
		t.Context(), token, "email_registration_challenge",
		"registration_email", "user@example.com", "123456", "",
		5, time.Minute, TokenCodeReserve,
	)
	require.NoError(t, err)
	require.Equal(t, TokenCodeVerified, status)
	require.Positive(t, server.TTL(key))
	require.LessOrEqual(t, server.TTL(key), initialTTL)

	require.NoError(t, tm.ReleaseTokenCodeReservation(
		t.Context(), token, "email_registration_challenge", "registration_email", "user@example.com",
	))
	require.Positive(t, server.TTL(key))
}

func TestTokenManagerConsumeTokenDataAtomicallyDeletesToken(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	redisUtil.SetClient(client)

	tm := NewTokenManager()
	token, err := tm.GenerateDataToken(t.Context(), "email_registration_verified", map[string]interface{}{
		"registration_email": "user@example.com",
	})
	require.NoError(t, err)

	data, err := tm.ConsumeTokenData(t.Context(), token, "email_registration_verified")
	require.NoError(t, err)
	require.Equal(t, "email_registration_verified", data.TokenType)
	require.Equal(t, "user@example.com", data.Extra["registration_email"])
	require.False(t, server.Exists(tm.getTokenKey(token, "email_registration_verified")))

	_, err = tm.ConsumeTokenData(t.Context(), token, "email_registration_verified")
	require.ErrorContains(t, err, "token not found")
}

func TestTokenManagerConsumeTokenDataAllowsOnlyOneConcurrentConsumer(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	redisUtil.SetClient(client)

	tm := NewTokenManager()
	token, err := tm.GenerateDataToken(t.Context(), "email_registration_verified", map[string]interface{}{
		"registration_email": "user@example.com",
	})
	require.NoError(t, err)

	const consumerCount = 8
	results := make(chan error, consumerCount)
	var wg sync.WaitGroup
	for range consumerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, consumeErr := tm.ConsumeTokenData(
				t.Context(),
				token,
				"email_registration_verified",
			)
			results <- consumeErr
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	notFound := 0
	for consumeErr := range results {
		if consumeErr == nil {
			successes++
			continue
		}
		require.ErrorContains(t, consumeErr, "token not found")
		notFound++
	}
	require.Equal(t, 1, successes)
	require.Equal(t, consumerCount-1, notFound)
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
