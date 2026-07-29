package util

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
			"access":                       60,    // 1 hour
			"refresh":                      43200, // 30 days (30 * 24 * 60 minutes)
			"reset_password":               30,    // 30 minutes
			"reset_password_verified":      10,    // 10 minutes
			"activation":                   1440,  // 24 hours
			"email_code":                   30,    // 30 minutes
			"member_invite":                1440,  // 24 hours
			"register":                     1440,  // 24 hours
			"sso_state":                    5,     // 5 minutes
			"sso_login_ticket":             5,     // 5 minutes
			"phone_code":                   5,     // 5 minutes
			"phone_verified":               10,    // 10 minutes
			"email_code_login":             5,     // 5 minutes
			"account_deletion":             5,     // 5 minutes
			"email_registration_challenge": 10,
			"email_registration_verified":  10,
		},
		MaxLoginAttempts: 5,
		MaxResetAttempts: 5,
		RateLimitWindow:  30, // 30 minutes
	}
}

var config = DefaultConfig()

// TokenManager manages token operations
type TokenManager struct{}

type TokenCodeAction string

const (
	TokenCodeConsume TokenCodeAction = "consume"
	TokenCodeReserve TokenCodeAction = "reserve"
)

type TokenCodeStatus int

const (
	TokenCodeInvalid TokenCodeStatus = iota
	TokenCodeVerified
	TokenCodeMismatch
	TokenCodeRateLimited
)

var verifyTokenCodeScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
  return {0, ''}
end

local data = cjson.decode(raw)
local claim = data[ARGV[1]]
if claim == nil or tostring(claim) ~= ARGV[2] then
  return {0, ''}
end

if data['code_reserved'] == true then
  return {0, ''}
end

local expected = data['code']
local master_matches = ARGV[4] ~= '' and ARGV[3] == ARGV[4]
if expected == nil or (tostring(expected) ~= ARGV[3] and not master_matches) then
  local attempts = redis.call('INCR', KEYS[2])
  if attempts == 1 then
    redis.call('PEXPIRE', KEYS[2], ARGV[6])
  end
  if attempts >= tonumber(ARGV[5]) then
    redis.call('DEL', KEYS[1])
    return {3, tostring(attempts)}
  end
  return {2, tostring(attempts)}
end

redis.call('DEL', KEYS[2])
if ARGV[7] == 'reserve' then
  local ttl = redis.call('PTTL', KEYS[1])
  if ttl <= 0 then
    redis.call('DEL', KEYS[1])
    return {0, ''}
  end
  data['code_reserved'] = true
  local encoded = cjson.encode(data)
  redis.call('SET', KEYS[1], encoded, 'PX', ttl)
  return {1, encoded}
end

redis.call('DEL', KEYS[1])
return {1, raw}
`)

var releaseTokenCodeReservationScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then
  return 0
end
local data = cjson.decode(raw)
if data[ARGV[1]] == nil or tostring(data[ARGV[1]]) ~= ARGV[2] or data['code_reserved'] ~= true then
  return 0
end
local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then
  redis.call('DEL', KEYS[1])
  return 0
end
data['code_reserved'] = nil
redis.call('SET', KEYS[1], cjson.encode(data), 'PX', ttl)
return 1
`)

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

// GenerateDataToken stores a short-lived token that is not tied to an account or email.
func (tm *TokenManager) GenerateDataToken(
	ctx context.Context,
	tokenType string,
	additionalData map[string]interface{},
) (string, error) {
	expiryMinutes, err := getTokenExpiryMinutes(tokenType)
	if err != nil {
		return "", fmt.Errorf("invalid token type or configuration: %w", err)
	}

	tokenData := map[string]interface{}{
		"token_type": tokenType,
	}
	for key, value := range additionalData {
		tokenData[key] = value
	}

	token := uuid.NewString()
	tokenDataJSON, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token data: %w", err)
	}

	tokenKey := tm.getTokenKey(token, tokenType)
	if err := redisUtil.GetClient().SetEx(ctx, tokenKey, tokenDataJSON, time.Duration(expiryMinutes)*time.Minute).Err(); err != nil {
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
	logger.Debug("Token data retrieved", "token_type", tokenType)

	return &tokenData, nil
}

// ConsumeTokenData atomically retrieves and revokes a one-time token.
func (tm *TokenManager) ConsumeTokenData(ctx context.Context, token, tokenType string) (*TokenData, error) {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(tokenType) == "" {
		return nil, errors.New("token and token type are required")
	}

	tokenDataJSON, err := redisUtil.GetClient().GetDel(ctx, tm.getTokenKey(token, tokenType)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("token not found: %w", err)
		}
		return nil, fmt.Errorf("consume token data: %w", err)
	}

	var rawData map[string]interface{}
	if err := json.Unmarshal([]byte(tokenDataJSON), &rawData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token data: %w", err)
	}
	tokenData := tokenDataFromRaw(rawData)
	if tokenData.TokenType != tokenType {
		return nil, errors.New("token type mismatch")
	}
	return tokenData, nil
}

// VerifyTokenCode atomically checks a token-bound code, applies the failed
// attempt limit, and either consumes the token or reserves it for a retryable
// follow-up operation.
func (tm *TokenManager) VerifyTokenCode(
	ctx context.Context,
	token, tokenType, claimKey, claimValue, providedCode, masterCode string,
	maxAttempts int,
	attemptWindow time.Duration,
	action TokenCodeAction,
) (*TokenData, TokenCodeStatus, error) {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(tokenType) == "" || claimKey == "" || maxAttempts <= 0 {
		return nil, TokenCodeInvalid, errors.New("token verification arguments are invalid")
	}
	if action != TokenCodeConsume && action != TokenCodeReserve {
		return nil, TokenCodeInvalid, errors.New("token code action is invalid")
	}
	if attemptWindow <= 0 {
		attemptWindow = 5 * time.Minute
	}

	result, err := verifyTokenCodeScript.Run(ctx, redisUtil.GetClient(), []string{
		tm.getTokenKey(token, tokenType),
		tm.getTokenUsageKey(token, tokenType),
	}, claimKey, claimValue, providedCode, strings.TrimSpace(masterCode), maxAttempts, attemptWindow.Milliseconds(), string(action)).Slice()
	if err != nil {
		return nil, TokenCodeInvalid, fmt.Errorf("verify token code: %w", err)
	}
	if len(result) != 2 {
		return nil, TokenCodeInvalid, errors.New("verify token code returned an invalid result")
	}
	statusValue, ok := result[0].(int64)
	if !ok {
		return nil, TokenCodeInvalid, errors.New("verify token code returned an invalid status")
	}
	status := TokenCodeStatus(statusValue)
	if status != TokenCodeVerified {
		return nil, status, nil
	}
	raw, ok := result[1].(string)
	if !ok || raw == "" {
		return nil, TokenCodeInvalid, errors.New("verify token code returned invalid token data")
	}
	var rawData map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &rawData); err != nil {
		return nil, TokenCodeInvalid, fmt.Errorf("decode verified token data: %w", err)
	}
	return tokenDataFromRaw(rawData), status, nil
}

func (tm *TokenManager) ReleaseTokenCodeReservation(ctx context.Context, token, tokenType, claimKey, claimValue string) error {
	result, err := releaseTokenCodeReservationScript.Run(ctx, redisUtil.GetClient(), []string{
		tm.getTokenKey(token, tokenType),
	}, claimKey, claimValue).Int64()
	if err != nil {
		return fmt.Errorf("release token code reservation: %w", err)
	}
	if result != 1 {
		return errors.New("token code reservation not found")
	}
	return nil
}

func tokenDataFromRaw(rawData map[string]interface{}) *TokenData {
	tokenData := &TokenData{Extra: make(map[string]interface{})}
	if accountID, ok := rawData["account_id"].(string); ok && accountID != "" {
		tokenData.AccountID = &accountID
	}
	if email, ok := rawData["email"].(string); ok && email != "" {
		tokenData.Email = &email
	}
	if tokenType, ok := rawData["token_type"].(string); ok {
		tokenData.TokenType = tokenType
	}
	for key, value := range rawData {
		if key != "account_id" && key != "email" && key != "token_type" {
			tokenData.Extra[key] = value
		}
	}
	return tokenData
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
	AccountID      string `json:"account_id"`
	Email          string `json:"email"`
	WorkspaceID    string `json:"workspace_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	InviterID      string `json:"inviter_id,omitempty"`
	Role           string `json:"role,omitempty"`
	ExpiresAt      int64  `json:"expires_at,omitempty"`
}

type InvitationTokenState struct {
	State     string `json:"state"`
	ExpiresAt int64  `json:"expires_at,omitempty"`
}

const invitationStateRetention = 7 * 24 * time.Hour

// InvitationReservation is an ownership handle for a globally reserved
// invitation. The opaque ID prevents one request from releasing or consuming a
// reservation held by another request.
type InvitationReservation struct {
	ID   string
	Data InvitationData
}

var reserveInvitationScript = redis.NewScript(`
local genericRaw = redis.call('GET', KEYS[1])
local legacyRaw = redis.call('GET', KEYS[2])
local genericTTL = 0
local legacyTTL = 0
if genericRaw then
  genericTTL = redis.call('PTTL', KEYS[1])
  if genericTTL <= 0 then
    redis.call('DEL', KEYS[1])
    genericRaw = false
    genericTTL = 0
  end
end
if legacyRaw then
  legacyTTL = redis.call('PTTL', KEYS[2])
  if legacyTTL <= 0 then
    redis.call('DEL', KEYS[2])
    legacyRaw = false
    legacyTTL = 0
  end
end
if not genericRaw and not legacyRaw then
  return {0, ''}
end

local accountID = ''
local email = ''
local workspaceID = ''
local organizationID = ''
local inviterID = ''
local role = ''
local expiresAt = 0
if genericRaw then
  local generic = cjson.decode(genericRaw)
  accountID = tostring(generic['account_id'] or '')
  email = tostring(generic['email'] or '')
  workspaceID = tostring(generic['workspace_id'] or '')
  organizationID = tostring(generic['organization_id'] or '')
  inviterID = tostring(generic['inviter_id'] or '')
  role = tostring(generic['role'] or '')
  expiresAt = tonumber(generic['expires_at'] or 0)
end
if legacyRaw then
  local legacyAccountID = tostring(legacyRaw)
  if accountID ~= '' and accountID ~= legacyAccountID then
    return {-1, ''}
  end
  accountID = legacyAccountID
  if email ~= '' and email ~= ARGV[2] then
    return {-1, ''}
  end
  if workspaceID ~= '' and workspaceID ~= ARGV[1] then
    return {-1, ''}
  end
  email = ARGV[2]
  workspaceID = ARGV[1]
end
if accountID == '' then
  return {-1, ''}
end
if ARGV[1] ~= '' and workspaceID ~= ARGV[1] then
  return {-1, ''}
end
if ARGV[2] ~= '' and email ~= ARGV[2] then
  return {-1, ''}
end

local ttl = math.max(genericTTL, legacyTTL)
if ttl <= 0 then
  return {0, ''}
end
local claim = cjson.encode({
  reservation_id = ARGV[3],
  account_id = accountID,
  email = email,
  workspace_id = workspaceID,
  organization_id = organizationID,
  inviter_id = inviterID,
  role = role,
  expires_at = expiresAt
})
local acquired = redis.call('SET', KEYS[3], claim, 'NX', 'PX', ttl)
if not acquired then
  return {0, ''}
end
return {1, claim}
`)

var releaseInvitationScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return 0 end
local claim = cjson.decode(raw)
if tostring(claim['reservation_id'] or '') ~= ARGV[1] then return 0 end
return redis.call('DEL', KEYS[1])
`)

var consumeInvitationScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[3])
if not raw then return 0 end
local claim = cjson.decode(raw)
if tostring(claim['reservation_id'] or '') ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1], KEYS[2], KEYS[3])
redis.call('SET', KEYS[4], ARGV[2], 'PX', ARGV[3])
return 1
`)

var revokeInvitationScript = redis.NewScript(`
local deleted = 0
local genericRaw = redis.call('GET', KEYS[1])
if genericRaw then
  if ARGV[1] == '' or ARGV[2] == '' then
    deleted = deleted + redis.call('DEL', KEYS[1])
  else
    local generic = cjson.decode(genericRaw)
    if tostring(generic['workspace_id'] or '') == ARGV[1] and tostring(generic['email'] or '') == ARGV[2] then
      deleted = deleted + redis.call('DEL', KEYS[1])
    end
  end
end
if ARGV[1] ~= '' and ARGV[2] ~= '' then
  deleted = deleted + redis.call('DEL', KEYS[2])
end
if deleted > 0 then
  redis.call('SET', KEYS[3], ARGV[3], 'PX', ARGV[4])
end
return deleted
`)

func (tm *TokenManager) invitationLegacyKey(workspaceID, email, token string) string {
	emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(email)))
	return fmt.Sprintf("member_invite_token:%s, %s:%s", workspaceID, emailHash, token)
}

func (tm *TokenManager) invitationClaimKey(token string) string {
	return fmt.Sprintf("member_invite:claim:%s", token)
}

func (tm *TokenManager) invitationStateKey(token string) string {
	return fmt.Sprintf("member_invite:state:%s", token)
}

// ReserveInvitationToken atomically claims an invitation across both the
// generic and legacy key formats. Neither source key is modified, so its TTL is
// preserved while database work is in progress.
func (tm *TokenManager) ReserveInvitationToken(ctx context.Context, token, workspaceID, email string) (*InvitationReservation, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("invitation token is required")
	}

	// Generic invitations carry the legacy lookup fields in their payload. Read
	// them before the atomic claim so callers that only have the token can still
	// lock and later clean up both aliases.
	genericKey := tm.getInvitationTokenKey(token)
	if raw, err := redisUtil.GetClient().Get(ctx, genericKey).Result(); err == nil {
		var generic InvitationData
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			return nil, fmt.Errorf("decode generic invitation: %w", err)
		}
		if workspaceID == "" {
			workspaceID = generic.WorkspaceID
		}
		if email == "" {
			email = generic.Email
		}
	} else if err != redis.Nil {
		return nil, fmt.Errorf("load generic invitation: %w", err)
	}

	legacyKey := fmt.Sprintf("member_invite:legacy-missing:%s", token)
	if workspaceID != "" && email != "" {
		legacyKey = tm.invitationLegacyKey(workspaceID, email, token)
	}
	reservationID := uuid.NewString()
	result, err := reserveInvitationScript.Run(ctx, redisUtil.GetClient(), []string{
		genericKey,
		legacyKey,
		tm.invitationClaimKey(token),
	}, workspaceID, email, reservationID).Slice()
	if err != nil {
		return nil, fmt.Errorf("reserve invitation token: %w", err)
	}
	if len(result) != 2 {
		return nil, errors.New("reserve invitation token returned an invalid result")
	}
	status, ok := result[0].(int64)
	if !ok || status != 1 {
		if status == -1 {
			return nil, errors.New("invitation token payload does not match request")
		}
		return nil, errors.New("invitation token does not exist or is already reserved")
	}
	rawClaim, ok := result[1].(string)
	if !ok || rawClaim == "" {
		return nil, errors.New("invitation reservation data is invalid")
	}
	var claim struct {
		ReservationID string `json:"reservation_id"`
		InvitationData
	}
	if err := json.Unmarshal([]byte(rawClaim), &claim); err != nil {
		return nil, fmt.Errorf("decode invitation reservation: %w", err)
	}
	if claim.ReservationID != reservationID || claim.AccountID == "" {
		return nil, errors.New("invitation reservation data is invalid")
	}
	return &InvitationReservation{ID: reservationID, Data: claim.InvitationData}, nil
}

func (tm *TokenManager) ReleaseInvitationReservation(ctx context.Context, token string, reservation *InvitationReservation) error {
	if reservation == nil || reservation.ID == "" {
		return errors.New("invitation reservation is required")
	}
	result, err := releaseInvitationScript.Run(ctx, redisUtil.GetClient(), []string{
		tm.invitationClaimKey(token),
	}, reservation.ID).Int64()
	if err != nil {
		return fmt.Errorf("release invitation reservation: %w", err)
	}
	if result != 1 {
		return errors.New("invitation reservation does not exist")
	}
	return nil
}

func (tm *TokenManager) ConsumeInvitationReservation(ctx context.Context, token string, reservation *InvitationReservation) error {
	if reservation == nil || reservation.ID == "" {
		return errors.New("invitation reservation is required")
	}
	legacyKey := fmt.Sprintf("member_invite:legacy-missing:%s", token)
	if reservation.Data.WorkspaceID != "" && reservation.Data.Email != "" {
		legacyKey = tm.invitationLegacyKey(reservation.Data.WorkspaceID, reservation.Data.Email, token)
	}
	result, err := consumeInvitationScript.Run(ctx, redisUtil.GetClient(), []string{
		tm.getInvitationTokenKey(token),
		legacyKey,
		tm.invitationClaimKey(token),
		tm.invitationStateKey(token),
	}, reservation.ID, `{"state":"used"}`, invitationStateRetention.Milliseconds()).Int64()
	if err != nil {
		return fmt.Errorf("consume invitation reservation: %w", err)
	}
	if result != 1 {
		return errors.New("invitation reservation does not exist")
	}
	return nil
}

// StoreInvitationToken store invitation token
func (tm *TokenManager) StoreInvitationToken(workspaceID, email, accountID, token string, expiryHours int) error {
	return tm.StoreInvitationTokenWithDetails(InvitationData{
		AccountID: accountID, Email: email, WorkspaceID: workspaceID,
	}, token, expiryHours)
}

func (tm *TokenManager) StoreInvitationTokenWithDetails(invitationData InvitationData, token string, expiryHours int) error {
	ctx := context.Background()
	expiry := time.Duration(expiryHours) * time.Hour
	invitationData.ExpiresAt = time.Now().Add(expiry).Unix()
	invitationDataJSON, err := json.Marshal(invitationData)
	if err != nil {
		return fmt.Errorf("failed to marshal invitation data: %w", err)
	}

	stateJSON, err := json.Marshal(InvitationTokenState{State: "pending", ExpiresAt: invitationData.ExpiresAt})
	if err != nil {
		return fmt.Errorf("failed to marshal invitation state: %w", err)
	}
	_, err = redisUtil.GetClient().TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SetEx(ctx, tm.getInvitationTokenKey(token), invitationDataJSON, expiry)
		pipe.SetEx(ctx, tm.invitationStateKey(token), stateJSON, expiry+invitationStateRetention)
		return nil
	})
	return err
}

func (tm *TokenManager) GetInvitationTokenState(token string) (string, error) {
	ctx := context.Background()
	if data, err := tm.GetInvitationByToken(token, "", ""); err != nil {
		return "", err
	} else if data != nil {
		return "pending", nil
	}
	raw, err := redisUtil.GetClient().Get(ctx, tm.invitationStateKey(token)).Result()
	if err == redis.Nil {
		return "invalid", nil
	}
	if err != nil {
		return "", err
	}
	var state InvitationTokenState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return "", err
	}
	if state.State == "pending" {
		return "expired", nil
	}
	return state.State, nil
}

// GetInvitationByToken get invitation data by token
func (tm *TokenManager) GetInvitationByToken(token, workspaceID, email string) (*InvitationData, error) {
	ctx := context.Background()

	if workspaceID != "" && email != "" {
		// Use the existing invite token key format.
		cacheKey := tm.invitationLegacyKey(workspaceID, email, token)

		accountID, err := redisUtil.GetClient().Get(ctx, cacheKey).Result()
		if err != nil {
			if err != redis.Nil {
				return nil, err
			}
			// Invitations created by newer versions use the generic key even when
			// callers still send workspace/email. Fall through safely and validate
			// the generic payload against both request bindings.
		} else {
			return &InvitationData{
				AccountID:   accountID,
				Email:       email,
				WorkspaceID: workspaceID,
			}, nil
		}
	}

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
	if workspaceID != "" && invitation.WorkspaceID != workspaceID {
		return nil, nil
	}
	if email != "" && invitation.Email != email {
		return nil, nil
	}
	return &invitation, nil
}

// RevokeInvitationToken revoke invitation token
func (tm *TokenManager) RevokeInvitationToken(workspaceID, email, token string) error {
	ctx := context.Background()

	legacyKey := fmt.Sprintf("member_invite:legacy-missing:%s", token)
	if workspaceID != "" && email != "" {
		legacyKey = tm.invitationLegacyKey(workspaceID, email, token)
	}
	deleted, err := revokeInvitationScript.Run(ctx, redisUtil.GetClient(), []string{
		tm.getInvitationTokenKey(token), legacyKey, tm.invitationStateKey(token),
	}, workspaceID, email, `{"state":"revoked"}`, invitationStateRetention.Milliseconds()).Int64()
	if err != nil {
		return err
	}
	if deleted < 1 {
		return errors.New("invitation token does not exist or was already consumed")
	}
	return nil
}

// getInvitationTokenKey get generic invitation token key
func (tm *TokenManager) getInvitationTokenKey(token string) string {
	return fmt.Sprintf("member_invite:token:%s", token)
}
