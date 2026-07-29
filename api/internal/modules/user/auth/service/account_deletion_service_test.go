package service

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/config"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestAccountDeletionCodeIsBoundToAuthenticatedAccount(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})

	previousConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{Server: config.ServerConfig{Mode: "release", Environment: "production"}}
	t.Cleanup(func() { config.GlobalConfig = previousConfig })

	service := &AccountService{tokenMgr: helper.NewTokenManager()}
	account := &auth_model.Account{ID: "account-1", Email: "user@example.com"}
	token, code, err := service.GenerateAccountDeletionVerificationCode(t.Context(), account)
	require.NoError(t, err)

	valid, err := service.VerifyAccountDeletionCode(t.Context(), "account-2", token, code)
	require.Error(t, err)
	require.False(t, valid)

	valid, err = service.VerifyAccountDeletionCode(t.Context(), account.ID, token, code)
	require.NoError(t, err)
	require.True(t, valid)

	valid, err = service.VerifyAccountDeletionCode(t.Context(), account.ID, token, code)
	require.Error(t, err)
	require.False(t, valid)
}
