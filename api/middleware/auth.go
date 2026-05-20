package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	"github.com/zgiai/ginext/internal/util"
	"github.com/zgiai/ginext/pkg/database"
	jwtpkg "github.com/zgiai/ginext/pkg/jwt"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
	"gorm.io/gorm"
)

const accountServiceKey = "account_service"

const tenantServiceKey = "tenant_service"

const enterpriseServiceKey = "enterprise_service"

var (
	errAuthenticatedAccountBanned = errors.New("authenticated account is banned")
	errAuthenticatedAccountFrozen = errors.New("authenticated account is frozen")
	errAuthenticatedAccountClosed = errors.New("authenticated account is closed")
)

func ensureAuthenticatedAccount(ctx context.Context, accountID string) error {
	var account auth_model.Account
	if err := database.GetDB().
		WithContext(ctx).
		Select("id", "status").
		Where("id = ?", accountID).
		Take(&account).Error; err != nil {
		return err
	}

	switch account.Status {
	case auth_model.AccountStatusBanned:
		return errAuthenticatedAccountBanned
	case auth_model.AccountStatusFrozen:
		return errAuthenticatedAccountFrozen
	case auth_model.AccountStatusClosed:
		return errAuthenticatedAccountClosed
	default:
		return nil
	}
}

func failAccountAuthorization(c *gin.Context, accountID string, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, errAuthenticatedAccountBanned):
		logger.Warn("auth: account is banned: %s", accountID)
		response.Fail(c, response.ErrAccountBanned)
	case errors.Is(err, errAuthenticatedAccountFrozen):
		logger.Warn("auth: account is frozen: %s", accountID)
		response.Fail(c, response.ErrAccountFrozen)
	case errors.Is(err, errAuthenticatedAccountClosed):
		logger.Warn("auth: account is closed: %s", accountID)
		response.Fail(c, response.ErrUnauthorized)
	case errors.Is(err, gorm.ErrRecordNotFound):
		logger.Warn("auth: account not found: %s", accountID)
		response.Fail(c, response.ErrGetUserInfoFailed)
	default:
		logger.Warn("auth: failed to load user or user invalid: %s", accountID)
		response.Fail(c, response.ErrUserNotFound)
	}

	c.Abort()
}

func SetAccountService(accountService interfaces.AccountService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("account_service", accountService)
		c.Next()
	}
}

func SetWorkspaceManagementService(workspaceManagementService interfaces.WorkspaceManagementService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("workspace_management_service", workspaceManagementService)
		c.Next()
	}
}

func SetEnterpriseService(organizationService interface{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("organization_service", organizationService)
		c.Next()
	}
}

func JWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Fail(c, response.ErrAuthHeaderRequired)
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			response.Fail(c, response.ErrInvalidAuthFormat)
			c.Abort()
			return
		}

		user_id, err := jwtpkg.GetUserIDFromToken(parts[1])
		if err != nil {
			logger.Error("Failed to parse token", err)
			response.Fail(c, response.ErrTokenInvalid)
			c.Abort()
			return
		}

		if err := ensureAuthenticatedAccount(c.Request.Context(), user_id); err != nil {
			failAccountAuthorization(c, user_id, err)
			return
		}

		// Store user information in context
		c.Set("account_id", user_id)
		c.Next()
	}
}

func JWTWithTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Fail(c, response.ErrAuthHeaderRequired)
			c.Abort()
			return
		}

		// Check Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			response.Fail(c, response.ErrInvalidAuthFormat)
			c.Abort()
			return
		}

		// Parse token
		user_id, err := jwtpkg.GetUserIDFromToken(parts[1])
		if err != nil {
			logger.Error("Failed to parse token", err)
			response.Fail(c, response.ErrTokenInvalid)
			c.Abort()
			return
		}

		if err := ensureAuthenticatedAccount(c.Request.Context(), user_id); err != nil {
			failAccountAuthorization(c, user_id, err)
			return
		}

		// Store user information in context
		c.Set("account_id", user_id)

		if needsCurrentWorkspaceScope(c.Request.URL.Path) {
			organizationID := getCurrentUserTenantIDInternal(c)
			if organizationID == "" {
				response.Fail(c, response.ErrWorkspaceNotFound)
				c.Abort()
				return
			}

			util.SetOrganizationID(c, organizationID)

			workspaceID := getCurrentWorkspaceIDInternal(c)
			if workspaceID == "" {
				response.Fail(c, response.ErrWorkspaceNotFound)
				c.Abort()
				return
			}

			util.SetWorkspaceScopeCompat(c, workspaceID)
			c.Next()
			return
		}

		tenantID := resolveTenantID(c)
		if tenantID == "" {
			response.Fail(c, response.ErrWorkspaceNotFound)
			c.Abort()
			return
		}

		util.SetOrganizationID(c, tenantID)

		// Legacy compatibility for existing callers that still read tenant_id.
		// New permission logic should prefer canonical organization_id.
		c.Set("tenant_id", tenantID)

		c.Next()
	}
}

func resolveTenantID(c *gin.Context) string {
	if tenantID, exists := c.Get("tenant_id"); exists {
		if tenantIDStr, ok := tenantID.(string); ok {
			logger.Debug("resolveTenantID: found in context: %s", tenantIDStr)
			return tenantIDStr
		}
		logger.Warn("resolveTenantID: tenant_id in context is not a string")
	}

	// Try to read tenant_id from JWT claims if provided
	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			if claims, err := jwtpkg.ParseTokenFixed(parts[1]); err == nil {
				if tid, ok := claims["tenant_id"].(string); ok && tid != "" {
					logger.Debug("resolveTenantID: found in JWT claims: %s", tid)
					return tid
				}
				logger.Debug("resolveTenantID: JWT has no tenant_id claim")
			} else {
				logger.Warn("resolveTenantID: failed to parse token for claims")
			}
		}
	}

	logger.Debug("resolveTenantID: account current tenant")
	return getCurrentUserTenantIDInternal(c)
}

func needsCurrentWorkspaceScope(path string) bool {
	return hasScopeRoutePrefix(path, "/console/api/workspaces/current") ||
		hasScopeRoutePrefix(path, "/console/api/workspaces/default")
}

func hasScopeRoutePrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

//
//func GetTenantID(c *gin.Context) string {
//	return util.GetTenantID(c)
//}

func getCurrentUserTenantIDInternal(c *gin.Context) string {
	accountID, exists := c.Get("account_id")
	if !exists {
		logger.Error("account_id not found in context", nil)
		return ""
	}

	// Safe type assertion
	accountIDStr, ok := accountID.(string)
	if !ok {
		logger.Error("account_id is not a string type", nil)
		return ""
	}
	logger.Debug("getCurrentUserTenantIDInternal: found account_id: %s", accountIDStr)

	accountService, exists := c.Get(accountServiceKey)
	if !exists {
		logger.Error("AccountService not found in context, skipping tenant resolution", nil)
		return ""
	}
	logger.Debug("getCurrentUserTenantIDInternal: found account service")

	service := accountService.(interfaces.AccountService)
	logger.Debug("getCurrentUserTenantIDInternal: calling EnsureCurrentOrganizationID for account: %s", accountIDStr)
	tenantID, err := service.EnsureCurrentOrganizationID(context.Background(), accountIDStr)
	if err != nil {
		logger.Error("Failed to get current user tenant ID: %v", err)
		return ""
	}
	logger.Debug("getCurrentUserTenantIDInternal: got tenant_id: %s", tenantID)

	return tenantID
}

func getCurrentWorkspaceIDInternal(c *gin.Context) string {
	if workspaceID := util.GetWorkspaceID(c); workspaceID != "" {
		logger.Debug("getCurrentWorkspaceIDInternal: found in context: %s", workspaceID)
		return workspaceID
	}

	accountID, exists := c.Get("account_id")
	if !exists {
		logger.Error("account_id not found in context", nil)
		return ""
	}

	accountIDStr, ok := accountID.(string)
	if !ok {
		logger.Error("account_id is not a string type", nil)
		return ""
	}
	logger.Debug("getCurrentWorkspaceIDInternal: found account_id: %s", accountIDStr)

	accountService, exists := c.Get(accountServiceKey)
	if !exists {
		logger.Error("AccountService not found in context, skipping workspace resolution", nil)
		return ""
	}

	service := accountService.(interfaces.AccountService)
	logger.Debug("getCurrentWorkspaceIDInternal: calling GetCurrentWorkspace for account: %s", accountIDStr)
	currentWorkspace, err := service.GetCurrentWorkspace(context.Background(), accountIDStr)
	if err != nil {
		logger.Error("Failed to get current workspace ID: %v", err)
		return ""
	}
	if currentWorkspace == nil {
		logger.Warn("getCurrentWorkspaceIDInternal: current workspace is nil for account: %s", accountIDStr)
		return ""
	}

	logger.Debug("getCurrentWorkspaceIDInternal: got workspace_id: %s", currentWorkspace.ID)
	return currentWorkspace.ID
}

func needsTenantID(path string) bool {
	consoleAPIPaths := []string{
		"/console/api/files",
		"/console/api/datasets",
		"/console/api/plugins",
		"/console/api/models",
	}

	for _, apiPath := range consoleAPIPaths {
		if strings.Contains(path, apiPath) {
			return true
		}
	}

	return false
}

// shouldSkipTenantResolution returns true for routes that don't require a resolved tenant
func shouldSkipTenantResolution(path string) bool {
	skipPaths := []string{}

	for _, skipPath := range skipPaths {
		if strings.HasPrefix(path, skipPath) {
			return true
		}
	}

	return false
}

func JWTWithOrganizationAndService(accountService interfaces.AccountService) gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		c.Set(accountServiceKey, accountService)

		logger.Debug("auth: entering JWTWithTenantAndService for %s", c.Request.URL.Path)
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn("auth: missing Authorization header")
			response.Fail(c, response.ErrAuthHeaderRequired)
			c.Abort()
			return
		}

		// Check Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			logger.Warn("auth: invalid auth format: %s", authHeader)
			response.Fail(c, response.ErrInvalidAuthFormat)
			c.Abort()
			return
		}

		// Parse token
		user_id, err := jwtpkg.GetUserIDFromToken(parts[1])
		if err != nil {
			logger.Error("Failed to parse token", err)
			response.Fail(c, response.ErrTokenInvalid)
			c.Abort()
			return
		}

		account, err := accountService.LoadUser(c.Request.Context(), user_id)
		if err != nil || account == nil {
			switch {
			case err != nil && strings.Contains(err.Error(), "account is banned"):
				failAccountAuthorization(c, user_id, errAuthenticatedAccountBanned)
			case err != nil && strings.Contains(err.Error(), "account is frozen"):
				failAccountAuthorization(c, user_id, errAuthenticatedAccountFrozen)
			case err != nil && strings.Contains(err.Error(), "account is closed"):
				failAccountAuthorization(c, user_id, errAuthenticatedAccountClosed)
			default:
				failAccountAuthorization(c, user_id, err)
			}
			return
		}

		// Store user information in context
		c.Set("account_id", user_id)

		tenantID := resolveTenantID(c)
		if tenantID == "" {
			if !shouldSkipTenantResolution(c.Request.URL.Path) {
				logger.Warn("auth: tenant resolve failed; account_id=%s path=%s", user_id, c.Request.URL.Path)
				response.Fail(c, response.ErrWorkspaceNotFound)
				c.Abort()
				return
			}
			logger.Debug("auth: skipping tenant resolution for path=%s", c.Request.URL.Path)
		}
		logger.Debug("auth: tenant resolved: %s", tenantID)

		util.SetOrganizationID(c, tenantID)

		// Legacy compatibility for existing callers that still read tenant_id.
		// New permission logic should prefer canonical organization_id.
		c.Set("tenant_id", tenantID)

		c.Next()
	})
}

// IsAdminOrOwner checks if the current user is an admin or owner
func IsAdminOrOwner(c *gin.Context) bool {
	// Get account ID from context
	accountIDRaw, exists := c.Get("account_id")
	if !exists {
		logger.Error("account_id not found in context for IsAdminOrOwner check", nil)
		return false
	}

	accountID, ok := accountIDRaw.(string)
	if !ok {
		logger.Error("account_id is not a string", nil)
		return false
	}

	workspaceID := resolveWorkspaceScopeForPermission(c)
	if workspaceID == "" {
		logger.Error("workspace_id not found in context for IsAdminOrOwner check", nil)
		return false
	}

	// Get tenant service from context
	tenantServiceRaw, exists := c.Get(tenantServiceKey)
	if !exists {
		logger.Error("tenant_service not found in context for IsAdminOrOwner check", nil)
		return false
	}

	tenantService, ok := tenantServiceRaw.(interfaces.WorkspaceManagementService)
	if !ok {
		logger.Error("tenant_service is not the correct type", nil)
		return false
	}

	// Get user role in tenant
	userRole, err := tenantService.GetUserRole(c.Request.Context(), accountID, workspaceID)
	if err != nil {
		logger.Error("Failed to get user role for IsAdminOrOwner check", err)
		return false
	}

	// If user has no role in tenant, they are not admin or owner
	if userRole == nil {
		return false
	}

	// Check if role is privileged (admin or owner)
	return userRole.IsPrivilegedRole()
}

// CheckAdminOrOwnerRole is a helper function for checking admin or owner role
// This provides a consistent way to check permissions across handlers
func CheckAdminOrOwnerRole(ctx context.Context, tenantService interfaces.WorkspaceManagementService, accountID, tenantID string) (bool, error) {
	// First check direct tenant role
	userRole, err := tenantService.GetUserRole(ctx, accountID, tenantID)
	if err != nil {
		return false, err
	}

	// If user has direct role in tenant and it's privileged, return true
	if userRole != nil && userRole.IsPrivilegedRole() {
		return true, nil
	}

	// No permissions found
	return false, nil
}

// CheckAdminOrOwnerRoleWithEnterpriseGroup is an enhanced version that also checks enterprise group permissions
func CheckAdminOrOwnerRoleWithEnterpriseGroup(ctx context.Context, tenantService interfaces.WorkspaceManagementService, accountService interfaces.AccountService, accountID, tenantID string) (bool, error) {
	logger.Info("Checking permissions", map[string]interface{}{
		"account_id": accountID,
		"tenant_id":  tenantID,
	})

	// First check direct tenant role
	userRole, err := tenantService.GetUserRole(ctx, accountID, tenantID)
	logger.Info("Direct tenant role check", map[string]interface{}{
		"role":  userRole,
		"error": err,
	})
	if err != nil {
		return false, err
	}

	// If user has direct role in tenant and it's privileged, return true
	if userRole != nil && userRole.IsPrivilegedRole() {
		logger.Info("Permission granted via direct tenant role", nil)
		return true, nil
	}

	// If no direct role or not privileged, check enterprise group permissions
	// Check if user is enterprise admin/owner and target tenant is in their enterprise group
	isEnterpriseAdmin, err := accountService.CheckOrganizationpAdminByWorkspace(ctx, accountID, tenantID)
	logger.Info("Enterprise group admin check", map[string]interface{}{
		"is_group_admin": isEnterpriseAdmin,
		"error":          err,
	})
	if err != nil {
		logger.Error("Failed to check enterprise group admin status", err)
		return false, nil // Don't fail the request, just deny permission
	}

	if isEnterpriseAdmin {
		logger.Info("User granted access via enterprise group admin role", nil)
		return true, nil
	}

	logger.Info("Permission denied - no valid role found", nil)
	// No permissions found
	return false, nil
}

// IsEditor checks if the current user is an editor (has edit permissions)
func IsEditor(c *gin.Context) bool {
	// Get account ID from context
	accountIDRaw, exists := c.Get("account_id")
	if !exists {
		logger.Error("account_id not found in context for IsEditor check", nil)
		return false
	}

	accountID, ok := accountIDRaw.(string)
	if !ok {
		logger.Error("account_id is not a string", nil)
		return false
	}

	workspaceID := resolveWorkspaceScopeForPermission(c)
	if workspaceID == "" {
		logger.Error("workspace_id not found in context for IsEditor check", nil)
		return false
	}

	// Get tenant service from context
	tenantServiceRaw, exists := c.Get(tenantServiceKey)
	if !exists {
		logger.Error("tenant_service not found in context for IsEditor check", nil)
		return false
	}

	tenantService, ok := tenantServiceRaw.(interfaces.WorkspaceManagementService)
	if !ok {
		logger.Error("tenant_service is not the correct type", nil)
		return false
	}

	// Get user role in tenant
	userRole, err := tenantService.GetUserRole(c.Request.Context(), accountID, workspaceID)
	if err != nil {
		logger.Error("Failed to get user role for IsEditor check", err)
		return false
	}

	// If user has no role in tenant, they are not editor
	if userRole == nil {
		return false
	}

	// Check if role has edit permissions (admin, owner, or editor)
	return userRole.IsEditingRole()
}

func resolveWorkspaceScopeForPermission(c *gin.Context) string {
	if workspaceID := util.GetWorkspaceID(c); workspaceID != "" {
		return workspaceID
	}

	// Legacy compatibility for workspace permission checks that have not
	// migrated to canonical workspace_id yet.
	return util.GetTenantID(c)
}

// CheckEditorRole is a helper function for checking editor role
// This provides a consistent way to check editor permissions across handlers
func CheckEditorRole(ctx context.Context, tenantService interfaces.WorkspaceManagementService, accountID, tenantID string) (bool, error) {
	// Get user role in tenant
	userRole, err := tenantService.GetUserRole(ctx, accountID, tenantID)
	if err != nil {
		return false, err
	}

	// If user has no role in tenant, they are not editor
	if userRole == nil {
		return false, nil
	}

	// Check if role has edit permissions
	return userRole.IsEditingRole(), nil
}

// EditorRequired middleware that enforces editor permissions
func EditorRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsEditor(c) {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminOrOwnerRequired middleware that enforces admin or owner permissions
func AdminOrOwnerRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsAdminOrOwner(c) {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetAccountID returns the account ID from the gin context
func GetAccountID(c *gin.Context) string {
	if accountID, exists := c.Get("account_id"); exists {
		if id, ok := accountID.(string); ok {
			return id
		}
	}
	return ""
}

// SystemAdminRequired middleware that enforces system admin permissions
// This middleware requires JWT authentication and system admin role
func SystemAdminRequired(accountService interfaces.AccountService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Set account service in context for IsSystemAdmin check
		c.Set(accountServiceKey, accountService)

		// Validate JWT token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn("SystemAdminRequired: missing Authorization header")
			response.Fail(c, response.ErrAuthHeaderRequired)
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			logger.Warn("SystemAdminRequired: invalid auth format")
			response.Fail(c, response.ErrInvalidAuthFormat)
			c.Abort()
			return
		}

		userID, err := jwtpkg.GetUserIDFromToken(parts[1])
		if err != nil {
			logger.Error("SystemAdminRequired: failed to parse token", err)
			response.Fail(c, response.ErrTokenInvalid)
			c.Abort()
			return
		}

		// Store user information in context
		c.Set("account_id", userID)

		var account struct {
			IsSuperAdmin bool `gorm:"column:is_super_admin"`
		}
		err = database.GetDB().
			WithContext(c.Request.Context()).
			Table("accounts").
			Select("is_super_admin").
			Where("id = ? AND deleted_at IS NULL", userID).
			Take(&account).Error
		if err != nil {
			logger.Error("SystemAdminRequired: failed to load account", err)
			response.Fail(c, response.ErrUserNotFound)
			c.Abort()
			return
		}
		if !account.IsSuperAdmin {
			logger.Warn("SystemAdminRequired: user %s is not a system admin", userID)
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}

		logger.Debug("SystemAdminRequired: user %s authorized as system admin", userID)
		c.Next()
	}
}

// IsOrganizationAdminOrOwner checks if the current user is an admin or owner of the enterprise that the current tenant belongs to
func IsOrganizationAdminOrOwner(c *gin.Context) bool {
	// Get account ID from context
	accountIDRaw, exists := c.Get("account_id")
	if !exists {
		logger.Error("account_id not found in context for IsEnterpriseAdminOrOwner check", nil)
		return false
	}

	accountID, ok := accountIDRaw.(string)
	if !ok {
		logger.Error("account_id is not a string", nil)
		return false
	}

	organizationID := resolveOrganizationScopeForPermission(c)
	if organizationID == "" {
		logger.Error("organization_id not found in context for IsEnterpriseAdminOrOwner check", nil)
		return false
	}

	// Get account service from context
	accountServiceRaw, exists := c.Get(accountServiceKey)
	if !exists {
		logger.Error("account_service not found in context for IsEnterpriseAdminOrOwner check", nil)
		return false
	}

	accountService, ok := accountServiceRaw.(interfaces.AccountService)
	if !ok {
		logger.Error("account_service is not the correct type", nil)
		return false
	}

	isGroupAdmin, err := accountService.IsOrganizationAdminOrOwner(c.Request.Context(), organizationID, accountID)
	if err != nil {
		logger.Error("Failed to check organization admin/owner status", err)
		return false
	}

	return isGroupAdmin
}

// EnterpriseAdminOrOwnerRequired middleware that enforces enterprise admin or owner permissions
func EnterpriseAdminOrOwnerRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !IsOrganizationAdminOrOwner(c) {
			response.Fail(c, response.ErrPermissionDenied)
			c.Abort()
			return
		}
		c.Next()
	}
}

func resolveOrganizationScopeForPermission(c *gin.Context) string {
	if organizationID := util.GetOrganizationID(c); organizationID != "" {
		return organizationID
	}

	// Legacy compatibility for routes that have not been migrated yet.
	return util.GetTenantID(c)
}

// WebAppAuthMiddleware handles public web-app identity and authenticated access.
// Supports four scenarios:
// 1. Authorization header only (JWT) - authenticated user
// 2. X-User-Account-Id header only - virtual user
// 3. Authorization: Bearer <uuid> only - legacy/public virtual user
// 4. JWT + X-User-Account-Id - user migration scenario
func WebAppAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		virtualUserID := c.GetHeader("X-User-Account-Id")

		var accountID string
		var isAuthenticated bool
		var virtualAccountID string
		var authenticatedAccountID string
		var migrationRequired bool

		logger.Debug("WebAppAuth: Processing authentication", map[string]interface{}{
			"has_authorization": authHeader != "",
			"has_virtual_id":    virtualUserID != "",
			"path":              c.Request.URL.Path,
			"method":            c.Request.Method,
		})

		// Process Authorization header (JWT token or legacy/public UUID)
		if authHeader != "" {
			// Check Bearer token format
			parts := strings.SplitN(authHeader, " ", 2)
			if !(len(parts) == 2 && parts[0] == "Bearer") {
				logger.Warn("WebAppAuth: Invalid auth format: %s", authHeader)
				response.Fail(c, response.ErrInvalidAuthFormat)
				c.Abort()
				return
			}

			token := parts[1]

			// Public web-app clients may send a stable UUID in Authorization to continue
			// anonymous conversations without a login session.
			if legacyUserID, err := uuid.Parse(token); err == nil {
				virtualAccountID = legacyUserID.String()
				logger.Debug("WebAppAuth: UUID bearer token treated as virtual user ID: %s", virtualAccountID)
			} else {
				// Parse JWT token to extract user_id
				userID, err := jwtpkg.GetUserIDFromToken(token)
				if err != nil {
					logger.Error("WebAppAuth: Failed to parse JWT token", err)
					response.Fail(c, response.ErrTokenInvalid)
					c.Abort()
					return
				}

				authenticatedAccountID = userID
				isAuthenticated = true
				logger.Debug("WebAppAuth: JWT token parsed successfully, user_id=%s", userID)
			}
		}

		// Process X-User-Account-Id header (virtual user)
		if virtualUserID != "" {
			// Validate UUID format
			if _, err := uuid.Parse(virtualUserID); err != nil {
				logger.Warn("WebAppAuth: Invalid virtual user ID format: %s", virtualUserID)
				response.Fail(c, response.ErrInvalidVirtualUserID)
				c.Abort()
				return
			}

			if virtualAccountID != "" && virtualAccountID != virtualUserID {
				logger.Warn("WebAppAuth: Conflicting virtual user IDs in Authorization and X-User-Account-Id")
				response.Fail(c, response.ErrInvalidAuthFormat)
				c.Abort()
				return
			}

			virtualAccountID = virtualUserID
			logger.Debug("WebAppAuth: Virtual user ID validated: %s", virtualUserID)
		}

		// Determine final account_id and migration flag
		if authenticatedAccountID != "" && virtualAccountID != "" {
			// Both headers present - migration scenario
			accountID = authenticatedAccountID
			migrationRequired = true
			logger.Info("WebAppAuth: Migration scenario detected", map[string]interface{}{
				"virtual_account_id":       virtualAccountID,
				"authenticated_account_id": authenticatedAccountID,
			})
		} else if authenticatedAccountID != "" {
			// Only JWT token - authenticated user
			accountID = authenticatedAccountID
			logger.Debug("WebAppAuth: Authenticated user mode, account_id=%s", accountID)
		} else if virtualAccountID != "" {
			// Only virtual user ID - virtual user
			accountID = virtualAccountID
			isAuthenticated = false
			logger.Debug("WebAppAuth: Virtual user mode, account_id=%s", accountID)
		} else {
			// No authentication provided
			logger.Warn("WebAppAuth: No authentication headers provided")
			response.Fail(c, response.ErrAuthHeaderRequired)
			c.Abort()
			return
		}

		// Set context variables
		c.Set("account_id", accountID)
		c.Set("is_authenticated", isAuthenticated)
		c.Set("virtual_account_id", virtualAccountID)
		c.Set("authenticated_account_id", authenticatedAccountID)
		c.Set("migration_required", migrationRequired)

		// Determine authentication mode for logging
		authMode := "unknown"
		if migrationRequired {
			authMode = "migration"
		} else if isAuthenticated {
			authMode = "authenticated"
		} else if virtualAccountID != "" {
			authMode = "virtual"
		}

		logger.Info("WebAppAuth: Authentication completed", map[string]interface{}{
			"auth_mode":          authMode,
			"account_id":         accountID,
			"is_authenticated":   isAuthenticated,
			"migration_required": migrationRequired,
			"path":               c.Request.URL.Path,
		})

		// Note: tenant_id is NOT set here - it will be resolved in the handler
		// using the caller's account_id to query workspace_members

		c.Next()
	}
}
