package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apiKeyModule "github.com/zgiai/zgi/api/internal/modules/api_key"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// APIKeyAuthMiddleware validates API key and extracts agent/tenant info
func APIKeyAuthMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract API key from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Fail(c, response.ErrorCode{Code: 401001, Message: "Authorization header required", UserVisible: true})
			c.Abort()
			return
		}

		// Check if it's Bearer token format
		if !strings.HasPrefix(authHeader, "Bearer ") {
			response.Fail(c, response.ErrorCode{Code: 401002, Message: "Invalid authorization format. Expected: Bearer <token>", UserVisible: true})
			c.Abort()
			return
		}

		// Extract the actual token
		apiKey := strings.TrimPrefix(authHeader, "Bearer ")
		if apiKey == "" {
			response.Fail(c, response.ErrorCode{Code: 401003, Message: "API key cannot be empty", UserVisible: true})
			c.Abort()
			return
		}

		apiKeyInfo, err := validateAPIKey(db, apiKey)
		if err != nil {
			response.Fail(c, response.ErrorCode{Code: 401004, Message: "Invalid API key", UserVisible: true})
			c.Abort()
			return
		}

		// Set context values for downstream handlers
		c.Set("api_key_info", apiKeyInfo)
		c.Set("agent_id", apiKeyInfo.AgentID.String())
		util.SetWorkspaceScopeCompat(c, apiKeyInfo.TenantID.String())
		c.Set("api_key_id", apiKeyInfo.ID.String())

		// Set workflow execution context parameters for external API calls
		c.Set("invoke_from", "external-api")
		c.Set("created_from", "external-api")
		c.Set("created_by_role", "end_user")

		// Update last used timestamp
		go updateLastUsed(db, apiKeyInfo.ID)

		c.Next()
	}
}

// APIKeyInfo represents the validated API key information
type APIKeyInfo struct {
	ID         uuid.UUID  `json:"id"`
	AgentID    uuid.UUID  `json:"agent_id"`
	TenantID   uuid.UUID  `json:"tenant_id"`
	KeyPrefix  string     `json:"key_prefix"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	ExpiresAt  *time.Time `json:"expires_at"`
	UsageCount int64      `json:"usage_count"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// validateAPIKey validates the API key and returns the associated information
func validateAPIKey(db *gorm.DB, key string) (*APIKeyInfo, error) {
	// Calculate hash of the provided key
	hash := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(hash[:])

	// Query database for matching hash
	var apiKey apiKeyModule.APIKey
	err := db.Where("key_hash = ? AND status = 'active'", keyHash).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		First(&apiKey).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("API key not found or expired")
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	return &APIKeyInfo{
		ID:         apiKey.ID,
		AgentID:    apiKey.AgentID,
		TenantID:   apiKey.TenantID,
		KeyPrefix:  apiKey.KeyPrefix,
		Name:       apiKey.Name,
		Status:     apiKey.Status,
		ExpiresAt:  apiKey.ExpiresAt,
		UsageCount: apiKey.UsageCount,
		LastUsedAt: apiKey.LastUsedAt,
		CreatedAt:  apiKey.CreatedAt,
	}, nil
}

// updateLastUsed updates the last used timestamp and usage counters for the API key
func updateLastUsed(db *gorm.DB, keyID uuid.UUID) {
	now := time.Now()

	updates := map[string]interface{}{
		"last_used_at": now,
		"usage_count":  gorm.Expr("usage_count + 1"),
	}

	db.Model(&apiKeyModule.APIKey{}).
		Where("id = ?", keyID).
		Updates(updates)
}
