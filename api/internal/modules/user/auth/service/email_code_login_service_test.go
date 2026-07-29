package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/gorm"
)

func TestEmailCodeLoginSendsAndConsumesOneTimeCode(t *testing.T) {
	tokenMgr := newTestEmailCodeLoginTokenManager(t)
	accounts := &fakeEmailCodeLoginAccounts{
		tokenMgr: tokenMgr,
		account: &auth_model.Account{
			ID:     "account-1",
			Email:  "user@example.com",
			Name:   "User",
			Status: auth_model.AccountStatusActive,
		},
	}
	service := NewEmailCodeLoginService(accounts, tokenMgr, EmailCodeLoginOptions{
		Enabled:         true,
		MaxCodeAttempts: 5,
		SendCooldown:    time.Minute,
	})

	sent, err := service.SendCode(t.Context(), EmailCodeLoginSendRequest{
		Email:    " User@Example.com ",
		Language: "zh-CN",
	}, "127.0.0.1")
	require.NoError(t, err)
	require.NotEmpty(t, sent.Token)
	require.Equal(t, "user@example.com", accounts.sentTo)
	require.Equal(t, "zh-Hans", accounts.language)

	_, err = service.VerifyAndLogin(t.Context(), EmailCodeLoginVerifyRequest{
		Email: "other@example.com", Code: accounts.code, Token: sent.Token,
	}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailCodeLoginTokenInvalid)

	login, err := service.VerifyAndLogin(t.Context(), EmailCodeLoginVerifyRequest{
		Email: "user@example.com", Code: accounts.code, Token: sent.Token,
	}, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "access-token", login.AccessToken)
	require.Equal(t, "refresh-token", login.RefreshToken)
	require.Equal(t, "127.0.0.1", accounts.loginIP)

	_, err = service.VerifyAndLogin(t.Context(), EmailCodeLoginVerifyRequest{
		Email: "user@example.com", Code: accounts.code, Token: sent.Token,
	}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailCodeLoginTokenInvalid)
}

func TestEmailCodeLoginLocksTokenAndLimitsSending(t *testing.T) {
	tokenMgr := newTestEmailCodeLoginTokenManager(t)
	accounts := &fakeEmailCodeLoginAccounts{
		tokenMgr: tokenMgr,
		account:  &auth_model.Account{ID: "account-1", Email: "user@example.com", Status: auth_model.AccountStatusActive},
	}
	service := NewEmailCodeLoginService(accounts, tokenMgr, EmailCodeLoginOptions{
		Enabled:         true,
		MaxCodeAttempts: 3,
		SendCooldown:    time.Minute,
	})

	sent, err := service.SendCode(t.Context(), EmailCodeLoginSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.NoError(t, err)
	_, err = service.SendCode(t.Context(), EmailCodeLoginSendRequest{Email: "USER@example.com"}, "127.0.0.2")
	require.ErrorIs(t, err, ErrEmailCodeLoginRateLimited)

	for attempt := 1; attempt < 3; attempt++ {
		_, err = service.VerifyAndLogin(t.Context(), EmailCodeLoginVerifyRequest{
			Email: "user@example.com", Code: "000000", Token: sent.Token,
		}, "127.0.0.1")
		require.ErrorIs(t, err, ErrEmailCodeLoginCodeInvalid)
	}
	_, err = service.VerifyAndLogin(t.Context(), EmailCodeLoginVerifyRequest{
		Email: "user@example.com", Code: "000000", Token: sent.Token,
	}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailCodeLoginRateLimited)
	_, err = service.VerifyAndLogin(t.Context(), EmailCodeLoginVerifyRequest{
		Email: "user@example.com", Code: accounts.code, Token: sent.Token,
	}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailCodeLoginTokenInvalid)
}

func TestEmailCodeLoginDisabledMissingAccountAndSendFailure(t *testing.T) {
	tokenMgr := newTestEmailCodeLoginTokenManager(t)
	accounts := &fakeEmailCodeLoginAccounts{tokenMgr: tokenMgr}
	disabled := NewEmailCodeLoginService(accounts, tokenMgr, EmailCodeLoginOptions{})
	_, err := disabled.SendCode(t.Context(), EmailCodeLoginSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailCodeLoginDisabled)

	enabled := NewEmailCodeLoginService(accounts, tokenMgr, EmailCodeLoginOptions{Enabled: true})
	_, err = enabled.SendCode(t.Context(), EmailCodeLoginSendRequest{Email: "missing@example.com"}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailCodeLoginAccountMissing)

	accounts.account = &auth_model.Account{ID: "account-1", Email: "user@example.com", Status: auth_model.AccountStatusActive}
	accounts.sendErr = errors.New("provider unavailable")
	_, err = enabled.SendCode(t.Context(), EmailCodeLoginSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailCodeLoginSendFailed)
}

func TestEmailCodeLoginRejectsPendingAccount(t *testing.T) {
	tokenMgr := newTestEmailCodeLoginTokenManager(t)
	accounts := &fakeEmailCodeLoginAccounts{
		tokenMgr: tokenMgr,
		account: &auth_model.Account{
			ID: "pending-account", Email: "invited@example.com", Status: auth_model.AccountStatusPending,
		},
	}
	service := NewEmailCodeLoginService(accounts, tokenMgr, EmailCodeLoginOptions{Enabled: true})

	_, err := service.SendCode(t.Context(), EmailCodeLoginSendRequest{Email: "invited@example.com"}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailCodeLoginAccountBlocked)
	require.Empty(t, accounts.sentTo)
}

func TestEmailCodeLoginKeepsCodeWhenSessionCreationFails(t *testing.T) {
	tokenMgr := newTestEmailCodeLoginTokenManager(t)
	accounts := &fakeEmailCodeLoginAccounts{
		tokenMgr: tokenMgr,
		account:  &auth_model.Account{ID: "account-1", Email: "user@example.com", Status: auth_model.AccountStatusActive},
		loginErr: errors.New("redis unavailable"),
	}
	service := NewEmailCodeLoginService(accounts, tokenMgr, EmailCodeLoginOptions{Enabled: true})
	sent, err := service.SendCode(t.Context(), EmailCodeLoginSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.NoError(t, err)

	_, err = service.VerifyAndLogin(t.Context(), EmailCodeLoginVerifyRequest{
		Email: "user@example.com", Code: accounts.code, Token: sent.Token,
	}, "127.0.0.1")
	require.ErrorContains(t, err, "complete email code login")

	accounts.loginErr = nil
	_, err = service.VerifyAndLogin(t.Context(), EmailCodeLoginVerifyRequest{
		Email: "user@example.com", Code: accounts.code, Token: sent.Token,
	}, "127.0.0.1")
	require.NoError(t, err)
}

func newTestEmailCodeLoginTokenManager(t *testing.T) *helper.TokenManager {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	return helper.NewTokenManager()
}

type fakeEmailCodeLoginAccounts struct {
	tokenMgr *helper.TokenManager
	account  *auth_model.Account
	sentTo   string
	language string
	code     string
	sendErr  error
	loginIP  string
	loginErr error
}

func (f *fakeEmailCodeLoginAccounts) GetUserThroughEmail(_ context.Context, email string) (*auth_model.Account, error) {
	if f.account == nil || f.account.Email != email {
		return nil, gorm.ErrRecordNotFound
	}
	return f.account, nil
}

func (f *fakeEmailCodeLoginAccounts) SendEmailCodeLoginEmail(ctx context.Context, account *auth_model.Account, email, language string) (string, error) {
	if f.sendErr != nil {
		return "", f.sendErr
	}
	f.sentTo = email
	f.language = language
	f.code = "123456"
	return f.tokenMgr.GenerateToken(ctx, EmailCodeLoginTokenType, account, &email, map[string]interface{}{"code": f.code})
}

func (f *fakeEmailCodeLoginAccounts) IsEmailSendIPLimit(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakeEmailCodeLoginAccounts) LoginCommon(_ *auth_model.Account, ipAddress string) (*auth_model.TokenPair, error) {
	f.loginIP = ipAddress
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	return &auth_model.TokenPair{AccessToken: "access-token", RefreshToken: "refresh-token"}, nil
}
