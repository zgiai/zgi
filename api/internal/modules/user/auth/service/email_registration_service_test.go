package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
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
	require.NotNil(t, accounts.createWorkspaceRequired)
	require.False(t, *accounts.createWorkspaceRequired)
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

func TestEmailRegistrationSendFailsClosedWhenAccountLookupFails(t *testing.T) {
	accounts := &fakeEmailRegistrationAccounts{existsErr: errors.New("database unavailable")}
	sender := &fakeEmailRegistrationSender{}
	service := NewEmailRegistrationService(
		accounts,
		newTestEmailRegistrationTokenManager(t),
		sender,
		EmailRegistrationOptions{AllowRegister: true},
	)

	_, err := service.SendCode(
		t.Context(),
		EmailRegistrationSendRequest{Email: "user@example.com"},
		"127.0.0.1",
	)
	require.ErrorContains(t, err, "check existing registration account")
	require.Empty(t, sender.to)
}

func TestEmailRegistrationFinishFailsClosedWhenAccountLookupFails(t *testing.T) {
	tokenManager := newTestEmailRegistrationTokenManager(t)
	accounts := &fakeEmailRegistrationAccounts{existsErr: errors.New("database unavailable")}
	service := NewEmailRegistrationService(
		accounts,
		tokenManager,
		&fakeEmailRegistrationSender{},
		EmailRegistrationOptions{AllowRegister: true},
	)
	verifiedToken, err := tokenManager.GenerateDataToken(
		t.Context(),
		EmailRegistrationVerifiedTokenType,
		map[string]interface{}{
			"registration_email": "user@example.com",
			"language":           "en-US",
			"code":               "verified",
		},
	)
	require.NoError(t, err)

	_, err = service.Finish(t.Context(), EmailRegistrationFinishRequest{
		Token:           verifiedToken,
		Name:            "User",
		Password:        "secret123",
		PasswordConfirm: "secret123",
	}, "127.0.0.1")
	require.ErrorContains(t, err, "check registration account before finish")
	require.Zero(t, accounts.registerCalls)
	require.Empty(t, accounts.loginIP)
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

func TestEmailRegistrationReleasesChallengeWhenVerifiedTokenIssuanceFails(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx, cancel := context.WithCancel(context.Background())
	client.AddHook(&failRedisCommandOnceHook{
		command:   "setex",
		keyPrefix: "token:" + EmailRegistrationVerifiedTokenType + ":",
		onFailure: cancel,
	})
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})

	tokenManager := helper.NewTokenManager()
	accounts := &fakeEmailRegistrationAccounts{}
	sender := &fakeEmailRegistrationSender{}
	service := NewEmailRegistrationService(accounts, tokenManager, sender, EmailRegistrationOptions{AllowRegister: true})

	sent, err := service.SendCode(t.Context(), EmailRegistrationSendRequest{Email: "user@example.com"}, "127.0.0.1")
	require.NoError(t, err)
	request := EmailRegistrationVerifyRequest{Email: "user@example.com", Code: sender.code, Token: sent.Token}

	_, err = service.VerifyCode(ctx, request)
	require.ErrorContains(t, err, "generate verified email registration token")
	require.ErrorIs(t, ctx.Err(), context.Canceled)

	verified, err := service.VerifyCode(t.Context(), request)
	require.NoError(t, err)
	require.NotEmpty(t, verified.Token)
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
	existsErr                 error
	limited                   bool
	limitErr                  error
	registerCalls             int
	registeredEmail           string
	loginIP                   string
	registerErr               error
	registerErrCreatesAccount bool
	loginErr                  error
	createWorkspaceRequired   *bool
}

type failRedisCommandOnceHook struct {
	command   string
	keyPrefix string
	onFailure func()
	failed    atomic.Bool
}

func (h *failRedisCommandOnceHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h *failRedisCommandOnceHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if cmd.Name() == h.command && len(args) > 1 &&
			strings.HasPrefix(fmt.Sprint(args[1]), h.keyPrefix) &&
			!h.failed.Swap(true) {
			if h.onFailure != nil {
				h.onFailure()
			}
			return errors.New("injected verified token write failure")
		}
		return next(ctx, cmd)
	}
}

func (h *failRedisCommandOnceHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		return next(ctx, cmds)
	}
}

func (f *fakeEmailRegistrationAccounts) ExistsByEmail(_ context.Context, email string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.existingEmail == email, nil
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
	createWorkspaceRequired *bool,
) (*auth_model.Account, error) {
	f.registerCalls++
	if createWorkspaceRequired != nil {
		value := *createWorkspaceRequired
		f.createWorkspaceRequired = &value
	}
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
