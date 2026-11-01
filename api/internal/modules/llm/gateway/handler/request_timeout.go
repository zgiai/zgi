package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/requesttimeout"
)

// GatewayRequestTimeout reads X-ZGI-Request-Timeout once at the public edge
// and records it on the request context, where the adapters size their HTTP
// budget and the Console platform proxy receives it. A header that is present
// but not a whole number of seconds within the configured maximum is rejected
// with 400 llm.request.invalid; it is never clamped.
func GatewayRequestTimeout() gin.HandlerFunc {
	headerKey := http.CanonicalHeaderKey(requesttimeout.HeaderName)
	return func(c *gin.Context) {
		values, present := c.Request.Header[headerKey]
		if !present {
			c.Next()
			return
		}
		timeout, err := requesttimeout.Parse(values[0])
		if err != nil {
			abortWithProtocolError(c, invalidRequestProtocolError(err.Error()))
			return
		}
		c.Request = c.Request.WithContext(requesttimeout.WithTimeout(c.Request.Context(), timeout))
		c.Next()
	}
}
