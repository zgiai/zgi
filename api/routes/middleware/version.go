package middleware

import (
	"github.com/gin-gonic/gin"
)

func VersionMiddleware(version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("api_version", version)
		c.Next()
	}
}

func GetVersion(c *gin.Context) string {
	if v, exists := c.Get("api_version"); exists {
		return v.(string)
	}
	return "v1"
}
