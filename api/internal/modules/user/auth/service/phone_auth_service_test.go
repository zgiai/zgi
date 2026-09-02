package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/gorm"
)

func TestPhoneAuthRegistrationFlow(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	accounts := &fakePhoneAuthAccounts{}
	sender := &fakePhoneCodeSender{}
	service := NewPhoneAuthService(accounts, tokenManager, sender, PhoneAuthOptions{AllowRegister: true})

	sendResponse, err := service.SendCode(t.Context(), PhoneCodeSendRequest{
		Phone:       "13800138000",
		CountryCode: "CN",
		Scene:       PhoneSceneRegister,
	})
	require.NoError(t, err)
	require.NotEmpty(t, sendResponse.Token)
	require.Equal(t, "+8613800138000", sender.phoneE164)
	require.Equal(t, PhoneSceneRegister, sender.scene)
	require.Len(t, sender.code, 6)

	verifyResponse, err := service.VerifyCode(t.Context(), PhoneCodeVerifyRequest{
		Phone:       "13800138000",
		CountryCode: "CN",
		Scene:       PhoneSceneRegister,
		Token:       sendResponse.Token,
		Code:        sender.code,
	})
	require.NoError(t, err)
	require.True(t, verifyResponse.IsValid)
	require.NotEmpty(t, verifyResponse.VerifiedToken)

	password := "secret123"
	loginResponse, err := service.RegisterByPhone(t.Context(), PhoneRegisterRequest{
		Phone:         "13800138000",
		CountryCode:   "CN",
		VerifiedToken: verifyResponse.VerifiedToken,
		Name:          "Phone User",
		Password:      &password,
	}, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, "access-token", loginResponse.AccessToken)
	require.Equal(t, "+8613800138000", derefString(accounts.accountByMobile.MobileE164))
	require.Equal(t, "Phone User", accounts.accountByMobile.Name)
	require.Equal(t, "127.0.0.1", accounts.lastLoginIP)
	require.NotNil(t, accounts.createWorkspaceRequired)
	require.True(t, *accounts.createWorkspaceRequired)

	_, err = tokenManager.GetTokenData(verifyResponse.VerifiedToken, PhoneVerifiedTokenType)
	require.Error(t, err)
}

func TestPhoneRegistrationAcceptsValidatedInviteWithoutPersonalWorkspace(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	accounts := &fakePhoneAuthAccounts{}
	sender := &fakePhoneCodeSender{}
	invitation := &fakeRegistrationInvitationGateway{link: &workspace_model.OrganizationInviteLink{
		ID: "phone-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
	}}
	service := NewPhoneAuthService(accounts, tokenManager, sender, PhoneAuthOptions{AllowRegister: true})
	service.SetRegistrationInvitationGateway(invitation)

	sent, err := service.SendCode(t.Context(), PhoneCodeSendRequest{Phone: "13800138000", CountryCode: "CN", Scene: PhoneSceneRegister})
	require.NoError(t, err)
	verified, err := service.VerifyCode(t.Context(), PhoneCodeVerifyRequest{
		Phone: "13800138000", CountryCode: "CN", Scene: PhoneSceneRegister, Token: sent.Token, Code: sender.code,
	})
	require.NoError(t, err)
	password := "secret123"
	result, err := service.RegisterByPhone(t.Context(), PhoneRegisterRequest{
		Phone: "13800138000", CountryCode: "CN", VerifiedToken: verified.VerifiedToken,
		Name: "Invited Phone", Password: &password, InviteToken: "phone-invite",
	}, "127.0.0.1")

	require.NoError(t, err)
	require.NotNil(t, accounts.createWorkspaceRequired)
	require.False(t, *accounts.createWorkspaceRequired)
	require.Equal(t, "new-account", invitation.acceptedAccountID)
	require.NotNil(t, result.Invitation)
	require.Equal(t, "approved", result.Invitation.Status)
}

func TestPhoneInviteRegistrationRejectsUnboundExistingAccount(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	mobile := "+8613800138000"
	accounts := &fakePhoneAuthAccounts{accountByMobile: &auth_model.Account{
		ID: "pre-existing-account", MobileE164: &mobile, Status: auth_model.AccountStatusActive,
	}}
	invitation := &fakeRegistrationInvitationGateway{link: &workspace_model.OrganizationInviteLink{
		ID: "existing-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
	}}
	service := NewPhoneAuthService(accounts, tokenManager, &fakePhoneCodeSender{}, PhoneAuthOptions{AllowRegister: true})
	service.SetRegistrationInvitationGateway(invitation)
	verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": mobile,
		"scene":      PhoneSceneRegister,
	})
	require.NoError(t, err)

	_, err = service.RegisterByPhone(t.Context(), PhoneRegisterRequest{
		Phone: "13800138000", CountryCode: "CN", VerifiedToken: verifiedToken, InviteToken: "phone-invite",
	}, "127.0.0.1")

	require.ErrorIs(t, err, ErrPhoneAccountExists)
	require.Empty(t, accounts.lastLoginIP)
	require.Empty(t, invitation.acceptedAccountID)
}

func TestPhoneInviteRegistrationAcceptanceFailureRemovesCreatedAccount(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	accounts := &fakePhoneAuthAccounts{}
	sender := &fakePhoneCodeSender{}
	invitation := &fakeRegistrationInvitationGateway{
		link: &workspace_model.OrganizationInviteLink{
			ID: "retry-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
		},
		acceptErr: errors.New("database unavailable"),
	}
	service := NewPhoneAuthService(accounts, tokenManager, sender, PhoneAuthOptions{AllowRegister: true})
	service.SetRegistrationInvitationGateway(invitation)
	verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": "+8613800138000",
		"scene":      PhoneSceneRegister,
	})
	require.NoError(t, err)
	req := PhoneRegisterRequest{
		Phone: "13800138000", CountryCode: "CN", VerifiedToken: verifiedToken,
		Name: "Retry Phone", InviteToken: "phone-invite",
	}

	_, err = service.RegisterByPhone(t.Context(), req, "127.0.0.1")
	require.ErrorIs(t, err, ErrRegistrationInvitationAcceptance)
	require.Equal(t, 1, accounts.registerCalls)
	require.Equal(t, 1, accounts.deleteCalls)
	require.Nil(t, accounts.accountByMobile, "failed invite acceptance must not strand the registered phone")
	_, err = tokenManager.GetTokenData(verifiedToken, PhoneVerifiedTokenType)
	require.Error(t, err, "cleanup must revoke the token bound to the removed account")

	invitation.acceptErr = nil
	sent, err := service.SendCode(t.Context(), PhoneCodeSendRequest{
		Phone: "13800138000", CountryCode: "CN", Scene: PhoneSceneRegister,
	})
	require.NoError(t, err, "the compensated phone must be able to start registration again")
	verified, err := service.VerifyCode(t.Context(), PhoneCodeVerifyRequest{
		Phone: "13800138000", CountryCode: "CN", Scene: PhoneSceneRegister,
		Token: sent.Token, Code: sender.code,
	})
	require.NoError(t, err)
	req.VerifiedToken = verified.VerifiedToken
	result, err := service.RegisterByPhone(t.Context(), req, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, 2, accounts.registerCalls, "retry must create a fresh account after compensation")
	require.NotNil(t, result.Invitation)
}

func TestPhoneInviteRegistrationAcceptanceFailureKeepsOriginalAndCleanupErrors(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	cleanupErr := errors.New("database cleanup unavailable")
	accounts := &fakePhoneAuthAccounts{deleteErr: cleanupErr}
	invitation := &fakeRegistrationInvitationGateway{
		link: &workspace_model.OrganizationInviteLink{
			ID: "cleanup-error-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
		},
		acceptErr: errors.New("invite acceptance unavailable"),
	}
	service := NewPhoneAuthService(accounts, tokenManager, &fakePhoneCodeSender{}, PhoneAuthOptions{AllowRegister: true})
	service.SetRegistrationInvitationGateway(invitation)
	verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": "+8613800138000",
		"scene":      PhoneSceneRegister,
	})
	require.NoError(t, err)

	_, err = service.RegisterByPhone(t.Context(), PhoneRegisterRequest{
		Phone: "13800138000", CountryCode: "CN", VerifiedToken: verifiedToken,
		Name: "Cleanup Error", InviteToken: "phone-invite",
	}, "127.0.0.1")

	require.ErrorIs(t, err, ErrRegistrationInvitationAcceptance)
	require.ErrorIs(t, err, cleanupErr)
	require.ErrorContains(t, err, "remove unbound phone registration account")
	require.Equal(t, 1, accounts.deleteCalls)
	require.NotNil(t, accounts.accountByMobile, "failed cleanup must not be reported as a successful removal")
}

func TestPhoneInviteRegistrationLoginFailureRemovesCreatedAccount(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	loginErr := errors.New("login token unavailable")
	accounts := &fakePhoneAuthAccounts{loginErr: loginErr}
	sender := &fakePhoneCodeSender{}
	invitation := &fakeRegistrationInvitationGateway{link: &workspace_model.OrganizationInviteLink{
		ID: "login-failure-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
	}}
	service := NewPhoneAuthService(accounts, tokenManager, sender, PhoneAuthOptions{AllowRegister: true})
	service.SetRegistrationInvitationGateway(invitation)
	verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": "+8613800138000",
		"scene":      PhoneSceneRegister,
	})
	require.NoError(t, err)
	req := PhoneRegisterRequest{
		Phone: "13800138000", CountryCode: "CN", VerifiedToken: verifiedToken,
		Name: "Login Retry", InviteToken: "phone-invite",
	}

	_, err = service.RegisterByPhone(t.Context(), req, "127.0.0.1")
	require.ErrorIs(t, err, loginErr)
	require.Equal(t, 1, accounts.deleteCalls)
	require.Nil(t, accounts.accountByMobile, "failed login must not strand the newly registered invited phone")
	require.Empty(t, invitation.acceptedAccountID, "invite acceptance must not run after login failure")
	_, err = tokenManager.GetTokenData(verifiedToken, PhoneVerifiedTokenType)
	require.Error(t, err, "cleanup must revoke the token bound to the removed account")

	accounts.loginErr = nil
	sent, err := service.SendCode(t.Context(), PhoneCodeSendRequest{
		Phone: "13800138000", CountryCode: "CN", Scene: PhoneSceneRegister,
	})
	require.NoError(t, err, "the compensated phone must be able to start registration again")
	verified, err := service.VerifyCode(t.Context(), PhoneCodeVerifyRequest{
		Phone: "13800138000", CountryCode: "CN", Scene: PhoneSceneRegister,
		Token: sent.Token, Code: sender.code,
	})
	require.NoError(t, err)
	req.VerifiedToken = verified.VerifiedToken
	result, err := service.RegisterByPhone(t.Context(), req, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, 2, accounts.registerCalls)
	require.NotNil(t, result.Invitation)
}

func TestPhoneInviteRegistrationDoesNotCleanAccountCreatedByEarlierRequest(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	mobile := "+8613800138000"
	existingAccount := &auth_model.Account{
		ID: "earlier-request-account", MobileE164: &mobile, Status: auth_model.AccountStatusActive,
	}
	requestRefreshToken, err := tokenManager.GenerateToken(t.Context(), "refresh", existingAccount, nil, nil)
	require.NoError(t, err)
	concurrentRefreshToken, err := tokenManager.GenerateToken(t.Context(), "refresh", existingAccount, nil, nil)
	require.NoError(t, err)
	accounts := &fakePhoneAuthAccounts{
		accountByMobile: existingAccount,
		refreshToken:    requestRefreshToken,
	}
	invitation := &fakeRegistrationInvitationGateway{
		link: &workspace_model.OrganizationInviteLink{
			ID: "earlier-request-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
		},
		acceptErr: errors.New("invite acceptance unavailable"),
	}
	service := NewPhoneAuthService(accounts, tokenManager, &fakePhoneCodeSender{}, PhoneAuthOptions{AllowRegister: true})
	service.SetRegistrationInvitationGateway(invitation)
	verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": mobile,
		"scene":      PhoneSceneRegister,
	})
	require.NoError(t, err)
	require.NoError(t, tokenManager.BindTokenToAccount(
		t.Context(), verifiedToken, PhoneVerifiedTokenType, accounts.accountByMobile.ID,
	))

	_, err = service.RegisterByPhone(t.Context(), PhoneRegisterRequest{
		Phone: "13800138000", CountryCode: "CN", VerifiedToken: verifiedToken,
		Name: "Existing Retry", InviteToken: "phone-invite",
	}, "127.0.0.1")

	require.ErrorIs(t, err, ErrRegistrationInvitationAcceptance)
	require.Zero(t, accounts.deleteCalls, "only an account created by this request may be compensated")
	require.NotNil(t, accounts.accountByMobile)
	_, err = tokenManager.GetTokenData(requestRefreshToken, "refresh")
	require.Error(t, err, "the failed retry request's refresh token must be revoked")
	_, err = tokenManager.GetTokenData(concurrentRefreshToken, "refresh")
	require.NoError(t, err, "a concurrent request's refresh token must remain valid")
}

func TestPhoneInviteRegistrationKeepsAcceptanceErrorWhenRefreshRevocationFails(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	mobile := "+8613800138000"
	account := &auth_model.Account{
		ID: "existing-revoke-error", MobileE164: &mobile, Status: auth_model.AccountStatusActive,
	}
	accounts := &fakePhoneAuthAccounts{
		accountByMobile: account,
		refreshToken:    "request-refresh-token",
		afterLogin: func() {
			require.NoError(t, redisUtil.GetClient().Close())
		},
	}
	invitation := &fakeRegistrationInvitationGateway{
		link: &workspace_model.OrganizationInviteLink{
			ID: "revoke-error-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
		},
		acceptErr: errors.New("invite acceptance unavailable"),
	}
	service := NewPhoneAuthService(accounts, tokenManager, &fakePhoneCodeSender{}, PhoneAuthOptions{AllowRegister: true})
	service.SetRegistrationInvitationGateway(invitation)
	verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": mobile,
		"scene":      PhoneSceneRegister,
	})
	require.NoError(t, err)
	require.NoError(t, tokenManager.BindTokenToAccount(
		t.Context(), verifiedToken, PhoneVerifiedTokenType, account.ID,
	))

	_, err = service.RegisterByPhone(t.Context(), PhoneRegisterRequest{
		Phone: "13800138000", CountryCode: "CN", VerifiedToken: verifiedToken,
		Name: "Existing Revoke Error", InviteToken: "phone-invite",
	}, "127.0.0.1")

	require.ErrorIs(t, err, ErrRegistrationInvitationAcceptance)
	require.ErrorContains(t, err, "revoke phone registration refresh token")
	require.Zero(t, accounts.deleteCalls)
}

func TestPhoneInviteRegistrationCleanupRevokesOnlyIssuedRefreshToken(t *testing.T) {
	testCases := []struct {
		name        string
		accounts    func(*auth_model.Account) *fakePhoneAuthAccounts
		wantCleanup string
	}{
		{
			name: "conditional delete fails",
			accounts: func(account *auth_model.Account) *fakePhoneAuthAccounts {
				return &fakePhoneAuthAccounts{
					accountByMobile: account,
					deleteErr:       errors.New("database cleanup unavailable"),
				}
			},
			wantCleanup: "database cleanup unavailable",
		},
		{
			name: "conditional delete is skipped",
			accounts: func(account *auth_model.Account) *fakePhoneAuthAccounts {
				return &fakePhoneAuthAccounts{accountByMobile: account, skipDelete: true}
			},
			wantCleanup: "account is no longer unbound",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tokenManager := newTestPhoneTokenManager(t)
			mobile := "+8613800138000"
			account := &auth_model.Account{
				ID: "new-account", MobileE164: &mobile, Status: auth_model.AccountStatusActive,
			}
			accounts := testCase.accounts(account)
			service := NewPhoneAuthService(accounts, tokenManager, &fakePhoneCodeSender{}, PhoneAuthOptions{AllowRegister: true})
			verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
				"phone_e164": mobile,
				"scene":      PhoneSceneRegister,
			})
			require.NoError(t, err)
			require.NoError(t, tokenManager.BindTokenToAccount(
				t.Context(), verifiedToken, PhoneVerifiedTokenType, account.ID,
			))
			refreshToken, err := tokenManager.GenerateToken(t.Context(), "refresh", account, nil, nil)
			require.NoError(t, err)
			concurrentRefreshToken, err := tokenManager.GenerateToken(t.Context(), "refresh", account, nil, nil)
			require.NoError(t, err)
			acceptanceErr := fmt.Errorf("%w: invite unavailable", ErrRegistrationInvitationAcceptance)

			err = service.cleanupCreatedInvitedPhoneAccount(
				t.Context(), verifiedToken, refreshToken, account, acceptanceErr,
			)

			require.ErrorIs(t, err, ErrRegistrationInvitationAcceptance)
			require.ErrorContains(t, err, testCase.wantCleanup)
			_, err = tokenManager.GetTokenData(refreshToken, "refresh")
			require.Error(t, err, "this request's refresh token must be revoked even when account deletion is incomplete")
			_, err = tokenManager.GetTokenData(concurrentRefreshToken, "refresh")
			require.NoError(t, err, "compensation must not revoke a concurrent request's refresh token")
			_, err = tokenManager.GetTokenData(verifiedToken, PhoneVerifiedTokenType)
			require.NoError(t, err, "verification token stays bound while the account was not deleted")
		})
	}
}

func TestPhoneInviteRegistrationCompensatesAccountWhenRetryBindingFails(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	requestContext, cancelRequest := context.WithCancel(t.Context())
	accounts := &fakePhoneAuthAccounts{}
	invitation := &fakeRegistrationInvitationGateway{link: &workspace_model.OrganizationInviteLink{
		ID: "compensation-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
	}}
	service := NewPhoneAuthService(accounts, tokenManager, &fakePhoneCodeSender{}, PhoneAuthOptions{AllowRegister: true})
	service.SetRegistrationInvitationGateway(invitation)
	verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": "+8613800138000",
		"scene":      PhoneSceneRegister,
	})
	require.NoError(t, err)
	accounts.afterRegister = func() {
		require.NoError(t, tokenManager.RevokeToken(verifiedToken, PhoneVerifiedTokenType))
		cancelRequest()
	}

	_, err = service.RegisterByPhone(requestContext, PhoneRegisterRequest{
		Phone: "13800138000", CountryCode: "CN", VerifiedToken: verifiedToken,
		Name: "Compensated Phone", InviteToken: "phone-invite",
	}, "127.0.0.1")

	require.ErrorContains(t, err, "bind invited phone registration retry")
	require.Equal(t, 1, accounts.deleteCalls)
	require.NoError(t, accounts.deleteContextErr, "compensation must outlive a canceled request")
	require.Nil(t, accounts.accountByMobile, "failed token binding must not strand the registered phone")

	_, err = service.SendCode(t.Context(), PhoneCodeSendRequest{
		Phone: "13800138000", CountryCode: "CN", Scene: PhoneSceneRegister,
	})
	require.NoError(t, err, "the compensated phone must be able to start registration again")
}

func TestPhoneAuthRegistrationDisabled(t *testing.T) {
	service := NewPhoneAuthService(
		&fakePhoneAuthAccounts{},
		newTestPhoneTokenManager(t),
		&fakePhoneCodeSender{},
		PhoneAuthOptions{},
	)

	_, err := service.SendCode(t.Context(), PhoneCodeSendRequest{
		Phone:       "13800138000",
		CountryCode: "CN",
		Scene:       PhoneSceneRegister,
	})
	require.ErrorIs(t, err, ErrPhoneRegistrationDisabled)
}

func TestPhoneAuthRegisterRejectsVerifiedTokenForAnotherScene(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	service := NewPhoneAuthService(
		&fakePhoneAuthAccounts{},
		tokenManager,
		&fakePhoneCodeSender{},
		PhoneAuthOptions{AllowRegister: true},
	)
	verifiedToken, err := tokenManager.GenerateDataToken(t.Context(), PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": "+8613800138000",
		"scene":      PhoneSceneLogin,
	})
	require.NoError(t, err)

	_, err = service.RegisterByPhone(t.Context(), PhoneRegisterRequest{
		Phone:         "13800138000",
		CountryCode:   "CN",
		VerifiedToken: verifiedToken,
	}, "127.0.0.1")
	require.ErrorIs(t, err, ErrPhoneTokenInvalid)
}

func TestPhoneAuthSendCodeRejectsExistingPhone(t *testing.T) {
	mobile := "+8613800138000"
	service := NewPhoneAuthService(
		&fakePhoneAuthAccounts{accountByMobile: &auth_model.Account{ID: "existing", MobileE164: &mobile}},
		newTestPhoneTokenManager(t),
		&fakePhoneCodeSender{},
		PhoneAuthOptions{AllowRegister: true},
	)

	_, err := service.SendCode(t.Context(), PhoneCodeSendRequest{
		Phone:       "13800138000",
		CountryCode: "CN",
		Scene:       PhoneSceneRegister,
	})
	require.ErrorIs(t, err, ErrPhoneAccountExists)
}

func TestNotificationSMSPhoneCodeSenderUsesRegisterTemplate(t *testing.T) {
	smsService := &fakeNotificationSMSService{}
	sender := NewNotificationSMSPhoneCodeSender(smsService)

	result, err := sender.SendVerificationCode(t.Context(), "+8613800138000", PhoneSceneRegister, "123456")
	require.NoError(t, err)
	require.Equal(t, "message-1", result.RequestID)
	require.Equal(t, notificationsms.ProviderAliyun, result.Provider)
	require.Equal(t, notificationsms.TemplateAuthPhoneRegisterCode, smsService.request.Template)
	require.Equal(t, "123456", smsService.request.TemplateParams[notificationsms.TemplateParamVerificationCode])
}

func newTestPhoneTokenManager(t *testing.T) *helper.TokenManager {
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

type fakePhoneCodeSender struct {
	phoneE164 string
	scene     string
	code      string
	err       error
}

func (f *fakePhoneCodeSender) SendVerificationCode(_ context.Context, phoneE164 string, scene string, code string) (*PhoneCodeSendResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.phoneE164 = phoneE164
	f.scene = scene
	f.code = code
	return &PhoneCodeSendResult{RequestID: "sms-1", Provider: "fake"}, nil
}

type fakePhoneAuthAccounts struct {
	accountByMobile         *auth_model.Account
	lastLoginIP             string
	createWorkspaceRequired *bool
	registerCalls           int
	deleteCalls             int
	deleteContextErr        error
	deleteErr               error
	skipDelete              bool
	loginErr                error
	refreshToken            string
	afterRegister           func()
	afterLogin              func()
}

func (f *fakePhoneAuthAccounts) FindByPhone(_ context.Context, _ string) (*auth_model.Account, error) {
	if f.accountByMobile == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.accountByMobile, nil
}

func (f *fakePhoneAuthAccounts) RegisterByPhone(_ context.Context, phoneE164 string, name string, password *string, createWorkspaceRequired *bool) (*auth_model.Account, error) {
	f.registerCalls++
	if createWorkspaceRequired != nil {
		value := *createWorkspaceRequired
		f.createWorkspaceRequired = &value
	}
	account := &auth_model.Account{
		ID:         "new-account",
		Name:       name,
		Status:     auth_model.AccountStatusActive,
		MobileE164: &phoneE164,
	}
	if password != nil {
		hashedPassword, salt, err := helper.HashPasswordPBKDF2(*password)
		if err != nil {
			return nil, err
		}
		account.Password = &hashedPassword
		account.PasswordSalt = &salt
	}
	f.accountByMobile = account
	if f.afterRegister != nil {
		f.afterRegister()
	}
	return account, nil
}

func (f *fakePhoneAuthAccounts) DeleteUnboundAccountPermanently(ctx context.Context, account *auth_model.Account) (bool, error) {
	f.deleteCalls++
	f.deleteContextErr = ctx.Err()
	if f.deleteErr != nil {
		return false, f.deleteErr
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if f.skipDelete {
		return false, nil
	}
	if f.accountByMobile != nil && account != nil && f.accountByMobile.ID == account.ID {
		f.accountByMobile = nil
		return true, nil
	}
	return false, nil
}

func (f *fakePhoneAuthAccounts) LoginByAccount(_ context.Context, account *auth_model.Account, ipAddress string) (*dto.LoginResponse, error) {
	f.lastLoginIP = ipAddress
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	refreshToken := f.refreshToken
	if refreshToken == "" {
		refreshToken = "refresh-token"
	}
	response := &dto.LoginResponse{
		AccessToken:  "access-token",
		RefreshToken: refreshToken,
		Account: &dto.AccountProfileResponse{
			ID:     account.ID,
			Name:   account.Name,
			Status: string(account.Status),
		},
	}
	if f.afterLogin != nil {
		f.afterLogin()
	}
	return response, nil
}

func (f *fakePhoneAuthAccounts) UpdatePhonePassword(_ context.Context, account *auth_model.Account, password string) error {
	hashedPassword, salt, err := helper.HashPasswordPBKDF2(password)
	if err != nil {
		return err
	}
	account.Password = &hashedPassword
	account.PasswordSalt = &salt
	f.accountByMobile = account
	return nil
}

type fakeNotificationSMSService struct {
	request notificationsms.Request
	err     error
}

func (f *fakeNotificationSMSService) IsEnabled() bool {
	return true
}

func (f *fakeNotificationSMSService) ValidateTemplateParams(string, map[string]string) error {
	return nil
}

func (f *fakeNotificationSMSService) Send(_ context.Context, request notificationsms.Request) (*notificationsms.Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.request = request
	return &notificationsms.Result{
		Provider:  notificationsms.ProviderAliyun,
		Accepted:  true,
		MessageID: "message-1",
	}, nil
}
