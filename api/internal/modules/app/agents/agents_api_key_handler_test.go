package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestExternalAgentInvocationContextPreservesAPISource(t *testing.T) {
	ctx := externalAgentInvocationContext(context.Background())
	if got, _ := ctx.Value("invoke_from").(string); got != "external-api" {
		t.Fatalf("invoke_from = %q, want external-api", got)
	}
}

func TestPublishedAgentConfigRequiresExternalUserOnlyWhenMemoryEnabledWithSlots(t *testing.T) {
	tests := []struct {
		name string
		cfg  dto.AgentConfigResponse
		want bool
	}{
		{
			name: "disabled memory",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: false,
				AgentMemorySlots:   []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
			},
			want: false,
		},
		{
			name: "enabled memory without enabled slots",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: true,
				AgentMemorySlots: []dto.AgentMemorySlotConfig{
					{Key: "profile", Enabled: false},
					{Key: "", Enabled: true},
				},
			},
			want: false,
		},
		{
			name: "enabled memory with enabled slot",
			cfg: dto.AgentConfigResponse{
				AgentMemoryEnabled: true,
				AgentMemorySlots:   []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := publishedAgentConfigRequiresExternalUser(tt.cfg)
			if got != tt.want {
				t.Fatalf("publishedAgentConfigRequiresExternalUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExternalAgentMemoryUserIDStableAcrossAPIKeys(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()

	first := externalAgentMemoryUserID(workspaceID, agentID, " external-user-1 ")
	second := externalAgentMemoryUserID(workspaceID, agentID, "external-user-1")
	if first != second {
		t.Fatalf("externalAgentMemoryUserID() changed for same workspace/agent/user: %s != %s", first, second)
	}

	otherUser := externalAgentMemoryUserID(workspaceID, agentID, "External-User-1")
	if otherUser == first {
		t.Fatalf("externalAgentMemoryUserID() ignored user case, got same id %s", first)
	}

	otherAgent := externalAgentMemoryUserID(workspaceID, uuid.New(), "external-user-1")
	if otherAgent == first {
		t.Fatalf("externalAgentMemoryUserID() ignored agent scope, got same id %s", first)
	}
}

func TestChatAPIKeyAgentStreamsPublishedRuntimeSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &apiKeyChatRuntimeService{ids: ids}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"query":"Analyze the supplied bone-ageing cohort","response_mode":"streaming"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("organization_id", ids.organizationID.String())
	c.Set("workspace_id", ids.workspaceID.String())
	c.Set("account_id", ids.accountID.String())
	c.Set("agent_id", ids.agentID.String())

	handler.ChatAPIKeyAgent(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := w.Header().Get(agentSSEConversationIDHeader); got != ids.conversationID.String() {
		t.Fatalf("conversation header = %q, want %s", got, ids.conversationID)
	}
	if got := w.Header().Get(agentSSEMessageIDHeader); got != ids.messageID.String() {
		t.Fatalf("message header = %q, want %s", got, ids.messageID)
	}
	for _, expected := range []string{`"event":"message_start"`, `"event":"message"`, `"answer":"verified scientific result"`, `"event":"skill_call_end"`, `"event":"message_end"`, `"status":"completed"`} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("SSE body missing %q: %s", expected, w.Body.String())
		}
	}
	assertExternalAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastRunConfig.BillingAppID != ids.agentID.String() || runtimeSvc.lastRunConfig.BillingAppType != runtimemodel.ConversationCallerAgent {
		t.Fatalf("billing app = %q/%q, want %s/%s", runtimeSvc.lastRunConfig.BillingAppID, runtimeSvc.lastRunConfig.BillingAppType, ids.agentID, runtimemodel.ConversationCallerAgent)
	}
}

func TestStopAPIKeyAgentRuntimeConversationUsesExternalCallerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{}
	handler := NewAgentsHandler(nil, nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/conversations/"+ids.conversationID.String()+"/stop", nil)
	c.Params = gin.Params{{Key: "conversation_id", Value: ids.conversationID.String()}}
	c.Set("organization_id", ids.organizationID.String())
	c.Set("workspace_id", ids.workspaceID.String())
	c.Set("account_id", ids.accountID.String())
	c.Set("agent_id", ids.agentID.String())

	handler.StopAPIKeyAgentRuntimeConversation(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !runtimeSvc.getConversationCalled || !runtimeSvc.stopConversationCalled {
		t.Fatalf("get/stop calls = %v/%v, want true/true", runtimeSvc.getConversationCalled, runtimeSvc.stopConversationCalled)
	}
	assertExternalAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
}

func TestStopAPIKeyAgentRuntimeConversationRejectsForeignConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{getConversationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(nil, nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/conversations/"+ids.conversationID.String()+"/stop", nil)
	c.Params = gin.Params{{Key: "conversation_id", Value: ids.conversationID.String()}}
	c.Set("organization_id", ids.organizationID.String())
	c.Set("workspace_id", ids.workspaceID.String())
	c.Set("account_id", ids.accountID.String())
	c.Set("agent_id", ids.agentID.String())

	handler.StopAPIKeyAgentRuntimeConversation(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
	if runtimeSvc.stopConversationCalled {
		t.Fatal("StopConversation must not run after caller-scoped conversation denial")
	}
	assertExternalAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
}

func assertExternalAgentRuntimeCallerScope(t *testing.T, scope runtimeservice.Scope, caller runtimeservice.Caller, ids webAppRuntimePermissionIDs) {
	t.Helper()
	if scope.OrganizationID != ids.organizationID || scope.AccountID != ids.accountID {
		t.Fatalf("scope organization/account = %s/%s, want %s/%s", scope.OrganizationID, scope.AccountID, ids.organizationID, ids.accountID)
	}
	if scope.WorkspaceID == nil || *scope.WorkspaceID != ids.workspaceID || !scope.SkipAccessCheck {
		t.Fatalf("scope workspace/skip = %v/%v, want %s/true", scope.WorkspaceID, scope.SkipAccessCheck, ids.workspaceID)
	}
	if caller.Type != runtimemodel.ConversationCallerAgent || caller.Source != runtimemodel.ConversationSourceExternalAPI {
		t.Fatalf("caller type/source = %q/%q, want %q/%q", caller.Type, caller.Source, runtimemodel.ConversationCallerAgent, runtimemodel.ConversationSourceExternalAPI)
	}
	if caller.ID == nil || *caller.ID != ids.agentID || caller.SourceWebAppID != nil {
		t.Fatalf("caller agent/webapp = %v/%v, want %s/nil", caller.ID, caller.SourceWebAppID, ids.agentID)
	}
}

type apiKeyChatRuntimeService struct {
	runtimeservice.Service
	ids           webAppRuntimePermissionIDs
	lastScope     runtimeservice.Scope
	lastCaller    runtimeservice.Caller
	lastRunConfig runtimeservice.RunConfig
}

func (s *apiKeyChatRuntimeService) PrepareConfiguredChat(_ context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, config runtimeservice.RunConfig, _ runtimedto.ChatRequest) (*runtimeservice.PreparedChat, error) {
	s.lastScope = scope
	s.lastCaller = caller
	s.lastRunConfig = config
	return &runtimeservice.PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: s.ids.conversationID},
		Message:      &runtimemodel.Message{ID: s.ids.messageID, ConversationID: s.ids.conversationID, ModelName: "gpt-5.6-sol"},
		Scope:        scope,
		Caller:       caller,
		RunConfig:    config,
	}, nil
}

func (s *apiKeyChatRuntimeService) RunPreparedStream(_ context.Context, _ *runtimeservice.PreparedChat, onChunk func(string) error, onEvents ...func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	if err := onChunk("verified scientific result"); err != nil {
		return nil, err
	}
	if len(onEvents) > 0 && onEvents[0] != nil {
		if err := onEvents[0](runtimeservice.StreamEvent{ID: "event-1", EventType: "skill_call_end", Payload: map[string]interface{}{"method": "ordinary_least_squares"}}); err != nil {
			return nil, err
		}
	}
	return &runtimeservice.ChatResult{Status: runtimemodel.MessageStatusCompleted, Metadata: map[string]interface{}{"usage": map[string]interface{}{"total_tokens": 42}}}, nil
}
