package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"plugin_runner/internal/config"
)

var (
	rateMu      sync.Mutex
	reqCounters = make(map[string]int)
	windowStart = time.Now()
)

func ginRateLimit(c *gin.Context, cfg *config.Config, log *zap.Logger, key string) bool {
	rateMu.Lock()
	defer rateMu.Unlock()

	// reset window every minute
	if time.Since(windowStart) > time.Minute {
		reqCounters = make(map[string]int)
		windowStart = time.Now()
	}

	reqCounters[key]++

	limit := cfg.RateLimitPerMinute
	tenantLimit := cfg.TenantRateLimitPerMinute
	if tenantLimit > 0 {
		if reqCounters[key] > tenantLimit {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "tenant rate limit exceeded"})
			return true
		}
	}
	if limit > 0 {
		total := 0
		for _, v := range reqCounters {
			total += v
		}
		if total > limit {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return true
		}
	}
	return false
}
