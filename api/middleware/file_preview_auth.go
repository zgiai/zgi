package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	jwtpkg "github.com/zgiai/zgi/api/pkg/jwt"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// FilePreviewAuthMiddleware handles dual authentication for file preview endpoints
// Supports two authentication methods with priority:
// 1. URL signature authentication (timestamp, nonce, sign query parameters) - PRIORITY
// 2. JWT Authorization header (standard authenticated access) - FALLBACK
// URL signature is checked first. If not present or invalid, falls back to JWT header.
func FilePreviewAuthMiddleware(accountService interfaces.AccountService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(accountServiceKey, accountService)

		// Get file_id from URL parameter
		fileID := c.Param("file_id")
		if fileID == "" {
			logger.Warn("FilePreviewAuth: file_id parameter missing")
			response.Fail(c, response.ErrFileIdRequired)
			c.Abort()
			return
		}

		// Try URL signature authentication first (PRIORITY)
		timestamp := c.Query("timestamp")
		nonce := c.Query("nonce")
		sign := c.Query("sign")

		// Check if signature parameters are present
		if timestamp != "" && nonce != "" && sign != "" {
			logger.Debug("FilePreviewAuth: Attempting URL signature authentication")

			// Verify URL signature
			if util.VerifyFileSignature(fileID, timestamp, nonce, sign) {
				// URL signature authentication successful
				c.Set("auth_method", "url_signature")
				logger.Debug("FilePreviewAuth: URL signature authentication successful")
				c.Next()
				return
			}

			logger.Debug("FilePreviewAuth: URL signature verification failed, will try JWT fallback")
		}

		// Fall back to JWT authentication
		logger.Debug("FilePreviewAuth: Attempting JWT authentication")
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			logger.Warn("FilePreviewAuth: No valid authentication method found")
			response.Fail(c, response.ErrAuthHeaderRequired)
			c.Abort()
			return
		}

		// Check Bearer token format
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			logger.Warn("FilePreviewAuth: Invalid auth format: %s", authHeader)
			response.Fail(c, response.ErrInvalidAuthFormat)
			c.Abort()
			return
		}

		// Parse token
		userID, err := jwtpkg.GetUserIDFromToken(parts[1])
		if err != nil {
			logger.Error("FilePreviewAuth: JWT token parsing failed", err)
			response.Fail(c, response.ErrTokenInvalid)
			c.Abort()
			return
		}

		// JWT authentication successful
		c.Set("account_id", userID)
		c.Set("auth_method", "jwt")

		// Resolve tenant ID
		tenantID := resolveTenantID(c)
		if tenantID != "" {
			util.SetOrganizationScopeCompat(c, tenantID)
		}

		logger.Debug("FilePreviewAuth: JWT authentication successful, user_id=%s", userID)
		c.Next()
	}
}
