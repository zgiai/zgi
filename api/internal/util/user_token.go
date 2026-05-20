package util

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/user/auth/model"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/pkg/logger"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

// Config holds the application configuration
type Config struct {
	TokenExpiryMinutes map[string]int
	MaxLoginAttempts   int
	MaxResetAttempts   int
	RateLimitWindow    int // in minutes
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		TokenExpiryMinutes: map[string]int{
			"access":           60,    // 1 hour
			"refresh":          43200, // 30 days (30 * 24 * 60 minutes)
			"reset_password":   30,    // 30 minutes
			"activation":       1440,  // 24 hours
			"email_code":       30,    // 30 minutes
			"member_invite":    1440,  // 24 hours
			"register":         1440,  // 24 hours
			"sso_state":        5,     // 5 minutes
			"sso_login_ticket": 5,     // 5 minutes
		},
		MaxLoginAttempts: 5,
		MaxResetAttempts: 5,
		RateLimitWindow:  30, // 30 minutes
	}
}

var config = DefaultConfig()

// TokenManager manages token operations
type TokenManager struct{}

// TokenData represents token-related data
type TokenData struct {
	AccountID *string                `json:"account_id"`
	Email     *string                `json:"email"`
	TokenType string                 `json:"token_type"`
	Extra     map[string]interface{} `json:"-"`
}

func NewTokenManager() *TokenManager {
	return &TokenManager{}
}

// GenerateToken generates a new token
func (tm *TokenManager) GenerateToken(
	ctx context.Context,
	tokenType string,
	account *model.Account,
	email *string,
	additionalData map[string]interface{},
) (string, error) {
	if account == nil && email == nil {
		return "", errors.New("account or email must be provided")
	}

	var accountID, accountEmail string
	if account != nil {
		accountID = account.ID
		accountEmail = account.Email
	} else if email != nil {
		accountEmail = *email
	}

	// Get expiration time (minutes)
	expiryMinutes, err := getTokenExpiryMinutes(tokenType)
	if err != nil {
		logger.Error("Failed to get token expiry minutes", err)
		return "", fmt.Errorf("invalid token type or configuration: %w", err)
	}
	// Convert minutes to seconds
	expirySeconds := time.Duration(expiryMinutes*60) * time.Second

	// If accountID exists, revoke old token
	if accountID != "" {
		oldToken, err := tm.getCurrentTokenForAccount(ctx, accountID, tokenType)
		if err != nil {
			logger.Error("Failed to get current token for account", err)
		}
		if oldToken != "" {
			// TODO: revoke token, now can multi client login
			// if err := tm.revokeToken(ctx, oldToken, tokenType); err != nil {
			// 	logger.Error("Failed to revoke old token", err)
			// }
		}
	}

	token := uuid.NewString()
	tokenData := map[string]interface{}{
		"account_id": accountID,
		"email":      accountEmail,
		"token_type": tokenType,
	}
	if additionalData != nil {
		for k, v := range additionalData {
			tokenData[k] = v
		}
	}

	// Add logging
	logger.Info("Generating token", nil)

	tokenDataJSON, err := json.Marshal(tokenData)
	if err != nil {
		logger.Error("Failed to marshal token data", err)
		return "", fmt.Errorf("failed to marshal token data: %w", err)
	}

	// Use pipeline to ensure atomicity
	pipe := redisUtil.GetClient().Pipeline()

	// Store token data
	tokenKey := tm.getTokenKey(token, tokenType)
	pipe.SetEx(ctx, tokenKey, tokenDataJSON, expirySeconds)

	// If accountID exists, store current token
	if accountID != "" {
		currentTokenKey := fmt.Sprintf("current_token:%s:%s", tokenType, accountID)
		pipe.SetEx(ctx, currentTokenKey, token, expirySeconds)
	}

	// Execute pipeline commands
	if _, err := pipe.Exec(ctx); err != nil {
		logger.Error("Failed to store token", err)
		return "", fmt.Errorf("failed to store token: %w", err)
	}

	return token, nil
}

// getTokenExpiryMinutes get expiry time from configuration
func getTokenExpiryMinutes(tokenType string) (int, error) {
	// Here assumes you have config.TokenExpiryMinutes[tokenType] configuration
	expiryMinutes, ok := config.TokenExpiryMinutes[tokenType]
	if !ok {
		return 0, fmt.Errorf("expiry minutes for %s token is not set", tokenType)
	}
	return expiryMinutes, nil
}

// getTokenKey returns the Redis key for a token
func (tm *TokenManager) getTokenKey(token, tokenType string) string {
	return fmt.Sprintf("token:%s:%s", tokenType, token)
}

func (tm *TokenManager) getTokenUsageKey(token, tokenType string) string {
	return fmt.Sprintf("token_usage:%s:%s", tokenType, token)
}

// GetTokenData retrieves token data
func (tm *TokenManager) GetTokenData(token string, tokenType string) (*TokenData, error) {
	ctx := context.Background()

	// Use pipeline to get all related data
	pipe := redisUtil.GetClient().Pipeline()

	// Get token data
	tokenKey := tm.getTokenKey(token, tokenType)
	tokenDataCmd := pipe.Get(ctx, tokenKey)

	// Execute pipeline commands
	if _, err := pipe.Exec(ctx); err != nil {
		if err == redis.Nil {
			logger.Error("Token not found", err)
			return nil, fmt.Errorf("token not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get token data: %w", err)
	}

	// Get token data
	tokenDataJSON, err := tokenDataCmd.Result()
	if err != nil {
		if err == redis.Nil {
			logger.Error("Token not found", err)
			return nil, fmt.Errorf("token not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get token data: %w", err)
	}

	var tokenData TokenData
	var rawData map[string]interface{}

	if err := json.Unmarshal([]byte(tokenDataJSON), &rawData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token data: %w", err)
	}

	if accountID, ok := rawData["account_id"].(string); ok {
		tokenData.AccountID = &accountID

		// Only perform current token check for access and refresh tokens
		// temp: don't want to check for current token refresh
		// if tokenType == "access" || tokenType == "refresh" {
		// 	// If accountID exists, verify if this is the current token
		// 	currentToken, err := tm.getCurrentTokenForAccount(ctx, accountID, tokenType)
		// 	if err != nil {
		// 		return nil, fmt.Errorf("failed to get current token: %w", err)
		// 	}
		// 	if currentToken != token {
		// 		logger.Error("Token is not current", err)
		// 		return nil, fmt.Errorf("token is not current")
		// 	}
		// }
	}

	if email, ok := rawData["email"].(string); ok {
		tokenData.Email = &email
	}

	if tokenType, ok := rawData["token_type"].(string); ok {
		tokenData.TokenType = tokenType
	}

	tokenData.Extra = make(map[string]interface{})
	for k, v := range rawData {
		if k != "account_id" && k != "email" && k != "token_type" {
			tokenData.Extra[k] = v
		}
	}

	// Add logging
	logger.Info("Token data retrieved",
		"token_type", tokenType,
		"token", token,
		"token_data", tokenData)

	return &tokenData, nil
}

// getTokenForAccount gets the current token for an account
func (tm *TokenManager) getTokenForAccount(accountID string, tokenType string) string {
	key := tm.getAccountTokenKey(accountID, tokenType)
	ctx := context.Background()
	token, _ := redisUtil.GetClient().Get(ctx, key).Result()
	return token
}

// setCurrentTokenForAccount sets the current token for an account
func (tm *TokenManager) setCurrentTokenForAccount(ctx context.Context, accountID, token, tokenType string, expiryMinutes int) error {
	key := fmt.Sprintf("current_token:%s:%s", tokenType, accountID)
	// Convert minutes to seconds
	expirySeconds := time.Duration(expiryMinutes*60) * time.Second
	return redisUtil.GetClient().SetEx(ctx, key, token, expirySeconds).Err()
}

// getAccountTokenKey returns the Redis key for an account's token
func (tm *TokenManager) getAccountTokenKey(accountID string, tokenType string) string {
	return fmt.Sprintf("%s:account:%s", tokenType, accountID)
}

// getCurrentTokenForAccount get current account's token
func (tm *TokenManager) getCurrentTokenForAccount(ctx context.Context, accountID, tokenType string) (string, error) {
	key := fmt.Sprintf("current_token:%s:%s", tokenType, accountID)
	val, err := redisUtil.GetClient().Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// revokeToken revoke token
func (tm *TokenManager) revokeToken(ctx context.Context, token, tokenType string) error {
	tokenKey := tm.getTokenKey(token, tokenType)
	return redisUtil.GetClient().Del(ctx, tokenKey).Err()
}

// RevokeToken revokes a token
func (tm *TokenManager) RevokeToken(token string, tokenType string) error {
	ctx := context.Background()
	return tm.revokeToken(ctx, token, tokenType)
}

func (tm *TokenManager) RevokeCurrentTokenForAccount(ctx context.Context, accountID, tokenType string) error {
	client := redisUtil.GetClient()
	if client == nil || accountID == "" || tokenType == "" {
		return nil
	}

	currentTokenKey := fmt.Sprintf("current_token:%s:%s", tokenType, accountID)
	currentToken, err := client.Get(ctx, currentTokenKey).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("get current token for account: %w", err)
	}

	pipe := client.Pipeline()
	pipe.Del(ctx, currentTokenKey)
	if err != redis.Nil && currentToken != "" {
		pipe.Del(ctx, tm.getTokenKey(currentToken, tokenType))
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("revoke current token for account: %w", err)
	}

	return nil
}

func (tm *TokenManager) IncrementTokenUsage(ctx context.Context, token, tokenType string, expiration time.Duration) (int64, error) {
	usageKey := tm.getTokenUsageKey(token, tokenType)
	pipe := redisUtil.GetClient().TxPipeline()
	countCmd := pipe.Incr(ctx, usageKey)
	pipe.Expire(ctx, usageKey, expiration)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("increment token usage: %w", err)
	}

	count, err := countCmd.Result()
	if err != nil {
		return 0, fmt.Errorf("increment token usage result: %w", err)
	}
	return count, nil
}

func (tm *TokenManager) DecrementTokenUsage(ctx context.Context, token, tokenType string) (int64, error) {
	usageKey := tm.getTokenUsageKey(token, tokenType)
	count, err := redisUtil.GetClient().Decr(ctx, usageKey).Result()
	if err != nil {
		return 0, fmt.Errorf("decrement token usage: %w", err)
	}

	if count <= 0 {
		if err := redisUtil.GetClient().Del(ctx, usageKey).Err(); err != nil {
			return 0, fmt.Errorf("delete token usage: %w", err)
		}
		return 0, nil
	}

	return count, nil
}

// InvitationData invitation data structure
type InvitationData struct {
	AccountID   string `json:"account_id"`
	Email       string `json:"email"`
	WorkspaceID string `json:"workspace_id"`
}

// StoreInvitationToken store invitation token
func (tm *TokenManager) StoreInvitationToken(workspaceID, email, accountID, token string, expiryHours int) error {
	ctx := context.Background()

	generalKey := tm.getInvitationTokenKey(token)
	invitationData := map[string]string{
		"account_id":   accountID,
		"email":        email,
		"workspace_id": workspaceID,
	}
	invitationDataJSON, err := json.Marshal(invitationData)
	if err != nil {
		return fmt.Errorf("failed to marshal invitation data: %w", err)
	}

	// Store account_id as value
	expiry := time.Duration(expiryHours) * time.Hour
	return redisUtil.GetClient().SetEx(ctx, generalKey, invitationDataJSON, expiry).Err()
}

// GetInvitationByToken get invitation data by token
func (tm *TokenManager) GetInvitationByToken(token, workspaceID, email string) (*InvitationData, error) {
	ctx := context.Background()

	if workspaceID != "" && email != "" {
		// Use the existing invite token key format.
		emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(email)))
		cacheKey := fmt.Sprintf("member_invite_token:%s, %s:%s", workspaceID, emailHash, token)

		accountID, err := redisUtil.GetClient().Get(ctx, cacheKey).Result()
		if err != nil {
			if err == redis.Nil {
				return nil, nil // token does not exist
			}
			return nil, err
		}

		return &InvitationData{
			AccountID:   accountID,
			Email:       email,
			WorkspaceID: workspaceID,
		}, nil
	} else {
		// Fallback: use generic token key
		generalKey := tm.getInvitationTokenKey(token)
		data, err := redisUtil.GetClient().Get(ctx, generalKey).Result()
		if err != nil {
			if err == redis.Nil {
				return nil, nil
			}
			return nil, err
		}

		var invitation InvitationData
		if err := json.Unmarshal([]byte(data), &invitation); err != nil {
			return nil, err
		}

		return &invitation, nil
	}
}

// RevokeInvitationToken revoke invitation token
func (tm *TokenManager) RevokeInvitationToken(workspaceID, email, token string) error {
	ctx := context.Background()

	if workspaceID != "" && email != "" {
		emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(email)))
		cacheKey := fmt.Sprintf("member_invite_token:%s, %s:%s", workspaceID, emailHash, token)
		return redisUtil.GetClient().Del(ctx, cacheKey).Err()
	} else {
		generalKey := tm.getInvitationTokenKey(token)
		return redisUtil.GetClient().Del(ctx, generalKey).Err()
	}
}

// getInvitationTokenKey get generic invitation token key
func (tm *TokenManager) getInvitationTokenKey(token string) string {
	return fmt.Sprintf("member_invite:token:%s", token)
}
