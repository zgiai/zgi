package workflow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/pkg/database"
)

type fakeWebAppConversationAccessService struct {
	conversation *conversation.AgentConversation
	err          error

	gotConversationID uuid.UUID
	gotAgentID        uuid.UUID
}

func (s *fakeWebAppConversationAccessService) GetConversationByIDAndAgent(ctx context.Context, conversationID, agentID uuid.UUID) (*conversation.AgentConversation, error) {
	s.gotConversationID = conversationID
	s.gotAgentID = agentID
	if s.err != nil {
		return nil, s.err
	}
	return s.conversation, nil
}

func TestValidateWebAppConversationAccess_AllowsAccountOwnedConversation(t *testing.T) {
	conversationID := uuid.New()
	agentID := uuid.New()
	accountID := uuid.New()
	service := &fakeWebAppConversationAccessService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &accountID,
		},
	}

	err := validateWebAppConversationAccess(context.Background(), service, conversationID.String(), agentID.String(), accountID.String())
	if err != nil {
		t.Fatalf("validateWebAppConversationAccess error = %v, want nil", err)
	}
	if service.gotConversationID != conversationID {
		t.Fatalf("conversation id lookup = %s, want %s", service.gotConversationID, conversationID)
	}
	if service.gotAgentID != agentID {
		t.Fatalf("agent id lookup = %s, want %s", service.gotAgentID, agentID)
	}
}

func TestValidateWebAppConversationAccess_AllowsEndUserOwnedConversation(t *testing.T) {
	conversationID := uuid.New()
	agentID := uuid.New()
	accountID := uuid.New()
	service := &fakeWebAppConversationAccessService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromEndUserID: &accountID,
		},
	}

	err := validateWebAppConversationAccess(context.Background(), service, conversationID.String(), agentID.String(), accountID.String())
	if err != nil {
		t.Fatalf("validateWebAppConversationAccess error = %v, want nil", err)
	}
}

func TestValidateWebAppConversationAccess_RejectsOtherAccountConversation(t *testing.T) {
	conversationID := uuid.New()
	agentID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	service := &fakeWebAppConversationAccessService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &ownerID,
		},
	}

	err := validateWebAppConversationAccess(context.Background(), service, conversationID.String(), agentID.String(), callerID.String())
	if !errors.Is(err, errWebAppConversationAccessDenied) {
		t.Fatalf("validateWebAppConversationAccess error = %v, want %v", err, errWebAppConversationAccessDenied)
	}
}

func TestValidateWebAppConversationAccess_RejectsOtherAgentConversation(t *testing.T) {
	conversationID := uuid.New()
	agentID := uuid.New()
	accountID := uuid.New()
	service := &fakeWebAppConversationAccessService{err: errors.New("record not found")}

	err := validateWebAppConversationAccess(context.Background(), service, conversationID.String(), agentID.String(), accountID.String())
	if !errors.Is(err, errWebAppConversationNotFound) {
		t.Fatalf("validateWebAppConversationAccess error = %v, want %v", err, errWebAppConversationNotFound)
	}
}

func TestValidateWebAppConversationAccess_RejectsInvalidIDs(t *testing.T) {
	err := validateWebAppConversationAccess(context.Background(), &fakeWebAppConversationAccessService{}, "not-a-uuid", uuid.NewString(), uuid.NewString())
	if !errors.Is(err, errWebAppConversationInvalidID) {
		t.Fatalf("invalid conversation id error = %v, want %v", err, errWebAppConversationInvalidID)
	}

	err = validateWebAppConversationAccess(context.Background(), &fakeWebAppConversationAccessService{}, uuid.NewString(), "not-a-uuid", uuid.NewString())
	if !errors.Is(err, errWebAppConversationInvalidAgent) {
		t.Fatalf("invalid agent id error = %v, want %v", err, errWebAppConversationInvalidAgent)
	}

	err = validateWebAppConversationAccess(context.Background(), &fakeWebAppConversationAccessService{}, uuid.NewString(), uuid.NewString(), "not-a-uuid")
	if !errors.Is(err, errWebAppConversationInvalidAccount) {
		t.Fatalf("invalid account id error = %v, want %v", err, errWebAppConversationInvalidAccount)
	}
}

func TestPromoteWorkflowInputConversationIDToSystemInputPromotesLegacyConversationID(t *testing.T) {
	conversationID := uuid.NewString()
	inputs := map[string]interface{}{
		"conversation_id": " " + conversationID + " ",
	}

	promoteWorkflowInputConversationIDToSystemInput(inputs)

	if got := inputs["sys.conversation_id"]; got != conversationID {
		t.Fatalf("sys.conversation_id = %#v, want %q", got, conversationID)
	}
}

func TestPromoteWorkflowInputConversationIDToSystemInputPreservesSystemConversationID(t *testing.T) {
	inputs := map[string]interface{}{
		"conversation_id":     "legacy-conversation-id",
		"sys.conversation_id": "system-conversation-id",
	}

	promoteWorkflowInputConversationIDToSystemInput(inputs)

	if got := inputs["sys.conversation_id"]; got != "system-conversation-id" {
		t.Fatalf("sys.conversation_id = %#v, want %q", got, "system-conversation-id")
	}
}

func TestPromoteWorkflowInputConversationIDToSystemInputIgnoresEmptyLegacyConversationID(t *testing.T) {
	inputs := map[string]interface{}{
		"conversation_id": " ",
	}

	promoteWorkflowInputConversationIDToSystemInput(inputs)

	if _, exists := inputs["sys.conversation_id"]; exists {
		t.Fatalf("sys.conversation_id should not be set for empty legacy input: %#v", inputs)
	}
}

type fakeAdvancedChatConversationService struct {
	conversation.AgentConversationService

	conversation *conversation.AgentConversation
	err          error
	called       bool
	deleteCalled bool
}

func (s *fakeAdvancedChatConversationService) GetConversationByIDAndAgent(ctx context.Context, conversationID, agentID uuid.UUID) (*conversation.AgentConversation, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	return s.conversation, nil
}

func (s *fakeAdvancedChatConversationService) DeleteConversation(ctx context.Context, id uuid.UUID, deletedBy uuid.UUID) error {
	s.deleteCalled = true
	return nil
}

type publishedWorkflowRepository struct {
	*mockWorkflowRepository

	published *Workflow
}

func (r *publishedWorkflowRepository) GetLatestPublishedWorkflow(ctx context.Context, agentID string) (*Workflow, error) {
	return r.published, nil
}

func testChatWorkflow(agentID string) *Workflow {
	return &Workflow{
		ID:                    uuid.NewString(),
		TenantID:              uuid.NewString(),
		AppID:                 agentID,
		AgentID:               agentID,
		Type:                  dto.WorkflowTypeChat,
		Version:               "draft",
		Graph:                 `{}`,
		EnvironmentVariables:  `[]`,
		ConversationVariables: `[]`,
		CreatedBy:             uuid.NewString(),
	}
}

func TestRunAdvancedChatDraftWorkflow_RejectsForeignConversationBeforeRunning(t *testing.T) {
	agentID := uuid.New()
	conversationID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	conversationService := &fakeAdvancedChatConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &ownerID,
		},
	}
	service := &WorkflowService{
		advancedChatHandler: &AdvancedChatWorkflowHandler{conversationService: conversationService},
	}

	_, err := service.RunAdvancedChatDraftWorkflow(context.Background(), uuid.NewString(), agentID.String(), &dto.AdvancedChatDraftWorkflowRunRequest{
		Query:          "hello",
		Inputs:         map[string]interface{}{},
		ConversationID: conversationID.String(),
	}, callerID.String())

	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
	if err == nil || !strings.Contains(err.Error(), "conversation not found") {
		t.Fatalf("RunAdvancedChatDraftWorkflow error = %v, want conversation not found", err)
	}
}

func TestRunAdvancedChatWorkflow_RejectsForeignConversationBeforeRunning(t *testing.T) {
	agentID := uuid.New()
	conversationID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	conversationService := &fakeAdvancedChatConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromEndUserID: &ownerID,
		},
	}
	service := &WorkflowService{
		advancedChatHandler: &AdvancedChatWorkflowHandler{conversationService: conversationService},
	}

	_, err := service.RunAdvancedChatWorkflow(context.Background(), uuid.NewString(), agentID.String(), &dto.AdvancedChatDraftWorkflowRunRequest{
		Query:          "hello",
		Inputs:         map[string]interface{}{},
		ConversationID: conversationID.String(),
	}, callerID.String())

	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
	if err == nil || !strings.Contains(err.Error(), "conversation not found") {
		t.Fatalf("RunAdvancedChatWorkflow error = %v, want conversation not found", err)
	}
}

func TestRunDraftWorkflow_RejectsForeignSystemConversationBeforeRunning(t *testing.T) {
	agentID := uuid.New()
	conversationID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	conversationService := &fakeAdvancedChatConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &ownerID,
		},
	}
	service := &WorkflowService{
		repo:                &mockWorkflowRepository{draft: testChatWorkflow(agentID.String())},
		advancedChatHandler: &AdvancedChatWorkflowHandler{conversationService: conversationService},
	}

	_, err := service.RunDraftWorkflow(context.Background(), uuid.NewString(), agentID.String(), &dto.DraftWorkflowRunRequest{
		Inputs: map[string]interface{}{
			"sys.conversation_id": conversationID.String(),
		},
	}, callerID.String())

	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
	if err == nil || !strings.Contains(err.Error(), "conversation not found") {
		t.Fatalf("RunDraftWorkflow error = %v, want conversation not found", err)
	}
}

func TestRunPublishedWorkflow_RejectsForeignSystemConversationBeforeRunning(t *testing.T) {
	agentID := uuid.New()
	conversationID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	conversationService := &fakeAdvancedChatConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromEndUserID: &ownerID,
		},
	}
	repo := &publishedWorkflowRepository{
		mockWorkflowRepository: &mockWorkflowRepository{draft: testChatWorkflow(agentID.String())},
		published:              testChatWorkflow(agentID.String()),
	}
	service := &WorkflowService{
		repo:                repo,
		advancedChatHandler: &AdvancedChatWorkflowHandler{conversationService: conversationService},
	}

	_, err := service.RunPublishedWorkflow(context.Background(), uuid.NewString(), agentID.String(), &dto.DraftWorkflowRunRequest{
		Inputs: map[string]interface{}{
			"sys.conversation_id": conversationID.String(),
		},
	}, callerID.String())

	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
	if err == nil || !strings.Contains(err.Error(), "conversation not found") {
		t.Fatalf("RunPublishedWorkflow error = %v, want conversation not found", err)
	}
}

type fakeConversationQueryAgentsRepo struct {
	agents.AgentsRepository

	agent       *agents.Agent
	err         error
	gotWebAppID string
}

func (r *fakeConversationQueryAgentsRepo) GetByWebAppID(ctx context.Context, webAppID string) (*agents.Agent, error) {
	r.gotWebAppID = webAppID
	if r.err != nil {
		return nil, r.err
	}
	return r.agent, nil
}

type fakeConversationQueryMessageService struct {
	conversation.AgentMessageService

	called   bool
	messages []*conversation.AgentMessage
}

func (s *fakeConversationQueryMessageService) GetConversationMessages(ctx context.Context, conversationID uuid.UUID) ([]*conversation.AgentMessage, error) {
	s.called = true
	return s.messages, nil
}

func TestWorkflowConversationMetadataRejectsForeignAccountBeforeMessages(t *testing.T) {
	agentID := uuid.New()
	conversationID := uuid.New()
	ownerID := uuid.New()
	callerID := uuid.New()
	conversationService := &fakeAdvancedChatConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &ownerID,
			DialogueCount: 8,
		},
	}
	messageService := &fakeConversationQueryMessageService{
		messages: []*conversation.AgentMessage{
			{ID: uuid.New(), ConversationID: conversationID},
		},
	}
	handler := &WorkflowHandler{
		advancedChatHandler: &AdvancedChatWorkflowHandler{
			conversationService: conversationService,
			messageService:      messageService,
		},
	}

	latestMessageID, err := handler.getLatestMessageIDForCaller(context.Background(), conversationID.String(), agentID.String(), callerID.String())
	if !errors.Is(err, errWebAppConversationAccessDenied) {
		t.Fatalf("getLatestMessageIDForCaller error = %v, want %v", err, errWebAppConversationAccessDenied)
	}
	if latestMessageID != "" {
		t.Fatalf("latest message id = %q, want empty", latestMessageID)
	}
	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
	if messageService.called {
		t.Fatalf("message lookup was called for a foreign-account conversation")
	}

	conversationService.called = false
	dialogueCount := handler.getDialogueCountForCaller(context.Background(), conversationID.String(), agentID.String(), callerID.String())
	if dialogueCount != 1 {
		t.Fatalf("dialogue count = %d, want fallback 1 for foreign account", dialogueCount)
	}
	if !conversationService.called {
		t.Fatalf("conversation lookup was not called for dialogue count")
	}
}

func TestWorkflowServiceDialogueCountUsesCallerScopedConversation(t *testing.T) {
	agentID := uuid.New()
	conversationID := uuid.New()
	callerID := uuid.New()
	conversationService := &fakeAdvancedChatConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &callerID,
			DialogueCount: 4,
		},
	}
	service := &WorkflowService{
		advancedChatHandler: &AdvancedChatWorkflowHandler{conversationService: conversationService},
	}

	dialogueCount := service.getDialogueCountForCaller(context.Background(), conversationID.String(), agentID.String(), callerID.String())
	if dialogueCount != 5 {
		t.Fatalf("dialogue count = %d, want 5", dialogueCount)
	}
	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
}

func TestConversationQueryHandlerDetailRejectsForeignAccountBeforeMessages(t *testing.T) {
	handler, conversationService, messageService, ids := newConversationQueryAccessHandler(t, conversationOwnerFromAccount)

	ctx, recorder := newConversationQueryAccessContext(
		http.MethodGet,
		"/workflows/"+ids.webAppID+"/conversations/"+ids.conversationID.String(),
		ids.webAppID,
		ids.conversationID.String(),
		ids.callerID.String(),
	)

	handler.GetConversationDetail(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
	if messageService.called {
		t.Fatalf("message lookup was called for a foreign-account conversation")
	}
}

func TestConversationQueryHandlerDetailRejectsOtherAgentBeforeMessages(t *testing.T) {
	handler, conversationService, messageService, ids := newConversationQueryAccessHandler(t, conversationOwnerFromCaller)
	conversationService.err = errors.New("record not found")

	ctx, recorder := newConversationQueryAccessContext(
		http.MethodGet,
		"/workflows/"+ids.webAppID+"/conversations/"+ids.conversationID.String(),
		ids.webAppID,
		ids.conversationID.String(),
		ids.callerID.String(),
	)

	handler.GetConversationDetail(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
	if messageService.called {
		t.Fatalf("message lookup was called for another agent's conversation")
	}
}

func TestConversationQueryHandlerDeleteRejectsForeignAccountBeforeDelete(t *testing.T) {
	handler, conversationService, _, ids := newConversationQueryAccessHandler(t, conversationOwnerFromAccount)

	ctx, recorder := newConversationQueryAccessContext(
		http.MethodDelete,
		"/workflows/"+ids.webAppID+"/conversations/"+ids.conversationID.String(),
		ids.webAppID,
		ids.conversationID.String(),
		ids.callerID.String(),
	)

	handler.DeleteConversation(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !conversationService.called {
		t.Fatalf("conversation lookup was not called")
	}
	if conversationService.deleteCalled {
		t.Fatalf("delete was called for a foreign-account conversation")
	}
}

type conversationQueryAccessOwnerMode string

const (
	conversationOwnerFromCaller  conversationQueryAccessOwnerMode = "caller"
	conversationOwnerFromAccount conversationQueryAccessOwnerMode = "other_account"
)

type conversationQueryAccessIDs struct {
	agentID        uuid.UUID
	callerID       uuid.UUID
	conversationID uuid.UUID
	webAppID       string
}

func newConversationQueryAccessHandler(t *testing.T, ownerMode conversationQueryAccessOwnerMode) (*ConversationQueryHandler, *fakeAdvancedChatConversationService, *fakeConversationQueryMessageService, conversationQueryAccessIDs) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	oldDB := database.GetDB()
	database.SetDB(nil)
	t.Cleanup(func() {
		database.SetDB(oldDB)
	})

	ids := conversationQueryAccessIDs{
		agentID:        uuid.New(),
		callerID:       uuid.New(),
		conversationID: uuid.New(),
		webAppID:       uuid.NewString(),
	}
	ownerID := ids.callerID
	if ownerMode == conversationOwnerFromAccount {
		ownerID = uuid.New()
	}
	conversationService := &fakeAdvancedChatConversationService{
		conversation: &conversation.AgentConversation{
			ID:            ids.conversationID,
			AgentID:       ids.agentID,
			FromAccountID: &ownerID,
		},
	}
	messageService := &fakeConversationQueryMessageService{}

	handler := &ConversationQueryHandler{
		conversationService: conversationService,
		messageService:      messageService,
		agentsRepo: &fakeConversationQueryAgentsRepo{
			agent: &agents.Agent{
				ID:           ids.agentID,
				WebAppStatus: agents.AgentWebAppStatusActive,
			},
		},
	}

	return handler, conversationService, messageService, ids
}

func newConversationQueryAccessContext(method, target, webAppID, conversationID, accountID string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	ctx.Params = gin.Params{
		{Key: "web_app_id", Value: webAppID},
		{Key: "conversation_id", Value: conversationID},
	}
	ctx.Set("account_id", accountID)
	return ctx, recorder
}
