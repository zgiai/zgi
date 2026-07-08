package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"

	"github.com/zgiai/zgi/api/pkg/jwt"
	"github.com/zgiai/zgi/api/pkg/logger"
	"golang.org/x/sync/singleflight"

	"github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/internal/dto"
	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/shared/workspacebootstrap"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"

	"gorm.io/gorm"

	system_service "github.com/zgiai/zgi/api/internal/modules/system/service"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	"github.com/zgiai/zgi/api/internal/modules/user/auth/statuscache"
	workspacecache "github.com/zgiai/zgi/api/internal/modules/workspace/cache"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/email"
)

const (
	TokenTypeAccess        = "access"
	TokenTypeRefresh       = "refresh"
	TokenTypeResetPassword = "reset_password"

	resetPasswordRateLimitKeyPrefix = "reset_password_rate_limit:"
)

var ErrCurrentPasswordMismatch = errors.New("current password is incorrect")

func normalizedRateLimitEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func loginErrorRateLimitKey(email string) string {
	return helper.LoginErrorRateLimitKeyPrefix + normalizedRateLimitEmail(email)
}

func forgotPasswordErrorRateLimitKey(email string) string {
	return "forgot_password_error_rate_limit:" + normalizedRateLimitEmail(email)
}

func resetPasswordEmailRateLimitKey(email string) string {
	return resetPasswordRateLimitKeyPrefix + normalizedRateLimitEmail(email)
}

func accountAccessStatusError(status auth_model.AccountStatus) error {
	switch status {
	case auth_model.AccountStatusBanned:
		return errors.New("account is banned")
	case auth_model.AccountStatusFrozen:
		return errors.New("account is frozen")
	case auth_model.AccountStatusClosed:
		return errors.New("account is closed")
	default:
		return nil
	}
}

func accountLoginStatusError(status auth_model.AccountStatus) error {
	switch status {
	case auth_model.AccountStatusBanned:
		return errors.New("账户已被禁用")
	case auth_model.AccountStatusFrozen:
		return errors.New("account is frozen")
	case auth_model.AccountStatusClosed:
		return errors.New("invalid email or password")
	default:
		return nil
	}
}

// AccountService implements the shared.AccountService interface
// This is a simplified implementation that provides basic account operations
// More complex features can be added incrementally
type AccountService struct {
	accountRepo                   auth_repo.AccountRepository
	db                            *gorm.DB
	tokenMgr                      *helper.TokenManager
	workspaceManagementService    interfaces.WorkspaceManagementService
	billingService                interfaces.BillingService
	registerService               interfaces.RegisterService
	organizationManagementService interfaces.OrganizationManagementService
	organizationService           interfaces.OrganizationService
	officialRouteBootstrapper     interfaces.OfficialRouteBootstrapper
	systemConfigService           system_service.SystemConfigService
	eventBus                      interfaces.EventBus
	officialSignupRegistration    *officialSignupRegistrationService
	profileCacheMu                sync.RWMutex
	profileCache                  map[string]*accountProfileCacheEntry
	profileCacheGeneration        map[string]uint64
	profileCacheGroup             singleflight.Group
	accountContextGroup           singleflight.Group
}

const (
	accountProfileCacheTTL         = 2 * time.Minute
	accountProfileCacheMaxEntries  = 10000
	accountProfileColdDBBuildLimit = 16
)

var accountProfileColdDBBuildTokens = make(chan struct{}, accountProfileColdDBBuildLimit)

type accountProfileCacheEntry struct {
	profile   *dto.AccountProfileResponse
	updatedAt time.Time
}

type currentWorkspaceLookup interface {
	GetCurrentWorkspace(ctx context.Context, accountID string) (*workspace_model.WorkspaceMember, error)
	GetWorkspaceByID(ctx context.Context, id string) (*workspace_model.Workspace, error)
}

// NewAccountService creates a new AccountService
func NewAccountService(
	accountRepo auth_repo.AccountRepository,
	db *gorm.DB,
	tokenMgr *helper.TokenManager,
	tenantService interfaces.WorkspaceManagementService,
	billingService interfaces.BillingService,
	registerService interfaces.RegisterService,
	enterpriseGroupService interfaces.OrganizationManagementService,
	enterpriseService interfaces.OrganizationService,
	systemConfigService system_service.SystemConfigService,
	eventBus interfaces.EventBus,
	consoleProvider platformconsole.ConsoleProvider,
) *AccountService {
	return &AccountService{
		accountRepo:                   accountRepo,
		db:                            db,
		tokenMgr:                      tokenMgr,
		workspaceManagementService:    tenantService,
		billingService:                billingService,
		registerService:               registerService,
		organizationManagementService: enterpriseGroupService,
		organizationService:           enterpriseService,
		systemConfigService:           systemConfigService,
		eventBus:                      eventBus,
		officialSignupRegistration:    newOfficialSignupRegistrationService(enterpriseService, consoleProvider),
		profileCache:                  make(map[string]*accountProfileCacheEntry),
		profileCacheGeneration:        make(map[string]uint64),
	}
}

// Basic account operations (implemented)
func (s *AccountService) GetAccountByEmail(ctx context.Context, email string) (*auth_model.Account, error) {
	return s.accountRepo.GetAccountByEmail(ctx, email)
}

func (s *AccountService) GetAccountByID(ctx context.Context, id string) (*auth_model.Account, error) {
	return s.accountRepo.GetAccountById(ctx, id)
}

func (s *AccountService) GetAccountsByIDs(ctx context.Context, ids []string) (map[string]*auth_model.Account, error) {
	if len(ids) == 0 {
		return make(map[string]*auth_model.Account), nil
	}

	accounts, err := s.accountRepo.GetAccountsByIds(ctx, ids)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*auth_model.Account, len(accounts))
	for _, account := range accounts {
		result[account.ID] = account
	}

	return result, nil
}

func (s *AccountService) ExistsByEmail(ctx context.Context, email string) bool {
	exists, _ := s.accountRepo.ExistsByEmail(ctx, email)
	return exists
}

func (s *AccountService) LoadUser(ctx context.Context, userID string) (*auth_model.Account, error) {
	account, err := s.accountRepo.GetAccount(ctx, userID)
	if err != nil {
		return nil, err
	}

	if err := accountAccessStatusError(account.Status); err != nil {
		return nil, err
	}

	now := time.Now()
	if account.LastActiveAt == nil || now.Sub(*account.LastActiveAt) > 10*time.Minute {
		account.LastActiveAt = &now
		s.accountRepo.UpdateAccount(ctx, account)
	}

	return account, nil
}

// Complex operations (stub implementations - to be implemented later)
func (s *AccountService) SendResetPasswordEmail(ctx context.Context, account *auth_model.Account, eml string, language string) (string, error) {
	var accountEmail string
	if account != nil {
		accountEmail = account.Email
	} else if len(eml) != 0 {
		accountEmail = eml
	} else {
		return "", errors.New("eml must be provided")
	}

	if limited, err := s.isResetPasswordEmailRateLimited(ctx, accountEmail); err != nil || limited {
		return "", errors.New("Too many password reset emails have been sent. Please try again in 1 minutes.")
	}

	code := generate6DigitCode()

	var tokenEmail string

	if account != nil {
		tokenEmail = ""
	} else {
		tokenEmail = accountEmail
	}

	token, err := s.tokenMgr.GenerateToken(
		ctx,
		"reset_password",
		nil,
		&tokenEmail,
		map[string]interface{}{"code": code},
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	if err := email.SendResetPasswordMailTask(language, accountEmail, code); err != nil {
		return "", fmt.Errorf("failed to send eml: %w", err)
	}

	s.incrementResetPasswordRateLimit(accountEmail)

	return token, nil
}

func (s *AccountService) SendDirectAddMemberEmail(ctx context.Context, account *auth_model.Account, groupID, groupName, departmentName, language string) error {
	if account == nil {
		return errors.New("account must be provided")
	}

	consoleURL := email.Cfg.Email.ConsoleWebURL
	if consoleURL == "" {
		consoleURL = ""
	}

	token := uuid.New().String()
	expiryHours := 72

	if s.tokenMgr != nil {
		if err := s.tokenMgr.StoreInvitationToken("", account.Email, account.ID, token, expiryHours); err != nil {
			token = fmt.Sprintf("%s-%s-%d", groupID, account.ID, time.Now().Unix())
		}
	}

	targetURL := consoleURL
	if targetURL != "" && !strings.HasSuffix(targetURL, "/") {
		targetURL = targetURL + "/"
	}

	activationURL := fmt.Sprintf("%sactivate?email=%s&token=%s", targetURL, account.Email, token)

	return email.SendDirectAddMemberMail(language, account.Email, groupName, departmentName, activationURL)
}

func generate6DigitCode() string {
	rand.Seed(time.Now().UnixNano())
	codes := make([]string, 6)
	for i := 0; i < 6; i++ {
		codes[i] = strconv.Itoa(rand.Intn(10))
	}
	return strings.Join(codes, "")
}

func (s *AccountService) incrementResetPasswordRateLimit(email string) {
	ctx := context.Background()
	key := resetPasswordEmailRateLimitKey(email)
	redisUtil.GetClient().SetEx(ctx, key, "1", time.Minute)
}

func (s *AccountService) isResetPasswordEmailRateLimited(ctx context.Context, email string) (bool, error) {
	val, err := redisUtil.GetString(ctx, resetPasswordEmailRateLimitKey(email))
	if err != nil {
		return false, err
	}
	return val != "", nil
}

func (s *AccountService) Logout(ctx context.Context, accessToken, refreshToken string) error {
	// Only revoke refresh token which is stored in Redis
	err := s.tokenMgr.RevokeToken(refreshToken, TokenTypeRefresh)

	// Also clean up any old format storage for backward compatibility
	if refreshToken != "" {
		// Try to get account ID from refresh token to clean up old format
		if tokenData, err := s.tokenMgr.GetTokenData(refreshToken, TokenTypeRefresh); err == nil && tokenData != nil && tokenData.AccountID != nil {
			redisUtil.RedisClient.Del(ctx, "refresh_token:"+*tokenData.AccountID)
			redisUtil.RedisClient.Del(ctx, "refresh_token:"+refreshToken)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to revoke refresh token: %v", err)
	}
	return nil
}

func (s *AccountService) RefreshToken(ctx context.Context, refreshToken string) (*dto.TokenResponse, error) {
	// First try to get token data using TokenManager (new system)
	tokenData, err := s.tokenMgr.GetTokenData(refreshToken, TokenTypeRefresh)
	var accountID string

	if err != nil || tokenData == nil || tokenData.AccountID == nil {
		// Fallback: try the old storage format for backward compatibility
		storedAccountID, redisErr := redisUtil.RedisClient.Get(ctx, "refresh_token:"+refreshToken).Result()
		if redisErr != nil {
			return nil, errors.New("invalid or expired refresh token")
		}
		accountID = storedAccountID
	} else {
		accountID = *tokenData.AccountID
	}

	// Get account information
	account, err := s.accountRepo.GetAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	// Generate new access token using JWT (not TokenManager UUID)
	accessToken, err := s.GetAccountJWTToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate new refresh token
	newRefreshToken, err := s.tokenMgr.GenerateToken(
		ctx,
		TokenTypeRefresh,
		account,
		nil,
		map[string]interface{}{
			"account_id": account.ID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// Revoke the old refresh token
	s.tokenMgr.RevokeToken(refreshToken, TokenTypeRefresh)

	// Also clean up old format storage for backward compatibility
	redisUtil.RedisClient.Del(ctx, "refresh_token:"+refreshToken)
	redisUtil.RedisClient.Del(ctx, "refresh_token:"+account.ID)

	return &dto.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

func (s *AccountService) GetAccountProfile(ctx context.Context, accountID string) (*dto.AccountProfileResponse, error) {
	if profile, ok := s.getCachedAccountProfile(accountID); ok {
		return profile, nil
	}

	result, err, _ := s.profileCacheGroup.Do(accountID, func() (interface{}, error) {
		if profile, ok := s.getCachedAccountProfile(accountID); ok {
			return profile, nil
		}

		generation := s.accountProfileCacheGeneration(accountID)
		profile, err := s.getAccountProfileUncached(ctx, accountID)
		if err != nil {
			return nil, err
		}
		s.setCachedAccountProfileIfCurrent(accountID, profile, generation)
		return profile, nil
	})
	if err != nil {
		return nil, err
	}
	profile, ok := result.(*dto.AccountProfileResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected account profile cache result type %T", result)
	}
	return cloneAccountProfileResponse(profile), nil
}

func (s *AccountService) getAccountProfileUncached(ctx context.Context, accountID string) (*dto.AccountProfileResponse, error) {
	if err := acquireAccountProfileColdDBBuildSlot(ctx); err != nil {
		return nil, err
	}
	defer releaseAccountProfileColdDBBuildSlot()

	account, err := s.accountRepo.GetAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("account not found: %w", err)
	}

	accountContext, err := s.GetAccountContext(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account context: %w", err)
	}

	// Double check if accountContext is nil (though Ensure should have handled creation if it returned success)
	if accountContext == nil {
		accountContext = &auth_model.AccountContext{
			AccountID: accountID,
		}
	}

	organizationRole := "normal"
	if accountContext.CurrentOrganizationID != nil {
		role, err := s.organizationService.GetUserOrganizationRole(ctx, *accountContext.CurrentOrganizationID, accountID)
		if err == nil {
			organizationRole = string(role)
		}
	}

	return &dto.AccountProfileResponse{
		ID:                    account.ID,
		Name:                  account.Name,
		Email:                 account.Email,
		Avatar:                derefString(account.Avatar),
		InterfaceLanguage:     derefString(account.InterfaceLanguage),
		Timezone:              derefString(account.Timezone),
		Status:                string(account.Status),
		GroupRole:             organizationRole,
		OrganizationRole:      organizationRole,
		IsSuperAdmin:          frontendSuperAdminFlag(account),
		Extension:             account.Extensions,
		CurrentOrganizationID: accountContext.CurrentOrganizationID,
		CurrentWorkspaceID:    accountContext.CurrentWorkspaceID,
	}, nil
}

func (s *AccountService) getCachedAccountProfile(accountID string) (*dto.AccountProfileResponse, bool) {
	s.profileCacheMu.RLock()
	entry, ok := s.profileCache[accountID]
	if ok && time.Since(entry.updatedAt) < accountProfileCacheTTL {
		profile := cloneAccountProfileResponse(entry.profile)
		s.profileCacheMu.RUnlock()
		return profile, true
	}
	s.profileCacheMu.RUnlock()
	return nil, false
}

func (s *AccountService) setCachedAccountProfileIfCurrent(accountID string, profile *dto.AccountProfileResponse, generation uint64) {
	s.profileCacheMu.Lock()
	if s.profileCache == nil {
		s.profileCache = make(map[string]*accountProfileCacheEntry)
	}
	if s.profileCacheGeneration == nil {
		s.profileCacheGeneration = make(map[string]uint64)
	}
	if s.profileCacheGeneration[accountID] != generation {
		s.profileCacheMu.Unlock()
		return
	}
	now := time.Now()
	if _, exists := s.profileCache[accountID]; !exists && len(s.profileCache) >= accountProfileCacheMaxEntries {
		s.pruneAccountProfileCacheLocked(now)
	}
	s.profileCache[accountID] = &accountProfileCacheEntry{
		profile:   cloneAccountProfileResponse(profile),
		updatedAt: now,
	}
	s.profileCacheMu.Unlock()
}

func (s *AccountService) pruneAccountProfileCacheLocked(now time.Time) {
	for accountID, entry := range s.profileCache {
		if now.Sub(entry.updatedAt) >= accountProfileCacheTTL {
			delete(s.profileCache, accountID)
		}
	}
	if len(s.profileCache) < accountProfileCacheMaxEntries {
		return
	}

	var oldestAccountID string
	var oldestUpdatedAt time.Time
	for accountID, entry := range s.profileCache {
		if oldestAccountID == "" || entry.updatedAt.Before(oldestUpdatedAt) {
			oldestAccountID = accountID
			oldestUpdatedAt = entry.updatedAt
		}
	}
	if oldestAccountID != "" {
		delete(s.profileCache, oldestAccountID)
	}
}

func (s *AccountService) accountProfileCacheGeneration(accountID string) uint64 {
	s.profileCacheMu.RLock()
	generation := s.profileCacheGeneration[accountID]
	s.profileCacheMu.RUnlock()
	return generation
}

func (s *AccountService) InvalidateAccountProfileCache(accountID string) {
	s.profileCacheMu.Lock()
	if s.profileCacheGeneration == nil {
		s.profileCacheGeneration = make(map[string]uint64)
	}
	s.profileCacheGeneration[accountID]++
	delete(s.profileCache, accountID)
	s.profileCacheMu.Unlock()
	statuscache.InvalidateAccountStatus(context.Background(), accountID)
}

func (s *AccountService) invalidateAccountProfileCache(accountID string) {
	s.InvalidateAccountProfileCache(accountID)
}

func acquireAccountProfileColdDBBuildSlot(ctx context.Context) error {
	select {
	case accountProfileColdDBBuildTokens <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func releaseAccountProfileColdDBBuildSlot() {
	<-accountProfileColdDBBuildTokens
}

func cloneAccountProfileResponse(src *dto.AccountProfileResponse) *dto.AccountProfileResponse {
	if src == nil {
		return nil
	}
	clone := *src
	if src.IsSuperAdmin != nil {
		value := *src.IsSuperAdmin
		clone.IsSuperAdmin = &value
	}
	if src.CurrentOrganizationID != nil {
		value := *src.CurrentOrganizationID
		clone.CurrentOrganizationID = &value
	}
	if src.CurrentWorkspaceID != nil {
		value := *src.CurrentWorkspaceID
		clone.CurrentWorkspaceID = &value
	}
	if src.Extension != nil {
		clone.Extension = make(auth_model.JSONMap, len(src.Extension))
		for key, value := range src.Extension {
			clone.Extension[key] = value
		}
	}
	return &clone
}

func (s *AccountService) UpdateAccountProfile(ctx context.Context, accountID string, req *dto.UpdateProfileRequest) error {
	account, err := s.accountRepo.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	// Use pointer to distinguish between "not update" (nil) and "update to empty" (empty string pointer)
	if req.Name != nil {
		account.Name = *req.Name
	}
	if req.Avatar != nil {
		// Support clearing avatar by passing empty string
		if *req.Avatar == "" {
			account.Avatar = nil
		} else {
			account.Avatar = req.Avatar
		}
	}
	if req.Language != nil {
		account.InterfaceLanguage = req.Language
	}
	if req.Timezone != nil {
		account.Timezone = req.Timezone
	}

	if req.Mobile != nil {
		mobile := *req.Mobile
		updateAccountExtensions(account, &mobile, nil, nil, nil)
	}

	err = s.accountRepo.UpdateAccount(ctx, account)
	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	s.invalidateAccountProfileCache(accountID)
	return nil
}

func (s *AccountService) ActivateCheck(ctx context.Context, workspaceID, email, token string) (map[string]interface{}, bool) {
	invitationData, err := s.tokenMgr.GetInvitationByToken(token, workspaceID, email)
	if err != nil || invitationData == nil {
		return map[string]interface{}{
			"is_valid": false,
		}, false
	}

	if invitationData.WorkspaceID == "" {
		account, err := s.accountRepo.GetAccount(ctx, invitationData.AccountID)
		if err != nil || account == nil {
			return map[string]interface{}{
				"is_valid": false,
			}, false
		}
		if account.Email != invitationData.Email {
			return map[string]interface{}{
				"is_valid": false,
			}, false
		}

		return map[string]interface{}{
			"is_valid": true,
			"data": map[string]interface{}{
				"workspace_name": "",
				"workspace_id":   "",
				"email":          invitationData.Email,
			},
		}, true
	}

	tenant, err := s.workspaceManagementService.GetWorkspaceByID(ctx, invitationData.WorkspaceID)
	if err != nil || tenant == nil || tenant.Status != workspace_model.WorkspaceStatusNormal {
		return map[string]interface{}{
			"is_valid": false,
		}, false
	}

	invitationDataMap := map[string]string{
		"email": invitationData.Email,
	}
	tenantAccount, err := s.accountRepo.SelectAccountAndTenantAccountJoin(ctx, invitationDataMap, *tenant)
	if err != nil || tenantAccount == nil {
		return map[string]interface{}{
			"is_valid": false,
		}, false
	}

	if invitationData.AccountID != tenantAccount.Account.ID {
		return map[string]interface{}{
			"is_valid": false,
		}, false
	}

	return map[string]interface{}{
		"is_valid": true,
		"data": map[string]interface{}{
			"workspace_name": tenant.Name,
			"workspace_id":   tenant.ID,
			"email":          invitationData.Email,
		},
	}, true
}

func (s *AccountService) Activate(ctx context.Context, workspaceID, email, token, name, password, lang, timezone string) (interface{}, error) {
	invitationData, err := s.tokenMgr.GetInvitationByToken(token, workspaceID, email)
	if err != nil || invitationData == nil {
		return nil, errors.New("Auth Token is invalid or account already activated, please check again.")
	}

	if invitationData.WorkspaceID == "" {
		account, err := s.accountRepo.GetAccount(ctx, invitationData.AccountID)
		if err != nil || account == nil {
			return nil, errors.New("Auth Token is invalid or account already activated, please check again.")
		}
		if account.Email != invitationData.Email {
			return nil, errors.New("Auth Token is invalid or account already activated, please check again.")
		}

		account.Name = name

		if lang != "" {
			account.InterfaceLanguage = &lang
		}
		if timezone != "" {
			account.Timezone = &timezone
		}

		if password != "" {
			hashedPassword, salt, hashErr := helper.HashPasswordPBKDF2(password)
			if hashErr != nil {
				return nil, fmt.Errorf("failed to hash password: %w", hashErr)
			}
			account.Password = &hashedPassword
			account.PasswordSalt = &salt
		}

		if account.Status == auth_model.AccountStatusPending {
			account.Status = auth_model.AccountStatusActive
		}

		if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
			return nil, fmt.Errorf("failed to update account: %w", err)
		}

		if err := s.tokenMgr.RevokeInvitationToken("", email, token); err != nil {
			return nil, fmt.Errorf("failed to revoke token: %w", err)
		}

		return account, nil
	}

	return s.registerService.Activate(ctx, workspaceID, email, token, name, password, lang, timezone)
}

// func (s *AccountService) IsAllowRegister() bool {
//     return false
// }

func (s *AccountService) GetAccountJWTToken(ctx context.Context, account *auth_model.Account) (string, error) {
	return jwt.GenerateTokenFixed(account.ID, account.Name)
}

func (s *AccountService) Authenticate(ctx context.Context, email, password, inviteToken string) (*auth_model.Account, error) {
	account, err := s.accountRepo.GetAccountByEmail(ctx, email)
	if err != nil {
		return nil, errors.New("account not found")
	}

	if err := accountLoginStatusError(account.Status); err != nil {
		return nil, err
	}

	if password != "" && len(inviteToken) != 0 && (account.Password == nil || account.Status == auth_model.AccountStatusPending) {
		hashedPassword, salt, err := helper.HashPasswordPBKDF2(password)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		account.Password = &hashedPassword
		account.PasswordSalt = &salt

		if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
			return nil, fmt.Errorf("failed to update account password: %w", err)
		}
	}

	if account.Password == nil {
		return nil, errors.New("account password not set")
	}

	valid, err := helper.ComparePasswordPBKDF2(password, *account.Password, *account.PasswordSalt)
	if err != nil || !valid {
		return nil, errors.New("invalid email or password")
	}

	if account.Status == auth_model.AccountStatusPending {
		account.Status = auth_model.AccountStatusActive
		s.accountRepo.UpdateAccount(ctx, account)
	}

	return account, nil
}

func (s *AccountService) IsEmailSendIPLimit(ctx context.Context, ipAddress string) (bool, error) {
	minuteKey := "email_send_ip_limit_minute:" + ipAddress
	freezeKey := "email_send_ip_limit_freeze:" + ipAddress
	hourLimitKey := "email_send_ip_limit_hour:" + ipAddress

	rdsClient := redisUtil.GetClient()

	freezeVal, err := redisUtil.GetString(ctx, freezeKey)
	if err == nil && freezeVal != "" {
		return true, nil
	}

	currentMinuteCountStr, err := redisUtil.GetString(ctx, minuteKey)
	currentMinuteCount := 0
	if err == nil && currentMinuteCountStr != "" {
		currentMinuteCount, _ = strconv.Atoi(currentMinuteCountStr)
	}

	if currentMinuteCount > 50 {
		hourLimitCountStr, err := redisUtil.GetString(ctx, hourLimitKey)
		hourLimitCount := 0
		if err == nil && hourLimitCountStr != "" {
			hourLimitCount, _ = strconv.Atoi(hourLimitCountStr)
		}

		if hourLimitCount >= 1 {
			rdsClient.SetEx(ctx, freezeKey, "1", time.Hour)
			return true, nil
		} else {
			rdsClient.SetEx(ctx, hourLimitKey, strconv.Itoa(hourLimitCount+1), 10*time.Minute)
		}

		rdsClient.Incr(ctx, hourLimitKey)
		rdsClient.Expire(ctx, hourLimitKey, time.Hour)

		return true, nil
	}

	rdsClient.SetEx(ctx, minuteKey, strconv.Itoa(currentMinuteCount+1), time.Minute)
	rdsClient.Expire(ctx, minuteKey, time.Minute)
	return false, nil
}

func (s *AccountService) UpdateLoginInfo(account *auth_model.Account, ipAddress string) error {
	now := time.Now()
	account.LastLoginAt = &now
	account.LastLoginIp = &ipAddress

	return s.accountRepo.UpdateAccount(context.Background(), account)
}

func (s *AccountService) GetResetPasswordData(ctx context.Context, token string) (map[string]interface{}, error) {

	tokenData, err := s.tokenMgr.GetTokenData(token, TokenTypeResetPassword)
	if err != nil {
		return nil, err
	}

	if tokenData == nil {
		return nil, errors.New("invalid token")
	}

	result := make(map[string]interface{})
	if tokenData.Email != nil {
		result["email"] = *tokenData.Email
	}
	if code, ok := tokenData.Extra["code"]; ok {
		result["code"] = code
	}

	return result, nil

}

func (s *AccountService) LoginCommon(account *auth_model.Account, ipAddress string) (*auth_model.TokenPair, error) {
	accessToken, err := s.GetAccountJWTToken(context.Background(), account)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := s.tokenMgr.GenerateToken(
		context.Background(),
		"refresh",
		account,
		nil,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	if err := s.UpdateLoginInfo(account, ipAddress); err != nil {
		logger.Warn("Failed to update login info: %v", err)
	}

	return &auth_model.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AccountService) LoginRefactored(ctx context.Context, req *dto.LoginReq) *LoginResult {
	if result := s.preCheckLogin(ctx, req); result != nil {
		return result
	}

	account, result := s.authenticateUserLogin(ctx, req)
	if result != nil {
		return result
	}

	if result := s.checkOrganizationLogin(ctx, account); result != nil {
		return result
	}

	return s.generateTokenAndBuildResponseLogin(ctx, account, req.LastLoginIp, req.Email)
}

func (s *AccountService) preCheckLogin(ctx context.Context, req *dto.LoginReq) *LoginResult {
	if IsLoginErrorRateLimit(req.Email) {
		logger.WarnContext(ctx, "login rejected: too many failed attempts")
		return NewBusinessErrorResult(helper.EmailPasswordLoginLimitError)
	}

	if err := s.validateInvitation(ctx, req.Email, req.InviteToken); err != nil {
		logger.WarnContext(ctx, "login rejected: invalid invitation token")
		return NewBusinessErrorResult(helper.InvalidEmailError)
	}

	return nil
}

func (s *AccountService) authenticateUserLogin(ctx context.Context, req *dto.LoginReq) (*auth_model.Account, *LoginResult) {
	account, err := s.Authenticate(ctx, req.Email, req.Password, req.InviteToken)
	if err != nil {
		return nil, s.handleAuthenticationErrorLogin(ctx, err, req.Email, req.Language)
	}
	return account, nil
}

func (s *AccountService) checkOrganizationLogin(ctx context.Context, account *auth_model.Account) *LoginResult {
	if isSelfHostedDeployment() {
		return nil
	}

	// Ensure user has own organization when needed
	hasOwnerGroup, err := s.hasOwnedEnterpriseGroup(ctx, account.ID)
	if err != nil {
		logger.CriticalContext(ctx, "failed to check organization ownership", "account_id", account.ID, err)
		return NewBusinessErrorResult(helper.UnknownError)
	}

	if !hasOwnerGroup {
		logger.Info("creating owned organization for account", "account_id", account.ID)

		_, err := s.createWorkspaceForExistingAccount(ctx, account)
		if err != nil {
			logger.CriticalContext(ctx, "failed to create organization during login", "account_id", account.ID, err)
			if strings.Contains(err.Error(), "frozen") || strings.Contains(err.Error(), "freeze") {
				return NewBusinessErrorResult(helper.AccountInFreezeError)
			}
			return NewBusinessErrorResult(helper.UnknownError)
		}
		logger.Info("owned organization created for account", "account_id", account.ID)
	}

	return nil
}

func (s *AccountService) hasOwnedEnterpriseGroup(ctx context.Context, accountID string) (bool, error) {
	ownedOrg, err := s.organizationService.GetFirstOwnedOrganization(ctx, accountID)
	if err != nil {
		logger.CriticalContext(ctx, "failed to get first owned organization", "account_id", accountID, err)
		return false, err
	}
	if ownedOrg != nil {
		return true, nil
	}

	return false, nil
}

func (s *AccountService) generateTokenAndBuildResponseLogin(ctx context.Context, account *auth_model.Account, ipAddress, email string) *LoginResult {
	tokenPair, err := s.LoginCommon(account, ipAddress)
	if err != nil {
		logger.CriticalContext(ctx, "failed to generate login token", "account_id", account.ID, err)
		return NewBusinessErrorResult(helper.UnknownError)
	}

	ResetLoginErrorRateLimit(email)

	accountProfile := &dto.AccountProfileResponse{
		ID:                account.ID,
		Name:              account.Name,
		Email:             account.Email,
		Avatar:            derefString(account.Avatar),
		InterfaceLanguage: derefString(account.InterfaceLanguage),
		Timezone:          derefString(account.Timezone),
		Status:            string(account.Status),
		IsSuperAdmin:      frontendSuperAdminFlag(account),
	}

	return NewSuccessResult(tokenPair, accountProfile)
}

func (s *AccountService) handleAuthenticationErrorLogin(ctx context.Context, err error, email, language string) *LoginResult {
	switch err.Error() {
	case "账户已被禁用":
		logger.WarnContext(ctx, "login rejected: account disabled")
		return NewBusinessErrorResult(helper.AccountBannedError)

	case "account is frozen":
		logger.WarnContext(ctx, "login rejected: account frozen")
		return NewBusinessErrorResult(helper.AccountInFreezeError)

	case "invalid email or password":
		s.AddLoginErrorRateLimit(context.Background(), email)
		logger.WarnContext(ctx, "login rejected: invalid credentials")
		return NewBusinessErrorResult(helper.EmailOrPasswordMismatchError)

	case "account not found", "Account not found":
		return s.handleAccountNotFoundLogin(ctx)

	default:
		logger.CriticalContext(ctx, "unexpected authentication error", err)
		return NewBusinessErrorResult(helper.UnknownError)
	}
}

func (s *AccountService) handleAccountNotFoundLogin(ctx context.Context) *LoginResult {
	logger.WarnContext(ctx, "login rejected: account not found")
	return NewBusinessErrorResult(helper.AccountNotFoundError)
}

func (s *AccountService) RegisterEx(
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
) (*auth_model.Account, error) {
	var account *auth_model.Account
	var defaultOrganizationID string
	var defaultWorkspaceID string

	err := s.accountRepo.ExecuteInTransaction(ctx, func(tx *gorm.DB) error {
		accountRepository := s.accountRepo.WithTx(tx)
		groupService := s.organizationManagementService.WithTx(tx)
		tenantService := s.workspaceManagementService.WithTx(tx)

		var hashedPassword, salt string
		if password != nil {
			var err error
			hashedPassword, salt, err = helper.HashPasswordPBKDF2(*password)
			if err != nil {
				return fmt.Errorf("failed to hash password: %w", err)
			}
		}
		account = &auth_model.Account{
			Name:  name,
			Email: email,
		}
		if password != nil {
			account.Password = &hashedPassword
			account.PasswordSalt = &salt
		}
		err := accountRepository.CreateAccount(ctx, account)
		if err != nil {
			return fmt.Errorf("failed to create account: %w", err)
		}

		groupName := fmt.Sprintf("%s's group %s", account.Name, uuid.New().String()[:8])
		group, err := groupService.CreateOrganization(ctx, groupName)
		if err != nil {
			return fmt.Errorf("failed to create group: %w", err)
		}

		if err := groupService.UpsertOrganizationRole(ctx, group.ID, account.ID, workspace_model.OrganizationRoleOwner); err != nil {
			return fmt.Errorf("failed to upsert group role: %w", err)
		}

		// Create default workspace
		defaultTenant, err := tenantService.CreateWorkspace(ctx, fmt.Sprintf("%s's Workspace", account.Name), true)
		if err != nil {
			return fmt.Errorf("failed to create default tenant: %w", err)
		}

		// Add tenant to group
		if err := groupService.AddWorkspace(ctx, group.ID, defaultTenant.ID); err != nil {
			return fmt.Errorf("failed to add tenant to group: %w", err)
		}

		if err := workspacebootstrap.EnsureOwnerWorkspaceMember(ctx, tenantService, account.ID, defaultTenant.ID); err != nil {
			return fmt.Errorf("failed to initialize default workspace state: %w", err)
		}
		defaultOrganizationID = group.ID
		defaultWorkspaceID = defaultTenant.ID

		if s.systemConfigService != nil {
			if err := s.systemConfigService.ConfigDefaultPluginAndConfig(ctx, group.ID, account); err != nil {
				return fmt.Errorf("failed to configure default plugins: %w", err)
			}
		}

		if s.eventBus != nil {
			if err := s.eventBus.Publish(ctx, "tenant.created", defaultTenant); err != nil {
				return fmt.Errorf("failed to publish tenant created event: %w", err)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if err := s.initializeAccountWorkspaceContext(ctx, account.ID, defaultOrganizationID, defaultWorkspaceID); err != nil {
		return nil, err
	}
	s.bootstrapOfficialRoute(ctx, defaultOrganizationID)

	s.notifyOfficialSignupRegistration(ctx, account)

	return account, nil
}

type CreatedOrganizationInfo struct {
	Organization *workspace_model.Organization `json:"organization"`
	Workspace    *workspace_model.Workspace    `json:"workspace"`
}

// createWorkspaceForExistingAccount creates workspace for an existing account without creating a new account
func (s *AccountService) createWorkspaceForExistingAccount(ctx context.Context, account *auth_model.Account) (*CreatedOrganizationInfo, error) {
	var createdInfo *CreatedOrganizationInfo
	var defaultOrganizationID string
	var defaultWorkspaceID string

	err := s.accountRepo.ExecuteInTransaction(ctx, func(tx *gorm.DB) error {
		groupService := s.organizationManagementService.WithTx(tx)
		tenantService := s.workspaceManagementService.WithTx(tx)

		// 1. Create an enterprise group
		groupName := fmt.Sprintf("%s's group %s", account.Name, uuid.New().String()[:8])
		group, err := groupService.CreateOrganization(ctx, groupName)
		if err != nil {
			return fmt.Errorf("failed to create group: %w", err)
		}

		// 2. Set user as group Owner
		if err := groupService.UpsertOrganizationRole(ctx, group.ID, account.ID, workspace_model.OrganizationRoleOwner); err != nil {
			return fmt.Errorf("failed to upsert group role: %w", err)
		}

		// 4. Create default workspace
		defaultTenant, err := tenantService.CreateWorkspace(ctx, fmt.Sprintf("%s's Workspace", account.Name), true)
		if err != nil {
			return fmt.Errorf("failed to create default tenant: %w", err)
		}

		// 5. Add workspace to group
		if err := groupService.AddWorkspace(ctx, group.ID, defaultTenant.ID); err != nil {
			return fmt.Errorf("failed to add tenant to group: %w", err)
		}

		if err := workspacebootstrap.EnsureOwnerWorkspaceMember(ctx, tenantService, account.ID, defaultTenant.ID); err != nil {
			return fmt.Errorf("failed to initialize default workspace state: %w", err)
		}
		defaultOrganizationID = group.ID
		defaultWorkspaceID = defaultTenant.ID

		// 6. Configure default plugins
		if s.systemConfigService != nil {
			if err := s.systemConfigService.ConfigDefaultPluginAndConfig(ctx, group.ID, account); err != nil {
				return fmt.Errorf("failed to configure default plugins: %w", err)
			}
		}

		// 7. Create default chat app
		// Currently, no default app is created as it is unused. Commented out for now. If there is a need to initialize an app or agent during user creation in the future, implement it here.
		// 8. Publish tenant created event
		if s.eventBus != nil {
			if err := s.eventBus.Publish(ctx, "tenant.created", defaultTenant); err != nil {
				return fmt.Errorf("failed to publish tenant created event: %w", err)
			}
		}

		// 9. Create tenant member and switch to default workspace
		// Not needed as CreateWorkspaceMember, current user is organization admin, has permission for all team members.
		// Kept commented out for potential future use if logic changes.
		// if err := tenantService.CreateWorkspaceMember(ctx, defaultTenant.ID, account.ID, "owner"); err != nil {
		//  return fmt.Errorf("failed to create tenant member: %w", err)
		// }
		// Store the created info to return outside the transaction
		createdInfo = &CreatedOrganizationInfo{
			Organization: group,
			Workspace:    defaultTenant,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	if err := s.initializeAccountWorkspaceContext(ctx, account.ID, defaultOrganizationID, defaultWorkspaceID); err != nil {
		return nil, err
	}
	s.bootstrapOfficialRoute(ctx, defaultOrganizationID)

	return createdInfo, nil
}

func (s *AccountService) bootstrapOfficialRoute(ctx context.Context, organizationID string) {
	if s.officialRouteBootstrapper == nil || organizationID == "" {
		return
	}

	organizationUUID, err := uuid.Parse(organizationID)
	if err != nil {
		logger.Warn("Failed to parse organization ID for official route bootstrap: %v", err)
		return
	}

	if err := s.officialRouteBootstrapper.InitOfficialChannel(ctx, organizationUUID); err != nil {
		logger.Warn("Failed to bootstrap official route after organization creation: %v", err)
	}
}

func (s *AccountService) initializeAccountWorkspaceContext(ctx context.Context, accountID, organizationID, workspaceID string) error {
	if organizationID == "" || workspaceID == "" {
		return nil
	}

	if _, err := s.UpdateAccountContext(ctx, accountID, &organizationID, &workspaceID); err != nil {
		return fmt.Errorf("failed to initialize current workspace context: %w", err)
	}

	return nil
}

func (s *AccountService) CreateAccount(ctx context.Context, req *dto.CreateAccountRequest) (*auth_model.Account, error) {
	exists, err := s.accountRepo.ExistsByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to check account email: %w", err)
	}
	if exists {
		return nil, errors.New("email already exists")
	}

	hashedPassword, salt, err := helper.HashPasswordPBKDF2(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}
	account := &auth_model.Account{
		Name:         req.Name,
		Email:        req.Email,
		Password:     &hashedPassword,
		PasswordSalt: &salt,
		Status:       auth_model.AccountStatusPending,
	}
	if req.Language != "" {
		lang := req.Language
		account.InterfaceLanguage = &lang
	}
	if req.Timezone != "" {
		tz := req.Timezone
		account.Timezone = &tz
	}

	if err := s.accountRepo.CreateAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}

func (s *AccountService) GetAccountExtensionByID(ctx context.Context, id string) (auth_model.JSONMap, error) {
	account, err := s.accountRepo.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	return account.Extensions, nil
}

func (s *AccountService) UpdateAccount(ctx context.Context, id string, req *dto.UpdateAccountRequest) error {
	account, err := s.accountRepo.GetAccount(ctx, id)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	if req.Name != "" {
		account.Name = req.Name
	}
	if req.Avatar != "" {
		avatar := req.Avatar
		account.Avatar = &avatar
	}
	if req.Status != "" {
		account.Status = req.Status
	}
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return err
	}
	s.invalidateAccountProfileCache(id)
	return nil
}

func (s *AccountService) DeleteAccount(ctx context.Context, id string) error {
	account, err := s.accountRepo.GetAccount(ctx, id)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	if err := s.softDeleteAccount(ctx, account); err != nil {
		return err
	}

	s.revokeAccountSessionsBestEffort(ctx, id)
	return nil
}

func (s *AccountService) DeleteCurrentAccount(ctx context.Context, id, password string) error {
	account, err := s.accountRepo.GetAccount(ctx, id)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	if account.Password == nil || account.PasswordSalt == nil {
		return ErrCurrentPasswordMismatch
	}

	valid, err := helper.ComparePasswordPBKDF2(password, *account.Password, *account.PasswordSalt)
	if err != nil {
		return fmt.Errorf("failed to verify password: %w", err)
	}
	if !valid {
		return ErrCurrentPasswordMismatch
	}

	if err := s.softDeleteAccount(ctx, account); err != nil {
		return err
	}

	s.revokeAccountSessionsBestEffort(ctx, id)
	return nil
}

func (s *AccountService) ChangePassword(ctx context.Context, id string, oldPassword, newPassword string) error {
	account, err := s.accountRepo.GetAccount(ctx, id)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	valid, err := helper.ComparePasswordPBKDF2(oldPassword, *account.Password, *account.PasswordSalt)
	if err != nil {
		return fmt.Errorf("failed to verify password: %w", err)
	}
	if !valid {
		return ErrCurrentPasswordMismatch
	}

	hashedPassword, salt, err := helper.HashPasswordPBKDF2(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	account.Password = &hashedPassword
	account.PasswordSalt = &salt
	return s.accountRepo.UpdateAccount(ctx, account)
}

func (s *AccountService) ResetPassword(ctx context.Context, resetToken, newPassword string) error {
	tokenData, err := s.tokenMgr.GetTokenData(resetToken, "reset_password")
	if err != nil || tokenData == nil || tokenData.Email == nil {
		return errors.New("invalid or expired reset token")
	}
	account, err := s.accountRepo.GetAccountByEmail(ctx, *tokenData.Email)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	hashedPassword, salt, err := helper.HashPasswordPBKDF2(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	account.Password = &hashedPassword
	account.PasswordSalt = &salt
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	_ = s.tokenMgr.RevokeToken(resetToken, "reset_password")
	return nil
}

func (s *AccountService) VerifyAccount(ctx context.Context, token string) error {
	return nil
}

func (s *AccountService) Login(ctx context.Context, req *dto.LoginReq) (*auth_model.TokenPair, error, dto.LoginResponse, helper.ErrorResponse) {
	resp := dto.LoginResponse{}

	if IsLoginErrorRateLimit(req.Email) {
		logger.WarnContext(ctx, "login rejected: too many failed attempts")
		return nil, errors.New("登录错误次数限制"), resp, helper.EmailPasswordLoginLimitError
	}

	language := determineLanguage(req.Language)

	if err := s.validateInvitation(ctx, req.Email, req.InviteToken); err != nil {
		logger.WarnContext(ctx, "login rejected: invalid invitation token")
		return nil, err, resp, helper.InvalidEmailError
	}

	account, err := s.Authenticate(ctx, req.Email, req.Password, req.InviteToken)
	if err != nil {
		authenticationError, err, resp, errorResponse := s.handleAuthenticationError(ctx, err, req.Email, language)
		return authenticationError, err, resp, errorResponse
	}

	// tenants, err := s.tenantService.GetJoinWorkspaces(ctx, account)
	// if err != nil {
	// 	logger.Error("failed to get tenant: %v", err)
	// 	return nil, errors.New("failed to get tenant information"), resp, helper.UnknownError
	// }

	// if len(tenants) == 0 {
	// 	logger.Warn("user has no associated workspace: %s", req.Email)

	// 	return nil, errors.New("workspace not found"), resp, helper.NotAllowedCreateWorkspaceError
	// }

	tokenPair, err := s.LoginCommon(account, req.LastLoginIp)
	if err != nil {
		logger.CriticalContext(ctx, "failed to generate login token", "account_id", account.ID, err)
		return nil, errors.New("生成登录令牌失败"), resp, helper.UnknownError
	}

	ResetLoginErrorRateLimit(req.Email)

	resp = dto.LoginResponse{
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
			IsSuperAdmin:      frontendSuperAdminFlag(account),
		},
	}

	return tokenPair, nil, resp, helper.ErrorResponse{}
}

func frontendSuperAdminFlag(account *auth_model.Account) *bool {
	if account == nil || !isSelfHostedDeployment() {
		return nil
	}

	isSuperAdmin := account.IsSuperAdmin
	return &isSuperAdmin
}

func isSelfHostedDeployment() bool {
	cfg := config.GlobalConfig
	if cfg == nil {
		return true
	}

	return !strings.EqualFold(strings.TrimSpace(cfg.Platform.Edition), "CLOUD")
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *AccountService) validateInvitation(ctx context.Context, email string, inviteToken string) error {
	if inviteToken == "" {
		return nil
	}

	if s.registerService == nil {
		return errors.New("register service not initialized")
	}

	valid, err := s.registerService.ValidateInvitationCode(ctx, inviteToken)
	if err != nil {
		return fmt.Errorf("failed to validate invitation code: %w", err)
	}

	if !valid {
		return errors.New("invalid invitation code")
	}

	invitationData, err := s.registerService.GetInvitationData(ctx, inviteToken)
	if err != nil {
		return fmt.Errorf("failed to get invitation data: %w", err)
	}

	if invitedEmail, ok := invitationData["email"].(string); ok {
		if invitedEmail != email {
			return errors.New("email does not match invitation")
		}
	}

	return nil
}

func IsLoginErrorRateLimit(email string) bool {
	key := loginErrorRateLimitKey(email)

	ctx := context.Background()
	countStr, err := redisUtil.RedisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		return false
	} else if err != nil {
		return false
	}

	count, err := strconv.Atoi(countStr)
	if err != nil {
		return false
	}

	return count >= helper.LoginMaxErrorLimits
}
func determineLanguage(reqLanguage string) string {
	if reqLanguage == "zh-Hans" {
		return "zh-Hans"
	}
	return "en-US"
}

func (s *AccountService) handleAuthenticationError(ctx context.Context, err error, email, language string) (*auth_model.TokenPair, error, dto.LoginResponse, helper.ErrorResponse) {
	resp := dto.LoginResponse{}

	switch err.Error() {
	case "账户已被禁用":
		logger.WarnContext(ctx, "login rejected: account disabled")
		return nil, errors.New("账户已被禁用"), resp, helper.AccountBannedError

	case "account is frozen":
		logger.WarnContext(ctx, "login rejected: account frozen")
		return nil, errors.New("account is frozen"), resp, helper.AccountInFreezeError

	case "invalid email or password":
		s.AddLoginErrorRateLimit(context.Background(), email)
		logger.WarnContext(ctx, "login rejected: invalid credentials")
		return nil, errors.New("邮箱或密码不匹配"), resp, helper.EmailOrPasswordMismatchError

	case "account not found", "Account not found":
		logger.WarnContext(ctx, "login rejected: account not found")
		return nil, errors.New("未找到账户"), resp, helper.AccountNotFoundError

	case "用户名或密码错误":
		logger.WarnContext(ctx, "login rejected: invalid username or password")
		return nil, errors.New("未找到账户"), resp, helper.AccountNotFoundError

	default:
		logger.CriticalContext(ctx, "unexpected authentication error", err)
		return nil, errors.New("未知错误"), resp, helper.UnknownError
	}
}

func ResetLoginErrorRateLimit(email string) {
	key := loginErrorRateLimitKey(email)
	redisUtil.RedisClient.Del(context.Background(), key)
}

func (s *AccountService) SetTenantService(tenantService interfaces.WorkspaceManagementService) {
	s.workspaceManagementService = tenantService
}

func (s *AccountService) SetRegisterService(registerService interfaces.RegisterService) {
	s.registerService = registerService
}

func (s *AccountService) SetOrganizationService(organizationService interfaces.OrganizationService) {
	s.organizationService = organizationService
	if s.officialSignupRegistration != nil {
		s.officialSignupRegistration.SetOrganizationLookup(organizationService)
	}
}

func (s *AccountService) SetOfficialRouteBootstrapper(bootstrapper interfaces.OfficialRouteBootstrapper) {
	s.officialRouteBootstrapper = bootstrapper
}

// ///////////////////////////////////////////////////////////////////
// CheckRegisterValidity implements the CheckRegisterValidity method
func (s *AccountService) CheckRegisterValidity(ctx context.Context, email, code, token string) (bool, error) {
	return s.registerService.CheckRegisterValidity(ctx, email, code, token)
}

// ValidateResetPasswordToken implements the ValidateResetPasswordToken method
func (s *AccountService) ValidateResetPasswordToken(token, email, code string) (bool, string, error) {
	if s.IsForgotPasswordErrorRateLimit(email) {
		return false, "", errors.New("password reset limit reached")
	}

	tokenData, err := s.tokenMgr.GetTokenData(token, TokenTypeResetPassword)
	if err != nil || tokenData == nil || tokenData.Email == nil {
		return false, "", errors.New("invalid or expired token")
	}

	if email != *tokenData.Email {
		return false, *tokenData.Email, errors.New("invalid email")
	}

	codeInToken := ""
	if tokenData.Extra != nil {
		if v, ok := tokenData.Extra["code"]; ok {
			codeInToken, _ = v.(string)
		}
	}

	// Master verification code for testing/development
	masterCode := config.Current().Auth.MasterVerificationCode
	if code != codeInToken && (masterCode == "" || code != masterCode) {
		s.AddForgotPasswordErrorRateLimit(email)
		return false, *tokenData.Email, errors.New("invalid code")
	}

	s.ResetForgotPasswordErrorRateLimit(email)
	return true, *tokenData.Email, nil
}

// ResetPasswordWithAutoRegister implements the ResetPasswordWithAutoRegister method
func (s *AccountService) ResetPasswordWithAutoRegister(token, newPassword string) error {
	tokenData, err := s.tokenMgr.GetTokenData(token, TokenTypeResetPassword)
	if err != nil || tokenData == nil || tokenData.Email == nil {
		return errors.New("invalid or expired token")
	}

	_ = s.tokenMgr.RevokeToken(token, TokenTypeResetPassword)

	hashedPassword, salt, err := helper.HashPasswordPBKDF2(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	ctx := context.Background()
	account, err := s.accountRepo.GetAccountByEmail(ctx, *tokenData.Email)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}
	if err := accountAccessStatusError(account.Status); err != nil {
		return err
	}

	account.Password = &hashedPassword
	account.PasswordSalt = &salt
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// UpdateAccountPassword implements the UpdateAccountPassword method
func (s *AccountService) UpdateAccountPassword(ctx context.Context, account *auth_model.Account, password, newPassword string) error {
	if account.Password != nil {
		valid, err := helper.ComparePasswordPBKDF2(password, *account.Password, *account.PasswordSalt)
		if err != nil {
			return fmt.Errorf("failed to verify password: %w", err)
		}
		if !valid {
			return errors.New("current password is incorrect")
		}
	}

	hashedPassword, salt, err := helper.HashPasswordPBKDF2(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	account.Password = &hashedPassword
	account.PasswordSalt = &salt

	return s.accountRepo.UpdateAccount(ctx, account)
}

// CreateAccountEx implements the CreateAccountEx method
func (s *AccountService) CreateAccountEx(ctx context.Context, account *auth_model.Account, mobile string, gender *auth_model.GenderEnum) (auth_model.JSONMap, error) {
	var genderStr *string
	if gender != nil {
		g := string(*gender)
		genderStr = &g
	}
	mobileStr := &mobile
	if mobile == "" {
		mobileStr = nil
	}
	setAccountMobile(account, mobileStr)
	updateAccountExtensions(account, mobileStr, nil, nil, genderStr)

	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return nil, fmt.Errorf("failed to update account extensions: %w", err)
	}

	return account.Extensions, nil
}

// CreateAccountAndTenant implements the CreateAccountAndTenant method
func (s *AccountService) CreateAccountAndTenant(ctx context.Context, email, name, interfaceLanguage string, password *string) (*auth_model.Account, error) {
	createReq := &dto.CreateAccountRequest{
		Email:    email,
		Name:     name,
		Language: interfaceLanguage,
	}
	if password != nil {
		createReq.Password = *password
	}

	account, err := s.CreateAccount(ctx, createReq)
	if err != nil {
		return nil, err
	}

	return account, nil
}

// GenerateAccountDeletionVerificationCode implements the GenerateAccountDeletionVerificationCode method
func (s *AccountService) GenerateAccountDeletionVerificationCode(ctx context.Context, account *auth_model.Account) (string, string, error) {
	code := generateRandomCode(6)

	additionalData := map[string]interface{}{
		"code": code,
		"exp":  time.Now().Add(time.Hour).Unix(),
	}

	token, err := s.tokenMgr.GenerateToken(
		ctx,
		"account_deletion",
		nil,
		&account.Email,
		additionalData,
	)

	if err != nil {
		return "", "", err
	}

	return token, code, nil
}

// SendAccountDeletionVerificationEmail implements the SendAccountDeletionVerificationEmail method
func (s *AccountService) SendAccountDeletionVerificationEmail(ctx context.Context, account *auth_model.Account, code string) error {
	return nil
}

// VerifyAccountDeletionCode implements the VerifyAccountDeletionCode method
func (s *AccountService) VerifyAccountDeletionCode(ctx context.Context, token, code string) (bool, error) {
	tokenData, err := s.tokenMgr.GetTokenData(token, "account_deletion")
	if err != nil {
		return false, err
	}

	if tokenData == nil {
		return false, errors.New("invalid token")
	}

	storedCode, ok := tokenData.Extra["code"].(string)
	// Master verification code for testing/development
	masterCode := config.Current().Auth.MasterVerificationCode
	if !ok || (storedCode != code && (masterCode == "" || code != masterCode)) {
		return false, nil
	}

	return true, nil
}

// func buildAccountExtensionFromJSONMap(extensions auth_model.JSONMap) *auth_model.AccountExtension {
// 	ext := &auth_model.AccountExtension{}
// 	if extensions == nil {
// 		return ext
// 	}
// 	if mobile, ok := extensions["mobile"].(string); ok {
// 		ext.Mobile = &mobile
// 	}
// 	if wechat, ok := extensions["wechat"].(string); ok {
// 		ext.Wechat = &wechat
// 	}
// 	if address, ok := extensions["address"].(string); ok {
// 		ext.Address = &address
// 	}
// 	if gender, ok := extensions["gender"].(string); ok {
// 		g := auth_model.GenderEnum(gender)
// 		ext.Gender = &g
// 	}
// 	if birthdateStr, ok := extensions["birthdate"].(string); ok {
// 		if t, err := time.Parse(time.RFC3339, birthdateStr); err == nil {
// 			ext.Birthdate = &t
// 		}
// 	}
// 	return ext
// }

func updateAccountExtensions(account *auth_model.Account, mobile, wechat, address *string, gender *string) {
	if account.Extensions == nil {
		account.Extensions = make(auth_model.JSONMap)
	}
	if mobile != nil {
		if *mobile == "" {
			delete(account.Extensions, "mobile")
		} else {
			account.Extensions["mobile"] = *mobile
		}
	}
	if wechat != nil {
		if *wechat == "" {
			delete(account.Extensions, "wechat")
		} else {
			account.Extensions["wechat"] = *wechat
		}
	}
	if address != nil {
		if *address == "" {
			delete(account.Extensions, "address")
		} else {
			account.Extensions["address"] = *address
		}
	}
	if gender != nil {
		if *gender == "" {
			delete(account.Extensions, "gender")
		} else {
			account.Extensions["gender"] = *gender
		}
	}
}

// DeleteAccountPermanently implements the DeleteAccountPermanently method
func (s *AccountService) DeleteAccountPermanently(ctx context.Context, account *auth_model.Account) error {
	return s.accountRepo.GetDB().WithContext(ctx).Unscoped().Where("id = ?", account.ID).Delete(&auth_model.Account{}).Error
}

// SetAccountRole implements the SetAccountRole method
// GetGroupRole implements the GetGroupRole method
func (s *AccountService) GetGroupRole(ctx context.Context, accountID string) (string, error) {
	currentTenantJoin, err := s.workspaceManagementService.GetCurrentWorkspace(ctx, accountID)
	if err != nil {
		return "normal", err
	}
	if currentTenantJoin == nil {
		return "normal", nil
	}

	return s.accountRepo.GetGroupRoleByTenantID(ctx, accountID, currentTenantJoin.WorkspaceID)
}

// GetGroupRoleByTenantID implements the GetGroupRoleByTenantID method
func (s *AccountService) GetGroupRoleByTenantID(ctx context.Context, accountID string, tenantID string) (string, error) {
	return s.accountRepo.GetGroupRoleByTenantID(ctx, accountID, tenantID)
}

// IsOrganizationAdminOrOwner checks if the user is an admin or owner of the organization
func (s *AccountService) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	if s.organizationService == nil {
		return false, fmt.Errorf("enterprise service is not initialized")
	}
	return s.organizationService.IsOrganizationAdminOrOwner(ctx, organizationID, accountID)
}

// IsOrganizationMember checks if the account is a member of the organization.
func (s *AccountService) IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error) {
	if s.organizationService == nil {
		return false, fmt.Errorf("enterprise service is not initialized")
	}
	if organizationID == "" || accountID == "" {
		return false, nil
	}
	return s.organizationService.IsOrganizationMember(ctx, organizationID, accountID)
}

// CheckGroupAdminByWorkspace keeps legacy tenant naming for compatibility.
// tenantID can be either workspace ID or organization ID; repository resolves to organization scope.
func (s *AccountService) CheckGroupAdminByWorkspace(ctx context.Context, accountID, tenantID string) (bool, error) {
	groupIDs, err := s.accountRepo.GetEnterpriseGroupsByTenantID(ctx, tenantID)
	if err != nil {
		return false, fmt.Errorf("failed to get enterprise groups by tenant ID: %w", err)
	}

	if len(groupIDs) == 0 {
		return false, nil
	}

	for _, groupID := range groupIDs {
		role, err := s.accountRepo.GetAccountRoleInEnterpriseGroup(ctx, accountID, groupID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return false, fmt.Errorf("failed to get account role in enterprise group: %w", err)
		}

		if role == string(workspace_model.OrganizationRoleOwner) || role == string(workspace_model.OrganizationRoleAdmin) {
			return true, nil
		}
	}

	return false, nil
}

func (s *AccountService) CheckTenantAdmin(ctx context.Context, accountID string, tenantID string) (bool, error) {
	join, err := s.workspaceManagementService.GetByWorkspaceAndMember(ctx, tenantID, accountID)
	if err != nil || join == nil {
		return false, err
	}

	// Owner has all permissions
	if join.Role == workspace_model.WorkspaceRoleOwner {
		return true, nil
	}

	// Admin has most permissions
	if join.Role == workspace_model.WorkspaceRoleAdmin {
		return true, nil
	}
	return false, nil
}

// CloseAccount implements the CloseAccount method
func (s *AccountService) CloseAccount(ctx context.Context, account *auth_model.Account) error {
	account.Status = auth_model.AccountStatusClosed
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return err
	}
	s.invalidateAccountProfileCache(account.ID)
	return nil
}

func (s *AccountService) softDeleteAccount(ctx context.Context, account *auth_model.Account) error {
	if err := s.accountRepo.ExecuteInTransaction(ctx, func(tx *gorm.DB) error {
		txRepo := s.accountRepo.WithTx(tx)

		account.Status = auth_model.AccountStatusClosed
		if err := txRepo.UpdateAccount(ctx, account); err != nil {
			return fmt.Errorf("close account: %w", err)
		}

		if err := txRepo.DeleteAccount(ctx, account.ID); err != nil {
			return fmt.Errorf("soft delete account: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}
	s.invalidateAccountProfileCache(account.ID)
	return nil
}

func (s *AccountService) revokeAccountSessionsBestEffort(_ context.Context, accountID string) {
	if accountID == "" {
		return
	}

	revokeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.tokenMgr != nil {
		if err := s.tokenMgr.RevokeCurrentTokenForAccount(revokeCtx, accountID, TokenTypeRefresh); err != nil {
			logger.Warn("Failed to revoke refresh token for deleted account: %v", err)
		}
		if err := s.tokenMgr.RevokeCurrentTokenForAccount(revokeCtx, accountID, TokenTypeAccess); err != nil {
			logger.Warn("Failed to revoke access token for deleted account: %v", err)
		}
	}

	if redisClient := redisUtil.GetClient(); redisClient != nil {
		if err := redisClient.Del(revokeCtx, "refresh_token:"+accountID).Err(); err != nil {
			logger.Warn("Failed to revoke legacy refresh token for deleted account: %v", err)
		}
	}
}

// GetCurrentWorkspace implements the GetCurrentWorkspace method
func (s *AccountService) GetCurrentWorkspace(ctx context.Context, accountID string) (*workspace_model.Workspace, error) {
	return getCurrentWorkspaceStrict(ctx, s.workspaceManagementService, accountID)
}

func getCurrentWorkspaceStrict(ctx context.Context, lookup currentWorkspaceLookup, accountID string) (*workspace_model.Workspace, error) {
	currentJoin, err := lookup.GetCurrentWorkspace(ctx, accountID)
	if err != nil {
		return nil, err
	}

	if currentJoin == nil {
		return nil, nil
	}

	if strings.TrimSpace(currentJoin.WorkspaceID) == "" {
		return nil, nil
	}

	tenant, err := lookup.GetWorkspaceByID(ctx, currentJoin.WorkspaceID)
	if err != nil {
		return nil, err
	}

	return tenant, nil
}

// EnsureCurrentOrganizationID ensures that the account has a current organization set in the context
func (s *AccountService) EnsureCurrentOrganizationID(ctx context.Context, accountID string) (string, error) {
	ctxModel, err := s.GetAccountContext(ctx, accountID)
	if err != nil {
		return "", err
	}

	if ctxModel == nil {
		ctxModel = &auth_model.AccountContext{
			AccountID: accountID,
		}
	}

	if ctxModel.CurrentOrganizationID != nil {
		return *ctxModel.CurrentOrganizationID, nil
	}

	if s.populateDefaultOrganization(ctx, ctxModel) {
		if _, err := s.UpdateAccountContext(ctx, accountID, ctxModel.CurrentOrganizationID, nil); err != nil {
			logger.Warn("Failed to update account context with default organization: %v", err)
		}
		if ctxModel.CurrentOrganizationID != nil {
			return *ctxModel.CurrentOrganizationID, nil
		}
	}

	return "", fmt.Errorf("no organization found for account")
}

func (s *AccountService) GetAccountContext(ctx context.Context, accountID string) (*auth_model.AccountContext, error) {
	cacheToken := workspacecache.NewAccountScopedToken(ctx, accountID)
	if cached, ok := workspacecache.GetAccountContext(ctx, cacheToken); ok {
		return cached, nil
	}

	result, err, _ := s.accountContextGroup.Do(accountID, func() (interface{}, error) {
		cacheToken := workspacecache.NewAccountScopedToken(ctx, accountID)
		if cached, ok := workspacecache.GetAccountContext(ctx, cacheToken); ok {
			return cached, nil
		}
		return s.loadAccountContextUncached(ctx, accountID, cacheToken)
	})
	if err != nil {
		return nil, err
	}

	ctxModel, ok := result.(*auth_model.AccountContext)
	if !ok {
		return nil, fmt.Errorf("unexpected account context cache value %T", result)
	}
	return ctxModel, nil
}

func (s *AccountService) loadAccountContextUncached(ctx context.Context, accountID string, cacheToken workspacecache.AccountScopedToken) (*auth_model.AccountContext, error) {
	ctxModel, err := s.accountRepo.GetAccountContextByAccountID(ctx, accountID)
	if err != nil {
		return nil, err
	}

	isNew := false
	if ctxModel == nil {
		now := time.Now()
		ctxModel = &auth_model.AccountContext{
			AccountID: accountID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		isNew = true
	}

	changed, err := s.repairAccountContextWorkspace(ctx, ctxModel)
	if err != nil {
		return nil, err
	}
	if changed {
		ctxModel.UpdatedAt = time.Now()
	}

	if isNew {
		if err := s.accountRepo.CreateAccountContext(ctx, ctxModel); err != nil {
			return nil, err
		}
		workspacecache.SetAccountContext(ctx, cacheToken, ctxModel)
	} else if changed {
		if err := s.accountRepo.UpdateAccountContext(ctx, ctxModel); err != nil {
			logger.Warn("Failed to update account context with resolved workspace: %v", err)
		} else {
			s.invalidateAccountProfileCache(accountID)
			workspacecache.InvalidateAccount(ctx, accountID)
			workspacecache.SetAccountContext(ctx, workspacecache.NewAccountScopedToken(ctx, accountID), ctxModel)
		}
	} else {
		workspacecache.SetAccountContext(ctx, cacheToken, ctxModel)
	}

	return ctxModel, nil
}

func (s *AccountService) EnsureAccountContextForWorkspace(ctx context.Context, accountID, organizationID, workspaceID string) (*auth_model.AccountContext, bool, error) {
	accountID = strings.TrimSpace(accountID)
	organizationID = strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	if accountID == "" || organizationID == "" || workspaceID == "" {
		return nil, false, fmt.Errorf("account id, organization id and workspace id are required")
	}
	if s.workspaceManagementService == nil {
		return nil, false, fmt.Errorf("workspace management service is not initialized")
	}

	targetWorkspace, err := s.workspaceManagementService.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, fmt.Errorf("workspace %s not found", workspaceID)
		}
		return nil, false, err
	}
	if targetWorkspace == nil {
		return nil, false, fmt.Errorf("workspace %s not found", workspaceID)
	}
	if targetWorkspace.Status != workspace_model.WorkspaceStatusNormal {
		return nil, false, fmt.Errorf("workspace %s is not active", workspaceID)
	}
	if targetWorkspace.OrganizationID == nil || strings.TrimSpace(*targetWorkspace.OrganizationID) != organizationID {
		return nil, false, fmt.Errorf("workspace %s does not belong to organization %s", workspaceID, organizationID)
	}

	isMember, err := s.isAccountOrganizationMember(ctx, accountID, organizationID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check organization membership: %w", err)
	}
	if !isMember {
		return nil, false, fmt.Errorf("account %s is not a member of organization %s", accountID, organizationID)
	}

	canAccessTarget, err := s.canAccountAccessWorkspace(ctx, accountID, organizationID, workspaceID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to check workspace access: %w", err)
	}
	if !canAccessTarget {
		return nil, false, fmt.Errorf("account %s cannot access workspace %s", accountID, workspaceID)
	}

	ctxModel, err := s.accountRepo.GetAccountContextByAccountID(ctx, accountID)
	if err != nil {
		return nil, false, err
	}

	if ctxModel != nil {
		if ctxModel.AccountID == "" {
			ctxModel.AccountID = accountID
		}
		currentWorkspaceID := ptrStringValue(ctxModel.CurrentWorkspaceID)
		if currentWorkspaceID != "" {
			currentWorkspace, err := s.resolveWorkspaceOrganizationContext(ctx, accountID, currentWorkspaceID)
			if err != nil {
				return nil, false, err
			}
			if currentWorkspace != nil && currentWorkspace.OrganizationID != nil {
				resolvedOrganizationID := strings.TrimSpace(*currentWorkspace.OrganizationID)
				currentOrganizationID := ptrStringValue(ctxModel.CurrentOrganizationID)
				if resolvedOrganizationID != "" && currentOrganizationID == "" {
					updatedCtx, err := s.UpdateAccountContext(ctx, accountID, &resolvedOrganizationID, &currentWorkspace.ID)
					if err != nil {
						return nil, false, err
					}
					if currentWorkspace.ID == workspaceID {
						if err := s.syncCurrentWorkspaceMember(ctx, accountID, workspaceID); err != nil {
							return nil, true, err
						}
						return updatedCtx, true, nil
					}
					return updatedCtx, false, nil
				}
				if resolvedOrganizationID != "" && currentOrganizationID == resolvedOrganizationID {
					if currentWorkspace.ID == workspaceID {
						if err := s.syncCurrentWorkspaceMember(ctx, accountID, workspaceID); err != nil {
							return nil, true, err
						}
						return ctxModel, true, nil
					}
					return ctxModel, false, nil
				}
			}
		}
	}

	updatedCtx, err := s.UpdateAccountContext(ctx, accountID, &organizationID, &workspaceID)
	if err != nil {
		return nil, false, err
	}
	if err := s.syncCurrentWorkspaceMember(ctx, accountID, workspaceID); err != nil {
		return nil, true, err
	}
	return updatedCtx, true, nil
}

func (s *AccountService) populateDefaultOrganization(ctx context.Context, ctxModel *auth_model.AccountContext) bool {
	// 1. First owned
	group, err := s.organizationService.GetFirstOwnedOrganization(ctx, ctxModel.AccountID)
	if err == nil && group != nil {
		ctxModel.CurrentOrganizationID = &group.ID
		return true
	}

	// 2. First joined
	group, err = s.organizationService.GetFirstJoinedOrganization(ctx, ctxModel.AccountID)
	if err == nil && group != nil {
		ctxModel.CurrentOrganizationID = &group.ID
		return true
	}

	return false
}

func (s *AccountService) repairAccountContextWorkspace(ctx context.Context, ctxModel *auth_model.AccountContext) (bool, error) {
	if ctxModel == nil {
		return false, nil
	}
	if ctxModel.AccountID == "" {
		return false, fmt.Errorf("account context account id is required")
	}

	changed := false
	accountID := ctxModel.AccountID
	currentOrganizationID := ptrStringValue(ctxModel.CurrentOrganizationID)
	currentWorkspaceID := ptrStringValue(ctxModel.CurrentWorkspaceID)

	if currentOrganizationID != "" {
		isMember, err := s.isAccountOrganizationMember(ctx, accountID, currentOrganizationID)
		if err != nil {
			return false, fmt.Errorf("failed to check organization membership: %w", err)
		}
		if !isMember {
			changed = clearStringPtrIfChanged(&ctxModel.CurrentOrganizationID) || changed
			currentOrganizationID = ""
		}
	}

	if currentOrganizationID == "" && currentWorkspaceID != "" {
		workspace, err := s.resolveWorkspaceOrganizationContext(ctx, accountID, currentWorkspaceID)
		if err != nil {
			return false, err
		}
		if workspace != nil && workspace.OrganizationID != nil && *workspace.OrganizationID != "" {
			changed = setStringPtrIfChanged(&ctxModel.CurrentOrganizationID, *workspace.OrganizationID) || changed
			changed = setStringPtrIfChanged(&ctxModel.CurrentWorkspaceID, workspace.ID) || changed
			currentOrganizationID = *workspace.OrganizationID
			currentWorkspaceID = workspace.ID
		} else {
			changed = clearStringPtrIfChanged(&ctxModel.CurrentWorkspaceID) || changed
			currentWorkspaceID = ""
		}
	}

	if currentOrganizationID == "" {
		if s.populateDefaultOrganization(ctx, ctxModel) {
			changed = true
			currentOrganizationID = ptrStringValue(ctxModel.CurrentOrganizationID)
		}
	}

	if currentOrganizationID != "" {
		workspace, err := s.resolveWorkspaceForOrganizationContext(ctx, accountID, ctxModel, currentOrganizationID)
		if err != nil {
			return false, err
		}
		if workspace != nil {
			changed = setStringPtrIfChanged(&ctxModel.CurrentWorkspaceID, workspace.ID) || changed
			currentWorkspaceID = workspace.ID
		} else {
			changed = clearStringPtrIfChanged(&ctxModel.CurrentWorkspaceID) || changed
			currentWorkspaceID = ""
		}
	}

	if currentWorkspaceID == "" {
		workspace, err := s.resolveAnyAccessibleWorkspace(ctx, accountID)
		if err != nil {
			return false, err
		}
		if workspace != nil && workspace.OrganizationID != nil && *workspace.OrganizationID != "" {
			changed = setStringPtrIfChanged(&ctxModel.CurrentOrganizationID, *workspace.OrganizationID) || changed
			changed = setStringPtrIfChanged(&ctxModel.CurrentWorkspaceID, workspace.ID) || changed
		}
	}

	return changed, nil
}

func (s *AccountService) UpdateAccountContext(ctx context.Context, accountID string, organizationID, workspaceID *string) (*auth_model.AccountContext, error) {
	ctxModel, err := s.accountRepo.GetAccountContextByAccountID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	isNew := false
	if ctxModel == nil {
		ctxModel = &auth_model.AccountContext{
			AccountID: accountID,
		}
		isNew = true
	}

	if organizationID != nil && *organizationID != "" {
		isMember, err := s.isAccountOrganizationMember(ctx, accountID, *organizationID)
		if err != nil {
			return nil, fmt.Errorf("failed to check organization membership: %w", err)
		}
		if !isMember {
			return nil, fmt.Errorf("account %s is not a member of organization %s", accountID, *organizationID)
		}
	}

	var resolvedWorkspaceID *string
	if workspaceID != nil && *workspaceID != "" {
		workspace, err := s.workspaceManagementService.GetWorkspaceByID(ctx, *workspaceID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("workspace %s not found", *workspaceID)
			}
			return nil, err
		}
		if workspace == nil {
			return nil, fmt.Errorf("workspace %s not found", *workspaceID)
		}
		if workspace.Status != workspace_model.WorkspaceStatusNormal {
			return nil, fmt.Errorf("workspace %s is not active", *workspaceID)
		}

		var targetOrganizationID *string
		if organizationID != nil {
			if *organizationID != "" {
				targetOrganizationID = organizationID
			}
		} else if workspace.OrganizationID != nil && *workspace.OrganizationID != "" {
			resolvedOrganizationID := *workspace.OrganizationID
			organizationID = &resolvedOrganizationID
			targetOrganizationID = &resolvedOrganizationID
		} else if ctxModel.CurrentOrganizationID != nil && *ctxModel.CurrentOrganizationID != "" {
			targetOrganizationID = ctxModel.CurrentOrganizationID
		}

		if targetOrganizationID != nil {
			if workspace.OrganizationID == nil || *workspace.OrganizationID != *targetOrganizationID {
				return nil, fmt.Errorf("workspace %s does not belong to organization %s", *workspaceID, *targetOrganizationID)
			}
		} else if workspace.OrganizationID != nil && *workspace.OrganizationID != "" {
			targetOrganizationID = workspace.OrganizationID
		} else {
			return nil, fmt.Errorf("workspace %s does not belong to an organization", *workspaceID)
		}

		isMember, err := s.isAccountOrganizationMember(ctx, accountID, *targetOrganizationID)
		if err != nil {
			return nil, fmt.Errorf("failed to check organization membership: %w", err)
		}
		if !isMember {
			return nil, fmt.Errorf("account %s is not a member of organization %s", accountID, *targetOrganizationID)
		}

		canAccess, err := s.canAccountAccessWorkspace(ctx, accountID, *targetOrganizationID, workspace.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check workspace access: %w", err)
		}
		if !canAccess {
			return nil, fmt.Errorf("account %s cannot access workspace %s", accountID, workspace.ID)
		}
		resolvedWorkspaceID = workspaceID
	}

	if organizationID != nil && *organizationID != "" && (workspaceID == nil || *workspaceID == "") {
		workspace, err := s.resolveWorkspaceForOrganizationContext(ctx, accountID, ctxModel, *organizationID)
		if err != nil {
			return nil, err
		}
		if workspace != nil {
			workspaceIDValue := workspace.ID
			resolvedWorkspaceID = &workspaceIDValue
		}
	}

	if organizationID == nil && workspaceID != nil && *workspaceID == "" && ctxModel.CurrentOrganizationID != nil && *ctxModel.CurrentOrganizationID != "" {
		workspace, err := s.resolveDefaultWorkspaceForOrganization(ctx, accountID, *ctxModel.CurrentOrganizationID)
		if err != nil {
			return nil, err
		}
		if workspace != nil {
			workspaceIDValue := workspace.ID
			resolvedWorkspaceID = &workspaceIDValue
		}
	}

	if organizationID != nil {
		if *organizationID == "" {
			ctxModel.CurrentOrganizationID = nil
			ctxModel.CurrentWorkspaceID = nil
		} else {
			ctxModel.CurrentOrganizationID = organizationID
		}
	}
	if workspaceID != nil {
		if resolvedWorkspaceID != nil {
			ctxModel.CurrentWorkspaceID = resolvedWorkspaceID
		} else if *workspaceID == "" {
			ctxModel.CurrentWorkspaceID = nil
		} else {
			ctxModel.CurrentWorkspaceID = workspaceID
		}
	}
	if workspaceID == nil && organizationID != nil && *organizationID != "" {
		ctxModel.CurrentWorkspaceID = resolvedWorkspaceID
	}

	if ctxModel.CreatedAt.IsZero() {
		ctxModel.CreatedAt = now
	}

	ctxModel.UpdatedAt = now

	if ctxModel.AccountID == "" {
		ctxModel.AccountID = accountID
	}

	if isNew {
		if err := s.accountRepo.CreateAccountContext(ctx, ctxModel); err != nil {
			return nil, err
		}
	} else {
		if err := s.accountRepo.UpdateAccountContext(ctx, ctxModel); err != nil {
			return nil, err
		}
	}

	s.invalidateAccountProfileCache(accountID)
	workspacecache.InvalidateAccount(ctx, accountID)
	workspacecache.SetAccountContext(ctx, workspacecache.NewAccountScopedToken(ctx, accountID), ctxModel)
	return ctxModel, nil
}

func (s *AccountService) syncCurrentWorkspaceMember(ctx context.Context, accountID, workspaceID string) error {
	accountID = strings.TrimSpace(accountID)
	workspaceID = strings.TrimSpace(workspaceID)
	if s.db == nil || accountID == "" || workspaceID == "" {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&workspace_model.WorkspaceMember{}).
			Where("account_id = ?", accountID).
			Update("current", false).Error; err != nil {
			return fmt.Errorf("failed to clear current workspace: %w", err)
		}
		if err := tx.Model(&workspace_model.WorkspaceMember{}).
			Where("account_id = ? AND workspace_id = ?", accountID, workspaceID).
			Update("current", true).Error; err != nil {
			return fmt.Errorf("failed to set current workspace: %w", err)
		}
		return nil
	})
}

func (s *AccountService) resolveWorkspaceOrganizationContext(ctx context.Context, accountID, workspaceID string) (*workspace_model.Workspace, error) {
	workspace, err := s.workspaceManagementService.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if workspace == nil || workspace.OrganizationID == nil || *workspace.OrganizationID == "" || workspace.Status != workspace_model.WorkspaceStatusNormal {
		return nil, nil
	}

	isMember, err := s.isAccountOrganizationMember(ctx, accountID, *workspace.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to check organization membership: %w", err)
	}
	if !isMember {
		return nil, nil
	}

	canAccess, err := s.canAccountAccessWorkspace(ctx, accountID, *workspace.OrganizationID, workspace.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check workspace access: %w", err)
	}
	if !canAccess {
		return nil, nil
	}

	return workspace, nil
}

func (s *AccountService) resolveWorkspaceForOrganizationContext(ctx context.Context, accountID string, ctxModel *auth_model.AccountContext, organizationID string) (*workspace_model.Workspace, error) {
	if ctxModel.CurrentWorkspaceID != nil && *ctxModel.CurrentWorkspaceID != "" {
		isValid, err := s.isWorkspaceAccessibleInOrganization(ctx, accountID, *ctxModel.CurrentWorkspaceID, organizationID)
		if err != nil {
			return nil, fmt.Errorf("failed to check workspace organization: %w", err)
		}
		if isValid {
			workspace, err := s.workspaceManagementService.GetWorkspaceByID(ctx, *ctxModel.CurrentWorkspaceID)
			if err != nil {
				return nil, err
			}
			return workspace, nil
		}
	}

	return s.resolveDefaultWorkspaceForOrganization(ctx, accountID, organizationID)
}

func (s *AccountService) resolveDefaultWorkspaceForOrganization(ctx context.Context, accountID, organizationID string) (*workspace_model.Workspace, error) {
	if s.db == nil {
		return nil, nil
	}

	isAdmin, err := s.isOrganizationAdminOrOwner(ctx, organizationID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check organization role: %w", err)
	}

	query := s.db.WithContext(ctx).
		Table("workspaces").
		Select("workspaces.*").
		Where("workspaces.organization_id = ? AND workspaces.status = ?", organizationID, workspace_model.WorkspaceStatusNormal).
		Order("workspaces.created_at DESC")

	if !isAdmin {
		query = query.Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
			Where("workspace_members.account_id = ?", accountID)
	}

	var workspace workspace_model.Workspace
	if err := query.Limit(1).Scan(&workspace).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve current workspace: %w", err)
	}
	if workspace.ID == "" {
		return nil, nil
	}

	return &workspace, nil
}

func (s *AccountService) resolveAnyAccessibleWorkspace(ctx context.Context, accountID string) (*workspace_model.Workspace, error) {
	if s.db == nil {
		return nil, nil
	}

	var workspace workspace_model.Workspace
	err := s.db.WithContext(ctx).
		Table("workspaces").
		Select("workspaces.*").
		Joins("JOIN members AS organization_members ON organization_members.organization_id = workspaces.organization_id").
		Joins("LEFT JOIN workspace_members ON workspaces.id = workspace_members.workspace_id AND workspace_members.account_id = organization_members.account_id").
		Where("organization_members.account_id = ?", accountID).
		Where("workspaces.status = ?", workspace_model.WorkspaceStatusNormal).
		Where("workspaces.organization_id IS NOT NULL").
		Where("(organization_members.role IN ? OR workspace_members.account_id IS NOT NULL)", []workspace_model.OrganizationRole{workspace_model.OrganizationRoleOwner, workspace_model.OrganizationRoleAdmin}).
		Order("COALESCE(workspace_members.current, false) DESC, workspaces.created_at DESC").
		Limit(1).
		Scan(&workspace).Error
	if err != nil {
		return nil, fmt.Errorf("failed to resolve accessible workspace: %w", err)
	}
	if workspace.ID == "" {
		return nil, nil
	}

	return &workspace, nil
}

func (s *AccountService) isWorkspaceAccessibleInOrganization(ctx context.Context, accountID, workspaceID, organizationID string) (bool, error) {
	workspace, err := s.workspaceManagementService.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	if workspace == nil || workspace.OrganizationID == nil || *workspace.OrganizationID != organizationID || workspace.Status != workspace_model.WorkspaceStatusNormal {
		return false, nil
	}

	return s.canAccountAccessWorkspace(ctx, accountID, organizationID, workspaceID)
}

func (s *AccountService) canAccountAccessWorkspace(ctx context.Context, accountID, organizationID, workspaceID string) (bool, error) {
	isAdmin, err := s.isOrganizationAdminOrOwner(ctx, organizationID, accountID)
	if err != nil {
		return false, err
	}
	if isAdmin {
		return true, nil
	}

	join, err := s.workspaceManagementService.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	return join != nil, nil
}

func (s *AccountService) isOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	if s.organizationService == nil {
		return false, nil
	}
	return s.organizationService.IsOrganizationAdminOrOwner(ctx, organizationID, accountID)
}

func ptrStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func setStringPtrIfChanged(target **string, value string) bool {
	if target == nil {
		return false
	}
	trimmedValue := strings.TrimSpace(value)
	if *target != nil && **target == trimmedValue {
		return false
	}
	*target = &trimmedValue
	return true
}

func clearStringPtrIfChanged(target **string) bool {
	if target == nil || *target == nil {
		return false
	}
	*target = nil
	return true
}

// isAccountOrganizationMember checks if an account is a member of the specified organization
func (s *AccountService) isAccountOrganizationMember(ctx context.Context, accountID, organizationID string) (bool, error) {
	// Check if the account is a member of the organization
	// We need to check in the organization_members table
	isMember, err := s.organizationService.IsOrganizationMember(ctx, organizationID, accountID)
	if err != nil {
		return false, err
	}
	return isMember, nil
}

// isWorkspaceInOrganization checks if a workspace belongs to the specified organization
func (s *AccountService) isWorkspaceInOrganization(ctx context.Context, workspaceID, organizationID string) (bool, error) {
	// Get the workspace and check if its organization_id matches the given organizationID
	workspace, err := s.workspaceManagementService.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	// Check if workspace.OrganizationID is not nil and matches the provided organizationID
	if workspace.OrganizationID != nil {
		return *workspace.OrganizationID == organizationID, nil
	}
	return false, nil
}

// IsGroupOwner implements the IsGroupOwner method
func (s *AccountService) IsGroupOwner(ctx context.Context, accountID string, tenantID string) (bool, error) {
	role, err := s.accountRepo.GetGroupRoleByTenantID(ctx, accountID, tenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return role == "owner", nil
}

// UpsertOrganizationRole implements the UpsertOrganizationRole method
func (s *AccountService) UpsertOrganizationRole(ctx context.Context, tenantID string, accountID string, role string) error {
	return s.accountRepo.UpsertOrganizationRole(ctx, tenantID, accountID, role)
}

// IsLoginErrorRateLimit implements the IsLoginErrorRateLimit method
func (s *AccountService) IsLoginErrorRateLimit(ctx context.Context, email string) (bool, error) {
	key := loginErrorRateLimitKey(email)
	count, err := redisUtil.GetClient().Get(ctx, key).Int()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}
	return count >= helper.LoginMaxErrorLimits, nil
}

// ResetLoginErrorRateLimit implements the ResetLoginErrorRateLimit method
func (s *AccountService) ResetLoginErrorRateLimit(ctx context.Context, email string) error {
	key := loginErrorRateLimitKey(email)
	return redisUtil.GetClient().Del(ctx, key).Err()
}

// LinkAccountIntegrate implements the LinkAccountIntegrate method
func (s *AccountService) LinkAccountIntegrate(ctx context.Context, provider auth_model.AccountIntegrateProvider, openID string, account *auth_model.Account) error {
	if account == nil {
		return errors.New("account is required")
	}
	if provider == "" || openID == "" {
		return errors.New("provider and open id are required")
	}

	integration, err := s.accountRepo.GetAccountIntegrate(ctx, account.ID, provider)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if integration == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		return s.accountRepo.CreateAccountIntegrate(ctx, &auth_model.AccountIntegrate{
			ID:             uuid.New().String(),
			AccountID:      account.ID,
			Provider:       provider,
			OpenID:         openID,
			EncryptedToken: "",
		})
	}

	if integration.OpenID == openID {
		return nil
	}

	integration.OpenID = openID
	integration.EncryptedToken = ""
	return s.accountRepo.UpdateAccountIntegrate(ctx, integration)
}

// LoadLoggedInAccount implements the LoadLoggedInAccount method
func (s *AccountService) LoadLoggedInAccount(ctx context.Context, accountID string) (*auth_model.Account, error) {
	return s.LoadUser(ctx, accountID)
}

// RevokeResetPasswordToken implements the RevokeResetPasswordToken method
func (s *AccountService) RevokeResetPasswordToken(ctx context.Context, token string) error {
	return s.tokenMgr.RevokeToken(token, TokenTypeResetPassword)
}

// SendEmailCodeLoginEmail implements the SendEmailCodeLoginEmail method
func (s *AccountService) SendEmailCodeLoginEmail(ctx context.Context, account *auth_model.Account, email, language string) (string, error) {
	code := generateRandomCode(6)

	additionalData := map[string]interface{}{
		"code": code,
		"exp":  time.Now().Add(time.Minute * 10).Unix(),
	}

	token, err := s.tokenMgr.GenerateToken(
		ctx,
		"email_code_login",
		nil,
		&account.Email,
		additionalData,
	)

	if err != nil {
		return "", err
	}

	return token, nil
}

// GetEmailCodeLoginData implements the GetEmailCodeLoginData method
func (s *AccountService) GetEmailCodeLoginData(ctx context.Context, token string) (map[string]interface{}, error) {
	tokenData, err := s.tokenMgr.GetTokenData(token, "email_code_login")
	if err != nil {
		return nil, err
	}

	if tokenData == nil {
		return nil, errors.New("invalid token")
	}

	result := make(map[string]interface{})
	if tokenData.Email != nil {
		result["email"] = *tokenData.Email
	}
	if code, ok := tokenData.Extra["code"]; ok {
		result["code"] = code
	}

	return result, nil
}

// RevokeEmailCodeLoginToken implements the RevokeEmailCodeLoginToken method
func (s *AccountService) RevokeEmailCodeLoginToken(ctx context.Context, token string) error {
	return s.tokenMgr.RevokeToken(token, "email_code_login")
}

// GetUserThroughEmail implements the GetUserThroughEmail method
func (s *AccountService) GetUserThroughEmail(ctx context.Context, email string) (*auth_model.Account, error) {
	return s.accountRepo.GetAccountByEmail(ctx, email)
}

// GetAccountsNotInTenant implements the GetAccountsNotInTenant method
func (s *AccountService) GetAccountsNotInTenant(ctx context.Context, tenantID string, search *string, page, perPage int) (*PaginationResult, error) {
	tenant, err := s.workspaceManagementService.GetWorkspaceByID(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("invalid tenant ID: %w", err)
	}
	if tenant == nil {
		return nil, fmt.Errorf("invalid tenant ID")
	}

	db := s.db
	if db == nil {
		return nil, errors.New("database connection not initialized")
	}

	joinedSubquery := db.WithContext(ctx).
		Table("workspace_members").
		Select("account_id").
		Where("workspace_id = ?", tenantID)

	query := db.WithContext(ctx).Model(&auth_model.Account{}).
		Where("id NOT IN (?)", joinedSubquery)

	if search != nil && *search != "" {
		searchPattern := "%" + *search + "%"
		query = query.Where(
			"name ILIKE ? OR email ILIKE ?",
			searchPattern, searchPattern,
		)
	}

	query = query.Order("created_at DESC")

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count accounts: %w", err)
	}

	offset := (page - 1) * perPage
	var accounts []*auth_model.Account

	err = query.Offset(offset).Limit(perPage).Find(&accounts).Error
	if err != nil {
		return nil, fmt.Errorf("failed to fetch accounts not in tenant: %w", err)
	}

	totalPages := (int(total) + perPage - 1) / perPage

	items := make([]interface{}, len(accounts))
	for i, account := range accounts {
		items[i] = account
	}

	return &PaginationResult{
		Items:      items,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

// GetAccountsWithExtensions implements the GetAccountsWithExtensions method
func (s *AccountService) GetAccountsWithExtensions(ctx context.Context, args map[string]interface{}, currentAccount *auth_model.Account) (*PaginationResult, error) {
	db := s.db
	if db == nil {
		return nil, errors.New("database connection not initialized")
	}

	query := db.WithContext(ctx).Model(&auth_model.Account{})

	var currentTenantID string

	var tenantJoin workspace_model.WorkspaceMember
	err := db.WithContext(ctx).
		Where("account_id = ? AND current = ?", currentAccount.ID, true).
		First(&tenantJoin).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get current tenant: %w", err)
	}

	currentTenantID = tenantJoin.WorkspaceID
	if currentTenantID == "" {
		return nil, errors.New("current tenant not found")
	}

	subGroup := db.Where(
		"EXISTS (SELECT 1 FROM members WHERE account_id = accounts.id AND organization_id = ?)",
		currentTenantID,
	)

	subTenant := db.Where(
		"EXISTS (SELECT 1 FROM workspace_members taj "+
			"JOIN workspaces w ON taj.workspace_id = w.id "+
			"WHERE taj.account_id = accounts.id AND w.organization_id = ?)",
		currentTenantID,
	)

	query = query.Where(subGroup.Or(subTenant))

	if search, ok := args["search"].(string); ok && search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where(
			db.Where("name LIKE ?", searchPattern).
				Or("email LIKE ?", searchPattern),
		)
	}

	page := 1
	if p, ok := args["page"].(int); ok {
		page = p
	}
	limit := 20
	if l, ok := args["limit"].(int); ok {
		limit = l
	}

	accounts, total, err := s.accountRepo.GetAccountsWithExtensions(ctx, query, page, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get accounts: %w", err)
	}

	for _, account := range accounts {
		groupRole, err := s.accountRepo.GetGroupRoleByTenantID(ctx, account.ID, currentTenantID)
		if err != nil {
			groupRole = "normal"
		}
		account.GroupRole = groupRole
	}

	totalPages := (int(total) + limit - 1) / limit

	return &PaginationResult{
		Items:      accounts,
		Total:      total,
		Page:       page,
		PerPage:    limit,
		TotalPages: totalPages,
	}, nil
}

// GetAccountsWithExtensionsByEmail implements the GetAccountsWithExtensionsByEmail method
func (s *AccountService) GetAccountsWithExtensionsByEmail(ctx context.Context, email string) (*auth_model.Account, error) {
	return s.accountRepo.GetAccountWithExtensionsByEmail(ctx, email)
}

// GetAccountsWithExtensionsByID implements the GetAccountsWithExtensionsByID method
func (s *AccountService) GetAccountsWithExtensionsByID(ctx context.Context, id string) (*auth_model.Account, error) {
	return s.accountRepo.GetAccount(ctx, id)
}

// UpdateAccountBasicInfo implements the UpdateAccountBasicInfo method
func (s *AccountService) UpdateAccountBasicInfo(ctx context.Context, account *auth_model.Account, name, email, status *string) error {
	if name != nil {
		account.Name = *name
	}
	if email != nil {
		account.Email = *email
	}
	if status != nil {
		account.Status = auth_model.AccountStatus(*status)
	}
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return err
	}
	s.invalidateAccountProfileCache(account.ID)
	return nil
}

// UpdateAccountExtension implements the UpdateAccountExtension method
func (s *AccountService) UpdateAccountExtension(ctx context.Context, account *auth_model.Account, mobile, gender *string) error {
	setAccountMobile(account, mobile)
	updateAccountExtensions(account, mobile, nil, nil, gender)
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return err
	}
	s.invalidateAccountProfileCache(account.ID)
	return nil
}

// UpdateAccountEx implements the UpdateAccountEx method
func (s *AccountService) UpdateAccountEx(ctx context.Context, account *auth_model.Account, req *dto.UpdateAccountExRequest) error {
	var genderStr *string
	if req.Gender != nil {
		g := string(*req.Gender)
		genderStr = &g
	}

	var mobile, wechat, address *string
	if req.Mobile != "" {
		mobile = &req.Mobile
	}
	if req.Wechat != "" {
		wechat = &req.Wechat
	}
	if req.Address != "" {
		address = &req.Address
	}

	setAccountMobile(account, mobile)
	updateAccountExtensions(account, mobile, wechat, address, genderStr)
	if err := s.accountRepo.UpdateAccount(ctx, account); err != nil {
		return err
	}
	s.invalidateAccountProfileCache(account.ID)
	return nil
}

func setAccountMobile(account *auth_model.Account, mobile *string) {
	if account == nil || mobile == nil {
		return
	}
	if *mobile == "" {
		account.MobileE164 = nil
		return
	}

	value := *mobile
	account.MobileE164 = &value
}

// Helper functions
func generateRandomCode(length int) string {
	bytes := make([]byte, length/2)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

type PaginationResult struct {
	Items      interface{} `json:"items"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PerPage    int         `json:"per_page"`
	TotalPages int         `json:"total_pages"`
}

func (s *AccountService) IsAllowRegister() bool {
	return true
}

func (s *AccountService) AddForgotPasswordErrorRateLimit(email string) {
	helper.IncrForgotPasswordErrorCount(email)
}

func (s *AccountService) ResetForgotPasswordErrorRateLimit(email string) {
	ctx := context.Background()
	key := forgotPasswordErrorRateLimitKey(email)
	redisUtil.GetClient().Del(ctx, key)
}

func (s *AccountService) IsForgotPasswordErrorRateLimit(email string) bool {
	return helper.GetForgotPasswordErrorCount(email) >= 5
}

func (s *AccountService) AddLoginErrorRateLimit(ctx context.Context, email string) error {
	key := loginErrorRateLimitKey(email)
	pipe := redisUtil.GetClient().Pipeline()
	pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Duration(helper.DefaultConfig().RateLimitWindow)*time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("increment login error rate limit: %w", err)
	}
	return nil
}

func (s *AccountService) IsEditor(ctx context.Context, accountID string) (bool, error) {
	_, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("failed to get account: %w", err)
	}

	return true, nil
}
