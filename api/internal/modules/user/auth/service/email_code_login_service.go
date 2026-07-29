package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/gorm"
)

const (
	EmailCodeLoginTokenType = "email_code_login"

	defaultEmailCodeLoginMaxAttempts = 5
	defaultEmailCodeLoginCooldown    = time.Minute
)

var (
	ErrEmailCodeLoginDisabled       = errors.New("email code login is disabled")
	ErrEmailCodeLoginAccountMissing = errors.New("email code login account was not found")
	ErrEmailCodeLoginTokenInvalid   = errors.New("email code login token is invalid")
	ErrEmailCodeLoginCodeInvalid    = errors.New("email code login code is invalid")
	ErrEmailCodeLoginRateLimited    = errors.New("email code login rate limit exceeded")
	ErrEmailCodeLoginSendFailed     = errors.New("email code login message could not be sent")
	ErrEmailCodeLoginAccountBlocked = errors.New("email code login account is not active")
)

type EmailCodeLoginSendRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Language string `json:"language"`
}

type EmailCodeLoginSendResponse struct {
	Result string `json:"result"`
	Token  string `json:"data"`
}

type EmailCodeLoginVerifyRequest struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required"`
	Token string `json:"token" binding:"required"`
}

type EmailCodeLoginAccountGateway interface {
	GetUserThroughEmail(ctx context.Context, email string) (*auth_model.Account, error)
	SendEmailCodeLoginEmail(ctx context.Context, account *auth_model.Account, email, language string) (string, error)
	IsEmailSendIPLimit(ctx context.Context, ipAddress string) (bool, error)
	LoginCommon(account *auth_model.Account, ipAddress string) (*auth_model.TokenPair, error)
}

type EmailCodeLoginOptions struct {
	Enabled                     bool
	MasterVerificationCode      string
	AllowMasterVerificationCode bool
	MaxCodeAttempts             int
	SendCooldown                time.Duration
}

type EmailCodeLoginService struct {
	accounts    EmailCodeLoginAccountGateway
	tokenMgr    *helper.TokenManager
	options     EmailCodeLoginOptions
	rateLimiter *RedisEmailCodeLoginRateLimiter
}

func NewEmailCodeLoginService(accounts EmailCodeLoginAccountGateway, tokenMgr *helper.TokenManager, options EmailCodeLoginOptions) *EmailCodeLoginService {
	if options.MaxCodeAttempts <= 0 {
		options.MaxCodeAttempts = defaultEmailCodeLoginMaxAttempts
	}
	if options.SendCooldown <= 0 {
		options.SendCooldown = defaultEmailCodeLoginCooldown
	}
	return &EmailCodeLoginService{
		accounts:    accounts,
		tokenMgr:    tokenMgr,
		options:     options,
		rateLimiter: NewRedisEmailCodeLoginRateLimiter(options.SendCooldown),
	}
}

func (s *EmailCodeLoginService) SendCode(ctx context.Context, req EmailCodeLoginSendRequest, ipAddress string) (*EmailCodeLoginSendResponse, error) {
	if !s.options.Enabled {
		return nil, ErrEmailCodeLoginDisabled
	}
	emailAddress := normalizeEmailCodeLoginEmail(req.Email)
	if limited, err := s.accounts.IsEmailSendIPLimit(ctx, ipAddress); err != nil || limited {
		return nil, ErrEmailCodeLoginRateLimited
	}
	account, err := s.accounts.GetUserThroughEmail(ctx, emailAddress)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEmailCodeLoginAccountMissing
		}
		return nil, fmt.Errorf("load email code login account: %w", err)
	}
	if account == nil {
		return nil, ErrEmailCodeLoginAccountMissing
	}
	if account.Status != auth_model.AccountStatusActive {
		return nil, ErrEmailCodeLoginAccountBlocked
	}

	acquired, err := s.rateLimiter.Acquire(ctx, emailAddress)
	if err != nil || !acquired {
		return nil, ErrEmailCodeLoginRateLimited
	}
	releaseRateLimit := true
	defer func() {
		if releaseRateLimit {
			_ = s.rateLimiter.Release(context.WithoutCancel(ctx), emailAddress)
		}
	}()

	token, err := s.accounts.SendEmailCodeLoginEmail(ctx, account, emailAddress, normalizeEmailCodeLoginLanguage(req.Language))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmailCodeLoginSendFailed, err)
	}
	releaseRateLimit = false
	return &EmailCodeLoginSendResponse{Result: "success", Token: token}, nil
}

func (s *EmailCodeLoginService) VerifyAndLogin(ctx context.Context, req EmailCodeLoginVerifyRequest, ipAddress string) (*dto.LoginResponse, error) {
	if !s.options.Enabled {
		return nil, ErrEmailCodeLoginDisabled
	}
	emailAddress := normalizeEmailCodeLoginEmail(req.Email)
	tokenData, err := s.tokenMgr.GetTokenData(req.Token, EmailCodeLoginTokenType)
	if err != nil || tokenData == nil || tokenData.Email == nil || normalizeEmailCodeLoginEmail(*tokenData.Email) != emailAddress {
		return nil, ErrEmailCodeLoginTokenInvalid
	}

	expectedCode, ok := tokenData.Extra["code"].(string)
	masterCode := ""
	if s.options.AllowMasterVerificationCode {
		masterCode = strings.TrimSpace(s.options.MasterVerificationCode)
	}
	if !ok || (req.Code != expectedCode && (masterCode == "" || req.Code != masterCode)) {
		attempts, incrementErr := s.tokenMgr.IncrementTokenUsage(ctx, req.Token, EmailCodeLoginTokenType, 5*time.Minute)
		if incrementErr != nil {
			return nil, fmt.Errorf("track email login verification attempt: %w", incrementErr)
		}
		if attempts >= int64(s.options.MaxCodeAttempts) {
			_ = s.tokenMgr.RevokeToken(req.Token, EmailCodeLoginTokenType)
			return nil, ErrEmailCodeLoginRateLimited
		}
		return nil, ErrEmailCodeLoginCodeInvalid
	}
	account, err := s.accounts.GetUserThroughEmail(ctx, emailAddress)
	if err != nil || account == nil {
		return nil, ErrEmailCodeLoginAccountMissing
	}
	if account.Status != auth_model.AccountStatusActive {
		return nil, ErrEmailCodeLoginAccountBlocked
	}
	tokenPair, err := s.accounts.LoginCommon(account, ipAddress)
	if err != nil {
		return nil, fmt.Errorf("complete email code login: %w", err)
	}
	if _, err := s.tokenMgr.ConsumeTokenData(ctx, req.Token, EmailCodeLoginTokenType); err != nil {
		if tokenPair.RefreshToken != "" {
			_ = s.tokenMgr.RevokeToken(tokenPair.RefreshToken, "refresh")
		}
		return nil, ErrEmailCodeLoginTokenInvalid
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

type RedisEmailCodeLoginRateLimiter struct {
	cooldown time.Duration
}

func NewRedisEmailCodeLoginRateLimiter(cooldown time.Duration) *RedisEmailCodeLoginRateLimiter {
	return &RedisEmailCodeLoginRateLimiter{cooldown: cooldown}
}

func (l *RedisEmailCodeLoginRateLimiter) Acquire(ctx context.Context, emailAddress string) (bool, error) {
	return redisUtil.GetClient().SetNX(ctx, l.key(emailAddress), "1", l.cooldown).Result()
}

func (l *RedisEmailCodeLoginRateLimiter) Release(ctx context.Context, emailAddress string) error {
	return redisUtil.GetClient().Del(ctx, l.key(emailAddress)).Err()
}

func (l *RedisEmailCodeLoginRateLimiter) key(emailAddress string) string {
	digest := sha256.Sum256([]byte(normalizeEmailCodeLoginEmail(emailAddress)))
	return fmt.Sprintf("email_code_login_send_cooldown:%x", digest)
}

func normalizeEmailCodeLoginEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeEmailCodeLoginLanguage(value string) string {
	if value == "zh-Hans" || value == "zh-CN" {
		return "zh-Hans"
	}
	return "en-US"
}

var _ EmailCodeLoginAccountGateway = (interfaces.AccountService)(nil)
