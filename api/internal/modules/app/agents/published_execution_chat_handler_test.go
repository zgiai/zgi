package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type publishedWebAppChatRuntime struct {
	apiKeyChatRuntimeService
	prepared *runtimeservice.PreparedChat
}

func (s *publishedWebAppChatRuntime) PrepareConfiguredChat(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, config runtimeservice.RunConfig, request runtimedto.ChatRequest) (*runtimeservice.PreparedChat, error) {
	p, err := s.apiKeyChatRuntimeService.PrepareConfiguredChat(ctx, scope, caller, config, request)
	if err != nil {
		return nil, err
	}
	p.Conversation.OrganizationID, p.Conversation.AccountID, p.Conversation.WorkspaceID = scope.OrganizationID, scope.AccountID, scope.WorkspaceID
	p.Conversation.CallerType, p.Conversation.CallerID = caller.Type, caller.ID
	p.Conversation.Source, p.Conversation.SourceWebAppID = caller.Source, caller.SourceWebAppID
	s.prepared = p
	return p, nil
}

func TestPublishedWebAppInitialChatExecutionTools(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := tool_file.GlobalToolFileManager
	tool_file.GlobalToolFileManager = &tool_file.ToolFileManager{}
	t.Cleanup(func() { tool_file.GlobalToolFileManager = previous })
	t.Setenv("ZGI_AGENT_RUNTIME_PLATFORM_CLI_ENABLED", "false")
	for _, tc := range []struct {
		name          string
		authenticated bool
		applicationOn bool
		serverOn      bool
		entryAllowed  bool
		accessAllowed bool
		wantStatus    int
		wantTools     bool
	}{
		{"enabled", true, true, true, true, true, http.StatusOK, true},
		{"application disabled", true, false, true, true, true, http.StatusOK, false},
		{"server disabled", true, true, false, true, true, http.StatusOK, false},
		{"anonymous", false, true, true, true, true, http.StatusOK, false},
		{"entry denied", true, true, true, false, true, http.StatusBadRequest, false},
		{"application access denied", true, true, true, true, false, http.StatusForbidden, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, published := publishedExecutionFixture("webapp")
			published.Config.ExecutionRuntime.Enabled = tc.applicationOn
			allowlist := `{}`
			if tc.entryAllowed {
				allowlist = `{"agent:webapp":["` + published.OrganizationID + `"]}`
			}
			t.Setenv("ZGI_AGENT_RUNTIME_ENTRY_ALLOWLIST_JSON", allowlist)
			service := &publishedExecutionTestService{stubWebAppStatusHandlerService: &stubWebAppStatusHandlerService{
				publishedConfigResp: published, runtimeCapabilityResp: &dto.AgentWebAppRuntimeCapabilityResponse{Allowed: tc.accessAllowed, Reason: agentWebAppCapabilityReasonNoAccess},
			}}
			runtime := &publishedWebAppChatRuntime{apiKeyChatRuntimeService: apiKeyChatRuntimeService{ids: webAppRuntimePermissionIDs{
				conversationID: p.Conversation.ID, messageID: p.Message.ID,
			}}}
			gateway := &publishedExecutionTestGateway{}
			handler := NewAgentsHandler(service, nil, nil, nil, nil, runtime)
			handler.executionGateway = gateway
			handler.fileService = &executionUploadTestFiles{}
			handler.executionApprovals = agentExecutionApprovals{Enabled: tc.serverOn, GatewayReady: true, Profiles: map[string]agentExecutionApproval{
				"standard": {Backend: "local-gvisor", OrganizationIDs: []string{p.Scope.OrganizationID.String()}, PolicyIDs: []string{"offline"}},
			}}
			router := gin.New()
			router.POST("/webapps/:web_app_id/chat", func(c *gin.Context) {
				c.Set("account_id", p.Scope.AccountID.String())
				c.Set("is_authenticated", tc.authenticated)
				handler.ChatWebAppAgent(c)
			})
			request := httptest.NewRequest(http.MethodPost, "/webapps/"+published.WebAppID+"/chat", strings.NewReader(`{"query":"run a script","response_mode":"streaming","is_authenticated":true}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d; body=%s", response.Code, tc.wantStatus, response.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				if runtime.prepared != nil || gateway.creates != 0 {
					t.Fatal("denied initial chat reached runtime or Worker")
				}
				return
			}
			if runtime.prepared == nil || !strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
				t.Fatalf("ordinary chat did not prepare/stream: body=%s", response.Body.String())
			}
			factory := runtime.lastRunConfig.RuntimeToolFactory
			if (factory != nil) != tc.wantTools {
				t.Fatalf("initial chat execution factory present=%t, want %t", factory != nil, tc.wantTools)
			}
			if !tc.wantTools {
				return
			}
			tools := factory(runtime.prepared)
			if len(tools) != 2 {
				t.Fatalf("initial chat has %d runtime tools, want Shell and export", len(tools))
			}
			tools[0].Handler(t.Context(), adapter.ToolCall{ID: "initial-shell", Function: adapter.FunctionCall{Arguments: `{"command":"printf ok"}`}})
			owner := gateway.scope
			if gateway.creates != 1 || owner.SubjectID != p.Scope.AccountID.String() || owner.Identity == nil ||
				owner.Identity.Entry != "webapp" || owner.Identity.ApplicationID != published.AgentID || owner.Identity.PublicationRef != published.VersionUUID || owner.Identity.ConversationID != p.Conversation.ID.String() {
				t.Fatalf("initial Shell lost published final-user scope: creates=%d scope=%+v", gateway.creates, owner)
			}
			if runtime.lastRunConfig.RuntimeInputPolicy != runtimeservice.RuntimeInputPolicyCheckpointAndFollowUp {
				t.Fatal("initial chat changed runtime input policy")
			}
			if runtime.lastCaller.ID == nil || *runtime.lastCaller.ID == uuid.Nil || !executionruntime.InteractiveEntryImplemented("agent", runtime.lastCaller.Source) {
				t.Fatal("initial chat lost verified caller")
			}
		})
	}
}
