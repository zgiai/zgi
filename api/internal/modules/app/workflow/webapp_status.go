package workflow

import (
	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/internal/modules/app/agents"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/response"
)

func rejectInactiveWebApp(c *gin.Context, agent *agents.Agent, webAppID string) bool {
	if agent.IsWebAppActive() {
		return false
	}

	logger.Warn("Web app is offline", map[string]interface{}{
		"web_app_id": webAppID,
		"agent_id":   agent.ID.String(),
	})
	response.Fail(c, response.ErrWebAppOffline)
	return true
}
