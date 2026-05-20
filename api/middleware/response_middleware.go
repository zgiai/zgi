package middleware

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/pkg/response"
)

// ResponseMiddleware ensures all responses follow the unified format
func ResponseMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		// Create a custom response writer to capture response data
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// Record start time for performance metrics
		startTime := time.Now()

		// Process the request
		c.Next()

		// Record processing time
		processingTime := time.Since(startTime).Milliseconds()

		// Add performance headers
		c.Header("X-Processing-Time", string(rune(processingTime))+"ms")
		c.Header("X-Response-Format", "unified")

		// Validate response format if enabled
		if gin.Mode() == gin.DebugMode {
			validateResponseFormat(c, blw.body.String())
		}
	})
}

// bodyLogWriter wraps gin.ResponseWriter to capture response body
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w bodyLogWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// validateResponseFormat validates that responses follow unified format
func validateResponseFormat(c *gin.Context, responseBody string) {
	if responseBody == "" {
		return
	}

	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &responseData); err != nil {
		// Not JSON, skip validation
		return
	}

	// Check for unified response structure
	if hasUnifiedStructure(responseData) {
		return
	}

	// Check for forbidden direct JSON patterns
	if hasForbiddenPattern(responseData) {
		// Log warning about non-standard response format
		gin.DefaultWriter.Write([]byte("WARNING: Non-standard response format detected in " + c.Request.URL.Path + "\n"))
	}
}

// hasUnifiedStructure checks if response follows unified format
func hasUnifiedStructure(data map[string]interface{}) bool {
	// Check for standard success format: {"code", "message", "data", "result"}
	if code, hasCode := data["code"]; hasCode {
		if result, hasResult := data["result"]; hasResult {
			return code != nil && result != nil
		}
	}

	// Check for business error format: {"error_code", "description", "en_description"}
	if errorCode, hasErrorCode := data["error_code"]; hasErrorCode {
		if description, hasDescription := data["description"]; hasDescription {
			return errorCode != nil && description != nil
		}
	}

	// Check for special fail format: {"result": "fail", "data"}
	if result, hasResult := data["result"]; hasResult {
		if result == "fail" {
			return true
		}
	}

	return false
}

// hasForbiddenPattern checks for direct JSON response patterns
func hasForbiddenPattern(data map[string]interface{}) bool {
	// Check for direct error responses like {"error": "..."}
	if _, hasError := data["error"]; hasError {
		if _, hasCode := data["code"]; !hasCode {
			return true
		}
	}

	// Check for inconsistent message patterns
	if _, hasMessage := data["message"]; hasMessage {
		if _, hasCode := data["code"]; !hasCode {
			return true
		}
	}

	return false
}

// ResponseMetricsMiddleware collects response metrics
func ResponseMetricsMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		startTime := time.Now()

		// Process request
		c.Next()

		// Calculate metrics
		duration := time.Since(startTime)
		statusCode := c.Writer.Status()

		// Add metrics headers
		c.Header("X-Duration", duration.String())
		c.Header("X-Status-Code", string(rune(statusCode)))

		// Log metrics (in production, send to monitoring system)
		if gin.Mode() == gin.DebugMode {
			logResponseMetrics(c, duration, statusCode)
		}
	})
}

// logResponseMetrics logs response performance metrics
func logResponseMetrics(c *gin.Context, duration time.Duration, statusCode int) {
	gin.DefaultWriter.Write([]byte(
		"METRICS: " + c.Request.Method + " " + c.Request.URL.Path +
			" | Status: " + string(rune(statusCode)) +
			" | Duration: " + duration.String() + "\n",
	))
}

// ErrorHandlerMiddleware provides centralized error handling
func ErrorHandlerMiddleware() gin.HandlerFunc {
	return gin.HandlerFunc(func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Handle panic and return standardized error response
				response.Fail(c, response.ErrSystemError)
				c.Abort()
			}
		}()

		c.Next()

		// Handle any errors that were set but not responded to
		if len(c.Errors) > 0 {
			response.Fail(c, response.ErrSystemError)
		}
	})
}

// CORSMiddleware handles CORS with standardized responses
func CORSMiddleware() gin.HandlerFunc {
	allowedOrigins := config.Current().Server.CORSAllowOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}

	return gin.HandlerFunc(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		allowOrigin := "*"
		if allowedOrigins[0] != "*" {
			allowOrigin = allowedOrigins[0]
			for _, o := range allowedOrigins {
				if o == origin {
					allowOrigin = origin
					break
				}
			}
		}

		c.Header("Access-Control-Allow-Origin", allowOrigin)
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			response.Success(c, gin.H{"message": "CORS preflight successful"})
			c.Abort()
			return
		}

		c.Next()
	})
}

// SetupResponseMiddleware configures all response-related middleware
func SetupResponseMiddleware(r *gin.Engine) {
	// Apply middleware in order
	r.Use(ResponseMetricsMiddleware())
	r.Use(ErrorHandlerMiddleware())
	r.Use(ResponseMiddleware())
	r.Use(CORSMiddleware())
}
