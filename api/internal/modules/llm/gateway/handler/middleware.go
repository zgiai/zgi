// Package handler provides HTTP handlers and middleware for the LLM gateway.
package handler

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type principalAccessValidator interface {
	ValidatePrincipalAccess(ctx context.Context, apiKey *apikeymodel.TenantAPIKey) error
}

// LLMAPIKeyAuthMiddleware validates LLM API keys
func LLMAPIKeyAuthMiddleware(apiKeyRepo apikeyrepo.APIKeyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, errCode, ok := extractGatewayAPIKey(c)
		if !ok {
			abortWithProtocolError(c, invalidAPIKeyProtocolError(errCode))
			return
		}

		// 4. Validate API key
		keyInfo, err := apiKeyRepo.GetByKey(c.Request.Context(), apiKey)
		if err != nil {
			logger.WarnContext(c.Request.Context(), "API key authentication failed", err)
			abortWithProtocolError(c, invalidAPIKeyProtocolError("Invalid API key"))
			return
		}

		// Internal keys are for system use only and cannot be used via API
		if keyInfo.IsInternal {
			abortWithProtocolError(c, invalidAPIKeyProtocolError("Invalid API key"))
			return
		}

		// Personal keys must be reloaded before trusting cached lifecycle state.
		// This handles both revocation and re-enabling when Redis invalidation is
		// delayed or unavailable.
		if keyInfo.PrincipalType != nil || keyInfo.PrincipalID != nil {
			validator, supported := apiKeyRepo.(principalAccessValidator)
			if !supported {
				logger.WarnContext(c.Request.Context(), "API key principal access validator is unavailable")
				abortWithProtocolError(c, invalidAPIKeyProtocolError("API key access has been revoked"))
				return
			}
			if err := validator.ValidatePrincipalAccess(c.Request.Context(), keyInfo); err != nil {
				logger.WarnContext(c.Request.Context(), "API key principal access validation failed", err)
				abortWithProtocolError(c, invalidAPIKeyProtocolError("API key access has been revoked"))
				return
			}
		}

		// 5. Check if API key is active. Personal keys now contain the
		// authoritative row returned by ValidatePrincipalAccess.
		if !keyInfo.IsActive() {
			abortWithProtocolError(c, invalidAPIKeyProtocolError("API key is inactive or expired"))
			return
		}

		// 6. Check if API key has quota
		if !keyInfo.HasQuota() {
			abortWithProtocolError(c, quotaProtocolError())
			return
		}

		// 7. Set context values for downstream handlers
		c.Set("llm_api_key", keyInfo)
		c.Set("organization_id", keyInfo.OrganizationID)
		c.Set("api_key_id", keyInfo.ID)
		if keyInfo.WorkspaceID != nil {
			c.Set("workspace_id", *keyInfo.WorkspaceID)
		}
		if keyInfo.PrincipalType != nil {
			c.Set("principal_type", *keyInfo.PrincipalType)
		}
		if keyInfo.PrincipalID != nil {
			c.Set("principal_id", *keyInfo.PrincipalID)
		}
		if keyInfo.AccessGrantID != nil {
			c.Set("access_grant_id", *keyInfo.AccessGrantID)
		}

		// 8. Update last accessed time asynchronously
		go func(apiKeyID string) {
			updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = apiKeyRepo.UpdateAccessedAt(updateCtx, apiKeyID)
		}(keyInfo.ID)

		c.Next()
	}
}

func extractGatewayAPIKey(c *gin.Context) (string, string, bool) {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	xAPIKey := strings.TrimSpace(c.GetHeader("x-api-key"))

	bearerKey := ""
	if authHeader != "" {
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return "", "Invalid authorization format. Expected: Bearer <token>", false
		}
		bearerKey = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if bearerKey == "" {
			return "", "API key cannot be empty", false
		}
	}

	if xAPIKey != "" && bearerKey != "" && xAPIKey != bearerKey {
		return "", "Conflicting API key headers", false
	}
	if bearerKey != "" {
		return bearerKey, "", true
	}
	if xAPIKey != "" {
		return xAPIKey, "", true
	}
	return "", "Authorization or x-api-key header required", false
}
