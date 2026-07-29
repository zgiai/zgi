package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestEmailRegistrationRequiresVerifiedOneTimeToken(t *testing.T) {
	tokenManager := newTestEmailRegistrationTokenManager(t)
	accounts := &fakeEmailRegistrationAccounts{}
	sender := &fakeEmailRegistrationSender{}
	service := NewEmailRegistrationService(accounts, tokenManager, sender, EmailRegistrationOptions{
		AllowRegister:   true,
		MaxCodeAttempts: 5,
	})

	sendResponse, err := service.SendCode(t.Context(), EmailRegistrationSendRequest{
		Email:    " User@Example.com ",
		Language: "zh-Hans",
	}, "127.0.0.1")
	require.NoError(t, err)
	require.NotEmpty(t, sendResponse.Token)
	require.Equal(t, "user@example.com", sender.to)
	require.Len(t, sender.code, 6)
	require.NotEmpty(t, sender.idempotencyKey)

	_, err = service.Finish(t.Context(), EmailRegistrationFinishRequest{
		Token:           sendResponse.Token,
		Name:            "User",
		Password:        "secret123",
		PasswordConfirm: "secret123",
	}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailRegistrationTokenInvalid)
	require.Zero(t, accounts.registerCalls)

	verifyResponse, err := service.VerifyCode(t.Context(), EmailRegistrationVerifyRequest{
		Email: "user@example.com",
		Code:  sender.code,
		Token: sendResponse.Token,
	})
	require.NoError(t, err)
	require.True(t, verifyResponse.IsValid)
	require.NotEmpty(t, verifyResponse.Token)
	require.NotEqual(t, sendResponse.Token, verifyResponse.Token)

	_, err = service.VerifyCode(t.Context(), EmailRegistrationVerifyRequest{
		Email: "user@example.com",
		Code:  sender.code,
		Token: sendResponse.Token,
	})
	require.ErrorIs(t, err, ErrEmailRegistrationTokenInvalid)

	loginResponse, err := service.Finish(t.Context(), EmailRegistrationFinishRequest{
		Email:           "user@example.com",
		Token:           verifyResponse.Token,
		Name:            "User",
		Password:        "secret123",
		PasswordConfirm: "secret123",
	}, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "access-token", loginResponse.AccessToken)
	require.Equal(t, 1, accounts.registerCalls)
	require.Equal(t, "user@example.com", accounts.registeredEmail)
	require.Equal(t, "127.0.0.1", accounts.loginIP)

	_, err = service.Finish(t.Context(), EmailRegistrationFinishRequest{
		Token:           verifyResponse.Token,
		Name:            "User",
		Password:        "secret123",
		PasswordConfirm: "secret123",
	}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailRegistrationTokenInvalid)
	require.Equal(t, 1, accounts.registerCalls)
}

func TestEmailRegistrationLocksChallengeAfterMaximumAttempts(t *testing.T) {
	service, sender := newTestEmailRegistrationService(t, EmailRegistrationOptions{
		AllowRegister:   true,
		MaxCodeAttempts: 3,
	})
	sendResponse, err := service.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.NoError(t, err)

	for attempt := 1; attempt <= 2; attempt++ {
		_, err = service.VerifyCode(t.Context(), EmailRegistrationVerifyRequest{
			Email: "user@example.com", Code: "000000", Token: sendResponse.Token,
		})
		require.ErrorIs(t, err, ErrEmailRegistrationCodeInvalid)
	}
	_, err = service.VerifyCode(t.Context(), EmailRegistrationVerifyRequest{
		Email: "user@example.com", Code: "000000", Token: sendResponse.Token,
	})
	require.ErrorIs(t, err, ErrEmailRegistrationRateLimited)

	_, err = service.VerifyCode(t.Context(), EmailRegistrationVerifyRequest{
		Email: "user@example.com", Code: sender.code, Token: sendResponse.Token,
	})
	require.ErrorIs(t, err, ErrEmailRegistrationTokenInvalid)
}

func TestEmailRegistrationDisabledAndSendFailure(t *testing.T) {
	tokenManager := newTestEmailRegistrationTokenManager(t)
	accounts := &fakeEmailRegistrationAccounts{}
	disabled := NewEmailRegistrationService(accounts, tokenManager, &fakeEmailRegistrationSender{}, EmailRegistrationOptions{})

	_, err := disabled.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailRegistrationDisabled)

	sender := &fakeEmailRegistrationSender{err: errors.New("provider unavailable")}
	enabled := NewEmailRegistrationService(accounts, tokenManager, sender, EmailRegistrationOptions{AllowRegister: true})
	_, err = enabled.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailRegistrationSendFailed)
}

func TestEmailRegistrationSendCooldown(t *testing.T) {
	service, _ := newTestEmailRegistrationService(t, EmailRegistrationOptions{
		AllowRegister: true,
		SendCooldown:  time.Minute,
	})
	_, err := service.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.NoError(t, err)

	_, err = service.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "USER@example.com"}, "127.0.0.2")
	require.ErrorIs(t, err, ErrEmailRegistrationRateLimited)
}

func TestEmailRegistrationKeepsVerifiedTokenWhenRegistrationFails(t *testing.T) {
	tokenManager := newTestEmailRegistrationTokenManager(t)
	accounts := &fakeEmailRegistrationAccounts{registerErr: errors.New("database unavailable")}
	sender := &fakeEmailRegistrationSender{}
	service := NewEmailRegistrationService(accounts, tokenManager, sender, EmailRegistrationOptions{AllowRegister: true})

	sent, err := service.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.NoError(t, err)
	verified, err := service.VerifyCode(t.Context(), EmailRegistrationVerifyRequest{
		Email: "user@example.com", Code: sender.code, Token: sent.Token,
	})
	require.NoError(t, err)

	request := EmailRegistrationFinishRequest{
		Token: verified.Token, Name: "User", Password: "secret123", PasswordConfirm: "secret123",
	}
	_, err = service.Finish(t.Context(), request, "127.0.0.1")
	require.ErrorContains(t, err, "database unavailable")

	accounts.registerErr = nil
	_, err = service.Finish(t.Context(), request, "127.0.0.1")
	require.NoError(t, err)
}

func TestEmailRegistrationKeepsVerifiedTokenWhenAutomaticLoginFails(t *testing.T) {
	tokenManager := newTestEmailRegistrationTokenManager(t)
	accounts := &fakeEmailRegistrationAccounts{loginErr: errors.New("session unavailable")}
	sender := &fakeEmailRegistrationSender{}
	service := NewEmailRegistrationService(accounts, tokenManager, sender, EmailRegistrationOptions{AllowRegister: true})

	sent, err := service.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.NoError(t, err)
	verified, err := service.VerifyCode(t.Context(), EmailRegistrationVerifyRequest{
		Email: "user@example.com", Code: sender.code, Token: sent.Token,
	})
	require.NoError(t, err)
	request := EmailRegistrationFinishRequest{
		Token: verified.Token, Name: "User", Password: "secret123", PasswordConfirm: "secret123",
	}

	_, err = service.Finish(t.Context(), request, "127.0.0.1")
	require.ErrorContains(t, err, "session unavailable")
	accounts.loginErr = nil
	_, err = service.Finish(t.Context(), request, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, 1, accounts.registerCalls)
}

func TestEmailRegistrationRejectsShortPasswordAtServiceLayer(t *testing.T) {
	service, _ := newTestEmailRegistrationService(t, EmailRegistrationOptions{AllowRegister: true})
	_, err := service.Finish(t.Context(), EmailRegistrationFinishRequest{
		Token: "unused", Name: "User", Password: "short", PasswordConfirm: "short",
	}, "127.0.0.1")
	require.ErrorIs(t, err, ErrEmailRegistrationPasswordTooShort)
}

func TestEmailRegistrationDoesNotHidePostCreateRegistrationFailure(t *testing.T) {
	tokenManager := newTestEmailRegistrationTokenManager(t)
	accounts := &fakeEmailRegistrationAccounts{
		registerErr:               errors.New("initialize account workspace context: database unavailable"),
		registerErrCreatesAccount: true,
	}
	sender := &fakeEmailRegistrationSender{}
	service := NewEmailRegistrationService(accounts, tokenManager, sender, EmailRegistrationOptions{AllowRegister: true})

	sent, err := service.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.NoError(t, err)
	verified, err := service.VerifyCode(t.Context(), EmailRegistrationVerifyRequest{
		Email: "user@example.com", Code: sender.code, Token: sent.Token,
	})
	require.NoError(t, err)

	_, err = service.Finish(t.Context(), EmailRegistrationFinishRequest{
		Token: verified.Token, Name: "User", Password: "secret123", PasswordConfirm: "secret123",
	}, "127.0.0.1")
	require.ErrorContains(t, err, "initialize account workspace context")
	require.Empty(t, accounts.loginIP)
}

func newTestEmailRegistrationService(t *testing.T, options EmailRegistrationOptions) (*EmailRegistrationService, *fakeEmailRegistrationSender) {
	t.Helper()
	sender := &fakeEmailRegistrationSender{}
	return NewEmailRegistrationService(
		&fakeEmailRegistrationAccounts{},
		newTestEmailRegistrationTokenManager(t),
		sender,
		options,
	), sender
}

func newTestEmailRegistrationTokenManager(t *testing.T) *helper.TokenManager {
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

type fakeEmailRegistrationSender struct {
	language       string
	to             string
	code           string
	idempotencyKey string
	err            error
}

func (f *fakeEmailRegistrationSender) SendRegistrationCode(
	_ context.Context,
	language, to, code, idempotencyKey string,
) error {
	f.language = language
	f.to = to
	f.code = code
	f.idempotencyKey = idempotencyKey
	return f.err
}

type fakeEmailRegistrationAccounts struct {
	existingEmail             string
	limited                   bool
	limitErr                  error
	registerCalls             int
	registeredEmail           string
	loginIP                   string
	registerErr               error
	registerErrCreatesAccount bool
	loginErr                  error
}

func (f *fakeEmailRegistrationAccounts) ExistsByEmail(_ context.Context, email string) bool {
	return f.existingEmail == email
}

func (f *fakeEmailRegistrationAccounts) IsEmailSendIPLimit(_ context.Context, _ string) (bool, error) {
	return f.limited, f.limitErr
}

func (f *fakeEmailRegistrationAccounts) RegisterEx(
	_ context.Context,
	email string,
	name string,
	_ *string,
	_ *string,
	_ *string,
	_ *string,
	_ *auth_model.AccountStatus,
	_ *bool,
	_ *bool,
) (*auth_model.Account, error) {
	f.registerCalls++
	if f.registerErr != nil {
		if f.registerErrCreatesAccount {
			f.existingEmail = email
		}
		return nil, f.registerErr
	}
	f.registeredEmail = email
	f.existingEmail = email
	return &auth_model.Account{ID: "account-1", Email: email, Name: name}, nil
}

func (f *fakeEmailRegistrationAccounts) Login(
	_ context.Context,
	req *shared_dto.LoginReq,
) (*auth_model.TokenPair, error, shared_dto.LoginResponse, helper.ErrorResponse) {
	f.loginIP = req.LastLoginIp
	if f.loginErr != nil {
		return nil, f.loginErr, shared_dto.LoginResponse{}, helper.ErrorResponse{}
	}
	return &auth_model.TokenPair{AccessToken: "access-token"}, nil, shared_dto.LoginResponse{
		AccessToken: "access-token",
	}, helper.ErrorResponse{}
}
