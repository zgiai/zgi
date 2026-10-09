package agents

import (
	"errors"
	"fmt"
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
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestGovernanceContinuationClassifiesScopedDenialWithoutLeakingCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	productCatalog, err := appcatalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"console", "webapp", "external-api"} {
		for _, notFound := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/not_found=%t", entry, notFound), func(t *testing.T) {
				app, conversation, message := uuid.New(), uuid.New(), uuid.New()
				cause := errors.New("private-upstream-token")
				wantCode := response.ErrSystemError.Code
				if notFound {
					cause = fmt.Errorf("private-upstream-token: %w", runtimeservice.ErrNotFound)
					wantCode = response.ErrNotFound.Code
				}
				svc := &webAppRuntimePermissionService{toolGovernanceErr: cause}
				h := &AgentsHandler{chatRuntimeService: svc, appErrorProjector: projector}
				ctx := agentRuntimeContext{Scope: runtimeservice.Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
					Caller: runtimeservice.Caller{Type: "agent", ID: &app, Source: entry}}
				router := gin.New()
				router.POST("/conversations/:conversation_id/messages/:message_id/tool-governance/:correlation_id/continue", func(c *gin.Context) {
					h.continueRuntimeToolGovernance(c, ctx, nil)
				})
				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodPost, "/conversations/"+conversation.String()+"/messages/"+message.String()+"/tool-governance/frozen/continue", strings.NewReader(`{"action":"reject"}`))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Accept-Language", "zh-Hans")
				router.ServeHTTP(w, r)
				body := w.Body.String()
				if w.Code != http.StatusOK || !svc.toolGovernanceCalled || !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(body, fmt.Sprintf(`"code":%d`, wantCode)) {
					t.Fatalf("scoped continuation error: status=%d, body=%s", w.Code, body)
				}
				if strings.Contains(body, "private-upstream-token") || strings.Contains(body, "tool_end") || strings.Contains(body, "message_end") {
					t.Fatalf("denial leaked a cause or success event: %s", body)
				}
				if notFound && !strings.Contains(body, "未找到请求的内容") {
					t.Fatalf("scoped denial lost localized catalog message: %s", body)
				}
			})
		}
	}
}

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

func TestPublishedPlatformGovernanceRouteStreamsDedicatedDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, entry := range []string{"webapp", "external-api"} {
		for _, action := range []string{"approve", "reject"} {
			t.Run(entry+"/"+action, func(t *testing.T) {
				t.Setenv("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", "true")
				svc := &webAppRuntimePermissionService{}
				h := &AgentsHandler{chatRuntimeService: svc}
				app, conversation, message, org, user := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
				runtimeCtx := agentRuntimeContext{Scope: runtimeservice.Scope{OrganizationID: org, AccountID: user},
					Caller: runtimeservice.Caller{Type: "agent", ID: &app, Source: entry}}
				if action == "approve" {
					runtimeCtx.RunConfig.RuntimeToolFactory = func(*runtimeservice.PreparedChat) []skillloop.RuntimeTool { return nil }
				}
				router := gin.New()
				path := "/runtime/conversations/:conversation_id/messages/:message_id/tool-governance/:correlation_id/continue"
				router.POST(path, func(c *gin.Context) { h.continueRuntimeToolGovernance(c, runtimeCtx, nil) })
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, "/runtime/conversations/"+conversation.String()+"/messages/"+message.String()+"/tool-governance/frozen/continue", strings.NewReader(`{"action":"`+action+`","source":"console","caller_type":"agent"}`))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(w, req)
				if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") || !svc.toolGovernanceCalled || svc.lastCaller.Source != entry || svc.lastCaller.ID == nil || *svc.lastCaller.ID != app || svc.lastToolGovernanceRequest.Action != action {
					t.Fatalf("entry continuation: %d %s", w.Code, w.Body)
				}
			})
		}
	}
}

func TestExternalGovernanceWrappersDenyAnonymousAndRuntimeWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, entry := range []string{"webapp", "external-api"} {
		svc := &webAppRuntimePermissionService{}
		h := &AgentsHandler{chatRuntimeService: svc}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/continue", strings.NewReader(`{"action":"approve"}`))
		if entry == "webapp" {
			h.ContinueWebAppAgentRuntimeToolGovernance(c)
		} else {
			c.Set("runtime_workspace_id", uuid.NewString())
			h.ContinueAPIKeyAgentRuntimeToolGovernance(c)
		}
		if w.Code == http.StatusOK || svc.toolGovernanceCalled {
			t.Fatalf("ineligible %s reached continuation", entry)
		}
	}
}

func TestExternalGovernanceHandlersKeepVerifiedCallerWhenRejectingWithoutLinux(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, entry := range []string{"webapp", "external-api"} {
		t.Run(entry, func(t *testing.T) {
			ids := webAppRuntimePermissionIDs{organizationID: uuid.New(), workspaceID: uuid.New(), accountID: uuid.New(), agentID: uuid.New(), webAppID: uuid.New(), conversationID: uuid.New(), messageID: uuid.New()}
			svc := &webAppRuntimePermissionService{}
			var appService AgentsService = newWebAppRuntimePermissionAppService(ids)
			if entry == "external-api" {
				appService = newAgentRuntimePermissionAppService(ids)
			}
			h := NewAgentsHandler(appService, nil, nil, nil, nil, svc)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("account_id", ids.accountID.String())
				c.Set("is_authenticated", true)
				if entry == "external-api" {
					setAPIKeyAgentTestContext(c, ids)
				}
			})
			handler := h.ContinueWebAppAgentRuntimeToolGovernance
			if entry == "external-api" {
				handler = h.ContinueAPIKeyAgentRuntimeToolGovernance
			}
			router.POST("/:web_app_id/runtime/conversations/:conversation_id/messages/:message_id/tool-governance/:correlation_id/continue", handler)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/"+ids.webAppID.String()+"/runtime/conversations/"+ids.conversationID.String()+"/messages/"+ids.messageID.String()+"/tool-governance/pending/continue?user=end-user-1", strings.NewReader(`{"action":"reject","source":"console"}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK || !svc.toolGovernanceCalled || svc.lastCaller.Source != entry || svc.lastRunConfig.RuntimeToolFactory != nil {
				t.Fatalf("reject handler: %d %s", w.Code, w.Body)
			}
			if entry == "webapp" {
				assertWebAppRuntimeCallerScope(t, svc.lastScope, svc.lastCaller, ids)
			} else {
				assertExternalAgentRuntimeCallerScope(t, svc.lastScope, svc.lastCaller, ids)
			}
		})
	}
}
