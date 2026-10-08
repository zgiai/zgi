package agents

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestPublishedEntryWiringKeepsUnfinishedExecutionClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, entry := range []string{"webapp", "anonymous-webapp", "external-api"} {
		t.Run(entry, func(t *testing.T) {
			p, published := publishedExecutionFixture("webapp")
			t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", `{"agent:webapp":["`+published.OrganizationID+`"],"agent:external-api":["`+published.OrganizationID+`"]}`)
			service := &publishedExecutionTestService{stubWebAppStatusHandlerService: &stubWebAppStatusHandlerService{
				publishedConfigResp: published, runtimeCapabilityResp: &dto.AgentWebAppRuntimeCapabilityResponse{Allowed: true}}}
			ids := webAppRuntimePermissionIDs{organizationID: p.Scope.OrganizationID, workspaceID: *p.Scope.WorkspaceID, accountID: p.Scope.AccountID,
				agentID: *p.Caller.ID, webAppID: *p.Caller.SourceWebAppID, conversationID: uuid.New(), messageID: uuid.New()}
			runtime := &apiKeyChatRuntimeService{ids: ids}
			h := NewAgentsHandler(service, nil, nil, nil, nil, runtime)
			h.executionEntryReady = nil // Missing server readiness still fails closed.
			// Even a published opt-in plus operator allowlist cannot enable an entry
			// while code readiness is absent. Normal chat must not need a Worker.
			h.executionApprovals = agentExecutionApprovals{}
			h.executionGateway = nil
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("account_id", ids.accountID.String())
			c.Set("is_authenticated", entry != "anonymous-webapp")
			if entry == "external-api" {
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"query":"hello"}`))
				c.Request.Header.Set("Content-Type", "application/json")
				setAPIKeyAgentTestContext(c, ids)
				h.ChatAPIKeyAgent(c)
				if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "text/event-stream" || runtime.lastRequest.Query != "hello" || runtime.lastRunConfig.RuntimeToolFactory != nil {
					t.Fatalf("closed API changed ordinary chat: status=%d body=%s", recorder.Code, recorder.Body)
				}
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/continue", nil)
				c.Request.Header.Set(ExternalUserIDHeader, "end-user-1")
				result, ok := h.apiKeyAgentContinuationRuntimeContext(c)
				if !ok || result.RunConfig.RuntimeToolFactory != nil {
					t.Fatal("API continuation released unfinished execution")
				}
			} else {
				c.Request = httptest.NewRequest(http.MethodPost, "/webapps/"+ids.webAppID.String()+"/chat", nil)
				c.Params = gin.Params{{Key: "web_app_id", Value: ids.webAppID.String()}}
				result, ok := h.webAppAgentRuntimeContext(c)
				if !ok || result.RunConfig.RuntimeToolFactory != nil {
					t.Fatalf("closed WebApp changed ordinary chat: %s", recorder.Body)
				}
			}
		})
	}
}
