package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

const (
	PhoneCodeTokenType     = "phone_code"
	PhoneVerifiedTokenType = "phone_verified"

	PhoneSceneRegister      = "register"
	PhoneSceneLogin         = "login"
	PhoneSceneResetPassword = "reset_password"
)

var (
	ErrPhoneAccountExists        = errors.New("phone account already exists")
	ErrPhoneAccountNotFound      = errors.New("phone account not found")
	ErrPhoneTokenInvalid         = errors.New("phone verification token is invalid")
	ErrPhoneCodeInvalid          = errors.New("phone verification code is invalid")
	ErrPhoneAccountInactive      = errors.New("phone account is not active")
	ErrPhoneSceneUnsupported     = errors.New("phone verification scene is unsupported")
	ErrPhoneInvalid              = errors.New("phone number is invalid")
	ErrPhonePasswordMismatch     = errors.New("phone or password is invalid")
	ErrPhoneRegistrationDisabled = errors.New("phone registration is disabled")
)

type PhoneCodeSendRequest struct {
	Phone       string `json:"phone" binding:"required"`
	CountryCode string `json:"country_code"`
	Scene       string `json:"scene" binding:"required"`
}

type PhoneCheckRequest struct {
	Phone       string `json:"phone" binding:"required"`
	CountryCode string `json:"country_code"`
}

type PhoneCheckResponse struct {
	PhoneE164    string `json:"phone_e164"`
	IsRegistered bool   `json:"is_registered"`
}

type PhoneCodeSendResponse struct {
	Token     string `json:"token"`
	RequestID string `json:"request_id"`
	Provider  string `json:"provider"`
}

type PhoneCodeVerifyRequest struct {
	Phone       string `json:"phone" binding:"required"`
	CountryCode string `json:"country_code"`
	Scene       string `json:"scene" binding:"required"`
	Token       string `json:"token" binding:"required"`
	Code        string `json:"code" binding:"required"`
}

type PhoneCodeVerifyResponse struct {
	IsValid       bool   `json:"is_valid"`
	VerifiedToken string `json:"verified_token"`
	PhoneE164     string `json:"phone_e164"`
}

type PhoneRegisterRequest struct {
	Phone         string  `json:"phone" binding:"required"`
	CountryCode   string  `json:"country_code"`
	VerifiedToken string  `json:"verified_token" binding:"required"`
	Name          string  `json:"name"`
	Password      *string `json:"password,omitempty" binding:"omitempty,min=8"`
}

type PhoneLoginRequest struct {
	Phone         string `json:"phone" binding:"required"`
	CountryCode   string `json:"country_code"`
	VerifiedToken string `json:"verified_token" binding:"required"`
}

type PhonePasswordLoginRequest struct {
	Phone       string `json:"phone" binding:"required"`
	CountryCode string `json:"country_code"`
	Password    string `json:"password" binding:"required,min=8"`
}

type PhoneResetPasswordRequest struct {
	Phone         string `json:"phone" binding:"required"`
	CountryCode   string `json:"country_code"`
	VerifiedToken string `json:"verified_token" binding:"required"`
	NewPassword   string `json:"new_password" binding:"required,min=8"`
}

type PhoneAuthAccountGateway interface {
	FindByPhone(ctx context.Context, phoneE164 string) (*auth_model.Account, error)
	RegisterByPhone(ctx context.Context, phoneE164 string, name string, password *string) (*auth_model.Account, error)
	LoginByAccount(ctx context.Context, account *auth_model.Account, ipAddress string) (*dto.LoginResponse, error)
	UpdatePhonePassword(ctx context.Context, account *auth_model.Account, password string) error
}

type PhoneCodeSendResult struct {
	RequestID string
	Provider  string
}

type PhoneCodeSender interface {
	SendVerificationCode(ctx context.Context, phoneE164 string, scene string, code string) (*PhoneCodeSendResult, error)
}

type PhoneAuthOptions struct {
	AllowRegister          bool
	MasterVerificationCode string
}

type PhoneAuthService struct {
	accounts   PhoneAuthAccountGateway
	tokenMgr   *helper.TokenManager
	codeSender PhoneCodeSender
	options    PhoneAuthOptions
}

func NewPhoneAuthService(accounts PhoneAuthAccountGateway, tokenMgr *helper.TokenManager, codeSender PhoneCodeSender, options PhoneAuthOptions) *PhoneAuthService {
	return &PhoneAuthService{
		accounts:   accounts,
		tokenMgr:   tokenMgr,
		codeSender: codeSender,
		options:    options,
	}
}

func (s *PhoneAuthService) CheckPhone(ctx context.Context, req PhoneCheckRequest) (*PhoneCheckResponse, error) {
	phoneE164, err := normalizePhoneRequest(req.Phone, req.CountryCode)
	if err != nil {
		return nil, err
	}

	account, err := s.accounts.FindByPhone(ctx, phoneE164)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &PhoneCheckResponse{PhoneE164: phoneE164}, nil
		}
		return nil, fmt.Errorf("find account by phone: %w", err)
	}

	return &PhoneCheckResponse{
		PhoneE164:    phoneE164,
		IsRegistered: account != nil,
	}, nil
}

func (s *PhoneAuthService) SendCode(ctx context.Context, req PhoneCodeSendRequest) (*PhoneCodeSendResponse, error) {
	if err := validatePhoneScene(req.Scene); err != nil {
		return nil, err
	}
	if req.Scene == PhoneSceneRegister && !s.options.AllowRegister {
		return nil, ErrPhoneRegistrationDisabled
	}

	phoneE164, err := normalizePhoneRequest(req.Phone, req.CountryCode)
	if err != nil {
		return nil, err
	}

	account, err := s.accounts.FindByPhone(ctx, phoneE164)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find account by phone: %w", err)
	}
	if req.Scene == PhoneSceneRegister && account != nil {
		return nil, ErrPhoneAccountExists
	}
	if req.Scene != PhoneSceneRegister && account == nil {
		return nil, ErrPhoneAccountNotFound
	}

	code := generate6DigitCode()
	token, err := s.tokenMgr.GenerateDataToken(ctx, PhoneCodeTokenType, map[string]interface{}{
		"phone_e164": phoneE164,
		"scene":      req.Scene,
		"code":       code,
	})
	if err != nil {
		return nil, fmt.Errorf("generate phone code token: %w", err)
	}

	result, err := s.codeSender.SendVerificationCode(ctx, phoneE164, req.Scene, code)
	if err != nil {
		_ = s.tokenMgr.RevokeToken(token, PhoneCodeTokenType)
		return nil, fmt.Errorf("send phone verification code: %w", err)
	}

	return &PhoneCodeSendResponse{
		Token:     token,
		RequestID: result.RequestID,
		Provider:  result.Provider,
	}, nil
}

func (s *PhoneAuthService) VerifyCode(ctx context.Context, req PhoneCodeVerifyRequest) (*PhoneCodeVerifyResponse, error) {
	if err := validatePhoneScene(req.Scene); err != nil {
		return nil, err
	}

	phoneE164, err := normalizePhoneRequest(req.Phone, req.CountryCode)
	if err != nil {
		return nil, err
	}

	tokenData, err := s.tokenMgr.GetTokenData(req.Token, PhoneCodeTokenType)
	if err != nil || tokenData == nil {
		return nil, ErrPhoneTokenInvalid
	}
	if tokenExtraString(tokenData.Extra, "phone_e164") != phoneE164 ||
		tokenExtraString(tokenData.Extra, "scene") != req.Scene {
		return nil, ErrPhoneTokenInvalid
	}

	expectedCode := tokenExtraString(tokenData.Extra, "code")
	masterCode := strings.TrimSpace(s.options.MasterVerificationCode)
	if req.Code != expectedCode && (masterCode == "" || req.Code != masterCode) {
		return nil, ErrPhoneCodeInvalid
	}

	verifiedToken, err := s.tokenMgr.GenerateDataToken(ctx, PhoneVerifiedTokenType, map[string]interface{}{
		"phone_e164": phoneE164,
		"scene":      req.Scene,
	})
	if err != nil {
		return nil, fmt.Errorf("generate verified phone token: %w", err)
	}
	_ = s.tokenMgr.RevokeToken(req.Token, PhoneCodeTokenType)

	return &PhoneCodeVerifyResponse{
		IsValid:       true,
		VerifiedToken: verifiedToken,
		PhoneE164:     phoneE164,
	}, nil
}

func (s *PhoneAuthService) RegisterByPhone(ctx context.Context, req PhoneRegisterRequest, ipAddress string) (*dto.LoginResponse, error) {
	if !s.options.AllowRegister {
		return nil, ErrPhoneRegistrationDisabled
	}

	phoneE164, err := s.consumeVerifiedToken(req.Phone, req.CountryCode, req.VerifiedToken, PhoneSceneRegister)
	if err != nil {
		return nil, err
	}

	account, err := s.accounts.FindByPhone(ctx, phoneE164)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("find account by phone: %w", err)
	}
	if account != nil {
		return nil, ErrPhoneAccountExists
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = defaultPhoneAccountName(phoneE164)
	}
	account, err = s.accounts.RegisterByPhone(ctx, phoneE164, name, req.Password)
	if err != nil {
		return nil, fmt.Errorf("register account by phone: %w", err)
	}

	_ = s.tokenMgr.RevokeToken(req.VerifiedToken, PhoneVerifiedTokenType)
	return s.accounts.LoginByAccount(ctx, account, ipAddress)
}

func (s *PhoneAuthService) LoginByPhone(ctx context.Context, req PhoneLoginRequest, ipAddress string) (*dto.LoginResponse, error) {
	phoneE164, err := s.consumeVerifiedToken(req.Phone, req.CountryCode, req.VerifiedToken, PhoneSceneLogin)
	if err != nil {
		return nil, err
	}

	account, err := s.activePhoneAccount(ctx, phoneE164)
	if err != nil {
		return nil, err
	}
	_ = s.tokenMgr.RevokeToken(req.VerifiedToken, PhoneVerifiedTokenType)
	return s.accounts.LoginByAccount(ctx, account, ipAddress)
}

func (s *PhoneAuthService) LoginByPhonePassword(ctx context.Context, req PhonePasswordLoginRequest, ipAddress string) (*dto.LoginResponse, error) {
	phoneE164, err := normalizePhoneRequest(req.Phone, req.CountryCode)
	if err != nil {
		return nil, err
	}

	account, err := s.activePhoneAccount(ctx, phoneE164)
	if err != nil {
		return nil, err
	}
	if account.Password == nil || account.PasswordSalt == nil {
		return nil, ErrPhonePasswordMismatch
	}

	valid, err := helper.ComparePasswordPBKDF2(req.Password, *account.Password, *account.PasswordSalt)
	if err != nil {
		return nil, fmt.Errorf("verify phone password: %w", err)
	}
	if !valid {
		return nil, ErrPhonePasswordMismatch
	}

	return s.accounts.LoginByAccount(ctx, account, ipAddress)
}

func (s *PhoneAuthService) ResetPasswordByPhone(ctx context.Context, req PhoneResetPasswordRequest) error {
	phoneE164, err := s.consumeVerifiedToken(req.Phone, req.CountryCode, req.VerifiedToken, PhoneSceneResetPassword)
	if err != nil {
		return err
	}

	account, err := s.activePhoneAccount(ctx, phoneE164)
	if err != nil {
		return err
	}
	if err := s.accounts.UpdatePhonePassword(ctx, account, req.NewPassword); err != nil {
		return fmt.Errorf("update phone password: %w", err)
	}
	_ = s.tokenMgr.RevokeToken(req.VerifiedToken, PhoneVerifiedTokenType)
	return nil
}

func (s *PhoneAuthService) activePhoneAccount(ctx context.Context, phoneE164 string) (*auth_model.Account, error) {
	account, err := s.accounts.FindByPhone(ctx, phoneE164)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPhoneAccountNotFound
		}
		return nil, fmt.Errorf("find account by phone: %w", err)
	}
	if account == nil {
		return nil, ErrPhoneAccountNotFound
	}
	if account.Status != "" && account.Status != auth_model.AccountStatusActive {
		return nil, fmt.Errorf("%w: %s", ErrPhoneAccountInactive, account.Status)
	}
	return account, nil
}

func (s *PhoneAuthService) consumeVerifiedToken(phone, countryCode, token, scene string) (string, error) {
	phoneE164, err := normalizePhoneRequest(phone, countryCode)
	if err != nil {
		return "", err
	}

	tokenData, err := s.tokenMgr.GetTokenData(token, PhoneVerifiedTokenType)
	if err != nil || tokenData == nil {
		return "", ErrPhoneTokenInvalid
	}
	if tokenExtraString(tokenData.Extra, "phone_e164") != phoneE164 ||
		tokenExtraString(tokenData.Extra, "scene") != scene {
		return "", ErrPhoneTokenInvalid
	}
	return phoneE164, nil
}

func normalizePhoneRequest(phone, countryCode string) (string, error) {
	phoneE164, err := normalizeSSOMobile(phone, countryCode)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPhoneInvalid, err)
	}
	if phoneE164 == "" {
		return "", ErrPhoneInvalid
	}
	return phoneE164, nil
}

func validatePhoneScene(scene string) error {
	switch scene {
	case PhoneSceneRegister, PhoneSceneLogin, PhoneSceneResetPassword:
		return nil
	default:
		return ErrPhoneSceneUnsupported
	}
}

func tokenExtraString(extra map[string]interface{}, key string) string {
	value, ok := extra[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func defaultPhoneAccountName(phoneE164 string) string {
	digits := normalizePhoneDigits(phoneE164, false)
	if len(digits) > 4 {
		return "user-" + digits[len(digits)-4:]
	}
	if digits != "" {
		return "user-" + digits
	}
	return "user"
}

type NotificationSMSPhoneCodeSender struct {
	service notificationsms.Service
}

func NewNotificationSMSPhoneCodeSender(service notificationsms.Service) *NotificationSMSPhoneCodeSender {
	return &NotificationSMSPhoneCodeSender{service: service}
}

func (s *NotificationSMSPhoneCodeSender) SendVerificationCode(ctx context.Context, phoneE164 string, scene string, code string) (*PhoneCodeSendResult, error) {
	template, err := phoneSceneTemplate(scene)
	if err != nil {
		return nil, err
	}
	result, err := s.service.Send(ctx, notificationsms.Request{
		Phone:    phoneE164,
		Template: template,
		TemplateParams: map[string]string{
			notificationsms.TemplateParamVerificationCode: code,
		},
		Source: "auth",
	})
	if err != nil {
		return nil, err
	}
	return &PhoneCodeSendResult{RequestID: result.MessageID, Provider: result.Provider}, nil
}

func phoneSceneTemplate(scene string) (string, error) {
	switch scene {
	case PhoneSceneRegister:
		return notificationsms.TemplateAuthPhoneRegisterCode, nil
	case PhoneSceneLogin:
		return notificationsms.TemplateAuthPhoneLoginCode, nil
	case PhoneSceneResetPassword:
		return notificationsms.TemplateAuthPhoneResetPasswordCode, nil
	default:
		return "", ErrPhoneSceneUnsupported
	}
}

func (s *AccountService) FindByPhone(ctx context.Context, phoneE164 string) (*auth_model.Account, error) {
	return s.accountRepo.GetAccountByNormalizedMobile(ctx, phoneE164)
}

func (s *AccountService) RegisterByPhone(ctx context.Context, phoneE164 string, name string, password *string) (*auth_model.Account, error) {
	language := "zh-Hans"
	createWorkspace := true
	return s.registerExWithMobile(ctx, "", name, password, nil, nil, &language, nil, nil, &createWorkspace, phoneE164)
}

func (s *AccountService) LoginByAccount(_ context.Context, account *auth_model.Account, ipAddress string) (*dto.LoginResponse, error) {
	tokenPair, err := s.LoginCommon(account, ipAddress)
	if err != nil {
		return nil, err
	}

	return &dto.LoginResponse{
		AccessToken:  tokenPair.AccessToken,
		RefreshToken: tokenPair.RefreshToken,
		Account: &dto.AccountProfileResponse{
			ID:                account.ID,
			Name:              account.Name,
			Email:             account.Email,
			Avatar:            derefString(account.Avatar),
			InterfaceLanguage: derefString(account.InterfaceLanguage),
			Timezone:          derefString(account.Timezone),
			Status:            string(account.Status),
			Extension:         account.Extensions,
		},
	}, nil
}

func (s *AccountService) UpdatePhonePassword(ctx context.Context, account *auth_model.Account, password string) error {
	hashedPassword, salt, err := helper.HashPasswordPBKDF2(password)
	if err != nil {
		return fmt.Errorf("hash phone password: %w", err)
	}
	account.Password = &hashedPassword
	account.PasswordSalt = &salt
	return s.accountRepo.UpdateAccount(ctx, account)
}
