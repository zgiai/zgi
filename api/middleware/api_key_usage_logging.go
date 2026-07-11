package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	apiKeyModule "github.com/zgiai/zgi/api/internal/modules/api_key"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const apiKeyUsageMaxCapturedResponseBytes = 64 * 1024

// responseWriter wraps gin.ResponseWriter to capture response data
type responseWriter struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if w.body.Len() < apiKeyUsageMaxCapturedResponseBytes {
		remaining := apiKeyUsageMaxCapturedResponseBytes - w.body.Len()
		if len(b) > remaining {
			_, _ = w.body.Write(b[:remaining])
		} else {
			_, _ = w.body.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// APIKeyUsageLoggingMiddleware logs API key usage details
func APIKeyUsageLoggingMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		// Get API key info from context (set by APIKeyAuthMiddleware)
		apiKeyInfo, exists := c.Get("api_key_info")
		if !exists {
			// If no API key info, skip logging (auth middleware should have handled this)
			c.Next()
			return
		}

		keyInfo, ok := apiKeyInfo.(*APIKeyInfo)
		if !ok {
			c.Next()
			return
		}

		// Read and store request body (for potential future use)
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			// Restore the body for downstream handlers
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// Create custom response writer to capture response
		responseBuffer := &bytes.Buffer{}
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:           responseBuffer,
			statusCode:     200, // default status
		}
		c.Writer = writer

		// Process request
		c.Next()

		// Calculate response time
		responseTime := time.Since(startTime)

		// Get response body size
		responseBodySize := writer.body.Len()

		// Prepare request headers (filter sensitive data)
		requestHeaders := make(map[string]interface{})
		for key, values := range c.Request.Header {
			requestHeaders[key] = sanitizedAPIKeyUsageHeader(key, values)
		}

		// Get client IP
		clientIP := c.ClientIP()

		// Get user agent
		userAgent := c.Request.UserAgent()

		// Prepare metadata
		metadata := map[string]interface{}{
			"request_method": c.Request.Method,
			"response_time":  responseTime.String(),
			"user_agent":     userAgent,
		}

		// Add workflow context if available
		if agentID, exists := c.Get("agent_id"); exists {
			metadata["agent_id"] = agentID
		}
		if tenantID, exists := c.Get("tenant_id"); exists {
			metadata["tenant_id"] = tenantID
		}

		// Parse tokens from response if available
		var tokensUsed int64

		// Try to extract tokens from response body (if it's JSON)
		if responseBodySize > 0 && strings.Contains(c.GetHeader("Content-Type"), "application/json") {
			var responseData map[string]interface{}
			if err := json.Unmarshal(writer.body.Bytes(), &responseData); err == nil {
				// Try to extract token information from various possible locations
				if data, ok := responseData["data"].(map[string]interface{}); ok {
					if tokens, ok := data["total_tokens"].(float64); ok {
						tokensUsed = int64(tokens)
					}
				}
				// Also check root level
				if tokens, ok := responseData["total_tokens"].(float64); ok {
					tokensUsed = int64(tokens)
				}
			}
		}

		// Prepare error message if response indicates error
		var errorMessage *string
		if writer.statusCode >= 400 {
			if responseBodySize > 0 {
				errorMsg := writer.body.String()
				if len(errorMsg) > 1000 {
					errorMsg = errorMsg[:1000] + "..."
				}
				errorMessage = &errorMsg
			}
		}

		requestPath := c.Request.URL.Path
		requestHeadersJSON, _ := json.Marshal(requestHeaders)
		metadataJSON, _ := json.Marshal(metadata)

		// Create usage log entry
		usageLog := &apiKeyModule.APIKeyUsageLog{
			ID:                 uuid.New(),
			APIKeyID:           keyInfo.ID,
			AgentID:            keyInfo.AgentID,
			TenantID:           keyInfo.TenantID,
			RequestPath:        requestPath,
			RequestIP:          clientIP,
			UserAgent:          &userAgent,
			RequestHeaders:     datatypes.JSON(requestHeadersJSON),
			RequestBodySize:    int64(len(requestBody)),
			ResponseStatusCode: writer.statusCode,
			ResponseBodySize:   int64(responseBodySize),
			ResponseTimeMS:     int(responseTime.Milliseconds()),
			TokensUsed:         tokensUsed,
			ErrorMessage:       errorMessage,
			Metadata:           datatypes.JSON(metadataJSON),
			CreatedAt:          startTime,
		}

		// Log usage asynchronously to avoid blocking the response
		go func() {
			if err := logAPIKeyUsage(db, usageLog); err != nil {
				logger.Error("Failed to log API key usage", err)
			}
		}()
	}
}

func sanitizedAPIKeyUsageHeader(key string, values []string) interface{} {
	switch {
	case strings.EqualFold(key, "Authorization"):
		return "Bearer ***"
	case strings.EqualFold(key, "X-API-Key"):
		return "***"
	default:
		return values
	}
}

// logAPIKeyUsage saves the usage log to database
func logAPIKeyUsage(db *gorm.DB, usageLog *apiKeyModule.APIKeyUsageLog) error {
	// Use GORM to create the record
	if err := db.Create(usageLog).Error; err != nil {
		return fmt.Errorf("failed to insert usage log: %w", err)
	}

	// Update API key usage count and last_used_at
	if err := db.Model(&apiKeyModule.APIKey{}).
		Where("id = ?", usageLog.APIKeyID).
		Updates(map[string]interface{}{
			"usage_count":  gorm.Expr("usage_count + 1"),
			"last_used_at": time.Now(),
		}).Error; err != nil {
		logger.Error("Failed to update API key usage count", err)
		// Don't fail the whole operation if this update fails
	}

	return nil
}

// getErrorMessage safely extracts error message from pointer
func getErrorMessage(errorMessage *string) string {
	if errorMessage == nil {
		return ""
	}
	return *errorMessage
}
