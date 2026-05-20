// Package handler provides HTTP handlers and middleware for the LLM gateway.
package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apikeyrepo "github.com/zgiai/ginext/internal/modules/llm/apikey/repository"
	"github.com/zgiai/ginext/internal/modules/llm/gateway/types"
	"github.com/zgiai/ginext/pkg/response"
)

// LLMAPIKeyAuthMiddleware validates LLM API keys
func LLMAPIKeyAuthMiddleware(apiKeyRepo apikeyrepo.APIKeyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey, errCode, ok := extractGatewayAPIKey(c)
		if !ok {
			response.Fail(c, response.ErrorCode{
				Code:        types.ErrCodeInvalidAPIKey.Code,
				Message:     errCode,
				UserVisible: true,
			})
			c.Abort()
			return
		}

		// 4. Validate API key
		keyInfo, err := apiKeyRepo.GetByKey(c.Request.Context(), apiKey)
		if err != nil {
			response.Fail(c, response.ErrorCode{
				Code:        types.ErrCodeInvalidAPIKey.Code,
				Message:     fmt.Sprintf("Invalid API key: %v", err),
				UserVisible: true,
			})
			c.Abort()
			return
		}

		// Internal keys are for system use only and cannot be used via API
		if keyInfo.IsInternal {
			response.Fail(c, response.ErrorCode{
				Code:        types.ErrCodeInvalidAPIKey.Code,
				Message:     "Internal API keys cannot be used for external API calls",
				UserVisible: true,
			})
			c.Abort()
			return
		}

		// 5. Check if API key is active
		if !keyInfo.IsActive() {
			response.Fail(c, response.ErrorCode{
				Code:        types.ErrCodeAPIKeyInactive.Code,
				Message:     "API key is inactive or expired",
				UserVisible: true,
			})
			c.Abort()
			return
		}

		// 6. Check if API key has quota
		if !keyInfo.HasQuota() {
			response.Fail(c, response.ErrorCode{
				Code:        types.ErrCodeInsufficientQuota.Code,
				Message:     "API key has insufficient quota",
				UserVisible: true,
			})
			c.Abort()
			return
		}

		// 7. Set context values for downstream handlers
		c.Set("llm_api_key", keyInfo)
		c.Set("organization_id", keyInfo.OrganizationID)
		c.Set("api_key_id", keyInfo.ID)

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
