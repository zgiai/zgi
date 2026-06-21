package workflow

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

func rejectInactiveWebApp(c *gin.Context, agent *agents.Agent, webAppID string) bool {
	allowed, err := publicCompatibleWebAppRuntimeAllowed(c.Request.Context(), agent)
	if err != nil {
		logger.Error("Failed to resolve web app runtime policy", err)
		response.Fail(c, response.ErrSystemError)
		return true
	}
	if allowed {
		return false
	}

	logger.Warn("Web app is offline", map[string]interface{}{
		"web_app_id": webAppID,
		"agent_id":   agent.ID.String(),
	})
	response.Fail(c, response.ErrWebAppOffline)
	return true
}

func publicCompatibleWebAppRuntimeAllowed(ctx context.Context, agent *agents.Agent) (bool, error) {
	if agent == nil {
		return false, fmt.Errorf("agent is required")
	}
	fallback := runtimeauth.PolicyFromAgentFields(string(agent.WebAppStatus), agent.EnableAPI)
	auth, err := runtimeauth.NewStore(database.GetDB()).GetResourceAuthorization(
		ctx,
		runtimeauth.PublishedRuntimeResourceAgent,
		agent.ID,
		fallback,
	)
	if err != nil {
		return false, err
	}
	return auth.Evaluate(runtimeauth.PublishedRuntimeSurfaceWebApp, runtimeauth.RuntimeAudience{OrganizationID: auth.OrganizationID}).Allowed, nil
}
