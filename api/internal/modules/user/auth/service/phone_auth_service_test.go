package service

import (
	"context"
	"errors"
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

func TestPhoneInviteRegistrationRetryRequiresBoundCreatedAccount(t *testing.T) {
	tokenManager := newTestPhoneTokenManager(t)
	accounts := &fakePhoneAuthAccounts{}
	invitation := &fakeRegistrationInvitationGateway{
		link: &workspace_model.OrganizationInviteLink{
			ID: "retry-link", OrganizationID: "organization-phone", Token: "phone-invite", Status: "active",
		},
		acceptErr: errors.New("database unavailable"),
	}
	service := NewPhoneAuthService(accounts, tokenManager, &fakePhoneCodeSender{}, PhoneAuthOptions{AllowRegister: true})
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
	bound, err := tokenManager.GetTokenData(verifiedToken, PhoneVerifiedTokenType)
	require.NoError(t, err)
	require.NotNil(t, bound.AccountID)
	require.Equal(t, "new-account", *bound.AccountID)

	invitation.acceptErr = nil
	result, err := service.RegisterByPhone(t.Context(), req, "127.0.0.1")
	require.NoError(t, err)
	require.Equal(t, 1, accounts.registerCalls, "retry must reuse only the account bound to this verified token")
	require.NotNil(t, result.Invitation)
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
	afterRegister           func()
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

func (f *fakePhoneAuthAccounts) DeleteAccountPermanently(ctx context.Context, account *auth_model.Account) error {
	f.deleteCalls++
	f.deleteContextErr = ctx.Err()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.accountByMobile != nil && account != nil && f.accountByMobile.ID == account.ID {
		f.accountByMobile = nil
	}
	return nil
}

func (f *fakePhoneAuthAccounts) LoginByAccount(_ context.Context, account *auth_model.Account, ipAddress string) (*dto.LoginResponse, error) {
	f.lastLoginIP = ipAddress
	return &dto.LoginResponse{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		Account: &dto.AccountProfileResponse{
			ID:     account.ID,
			Name:   account.Name,
			Status: string(account.Status),
		},
	}, nil
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
