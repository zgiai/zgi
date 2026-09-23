package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
	"github.com/zgiai/zgi/api/pkg/response"
)

func TestGetAPIKeyAgentRuntimeConfigReturnsPublishedLandingPageFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		webAppID:       uuid.New(),
	}
	appService := newAgentRuntimePermissionAppService(ids)
	appService.published = &dto.AgentWebAppRuntimeConfigResponse{
		AgentID:     ids.agentID.String(),
		WebAppID:    ids.webAppID.String(),
		AgentType:   "AGENT",
		Name:        "Research Agent",
		Description: "Finds evidence",
		Version:     "v7",
		VersionUUID: uuid.NewString(),
		Config: dto.AgentConfigResponse{
			HomeTitle:          "Ask the research agent",
			OpeningStatement:   "What should we investigate?",
			InputPlaceholder:   "Enter a research question",
			ThemeColor:         "#155EEF",
			SuggestedQuestions: []string{"Summarize the evidence"},
			FileUpload:         true,
			AgentMemoryEnabled: true,
			AgentMemorySlots:   []dto.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
		},
	}
	handler := NewAgentsHandler(appService, nil, nil, nil, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/agents/config", nil)
	setAPIKeyAgentTestContext(c, ids)
	handler.GetAPIKeyAgentRuntimeConfig(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var body struct {
		Data apiKeyAgentRuntimeConfigResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode config response: %v", err)
	}
	if body.Data.Name != "Research Agent" || body.Data.HomeTitle != "Ask the research agent" || !body.Data.FileUploadEnabled || !body.Data.MemoryRequiresUser {
		t.Fatalf("published config response = %#v", body.Data)
	}
}

func TestContinueAPIKeyAgentRuntimeUserInputUsesPublishedExternalScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		conversationID: uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/conversations/"+ids.conversationID.String()+"/messages/"+ids.messageID.String()+"/user-input/ask-1/continue?user=end-user-1", bytes.NewBufferString(`{"answers":{"target":"Published Agent"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "conversation_id", Value: ids.conversationID.String()},
		{Key: "message_id", Value: ids.messageID.String()},
		{Key: "request_id", Value: "ask-1"},
	}
	setAPIKeyAgentTestContext(c, ids)
	handler.ContinueAPIKeyAgentRuntimeUserInput(c)

	if !runtimeSvc.userInputContinuationCalled {
		t.Fatal("RunConfiguredUserInputContinuationStream was not called")
	}
	assertExternalAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastRunConfig.SystemPromptVersion != "agent.published.v1" {
		t.Fatalf("SystemPromptVersion = %q, want agent.published.v1", runtimeSvc.lastRunConfig.SystemPromptVersion)
	}
	if runtimeSvc.lastUserInputRequest.Answers["target"] != "Published Agent" {
		t.Fatalf("answers = %#v", runtimeSvc.lastUserInputRequest.Answers)
	}
	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", w.Header().Get("Content-Type"))
	}
}

func TestContinueAPIKeyAgentRuntimeUserInputSanitizesUnexpectedStreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(), workspaceID: uuid.New(), accountID: uuid.New(), agentID: uuid.New(),
		conversationID: uuid.New(), messageID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{userInputContinuationErr: errors.New("redis password leaked by internal error")}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/conversations/"+ids.conversationID.String()+"/messages/"+ids.messageID.String()+"/user-input/ask-1/continue?user=end-user-1", bytes.NewBufferString(`{"answers":{"target":"Published Agent"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "conversation_id", Value: ids.conversationID.String()},
		{Key: "message_id", Value: ids.messageID.String()},
		{Key: "request_id", Value: "ask-1"},
	}
	setAPIKeyAgentTestContext(c, ids)
	handler.ContinueAPIKeyAgentRuntimeUserInput(c)

	body := w.Body.String()
	if strings.Contains(body, "redis password") || !strings.Contains(body, response.ErrSystemError.Message) {
		t.Fatalf("continuation stream error was not sanitized: %s", body)
	}
	if !strings.Contains(body, `"code":399001`) {
		t.Fatalf("continuation stream error has no stable public code: %s", body)
	}
}

func TestStreamAPIKeyAgentRuntimeEventsUsesCallerScopeAndSanitizesErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(), workspaceID: uuid.New(), accountID: uuid.New(), agentID: uuid.New(),
		conversationID: uuid.New(), messageID: uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{streamForCallerErr: errors.New("redis password leaked by internal error")}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/agents/conversations/"+ids.conversationID.String()+"/events?message_id="+ids.messageID.String(), nil)
	c.Params = gin.Params{{Key: "conversation_id", Value: ids.conversationID.String()}}
	setAPIKeyAgentTestContext(c, ids)
	handler.StreamAPIKeyAgentRuntimeEvents(c)

	if !runtimeSvc.streamForCallerCalled || runtimeSvc.streamCalled {
		t.Fatalf("caller-scoped/legacy stream calls = %v/%v", runtimeSvc.streamForCallerCalled, runtimeSvc.streamCalled)
	}
	assertExternalAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if strings.Contains(w.Body.String(), "redis password") || !strings.Contains(w.Body.String(), response.ErrSystemError.Message) {
		t.Fatalf("event stream error was not sanitized: %s", w.Body.String())
	}
}

func TestExternalAgentSSEOmitsSidebarOnlyClientCollaborationEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(externalAgentAPIContextKey, true)

	for _, event := range []string{"client_action_required", "client_action_result"} {
		if err := writeAgentSSEEvent(c, "", event, gin.H{"action_id": "sidebar-action"}); err != nil {
			t.Fatalf("write %s: %v", event, err)
		}
	}
	if err := writeAgentSSEEvent(c, "", "tool_governance_decision", gin.H{"requires_approval": false}); err != nil {
		t.Fatalf("write public governance event: %v", err)
	}

	if strings.Contains(w.Body.String(), "client_action") {
		t.Fatalf("external Agent stream exposed sidebar-only event: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "event: tool_governance_decision") {
		t.Fatalf("external Agent stream omitted public governance event: %s", w.Body.String())
	}
}

func TestChatAPIKeyAgentSanitizesUnexpectedPublishedConfigError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	appService := newAgentRuntimePermissionAppService(ids)
	appService.publishedErr = errors.New("database password leaked by internal error")
	handler := NewAgentsHandler(appService, nil, nil, nil, nil, &apiKeyChatRuntimeService{ids: ids})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"query":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	setAPIKeyAgentTestContext(c, ids)
	handler.ChatAPIKeyAgent(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "database password") {
		t.Fatalf("response leaked internal error: %s", w.Body.String())
	}
}

func TestRegenerateAPIKeyAgentRuntimeMessageAllowsEmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
		messageID:      uuid.New(),
	}
	runtimeSvc := &webAppRuntimePermissionService{prepareRegenerationErr: runtimeservice.ErrNotFound}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/messages/"+ids.messageID.String()+"/regenerate", nil)
	c.Params = gin.Params{{Key: "message_id", Value: ids.messageID.String()}}
	setAPIKeyAgentTestContext(c, ids)
	handler.RegenerateAPIKeyAgentRuntimeMessage(c)

	if !runtimeSvc.prepareRegenerationCalled {
		t.Fatal("PrepareConfiguredRootRegeneration was not called for an empty request body")
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func setAPIKeyAgentTestContext(c *gin.Context, ids webAppRuntimePermissionIDs) {
	c.Set("organization_id", ids.organizationID.String())
	c.Set("workspace_id", ids.workspaceID.String())
	c.Set("account_id", ids.accountID.String())
	c.Set("agent_id", ids.agentID.String())
	c.Request.Header.Set(ExternalUserIDHeader, "end-user-1")
}

func TestExternalAgentInvocationContextPreservesAPISource(t *testing.T) {
	ctx := externalAgentInvocationContext(context.Background())
	if got, _ := ctx.Value("invoke_from").(string); got != "external-api" {
		t.Fatalf("invoke_from = %q, want external-api", got)
	}
}

func TestPublishedAgentConfigRequiresExternalUserWheneverMemoryIsEnabled(t *testing.T) {
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
			want: true,
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

func TestDeriveExternalUserIDStableAcrossAPIKeys(t *testing.T) {
	workspaceID := uuid.New()
	agentID := uuid.New()

	first := DeriveExternalUserID(workspaceID, agentID, " external-user-1 ")
	second := DeriveExternalUserID(workspaceID, agentID, "external-user-1")
	if first != second {
		t.Fatalf("DeriveExternalUserID() changed for same workspace/agent/user: %s != %s", first, second)
	}

	otherUser := DeriveExternalUserID(workspaceID, agentID, "External-User-1")
	if otherUser == first {
		t.Fatalf("DeriveExternalUserID() ignored user case, got same id %s", first)
	}

	otherAgent := DeriveExternalUserID(workspaceID, uuid.New(), "external-user-1")
	if otherAgent == first {
		t.Fatalf("DeriveExternalUserID() ignored agent scope, got same id %s", first)
	}
}

func TestScopeWithAgentResourceWorkspaceKeepsRuntimeDataWorkspaceSeparate(t *testing.T) {
	runtimeDataWorkspaceID := uuid.New()
	agentResourceWorkspaceID := uuid.New()
	scope, err := scopeWithAgentResourceWorkspace(runtimeservice.Scope{WorkspaceID: &runtimeDataWorkspaceID}, agentResourceWorkspaceID.String())
	if err != nil {
		t.Fatal(err)
	}
	if scope.WorkspaceID == nil || *scope.WorkspaceID != runtimeDataWorkspaceID {
		t.Fatalf("runtime data workspace = %v, want %s", scope.WorkspaceID, runtimeDataWorkspaceID)
	}
	if scope.AgentResourceWorkspaceID() == nil || *scope.AgentResourceWorkspaceID() != agentResourceWorkspaceID {
		t.Fatalf("agent resource workspace = %v, want %s", scope.AgentResourceWorkspaceID(), agentResourceWorkspaceID)
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
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"query":"Analyze the supplied bone-ageing cohort","response_mode":"streaming","max_task_tokens":42000}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("organization_id", ids.organizationID.String())
	c.Set("workspace_id", ids.workspaceID.String())
	c.Set("account_id", ids.accountID.String())
	c.Set("agent_id", ids.agentID.String())
	c.Request.Header.Set(ExternalUserIDHeader, "end-user-1")

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
	for _, expected := range []string{`"event":"message_start"`, `"replace":false`, `"event":"message"`, `"answer":"verified scientific result"`, `"event":"skill_call_end"`, `"event":"token_usage_updated"`, `"used_tokens":42`, `"event":"message_end"`, `"status":"completed"`} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("SSE body missing %q: %s", expected, w.Body.String())
		}
	}
	assertExternalAgentRuntimeCallerScope(t, runtimeSvc.lastScope, runtimeSvc.lastCaller, ids)
	if runtimeSvc.lastRunConfig.BillingAppID != ids.agentID.String() || runtimeSvc.lastRunConfig.BillingAppType != runtimemodel.ConversationCallerAgent {
		t.Fatalf("billing app = %q/%q, want %s/%s", runtimeSvc.lastRunConfig.BillingAppID, runtimeSvc.lastRunConfig.BillingAppType, ids.agentID, runtimemodel.ConversationCallerAgent)
	}
	if runtimeSvc.lastRequest.MaxTaskTokens != 42000 {
		t.Fatalf("max task tokens = %d, want 42000", runtimeSvc.lastRequest.MaxTaskTokens)
	}
}

func TestChatAPIKeyAgentWritesRecoverableWaitingMessageEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		status   string
		metadata map[string]interface{}
	}{
		{
			name:   "workflow approval",
			status: runtimemodel.MessageStatusWaitingApproval,
			metadata: map[string]interface{}{
				"agent_workflow_continuation": map[string]interface{}{"status": "waiting_approval"},
			},
		},
		{
			name:   "skill user input",
			status: runtimemodel.MessageStatusWaitingQuestion,
			metadata: map[string]interface{}{
				"user_input_request": map[string]interface{}{"request_id": "ask-1"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ids := webAppRuntimePermissionIDs{
				organizationID: uuid.New(), workspaceID: uuid.New(), accountID: uuid.New(), agentID: uuid.New(),
				conversationID: uuid.New(), messageID: uuid.New(),
			}
			runtimeSvc := &apiKeyChatRuntimeService{
				ids: ids,
				result: &runtimeservice.ChatResult{
					Status:            test.status,
					Metadata:          test.metadata,
					MessageEndEventID: "event-message-end-1",
				},
			}
			handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"query":"continue the workflow"}`))
			c.Request.Header.Set("Content-Type", "application/json")
			setAPIKeyAgentTestContext(c, ids)
			handler.ChatAPIKeyAgent(c)

			body := w.Body.String()
			if count := strings.Count(body, `"event":"message_end"`); count != 1 {
				t.Fatalf("message_end count = %d, want 1; body=%s", count, body)
			}
			if !strings.Contains(body, `id: event-message-end-1`) || !strings.Contains(body, `"status":"`+test.status+`"`) {
				t.Fatalf("waiting message_end lost status or event id: %s", body)
			}
			if strings.Contains(body, `"status":"completed"`) {
				t.Fatalf("waiting stream was marked completed: %s", body)
			}
		})
	}
}

func TestChatAPIKeyAgentSanitizesUnexpectedStreamError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ids := webAppRuntimePermissionIDs{
		organizationID: uuid.New(), workspaceID: uuid.New(), accountID: uuid.New(), agentID: uuid.New(),
		conversationID: uuid.New(), messageID: uuid.New(),
	}
	runtimeSvc := &apiKeyChatRuntimeService{ids: ids, runErr: errors.New("database password leaked by internal error")}
	handler := NewAgentsHandler(newAgentRuntimePermissionAppService(ids), nil, nil, nil, nil, runtimeSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/agents/chat", strings.NewReader(`{"query":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	setAPIKeyAgentTestContext(c, ids)
	handler.ChatAPIKeyAgent(c)

	body := w.Body.String()
	if strings.Contains(body, "database password") || !strings.Contains(body, response.ErrSystemError.Message) {
		t.Fatalf("chat stream error was not sanitized: %s", body)
	}
	if strings.Count(body, `"event":"error"`) != 1 || strings.Count(body, `"event":"message_end"`) != 1 {
		t.Fatalf("chat stream error has inconsistent terminal events: %s", body)
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
	c.Request.Header.Set(ExternalUserIDHeader, "end-user-1")

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
	c.Request.Header.Set(ExternalUserIDHeader, "end-user-1")

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
	externalUserID := DeriveExternalUserID(ids.workspaceID, ids.agentID, "end-user-1")
	if scope.ExternalUserID == nil || *scope.ExternalUserID != externalUserID {
		t.Fatalf("scope external user = %v, want %s", scope.ExternalUserID, externalUserID)
	}
	if scope.AgentMemoryUserID == nil || *scope.AgentMemoryUserID != externalUserID {
		t.Fatalf("scope memory user = %v, want %s", scope.AgentMemoryUserID, externalUserID)
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
	lastRequest   runtimedto.ChatRequest
	result        *runtimeservice.ChatResult
	runErr        error
}

func (s *apiKeyChatRuntimeService) PrepareConfiguredChat(_ context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, config runtimeservice.RunConfig, request runtimedto.ChatRequest) (*runtimeservice.PreparedChat, error) {
	s.lastScope = scope
	s.lastCaller = caller
	s.lastRunConfig = config
	s.lastRequest = request
	return &runtimeservice.PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: s.ids.conversationID},
		Message:      &runtimemodel.Message{ID: s.ids.messageID, ConversationID: s.ids.conversationID, ModelName: "gpt-5.6-sol"},
		Scope:        scope,
		Caller:       caller,
		RunConfig:    config,
	}, nil
}

func (s *apiKeyChatRuntimeService) RunPreparedStream(_ context.Context, _ *runtimeservice.PreparedChat, onChunk func(string) error, onEvents ...func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	if s.runErr != nil {
		return nil, s.runErr
	}
	if err := onChunk("verified scientific result"); err != nil {
		return nil, err
	}
	if len(onEvents) > 0 && onEvents[0] != nil {
		if err := onEvents[0](runtimeservice.StreamEvent{ID: "event-1", EventType: "skill_call_end", Payload: map[string]interface{}{"method": "ordinary_least_squares"}}); err != nil {
			return nil, err
		}
		if err := onEvents[0](runtimeservice.StreamEvent{ID: "event-2", EventType: "token_usage_updated", Payload: map[string]interface{}{
			"usage":             map[string]interface{}{"prompt_tokens": 30, "completion_tokens": 12, "total_tokens": 42},
			"task_token_budget": map[string]interface{}{"used_tokens": 42, "max_tokens": 42000, "remaining_tokens": 41958, "status": "running"},
		}}); err != nil {
			return nil, err
		}
	}
	if s.result != nil {
		return s.result, nil
	}
	return &runtimeservice.ChatResult{Status: runtimemodel.MessageStatusCompleted, Metadata: map[string]interface{}{"usage": map[string]interface{}{"total_tokens": 42}}}, nil
}

func TestRuntimeWorkspaceMemoryFailsClosedWithoutRelationshipMemory(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_ENABLED", "")
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/connect/v1/apps/app-key/memory", nil)
	c.Set("runtime_workspace_principal", &runtimeworkspace.Principal{})
	NewAgentsHandler(nil, nil, nil, nil, nil).GetAPIKeyAgentMemory(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestRuntimeWorkspaceRunConfigKeepsPublishedCapabilities(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_ENABLED", "")
	config := runtimeWorkspaceRunConfig(runtimeservice.RunConfig{
		EnabledSkillIDs:                  []string{"calculator", "agent-knowledge", "agent-database", "agent-workflow", "connector-runtime"},
		KnowledgeDatasetIDs:              []string{"dataset-1"},
		KnowledgeRetrievalConfig:         map[string]interface{}{"mode": "hybrid"},
		DatabaseBindings:                 []runtimeservice.AgentDatabaseBinding{{DataSourceID: "database-1", TableIDs: []string{"table-1"}}},
		WorkflowBindings:                 []runtimeservice.AgentWorkflowBinding{{BindingID: "binding-1", AgentID: "agent-1", WorkflowID: "workflow-1"}},
		BindingAuthorizations:            []runtimeservice.ResourceBindingAuthorization{{BindingType: "workflow", ResourceID: "workflow-1"}},
		ConnectorTargetScope:             "agent",
		ConnectorTargetVersionID:         "version-1",
		AgentMemoryEnabled:               true,
		AgentMemoryAutoExtractionEnabled: true,
		AgentMemorySlots:                 []runtimeservice.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
	})
	if strings.Join(config.EnabledSkillIDs, ",") != "calculator,agent-knowledge,agent-database,agent-workflow,connector-runtime" {
		t.Fatalf("published skills were changed: %#v", config.EnabledSkillIDs)
	}
	if len(config.KnowledgeDatasetIDs) != 1 || len(config.DatabaseBindings) != 1 || len(config.WorkflowBindings) != 1 || len(config.BindingAuthorizations) != 1 {
		t.Fatalf("published bindings were changed: %#v", config)
	}
	if config.ConnectorTargetScope != "agent" || config.ConnectorTargetVersionID != "version-1" {
		t.Fatalf("published connector target was changed: %#v", config)
	}
	if !config.ConnectorToolsDisabled {
		t.Fatalf("Runtime Workspace Connector projection remained enabled: %#v", config)
	}
	if config.AgentMemoryEnabled || config.AgentMemoryAutoExtractionEnabled || len(config.AgentMemorySlots) != 0 {
		t.Fatalf("memory remained enabled while relationship memory was disabled: %#v", config)
	}
}

func TestRuntimeWorkspaceRunConfigKeepsRelationshipMemory(t *testing.T) {
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_ENABLED", "true")
	config := runtimeWorkspaceRunConfig(runtimeservice.RunConfig{
		AgentMemoryEnabled:               true,
		AgentMemoryAutoExtractionEnabled: true,
		AgentMemorySlots:                 []runtimeservice.AgentMemorySlotConfig{{Key: "profile", Enabled: true}},
	})
	if !config.AgentMemoryEnabled || !config.AgentMemoryAutoExtractionEnabled || len(config.AgentMemorySlots) != 1 {
		t.Fatalf("relationship memory config = %#v, want enabled memory", config)
	}
}
