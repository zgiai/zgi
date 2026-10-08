package agents

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestConsolePlatformGovernanceContinuationIsScopedAndFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, action, source                                     string
		enabled, trusted, executionFailed, serviceFailed, called bool
	}{
		{name: "approve", action: "approve", source: "console", enabled: true, trusted: true, called: true},
		{name: "reject after revocation", action: "reject", source: "console", executionFailed: true, called: true},
		{name: "flag disabled", action: "approve", source: "console", trusted: true},
		{name: "untrusted tool", action: "approve", source: "console", enabled: true},
		{name: "worker unavailable", action: "approve", source: "console", enabled: true, trusted: true, executionFailed: true},
		{name: "public WebApp", action: "approve", source: "webapp", enabled: true, trusted: true},
		{name: "API key", action: "approve", source: "api", enabled: true, trusted: true},
		{name: "invalid action", action: "allow", source: "console", enabled: true, trusted: true},
		{name: "safe service error", action: "approve", source: "console", enabled: true, trusted: true, serviceFailed: true, called: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", "false")
			if tc.enabled {
				t.Setenv("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", "true")
			}
			orgID, accountID, agentID, conversationID, messageID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
			svc := &webAppRuntimePermissionService{}
			if tc.serviceFailed {
				svc.toolGovernanceErr = errors.New("private-token-and-upstream-url")
			}
			h := &AgentsHandler{chatRuntimeService: svc}
			runtimeCtx := agentRuntimeContext{
				Scope:  runtimeservice.Scope{OrganizationID: orgID, AccountID: accountID},
				Caller: runtimeservice.Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: tc.source},
			}
			if tc.executionFailed {
				runtimeCtx.ExecutionError = runtimeservice.ErrNotFound
			}
			runtimeCtx.RunConfig.RuntimeToolFactory = func(prepared *runtimeservice.PreparedChat) []skillloop.RuntimeTool {
				if prepared.Scope != runtimeCtx.Scope || prepared.Caller.ID == nil || *prepared.Caller.ID != agentID || prepared.Conversation.ID != conversationID || prepared.Message.ID != messageID {
					t.Error("approval factory lost authoritative scope")
				}
				return []skillloop.RuntimeTool{{
					Definition:                  adapter.Tool{Function: adapter.Function{Name: "shell_exec"}},
					PlatformInteractiveApproval: tc.trusted,
					BindPlatform:                func(skillloop.RuntimePlatformServices) {},
				}}
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/tool-governance/corr/continue", strings.NewReader(`{"action":"`+tc.action+`"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "conversation_id", Value: conversationID.String()}, {Key: "message_id", Value: messageID.String()}, {Key: "correlation_id", Value: "corr"}}
			h.continueConsoleRuntimeToolGovernance(c, runtimeCtx)
			if svc.toolGovernanceCalled != tc.called {
				t.Fatalf("continuation called=%v, want %v; response=%s", svc.toolGovernanceCalled, tc.called, w.Body.String())
			}
			if tc.called && (svc.lastCaller != runtimeCtx.Caller || svc.lastScope != runtimeCtx.Scope || svc.lastConversationID != conversationID || svc.lastMessageID != messageID || svc.lastCorrelationID != "corr" || svc.lastToolGovernanceRequest.Action != tc.action) {
				t.Fatal("continuation changed scope or frozen correlation")
			}
			if strings.Contains(w.Body.String(), "private-token") {
				t.Fatal("upstream error leaked into SSE")
			}
		})
	}
}
