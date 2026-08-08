package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestPasswordResetChallengeCannotBypassVerification(t *testing.T) {
	service, _, _ := newPasswordResetSecurityTestService(t)
	challenge := generatePasswordResetChallenge(t, service, "victim@example.com", "123456")

	err := service.ResetPassword(t.Context(), challenge, "NewPassword1")
	require.ErrorContains(t, err, "invalid or expired reset token")
}

func TestPasswordResetVerifiedTokenCompletesOnce(t *testing.T) {
	service, db, _ := newPasswordResetSecurityTestService(t)
	account := createPasswordResetTestAccount(t, db, "victim@example.com", "OldPassword1")
	challenge := generatePasswordResetChallenge(t, service, account.Email, "123456")

	valid, email, verifiedToken, err := service.ValidateResetPasswordToken(
		t.Context(), challenge, account.Email, "123456",
	)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, account.Email, email)
	require.NotEmpty(t, verifiedToken)

	require.NoError(t, service.ResetPassword(t.Context(), verifiedToken, "NewPassword1"))
	updated, err := service.accountRepo.GetAccountByEmail(t.Context(), account.Email)
	require.NoError(t, err)
	require.NotNil(t, updated.Password)
	require.NotNil(t, updated.PasswordSalt)
	matched, err := helper.ComparePasswordPBKDF2("NewPassword1", *updated.Password, *updated.PasswordSalt)
	require.NoError(t, err)
	require.True(t, matched)

	err = service.ResetPassword(t.Context(), verifiedToken, "AnotherPassword1")
	require.ErrorContains(t, err, "invalid or expired reset token")
}

func TestPasswordResetRejectsFrozenAccountWithoutConsumingVerifiedToken(t *testing.T) {
	service, db, _ := newPasswordResetSecurityTestService(t)
	account := createPasswordResetTestAccount(t, db, "frozen@example.com", "OldPassword1")
	account.Status = auth_model.AccountStatusFrozen
	require.NoError(t, db.Save(account).Error)
	challenge := generatePasswordResetChallenge(t, service, account.Email, "123456")
	valid, _, verifiedToken, err := service.ValidateResetPasswordToken(t.Context(), challenge, account.Email, "123456")
	require.NoError(t, err)
	require.True(t, valid)

	err = service.ResetPassword(t.Context(), verifiedToken, "NewPassword1")
	require.ErrorContains(t, err, "frozen")

	account.Status = auth_model.AccountStatusActive
	require.NoError(t, db.Save(account).Error)
	require.NoError(t, service.ResetPassword(t.Context(), verifiedToken, "NewPassword1"))
}

func TestPasswordResetReportsSuccessWhenPostCommitTokenConsumeFails(t *testing.T) {
	service, db, client := newPasswordResetSecurityTestService(t)
	account := createPasswordResetTestAccount(t, db, "consume-failure@example.com", "OldPassword1")
	challenge := generatePasswordResetChallenge(t, service, account.Email, "123456")
	valid, _, verifiedToken, err := service.ValidateResetPasswordToken(t.Context(), challenge, account.Email, "123456")
	require.NoError(t, err)
	require.True(t, valid)
	client.AddHook(&failResetVerifiedTokenConsumeOnceHook{})

	require.NoError(t, service.ResetPassword(t.Context(), verifiedToken, "NewPassword1"))
	updated, err := service.accountRepo.GetAccountByEmail(t.Context(), account.Email)
	require.NoError(t, err)
	matched, err := helper.ComparePasswordPBKDF2("NewPassword1", *updated.Password, *updated.PasswordSalt)
	require.NoError(t, err)
	require.True(t, matched)

	// The failed consume leaves the token reserved, so it still cannot replay.
	require.Error(t, service.ResetPassword(t.Context(), verifiedToken, "AnotherPassword1"))
}

func TestPasswordResetVerificationCanRetryAfterVerifiedTokenRedisFailure(t *testing.T) {
	service, _, client := newPasswordResetSecurityTestService(t)
	challenge := generatePasswordResetChallenge(t, service, "victim@example.com", "123456")
	hook := &failResetVerifiedTokenWriteOnceHook{}
	client.AddHook(hook)

	valid, _, verifiedToken, err := service.ValidateResetPasswordToken(
		t.Context(), challenge, "victim@example.com", "123456",
	)
	require.ErrorContains(t, err, "generate verified reset token")
	require.False(t, valid)
	require.Empty(t, verifiedToken)

	valid, _, verifiedToken, err = service.ValidateResetPasswordToken(
		t.Context(), challenge, "victim@example.com", "123456",
	)
	require.NoError(t, err)
	require.True(t, valid)
	require.NotEmpty(t, verifiedToken)
}

func newPasswordResetSecurityTestService(t *testing.T) (*AccountService, *gorm.DB, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousClient := redisUtil.GetClient()
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(previousClient)
	})

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&auth_model.Account{}))

	service := NewAccountService(
		auth_repo.NewAccountRepository(db),
		db,
		helper.NewTokenManager(),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	return service, db, client
}

func createPasswordResetTestAccount(t *testing.T, db *gorm.DB, email, password string) *auth_model.Account {
	t.Helper()
	hash, salt, err := helper.HashPasswordPBKDF2(password)
	require.NoError(t, err)
	account := &auth_model.Account{
		ID:           uuid.NewString(),
		Name:         "Reset User",
		Email:        email,
		Password:     &hash,
		PasswordSalt: &salt,
		Status:       auth_model.AccountStatusActive,
	}
	require.NoError(t, db.Create(account).Error)
	return account
}

func generatePasswordResetChallenge(t *testing.T, service *AccountService, email, code string) string {
	t.Helper()
	token, err := service.tokenMgr.GenerateToken(
		context.Background(),
		TokenTypeResetPassword,
		nil,
		&email,
		map[string]interface{}{"code": code},
	)
	require.NoError(t, err)
	return token
}

type failResetVerifiedTokenWriteOnceHook struct {
	failed atomic.Bool
}

type failResetVerifiedTokenConsumeOnceHook struct {
	failed atomic.Bool
}

func (h *failResetVerifiedTokenConsumeOnceHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (h *failResetVerifiedTokenConsumeOnceHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		keyMatches := false
		for _, arg := range args[1:] {
			if strings.HasPrefix(fmt.Sprint(arg), "token:"+TokenTypeResetVerified+":") {
				keyMatches = true
				break
			}
		}
		if cmd.Name() == "evalsha" && len(args) > 2 && fmt.Sprint(args[2]) == "1" &&
			keyMatches && !h.failed.Swap(true) {
			return errors.New("injected verified reset token consume failure")
		}
		return next(ctx, cmd)
	}
}

func (h *failResetVerifiedTokenConsumeOnceHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h *failResetVerifiedTokenWriteOnceHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h *failResetVerifiedTokenWriteOnceHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return next
}

func (h *failResetVerifiedTokenWriteOnceHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			args := cmd.Args()
			if cmd.Name() == "setex" && len(args) > 1 &&
				strings.HasPrefix(fmt.Sprint(args[1]), "token:"+TokenTypeResetVerified+":") &&
				!h.failed.Swap(true) {
				return errors.New("injected verified reset token write failure")
			}
		}
		return next(ctx, cmds)
	}
}
