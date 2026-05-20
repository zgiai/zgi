package auth_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	auth_service "github.com/zgiai/zgi/api/internal/modules/user/auth/service"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestLoginErrorRateLimitUsesNormalizedKeyAndTTL(t *testing.T) {
	server := setupRateLimitRedis(t)
	service := newRateLimitAccountService()
	ctx := context.Background()

	for i := 0; i < helper.LoginMaxErrorLimits; i++ {
		require.NoError(t, service.AddLoginErrorRateLimit(ctx, " USER@example.COM "))
	}

	key := "login_error_rate_limit:user@example.com"
	require.True(t, server.Exists(key))
	count, err := server.Get(key)
	require.NoError(t, err)
	require.Equal(t, "5", count)
	require.Positive(t, server.TTL(key))

	limited, err := service.IsLoginErrorRateLimit(ctx, "user@example.com")
	require.NoError(t, err)
	require.True(t, limited)
	require.True(t, auth_service.IsLoginErrorRateLimit("USER@example.com"))

	require.NoError(t, service.ResetLoginErrorRateLimit(ctx, " USER@example.COM "))
	require.False(t, server.Exists(key))
}

func TestForgotPasswordErrorRateLimitClearsSameKeyOnSuccess(t *testing.T) {
	server := setupRateLimitRedis(t)
	service := newRateLimitAccountService()
	ctx := context.Background()
	email := "user@example.com"
	code := "123456"

	token, err := helper.NewTokenManager().GenerateToken(ctx, "reset_password", nil, &email, map[string]interface{}{"code": code})
	require.NoError(t, err)

	valid, tokenEmail, err := service.ValidateResetPasswordToken(token, email, "000000")
	require.Error(t, err)
	require.False(t, valid)
	require.Equal(t, email, tokenEmail)

	key := "forgot_password_error_rate_limit:user@example.com"
	require.True(t, server.Exists(key))
	count, err := server.Get(key)
	require.NoError(t, err)
	require.Equal(t, "1", count)
	require.Positive(t, server.TTL(key))

	valid, tokenEmail, err = service.ValidateResetPasswordToken(token, email, code)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, email, tokenEmail)
	require.False(t, server.Exists(key))
}

func TestForgotPasswordErrorRateLimitBlocksAtThreshold(t *testing.T) {
	setupRateLimitRedis(t)
	service := newRateLimitAccountService()

	for i := 0; i < 5; i++ {
		service.AddForgotPasswordErrorRateLimit(" USER@example.COM ")
	}

	require.True(t, service.IsForgotPasswordErrorRateLimit("user@example.com"))
}

func TestResetPasswordEmailRateLimitUsesActualEmailKey(t *testing.T) {
	server := setupRateLimitRedis(t)
	service := newRateLimitAccountService()
	ctx := context.Background()

	server.Set("reset_password_rate_limit:other@example.com", "1")
	server.SetTTL("reset_password_rate_limit:other@example.com", time.Minute)

	_, err := service.SendResetPasswordEmail(ctx, nil, " OTHER@example.COM ", "en-US")
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "Too many password reset emails"))
	require.False(t, server.Exists("reset_password_rate_limit:user@example.com"))
}

func TestRateLimiterUsesConsistentZSetKeyAndSecondWindow(t *testing.T) {
	server := setupRateLimitRedis(t)
	limiter := helper.NewRateLimiter("unit", 2, 2)
	ctx := context.Background()

	limited, err := limiter.IsRateLimited(ctx, " ClientA ")
	require.NoError(t, err)
	require.False(t, limited)

	limiter.IncrementRateLimit(" ClientA ")
	limiter.IncrementRateLimit("clienta")

	key := "rate_limit:unit:clienta"
	require.True(t, server.Exists(key))
	members, err := server.ZMembers(key)
	require.NoError(t, err)
	require.Len(t, members, 2)
	require.Positive(t, server.TTL(key))
	require.LessOrEqual(t, server.TTL(key), 2*time.Second)

	limited, err = limiter.IsRateLimited(ctx, "CLIENTA")
	require.NoError(t, err)
	require.True(t, limited)

	server.FastForward(3 * time.Second)
	limited, err = limiter.IsRateLimited(ctx, "clienta")
	require.NoError(t, err)
	require.False(t, limited)
}

func setupRateLimitRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldClient := redisUtil.GetClient()
	redisUtil.SetClient(client)

	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(oldClient)
	})

	return server
}

func newRateLimitAccountService() *auth_service.AccountService {
	return auth_service.NewAccountService(
		nil,
		nil,
		helper.NewTokenManager(),
		nil,
		nil,
		nil,
		nil,
		nil,
		system_service.NewSystemConfigService(),
		nil,
		nil,
	)
}
