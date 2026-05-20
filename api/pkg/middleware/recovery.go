package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// Recovery middleware handles panic recovery
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// Log stack trace
				logger.Error("Panic recovered", fmt.Errorf("%v", err))
				logger.Debug("Stack trace", string(debug.Stack()))

				response.Fail(c, response.ErrSystemError)
				c.Abort()
			}
		}()
		c.Next()
	}
}
