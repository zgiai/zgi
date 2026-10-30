package agents

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

const (
	agentLegacyPermissionDeniedCode = "403001"
	agentLegacyNotFoundCode         = "404001"
)

// writeAgentLegacyFailure writes a legacy agent failure body unchanged, with an
// HTTP status that matches the failure: 403 for permission denials and 404 for
// a missing agent. Other failures keep the historical HTTP 200 so existing
// console callers see no contract change.
func writeAgentLegacyFailure(c *gin.Context, body gin.H, err error) {
	c.JSON(agentLegacyFailureStatus(body, err), body)
}

func agentLegacyFailureStatus(body gin.H, err error) int {
	switch body["code"] {
	case agentLegacyPermissionDeniedCode:
		return http.StatusForbidden
	case agentLegacyNotFoundCode:
		return http.StatusNotFound
	}
	if err == nil {
		return http.StatusOK
	}
	switch {
	case errors.Is(err, errAgentPermissionDenied),
		errors.Is(err, errAgentReadPermissionDenied),
		errors.Is(err, runtimeservice.ErrPermissionDenied),
		err.Error() == "permission denied":
		return http.StatusForbidden
	case errors.Is(err, runtimeservice.ErrNotFound),
		err.Error() == "agent not found":
		return http.StatusNotFound
	}
	return http.StatusOK
}

func agentPermissionDeniedBody() gin.H {
	return gin.H{"code": agentLegacyPermissionDeniedCode, "message": "Permission denied"}
}

func agentNotFoundBody() gin.H {
	return gin.H{"code": agentLegacyNotFoundCode, "message": "Agent not found"}
}
