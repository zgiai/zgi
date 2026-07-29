package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/email"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

const (
	EmailRegistrationChallengeTokenType = "email_registration_challenge"
	EmailRegistrationVerifiedTokenType  = "email_registration_verified"

	defaultEmailRegistrationMaxCodeAttempts = 5
	defaultEmailRegistrationSendCooldown    = time.Minute
)

var (
	ErrEmailRegistrationDisabled         = errors.New("email registration is disabled")
	ErrEmailRegistrationAccountExists    = errors.New("email account already exists")
	ErrEmailRegistrationTokenInvalid     = errors.New("email registration token is invalid")
	ErrEmailRegistrationCodeInvalid      = errors.New("email registration verification code is invalid")
	ErrEmailRegistrationRateLimited      = errors.New("email registration rate limit exceeded")
	ErrEmailRegistrationSendFailed       = errors.New("email registration message could not be sent")
	ErrEmailRegistrationPasswordMismatch = errors.New("email registration passwords do not match")
	ErrEmailRegistrationAccountFrozen    = errors.New("email registration account is frozen")
)

type EmailRegistrationSendRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Language string `json:"language"`
}

type EmailRegistrationSendResponse struct {
	Result string `json:"result"`
	Token  string `json:"data"`
}

type EmailRegistrationVerifyRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required"`
	Token string `json:"token" binding:"required"`
}

type EmailRegistrationVerifyResponse struct {
	IsValid bool   `json:"is_valid"`
	Email   string `json:"email"`
	Token   string `json:"token"`
}

type EmailRegistrationFinishRequest struct {
	Email           string `json:"email" binding:"omitempty,email"`
	Token           string `json:"token" binding:"required"`
	Name            string `json:"name" binding:"required"`
	Password        string `json:"password" binding:"required"`
	PasswordConfirm string `json:"password_confirm" binding:"required"`
}

type EmailRegistrationAccountGateway interface {
	ExistsByEmail(ctx context.Context, email string) bool
	IsEmailSendIPLimit(ctx context.Context, ipAddress string) (bool, error)
	RegisterEx(
		ctx context.Context,
		email string,
		name string,
		password *string,
		openID *string,
		provider *string,
		language *string,
		status *auth_model.AccountStatus,
		isSetup *bool,
		createWorkspaceRequired *bool,
	) (*auth_model.Account, error)
	Login(ctx context.Context, req *shared_dto.LoginReq) (*auth_model.TokenPair, error, shared_dto.LoginResponse, helper.ErrorResponse)
}

type EmailRegistrationCodeSender interface {
	SendRegistrationCode(ctx context.Context, language, to, code, idempotencyKey string) error
}

type EmailRegistrationOptions struct {
	AllowRegister               bool
	MasterVerificationCode      string
	AllowMasterVerificationCode bool
	MaxCodeAttempts             int
	SendCooldown                time.Duration
}

type EmailRegistrationService struct {
	accounts    EmailRegistrationAccountGateway
	tokenMgr    *helper.TokenManager
	codeSender  EmailRegistrationCodeSender
	rateLimiter *RedisEmailRegistrationRateLimiter
	options     EmailRegistrationOptions
}

func NewEmailRegistrationService(
	accounts EmailRegistrationAccountGateway,
	tokenMgr *helper.TokenManager,
	codeSender EmailRegistrationCodeSender,
	options EmailRegistrationOptions,
) *EmailRegistrationService {
	if options.MaxCodeAttempts <= 0 {
		options.MaxCodeAttempts = defaultEmailRegistrationMaxCodeAttempts
	}
	if options.SendCooldown <= 0 {
		options.SendCooldown = defaultEmailRegistrationSendCooldown
	}
	return &EmailRegistrationService{
		accounts:    accounts,
		tokenMgr:    tokenMgr,
		codeSender:  codeSender,
		rateLimiter: NewRedisEmailRegistrationRateLimiter(options.SendCooldown),
		options:     options,
	}
}

func (s *EmailRegistrationService) SendCode(
	ctx context.Context,
	req EmailRegistrationSendRequest,
	ipAddress string,
) (*EmailRegistrationSendResponse, error) {
	if !s.options.AllowRegister {
		return nil, ErrEmailRegistrationDisabled
	}

	normalizedEmail := normalizeRegistrationEmail(req.Email)
	if normalizedEmail == "" {
		return nil, ErrEmailRegistrationTokenInvalid
	}
	if limited, err := s.accounts.IsEmailSendIPLimit(ctx, ipAddress); err != nil || limited {
		return nil, ErrEmailRegistrationRateLimited
	}
	if s.accounts.ExistsByEmail(ctx, normalizedEmail) {
		return nil, ErrEmailRegistrationAccountExists
	}
	acquired, err := s.rateLimiter.Acquire(ctx, normalizedEmail)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmailRegistrationRateLimited, err)
	}
	if !acquired {
		return nil, ErrEmailRegistrationRateLimited
	}
	releaseRateLimit := true
	defer func() {
		if releaseRateLimit {
			_ = s.rateLimiter.Release(context.WithoutCancel(ctx), normalizedEmail)
		}
	}()

	code, err := generate6DigitCode()
	if err != nil {
		return nil, fmt.Errorf("generate email registration code: %w", err)
	}
	language := normalizeRegistrationLanguage(req.Language)
	token, err := s.tokenMgr.GenerateDataToken(ctx, EmailRegistrationChallengeTokenType, map[string]interface{}{
		"registration_email": normalizedEmail,
		"code":               code,
		"language":           language,
	})
	if err != nil {
		return nil, fmt.Errorf("generate email registration challenge: %w", err)
	}

	idempotencyKey := fmt.Sprintf("register:%x", sha256.Sum256([]byte(token)))
	if err := s.codeSender.SendRegistrationCode(ctx, language, normalizedEmail, code, idempotencyKey); err != nil {
		_ = s.tokenMgr.RevokeToken(token, EmailRegistrationChallengeTokenType)
		return nil, fmt.Errorf("%w: %v", ErrEmailRegistrationSendFailed, err)
	}
	releaseRateLimit = false

	return &EmailRegistrationSendResponse{Result: "success", Token: token}, nil
}

type RedisEmailRegistrationRateLimiter struct {
	cooldown time.Duration
}

func NewRedisEmailRegistrationRateLimiter(cooldown time.Duration) *RedisEmailRegistrationRateLimiter {
	return &RedisEmailRegistrationRateLimiter{cooldown: cooldown}
}

func (l *RedisEmailRegistrationRateLimiter) Acquire(ctx context.Context, emailAddress string) (bool, error) {
	key := l.key(emailAddress)
	acquired, err := redisUtil.GetClient().SetNX(ctx, key, "1", l.cooldown).Result()
	if err != nil {
		return false, fmt.Errorf("acquire email registration send cooldown: %w", err)
	}
	return acquired, nil
}

func (l *RedisEmailRegistrationRateLimiter) Release(ctx context.Context, emailAddress string) error {
	if err := redisUtil.GetClient().Del(ctx, l.key(emailAddress)).Err(); err != nil {
		return fmt.Errorf("release email registration send cooldown: %w", err)
	}
	return nil
}

func (l *RedisEmailRegistrationRateLimiter) key(emailAddress string) string {
	digest := sha256.Sum256([]byte(normalizeRegistrationEmail(emailAddress)))
	return fmt.Sprintf("email_registration_send_cooldown:%x", digest)
}

func (s *EmailRegistrationService) VerifyCode(
	ctx context.Context,
	req EmailRegistrationVerifyRequest,
) (*EmailRegistrationVerifyResponse, error) {
	if !s.options.AllowRegister {
		return nil, ErrEmailRegistrationDisabled
	}

	normalizedEmail := normalizeRegistrationEmail(req.Email)
	tokenData, err := s.tokenMgr.GetTokenData(req.Token, EmailRegistrationChallengeTokenType)
	if err != nil || tokenData == nil {
		return nil, ErrEmailRegistrationTokenInvalid
	}
	if tokenExtraString(tokenData.Extra, "registration_email") != normalizedEmail {
		return nil, ErrEmailRegistrationTokenInvalid
	}

	expectedCode := tokenExtraString(tokenData.Extra, "code")
	masterCode := ""
	if s.options.AllowMasterVerificationCode {
		masterCode = strings.TrimSpace(s.options.MasterVerificationCode)
	}
	if req.Code != expectedCode && (masterCode == "" || req.Code != masterCode) {
		attempts, incrementErr := s.tokenMgr.IncrementTokenUsage(
			ctx,
			req.Token,
			EmailRegistrationChallengeTokenType,
			10*time.Minute,
		)
		if incrementErr != nil {
			return nil, fmt.Errorf("track email registration verification attempt: %w", incrementErr)
		}
		if attempts >= int64(s.options.MaxCodeAttempts) {
			_ = s.tokenMgr.RevokeToken(req.Token, EmailRegistrationChallengeTokenType)
			return nil, ErrEmailRegistrationRateLimited
		}
		return nil, ErrEmailRegistrationCodeInvalid
	}
	consumedTokenData, err := s.tokenMgr.ConsumeTokenData(ctx, req.Token, EmailRegistrationChallengeTokenType)
	if err != nil || tokenExtraString(consumedTokenData.Extra, "registration_email") != normalizedEmail {
		return nil, ErrEmailRegistrationTokenInvalid
	}

	verifiedToken, err := s.tokenMgr.GenerateDataToken(ctx, EmailRegistrationVerifiedTokenType, map[string]interface{}{
		"registration_email": normalizedEmail,
		"language":           normalizeRegistrationLanguage(tokenExtraString(consumedTokenData.Extra, "language")),
	})
	if err != nil {
		return nil, fmt.Errorf("generate verified email registration token: %w", err)
	}
	return &EmailRegistrationVerifyResponse{
		IsValid: true,
		Email:   normalizedEmail,
		Token:   verifiedToken,
	}, nil
}

func (s *EmailRegistrationService) Finish(
	ctx context.Context,
	req EmailRegistrationFinishRequest,
	ipAddress string,
) (*shared_dto.LoginResponse, error) {
	if !s.options.AllowRegister {
		return nil, ErrEmailRegistrationDisabled
	}
	if req.Password != req.PasswordConfirm {
		return nil, ErrEmailRegistrationPasswordMismatch
	}

	tokenData, err := s.tokenMgr.ConsumeTokenData(ctx, req.Token, EmailRegistrationVerifiedTokenType)
	if err != nil || tokenData == nil {
		return nil, ErrEmailRegistrationTokenInvalid
	}
	emailAddress := normalizeRegistrationEmail(tokenExtraString(tokenData.Extra, "registration_email"))
	if emailAddress == "" || (req.Email != "" && normalizeRegistrationEmail(req.Email) != emailAddress) {
		return nil, ErrEmailRegistrationTokenInvalid
	}
	if s.accounts.ExistsByEmail(ctx, emailAddress) {
		return nil, ErrEmailRegistrationAccountExists
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.Split(emailAddress, "@")[0]
	}
	language := normalizeRegistrationLanguage(tokenExtraString(tokenData.Extra, "language"))
	createWorkspace := true
	if _, err := s.accounts.RegisterEx(
		ctx,
		emailAddress,
		name,
		&req.Password,
		nil,
		nil,
		&language,
		nil,
		nil,
		&createWorkspace,
	); err != nil {
		normalizedError := strings.ToLower(err.Error())
		if strings.Contains(normalizedError, "frozen") || strings.Contains(normalizedError, "freeze") {
			return nil, fmt.Errorf("%w: %v", ErrEmailRegistrationAccountFrozen, err)
		}
		return nil, fmt.Errorf("finish email registration: %w", err)
	}
	loginReq := &shared_dto.LoginReq{
		Email:       emailAddress,
		Password:    req.Password,
		LastLoginIp: ipAddress,
	}
	_, loginErr, loginResp, _ := s.accounts.Login(ctx, loginReq)
	if loginErr != nil {
		return nil, fmt.Errorf("login after email registration: %w", loginErr)
	}
	return &loginResp, nil
}

type PackageEmailRegistrationCodeSender struct{}

func NewPackageEmailRegistrationCodeSender() *PackageEmailRegistrationCodeSender {
	return &PackageEmailRegistrationCodeSender{}
}

func (s *PackageEmailRegistrationCodeSender) SendRegistrationCode(
	ctx context.Context,
	language, to, code, idempotencyKey string,
) error {
	return email.SendRegistrationMailTaskWithContext(ctx, language, to, code, idempotencyKey)
}

func normalizeRegistrationEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeRegistrationLanguage(value string) string {
	if value == "zh-Hans" {
		return "zh-Hans"
	}
	return "en-US"
}

var _ EmailRegistrationAccountGateway = (interfaces.AccountService)(nil)
