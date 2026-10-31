package middleware

import (
	"github.com/gin-gonic/gin"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

// RequestLocale records the request's Accept-Language on its context, so
// localized errors and the chat runtime's reply language can use it.
func RequestLocale() gin.HandlerFunc {
	return func(c *gin.Context) {
		if header := c.GetHeader("Accept-Language"); header != "" {
			c.Request = c.Request.WithContext(apptransport.WithLocale(c.Request.Context(), header))
		}
		c.Next()
	}
}
