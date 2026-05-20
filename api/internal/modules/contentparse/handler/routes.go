package handler

import "github.com/gin-gonic/gin"

const contentParseInternalRouteKey = "contentparse_internal_route"

// RegisterInternalRoutes registers internal management and shadow-inspection
// routes for the independent content parse policy platform.
//
// These routes are mounted under /console/api/internal and protected by the
// console-api HMAC middleware, keeping admin/shadow inspection separate from
// authenticated user-facing playground routes.
func RegisterInternalRoutes(
	rg *gin.RouterGroup,
	provider *ProviderHandler,
	policy *PolicyHandler,
	health *HealthHandler,
	artifact *ArtifactHandler,
	run *RunHandler,
	playground *PlaygroundHandler,
) {
	group := rg.Group("/content-parse")
	group.Use(func(c *gin.Context) {
		c.Set(contentParseInternalRouteKey, true)
		c.Next()
	})
	{
		if provider != nil {
			provider.RegisterRoutes(group)
		}
		if policy != nil {
			policy.RegisterRoutes(group)
		}
		if health != nil {
			health.RegisterRoutes(group)
		}
		if artifact != nil {
			artifact.RegisterRoutes(group)
		}
		if run != nil {
			run.RegisterRoutes(group)
		}
		if playground != nil {
			playground.RegisterRoutes(group)
		}
	}
}
