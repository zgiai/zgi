package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	agentspkg "github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/util"
)

func TestAgentRuntimeWorkflowRunsReturnsOnlyNewWebAppMessages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeHistoryHandler(t)

	webAppConversationID := uuid.New()
	consoleConversationID := uuid.New()
	legacyWebAppConversationID := uuid.New()
	webAppID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             webAppConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Title:          "webapp",
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	service.addConversation(&runtimemodel.Conversation{
		ID:             consoleConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Title:          "console",
		Source:         runtimemodel.ConversationSourceConsole,
	})
	service.addConversation(&runtimemodel.Conversation{
		ID:             legacyWebAppConversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &ids.agentID,
		Title:          "legacy webapp",
		Source:         runtimemodel.ConversationSourceWebApp,
	})
	webAppMessageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             webAppMessageID,
		ConversationID: webAppConversationID,
		Query:          "hello",
		Answer:         "world",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
		Metadata: map[string]interface{}{
			"elapsed_time_ms":       123.0,
			"system_prompt_version": "agent.v1",
			"usage":                 map[string]interface{}{"total_tokens": 9.0},
			"skill_step_count":      1.0,
		},
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: consoleConversationID,
		Query:          "console",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})
	service.addMessage(&runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: legacyWebAppConversationID,
		Query:          "legacy",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "agent_id", Value: ids.agentID.String()}}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/workflow-runs?triggered_from=web-app", nil)

	handler.GetWorkflowRuns(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	items := responseDataArray(t, recorder.Body.Bytes())
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1: %#v", len(items), items)
	}
	if got := items[0]["id"]; got != webAppMessageID.String() {
		t.Fatalf("id = %#v, want %s", got, webAppMessageID.String())
	}
	if got := items[0]["triggered_from"]; got != string(CreatedFromWebApp) {
		t.Fatalf("triggered_from = %#v, want %s", got, CreatedFromWebApp)
	}
}

func TestAgentRuntimeWorkflowRunDetailRejectsOtherAgentMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, service, ids := setupRuntimeHistoryHandler(t)

	otherAgentID := uuid.New()
	webAppID := uuid.New()
	conversationID := uuid.New()
	service.addConversation(&runtimemodel.Conversation{
		ID:             conversationID,
		OrganizationID: ids.organizationID,
		WorkspaceID:    &ids.workspaceID,
		AccountID:      ids.accountID,
		CallerType:     runtimemodel.ConversationCallerAgent,
		CallerID:       &otherAgentID,
		Title:          "other",
		Source:         runtimemodel.ConversationSourceWebApp,
		SourceWebAppID: &webAppID,
	})
	messageID := uuid.New()
	service.addMessage(&runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "secret",
		Answer:         "hidden",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "agent_id", Value: ids.agentID.String()},
		{Key: "run_id", Value: messageID.String()},
	}
	ctx.Set("account_id", ids.accountID.String())
	util.SetOrganizationID(ctx, ids.organizationID.String())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/agents/"+ids.agentID.String()+"/workflow-runs/"+messageID.String(), nil)

	handler.GetWorkflowRunDetail(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
}

func TestAgentRuntimeNodeExecutionsFromSkillInvocationsAndFinalAnswer(t *testing.T) {
	messageID := uuid.New()
	createdAt := time.Unix(1700000000, 0)
	updatedAt := createdAt.Add(2 * time.Second)
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: uuid.New(),
		Query:          "calculate",
		Answer:         "4",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
		Metadata: map[string]interface{}{
			"usage": map[string]interface{}{"total_tokens": 12.0},
			"skill_invocations": []interface{}{
				map[string]interface{}{
					"kind":        "tool_call",
					"skill_id":    "calculator",
					"tool_name":   "calculate",
					"status":      "success",
					"duration_ms": 25.0,
					"arguments":   map[string]interface{}{"expression": "2+2"},
					"result":      map[string]interface{}{"value": 4.0},
					"runtime_id":  "tool_call:calculator:calculate#1",
				},
			},
		},
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}

	items := buildRuntimeNodeExecutionResponses(message)

	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	if items[0].NodeType != "tool_call" || items[0].Title != "calculator / calculate" {
		t.Fatalf("first item = %#v, want calculator tool call", items[0])
	}
	if items[0].Status != "succeeded" || items[0].ElapsedTime != 25 {
		t.Fatalf("first item status/elapsed = %s/%v, want succeeded/25", items[0].Status, items[0].ElapsedTime)
	}
	if items[1].NodeType != "answer" || items[1].Outputs == nil {
		t.Fatalf("final item = %#v, want final answer node", items[1])
	}
}

func TestAgentRuntimeNodeExecutionsWithoutTraceKeepsFinalAnswer(t *testing.T) {
	message := &runtimemodel.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Query:          "hello",
		Answer:         "world",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "gpt-test",
		CreatedAt:      time.Unix(1700000000, 0),
		UpdatedAt:      time.Unix(1700000001, 0),
	}

	items := buildRuntimeNodeExecutionResponses(message)

	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	if items[0].NodeType != "answer" {
		t.Fatalf("node_type = %q, want answer", items[0].NodeType)
	}
}

type runtimeHistoryIDs struct {
	organizationID uuid.UUID
	workspaceID    uuid.UUID
	accountID      uuid.UUID
	agentID        uuid.UUID
}

func setupRuntimeHistoryHandler(t *testing.T) (*AgentHistoryDispatchHandler, *fakeRuntimeHistoryService, runtimeHistoryIDs) {
	t.Helper()
	ids := runtimeHistoryIDs{
		organizationID: uuid.New(),
		workspaceID:    uuid.New(),
		accountID:      uuid.New(),
		agentID:        uuid.New(),
	}
	service := newFakeRuntimeHistoryService()
	handler := NewAgentHistoryDispatchHandler(
		&fakeRuntimeAgentRepo{agent: &agentspkg.Agent{
			ID:         ids.agentID,
			TenantID:   ids.workspaceID,
			Name:       "agent",
			AgentsType: "AGENT",
			WebAppID:   uuid.New(),
		}},
		nil,
		nil,
		nil,
		service,
	)
	return handler, service, ids
}

func responseDataArray(t *testing.T, body []byte) []map[string]interface{} {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	payload, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data type = %T, want object: %s", resp["data"], string(body))
	}
	rawItems, ok := payload["data"].([]interface{})
	if !ok {
		t.Fatalf("payload data type = %T, want array: %#v", payload["data"], payload)
	}
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, raw := range rawItems {
		item, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("item type = %T, want object", raw)
		}
		items = append(items, item)
	}
	return items
}

type fakeRuntimeAgentRepo struct {
	agent *agentspkg.Agent
}

func (r *fakeRuntimeAgentRepo) GetByID(ctx context.Context, id string) (*agentspkg.Agent, error) {
	if r.agent != nil && r.agent.ID.String() == id {
		return r.agent, nil
	}
	return nil, fmt.Errorf("agent not found")
}

type fakeRuntimeHistoryService struct {
	conversations map[uuid.UUID]*runtimemodel.Conversation
	messages      []*runtimemodel.Message
}

func newFakeRuntimeHistoryService() *fakeRuntimeHistoryService {
	return &fakeRuntimeHistoryService{
		conversations: map[uuid.UUID]*runtimemodel.Conversation{},
		messages:      []*runtimemodel.Message{},
	}
}

func (s *fakeRuntimeHistoryService) addConversation(conversation *runtimemodel.Conversation) {
	if conversation.Status == "" {
		conversation.Status = runtimemodel.ConversationStatusNormal
	}
	if conversation.RuntimeStatus == "" {
		conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusIdle
	}
	if conversation.Metadata == nil {
		conversation.Metadata = map[string]interface{}{}
	}
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = time.Unix(1700000000, 0)
	}
	if conversation.UpdatedAt.IsZero() {
		conversation.UpdatedAt = conversation.CreatedAt
	}
	s.conversations[conversation.ID] = conversation
}

func (s *fakeRuntimeHistoryService) addMessage(message *runtimemodel.Message) {
	if message.Metadata == nil {
		message.Metadata = map[string]interface{}{}
	}
	if message.ModelParameters == nil {
		message.ModelParameters = map[string]interface{}{}
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Unix(1700000000, 0)
	}
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = message.CreatedAt.Add(time.Second)
	}
	s.messages = append(s.messages, message)
}

func (s *fakeRuntimeHistoryService) ListMessagesByCallerSource(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, source string, page, limit int) ([]*runtimemodel.Message, int64, error) {
	var filtered []*runtimemodel.Message
	for _, message := range s.messages {
		conversation := s.conversations[message.ConversationID]
		if !fakeConversationMatches(scope, caller, conversation) {
			continue
		}
		if source != "" && conversation.Source != source {
			continue
		}
		if source == runtimemodel.ConversationSourceWebApp && conversation.SourceWebAppID == nil {
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered, int64(len(filtered)), nil
}

func (s *fakeRuntimeHistoryService) GetMessageByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) (*runtimemodel.Message, *runtimemodel.Conversation, error) {
	for _, message := range s.messages {
		if message.ID != id {
			continue
		}
		conversation := s.conversations[message.ConversationID]
		if !fakeConversationMatches(scope, caller, conversation) {
			return nil, nil, runtimeservice.ErrNotFound
		}
		return message, conversation, nil
	}
	return nil, nil, runtimeservice.ErrNotFound
}

func fakeConversationMatches(scope runtimeservice.Scope, caller runtimeservice.Caller, conversation *runtimemodel.Conversation) bool {
	if conversation == nil {
		return false
	}
	if conversation.OrganizationID != scope.OrganizationID || conversation.AccountID != scope.AccountID {
		return false
	}
	if conversation.CallerType != caller.Type {
		return false
	}
	if caller.ID == nil {
		return conversation.CallerID == nil
	}
	return conversation.CallerID != nil && *conversation.CallerID == *caller.ID
}

func (s *fakeRuntimeHistoryService) CreateConversation(ctx context.Context, scope runtimeservice.Scope, title string) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CreateConversationForCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, title string) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListConversations(ctx context.Context, scope runtimeservice.Scope, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListConversationsByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetConversation(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetConversationByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, id uuid.UUID) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) UpdateConversation(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID, req runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) DeleteConversation(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListMessages(ctx context.Context, scope runtimeservice.Scope, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListMessagesByCaller(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, page, limit int) ([]*runtimemodel.Message, int64, error) {
	return s.ListMessagesByCallerSource(ctx, scope, caller, "", page, limit)
}

func (s *fakeRuntimeHistoryService) DeleteMessage(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) StopMessage(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) (*runtimemodel.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) StopConversation(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID) (*runtimeservice.StopConversationResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PrepareChat(ctx context.Context, scope runtimeservice.Scope, req runtimedto.ChatRequest) (*runtimeservice.PreparedChat, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PrepareConfiguredChat(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, config runtimeservice.RunConfig, req runtimedto.ChatRequest) (*runtimeservice.PreparedChat, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PrepareRootRegeneration(ctx context.Context, scope runtimeservice.Scope, id uuid.UUID, req runtimedto.RegenerateMessageRequest) (*runtimeservice.PreparedChat, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PrepareConfiguredRootRegeneration(ctx context.Context, scope runtimeservice.Scope, caller runtimeservice.Caller, config runtimeservice.RunConfig, id uuid.UUID, req runtimedto.RegenerateMessageRequest) (*runtimeservice.PreparedChat, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) RunPreparedStream(ctx context.Context, prepared *runtimeservice.PreparedChat, onChunk func(string) error, onEvent ...func(runtimeservice.StreamEvent) error) (*runtimeservice.ChatResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) StreamConversationEvents(ctx context.Context, scope runtimeservice.Scope, conversationID, messageID uuid.UUID, afterID string, onEvent func(runtimeservice.StreamEvent) error) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ListSkills(ctx context.Context, scope runtimeservice.Scope) ([]skills.SkillDiscoveryMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetSkill(ctx context.Context, scope runtimeservice.Scope, skillID string) (*skills.SkillDiscoveryMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetSkillConfig(ctx context.Context, scope runtimeservice.Scope) (*runtimeservice.SkillConfig, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) UpdateSkillConfig(ctx context.Context, scope runtimeservice.Scope, req runtimedto.UpdateSkillConfigRequest) (*runtimeservice.SkillConfig, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) GetAccountSkillPreference(ctx context.Context, scope runtimeservice.Scope, callerType string) (*runtimeservice.AccountSkillPreference, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) UpdateAccountSkillPreference(ctx context.Context, scope runtimeservice.Scope, callerType string, req runtimedto.UpdateAccountSkillPreferenceRequest) (*runtimeservice.AccountSkillPreference, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) PreviewImportCustomSkill(ctx context.Context, scope runtimeservice.Scope, fileHeader *multipart.FileHeader) (*runtimeservice.SkillImportPreview, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) ConfirmCustomSkillImport(ctx context.Context, scope runtimeservice.Scope, importID string, overwriteConfirmed bool) (*skills.SkillDiscoveryMetadata, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CancelCustomSkillImportPreview(ctx context.Context, scope runtimeservice.Scope, importID string) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) DeleteSkill(ctx context.Context, scope runtimeservice.Scope, skillID string) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CleanupStaleActiveMessages(ctx context.Context) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) CleanupExpiredCustomSkillImportPreviews(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

func (s *fakeRuntimeHistoryService) MigrateWebAppConversation(ctx context.Context, scope runtimeservice.Scope, sourceConversationID uuid.UUID) (*runtimemodel.Conversation, error) {
	return nil, fmt.Errorf("not implemented")
}
