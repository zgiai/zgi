package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/config"
)

// ConsoleAPIAuth verifies HMAC-SHA256 signed requests from console-api.
// Console-api signs requests with: HMAC-SHA256(CONSOLE_INTERNAL_API_KEY, timestamp + "|" + request_path)
// and sends X-Internal-Timestamp + X-Internal-Signature headers.
func ConsoleAPIAuth() gin.HandlerFunc {
	secret := config.Current().Console.InternalAPIKey

	return func(c *gin.Context) {
		if secret == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "401", "message": "internal API key not configured"})
			c.Abort()
			return
		}

		ts := c.GetHeader("X-Internal-Timestamp")
		sig := c.GetHeader("X-Internal-Signature")

		if ts == "" || sig == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "401", "message": "missing authentication headers"})
			c.Abort()
			return
		}

		// Verify timestamp within ±5 minutes
		tsInt, err := strconv.ParseInt(ts, 10, 64)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "401", "message": "invalid timestamp"})
			c.Abort()
			return
		}

		diff := math.Abs(float64(time.Now().Unix() - tsInt))
		if diff > 300 {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "401", "message": "timestamp expired"})
			c.Abort()
			return
		}

		// Verify HMAC signature
		message := ts + "|" + c.Request.URL.Path
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(message))
		expected := hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(sig), []byte(expected)) {
			c.JSON(http.StatusUnauthorized, gin.H{"code": "401", "message": "invalid signature"})
			c.Abort()
			return
		}

		c.Next()
	}
}
