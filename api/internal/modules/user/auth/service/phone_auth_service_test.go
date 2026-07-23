package service

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
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

	_, err = tokenManager.GetTokenData(verifyResponse.VerifiedToken, PhoneVerifiedTokenType)
	require.Error(t, err)
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
	accountByMobile *auth_model.Account
	lastLoginIP     string
}

func (f *fakePhoneAuthAccounts) FindByPhone(_ context.Context, _ string) (*auth_model.Account, error) {
	if f.accountByMobile == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return f.accountByMobile, nil
}

func (f *fakePhoneAuthAccounts) RegisterByPhone(_ context.Context, phoneE164 string, name string, password *string) (*auth_model.Account, error) {
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
	return account, nil
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
