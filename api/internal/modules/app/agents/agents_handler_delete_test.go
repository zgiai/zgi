package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type deleteAgentPermissionHandlerService struct {
	AgentsService
}

func (s *deleteAgentPermissionHandlerService) DeleteAgent(context.Context, string) error {
	return fmt.Errorf("locked workspace authorization: %w", errAgentPermissionDenied)
}

func TestDeleteAgentHandlerMapsWrappedPermissionDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAgentsHandler(&deleteAgentPermissionHandlerService{}, nil, nil, nil, nil)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodDelete, "/apps/agent-1", nil)
	c.Params = gin.Params{{Key: "agent_id", Value: "agent-1"}}
	c.Set("account_id", "account-1")
	c.Set("organization_id", "organization-1")

	handler.DeleteAgent(c)

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v; body=%s", err, recorder.Body.String())
	}
	if got := body["code"]; got != "403001" {
		t.Fatalf("response code = %v, want 403001; body=%s", got, recorder.Body.String())
	}
}
