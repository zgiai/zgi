package middleware

import (
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := safeRequestLogPath(c.Request.URL)

		c.Next()

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()

		logger.Info("HTTP Request",
			zap.Int("status", statusCode),
			zap.Duration("latency", latency),
			zap.String("client_ip", clientIP),
			zap.String("method", method),
			zap.String("path", path),
		)
	}
}

var sensitiveRequestQueryKeys = map[string]struct{}{
	"access_token":       {},
	"authorization_code": {},
	"client_assertion":   {},
	"client_secret":      {},
	"code":               {},
	"code_verifier":      {},
	"id_token":           {},
	"refresh_token":      {},
	"state":              {},
	"token":              {},
}

// safeRequestLogPath keeps useful query structure without persisting OAuth
// authorization codes, CSRF state, tokens, or client secrets. Provider
// callbacks necessarily transport those values in the URL, so logging the raw
// query would turn an otherwise short-lived credential into durable log data.
func safeRequestLogPath(requestURL *url.URL) string {
	if requestURL == nil {
		return ""
	}
	path := requestURL.Path
	if requestURL.RawQuery == "" {
		return path
	}
	values, err := url.ParseQuery(requestURL.RawQuery)
	if err != nil {
		return path + "?redacted_query=true"
	}
	redactAllValues := strings.HasSuffix(strings.ToLower(strings.TrimSpace(path)), "/integrations/oauth/callback")
	for key := range values {
		if _, sensitive := sensitiveRequestQueryKeys[strings.ToLower(strings.TrimSpace(key))]; redactAllValues || sensitive {
			values[key] = []string{"[REDACTED]"}
		}
	}
	return path + "?" + values.Encode()
}
