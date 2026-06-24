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

func rejectUnauthorizedWebAppRuntime(c *gin.Context, agent *agents.Agent, webAppID string) bool {
	err := authorizeWebAppRuntimeForAgent(
		c.Request.Context(),
		runtimeauth.NewStore(database.GetDB()),
		database.GetDB(),
		agent,
		c.GetString("account_id"),
		c.GetBool("is_authenticated"),
	)
	if err == nil {
		return false
	}

	logger.WarnContext(c.Request.Context(), "web app runtime authorization failed",
		"web_app_id", webAppID,
		"agent_id", agent.ID.String(),
		"account_id", c.GetString("account_id"),
		"is_authenticated", c.GetBool("is_authenticated"),
		err,
	)
	failWebAppRuntimeAuthorization(c, err)
	return true
}

func publicCompatibleWebAppRuntimeAllowed(ctx context.Context, agent *agents.Agent) (bool, error) {
	if agent == nil {
		return false, fmt.Errorf("agent is required")
	}
	if !agent.IsWebAppActive() {
		return false, nil
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
	return auth.Evaluate(runtimeauth.PublishedRuntimeSurfaceWebApp, runtimeauth.RuntimeAudience{}).Allowed, nil
}
