package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/middleware_depend"
	"github.com/zgiai/zgi/api/internal/util"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

var globalServiceProvider middleware_depend.ServiceProvider

func SetServiceProvider(sp middleware_depend.ServiceProvider) {
	globalServiceProvider = sp
}

func GetServiceProvider() middleware_depend.ServiceProvider {
	return globalServiceProvider
}

func JWTWithProvider() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Fail(c, response.ErrAuthHeaderRequired)
			c.Abort()
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			response.Fail(c, response.ErrInvalidAuthFormat)
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			response.Fail(c, response.ErrTokenRequired)
			c.Abort()
			return
		}

		userID, err := jwtpkg.GetUserIDFromToken(token)
		if err != nil {
			logger.Error("JWT parse error: %v", err)
			response.Fail(c, response.ErrTokenInvalid)
			c.Abort()
			return
		}

		if err := ensureAuthenticatedAccount(c.Request.Context(), userID); err != nil {
			failAccountAuthorization(c, userID, err)
			return
		}

		c.Set("account_id", userID)

		c.Next()
	}
}

func AccountInitRequiredWithProvider() gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID := c.GetString("account_id")
		if accountID == "" {
			response.Fail(c, response.ErrUnauthorized)
			c.Abort()
			return
		}

		if globalServiceProvider == nil {
			response.Fail(c, response.ErrSystemError)
			c.Abort()
			return
		}

		accountService := globalServiceProvider.GetAccountService()
		account, err := accountService.GetAccountByID(context.Background(), accountID)
		if err != nil {
			response.Fail(c, response.ErrAccountNotFound)
			c.Abort()
			return
		}

		if account.Status == "uninitialized" {
			response.Fail(c, response.ErrAccountNotInitialized)
			c.Abort()
			return
		}

		c.Set("current_account", account)
		c.Next()
	}
}

func TenantRequiredWithProvider() gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.GetString("tenant_id")
		if tenantID == "" {
			response.Fail(c, response.ErrWorkspaceNotFound)
			c.Abort()
			return
		}

		if globalServiceProvider == nil {
			response.Fail(c, response.ErrSystemError)
			c.Abort()
			return
		}

		tenantService := globalServiceProvider.GetTenantService()
		tenant, err := tenantService.GetWorkspaceByID(context.Background(), tenantID)
		if err != nil {
			response.Fail(c, response.ErrWorkspaceNotFound)
			c.Abort()
			return
		}

		c.Set("current_tenant", tenant)
		c.Next()
	}
}

func EnterpriseLicenseRequiredWithProvider() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalServiceProvider == nil {
			response.Fail(c, response.ErrSystemError)
			c.Abort()
			return
		}

		enterpriseService := globalServiceProvider.GetOrganizationService()
		hasLicense, err := enterpriseService.CheckLicense(context.Background())
		if err != nil || !hasLicense {
			response.Fail(c, response.ErrSystemError)
			c.Abort()
			return
		}

		c.Next()
	}
}

func SetupRequiredWithProvider() gin.HandlerFunc {
	return func(c *gin.Context) {
		if globalServiceProvider == nil {
			response.Fail(c, response.ErrSystemError)
			c.Abort()
			return
		}

		systemService := globalServiceProvider.GetSystemService()
		if systemService.IsSetupRequired() {
			setupInfo, err := systemService.GetSetupInfo(context.Background())
			if err != nil || setupInfo == nil {
				response.Fail(c, response.ErrSystemNotSetup)
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

func RateLimitWithProvider(key string, limit int, window int) gin.HandlerFunc {
	return func(c *gin.Context) {
		rateLimiter := util.NewRateLimiter(key, int64(limit), int64(window))

		clientIP := c.ClientIP()
		isLimited, err := rateLimiter.IsRateLimited(context.Background(), clientIP)
		if err != nil {
			logger.Error("Rate limiter error: %v", err)
			response.Fail(c, response.ErrSystemError)
			c.Abort()
			return
		}
		if isLimited {
			response.Fail(c, response.ErrRateLimitExceeded)
			c.Abort()
			return
		}

		rateLimiter.IncrementRateLimit(clientIP)
		c.Next()
	}
}
