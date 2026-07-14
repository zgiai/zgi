package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestNormalizeUserInputContinuationResponseRequiresEveryAnswer(t *testing.T) {
	request := map[string]interface{}{
		"questions": []interface{}{
			map[string]interface{}{"id": "target", "question": "Which target?"},
			map[string]interface{}{"question": "Include a summary?"},
		},
	}

	_, err := normalizeUserInputContinuationResponse("ask-1", request, map[string]string{
		"target": "Current Agent",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("normalizeUserInputContinuationResponse() error = %v, want ErrInvalidInput", err)
	}

	response, err := normalizeUserInputContinuationResponse("ask-1", request, map[string]string{
		"target": "Current Agent",
		"q2":     "Yes",
	})
	if err != nil {
		t.Fatalf("normalizeUserInputContinuationResponse() error = %v", err)
	}
	if got := stringFromAny(response["status"]); got != userInputContinuationStatusAnswered {
		t.Fatalf("response status = %q, want answered", got)
	}
	if got, _ := response["answer_count"].(int); got != 2 {
		t.Fatalf("answer_count = %d, want 2", got)
	}
}

func TestUserInputContinuationMessageRequiresPlanRevisionBeforeBusinessTools(t *testing.T) {
	message := userInputContinuationMessage(
		&runtimemodel.Message{Query: "Update the Agent after I choose the target"},
		map[string]interface{}{"message": "Choose the target."},
		map[string]interface{}{
			"request_id": "ask-1",
			"answers":    []interface{}{map[string]interface{}{"question_id": "target", "value": "Current Agent"}},
		},
	)
	content := stringFromAny(message.Content)
	for _, want := range []string{"revise the current plan with update_plan", "update_plan first and the next business tool in the same assistant response", "Preserve completed phases"} {
		if !strings.Contains(content, want) {
			t.Fatalf("continuation message = %q, want %q", content, want)
		}
	}
}

func TestBeginUserInputContinuationResumesCurrentLeafWithoutCreatingMessage(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		DialogueCount:        1,
	}
	message := &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Status:         runtimemodel.MessageStatusWaitingQuestion,
		Query:          "Update the Agent after clarifying the target",
		Metadata: map[string]interface{}{
			"user_input_request": map[string]interface{}{
				"request_id": "ask-1",
				"message":    "Choose the target.",
				"questions": []interface{}{
					map[string]interface{}{"id": "target", "question": "Which target?"},
				},
			},
		},
	}
	svc := &service{repos: &repository.Repositories{
		Conversation: &fixedUserInputConversationRepo{conversation: conversation},
		Message:      fixedUserInputMessageRepo{message: message},
	}}

	continuation, err := svc.beginUserInputContinuation(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID, SkipAccessCheck: true},
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		conversationID,
		messageID,
		"ask-1",
		map[string]string{"target": "Current Agent"},
	)
	if err != nil {
		t.Fatalf("beginUserInputContinuation() error = %v", err)
	}
	if continuation.Message.ID != messageID || continuation.Conversation.CurrentLeafMessageID == nil || *continuation.Conversation.CurrentLeafMessageID != messageID {
		t.Fatalf("continuation changed message identity: %#v", continuation)
	}
	if continuation.Conversation.DialogueCount != 1 {
		t.Fatalf("dialogue_count = %d, want 1", continuation.Conversation.DialogueCount)
	}
	if continuation.Message.Status != runtimemodel.MessageStatusStreaming {
		t.Fatalf("message status = %q, want streaming", continuation.Message.Status)
	}
	if _, exists := continuation.Message.Metadata["user_input_request"]; exists {
		t.Fatal("resolved request remains active")
	}

	_, err = svc.beginUserInputContinuation(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID, SkipAccessCheck: true},
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		conversationID,
		messageID,
		"ask-1",
		map[string]string{"target": "Current Agent"},
	)
	if !IsContinuationAlreadyRunningError(err) {
		t.Fatalf("duplicate begin error = %v, want continuation already running", err)
	}
}

func TestBeginUserInputContinuationEnforcesAgentRuntimeSource(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	webAppID := uuid.New()
	otherWebAppID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()

	tests := []struct {
		name         string
		conversation *runtimemodel.Conversation
		caller       Caller
		wantErr      error
	}{
		{
			name: "console cannot resume webapp",
			conversation: &runtimemodel.Conversation{
				ID: conversationID, OrganizationID: organizationID, AccountID: accountID,
				CallerType: runtimemodel.ConversationCallerAgent, CallerID: &agentID,
				Source: runtimemodel.ConversationSourceWebApp, SourceWebAppID: &webAppID,
				CurrentLeafMessageID: &messageID,
			},
			caller:  Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole},
			wantErr: ErrNotFound,
		},
		{
			name: "webapp cannot resume console",
			conversation: &runtimemodel.Conversation{
				ID: conversationID, OrganizationID: organizationID, AccountID: accountID,
				CallerType: runtimemodel.ConversationCallerAgent, CallerID: &agentID,
				Source: runtimemodel.ConversationSourceConsole, CurrentLeafMessageID: &messageID,
			},
			caller:  Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceWebApp, SourceWebAppID: &webAppID},
			wantErr: ErrNotFound,
		},
		{
			name: "webapp cannot resume another webapp",
			conversation: &runtimemodel.Conversation{
				ID: conversationID, OrganizationID: organizationID, AccountID: accountID,
				CallerType: runtimemodel.ConversationCallerAgent, CallerID: &agentID,
				Source: runtimemodel.ConversationSourceWebApp, SourceWebAppID: &otherWebAppID,
				CurrentLeafMessageID: &messageID,
			},
			caller:  Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceWebApp, SourceWebAppID: &webAppID},
			wantErr: ErrNotFound,
		},
		{
			name: "matching webapp resumes",
			conversation: &runtimemodel.Conversation{
				ID: conversationID, OrganizationID: organizationID, AccountID: accountID,
				CallerType: runtimemodel.ConversationCallerAgent, CallerID: &agentID,
				Source: runtimemodel.ConversationSourceWebApp, SourceWebAppID: &webAppID,
				CurrentLeafMessageID: &messageID,
			},
			caller: Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceWebApp, SourceWebAppID: &webAppID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := waitingUserInputTestMessage(messageID, conversationID, "ask-1")
			conversationRepo := &fixedUserInputConversationRepo{conversation: tt.conversation}
			svc := &service{repos: &repository.Repositories{
				Conversation: conversationRepo,
				Message:      fixedUserInputMessageRepo{message: message},
			}}

			continuation, err := svc.beginUserInputContinuation(
				context.Background(),
				Scope{OrganizationID: organizationID, AccountID: accountID, SkipAccessCheck: true},
				tt.caller,
				conversationID,
				messageID,
				"ask-1",
				map[string]string{"target": "Current Agent"},
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("beginUserInputContinuation() error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (continuation == nil || continuation.Message.ID != messageID) {
				t.Fatalf("continuation = %#v, want same message %s", continuation, messageID)
			}
			if conversationRepo.callerType != runtimemodel.ConversationCallerAgent || conversationRepo.callerID == nil || *conversationRepo.callerID != agentID {
				t.Fatalf("caller query = (%q, %v), want Agent %s", conversationRepo.callerType, conversationRepo.callerID, agentID)
			}
		})
	}
}

func TestBeginUserInputContinuationRejectsWrongRequestAndStatus(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()

	tests := []struct {
		name      string
		requestID string
		status    string
		wantErr   error
	}{
		{name: "wrong request", requestID: "ask-other", status: runtimemodel.MessageStatusWaitingQuestion, wantErr: ErrNotFound},
		{name: "wrong status", requestID: "ask-1", status: runtimemodel.MessageStatusCompleted, wantErr: ErrInvalidInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conversation := &runtimemodel.Conversation{
				ID: conversationID, OrganizationID: organizationID, AccountID: accountID,
				CurrentLeafMessageID: &messageID,
			}
			message := waitingUserInputTestMessage(messageID, conversationID, "ask-1")
			message.Status = tt.status
			svc := &service{repos: &repository.Repositories{
				Conversation: &fixedUserInputConversationRepo{conversation: conversation},
				Message:      fixedUserInputMessageRepo{message: message},
			}}

			_, err := svc.beginUserInputContinuation(
				context.Background(),
				Scope{OrganizationID: organizationID, AccountID: accountID, SkipAccessCheck: true},
				Caller{Type: runtimemodel.ConversationCallerAIChat},
				conversationID,
				messageID,
				tt.requestID,
				map[string]string{"target": "Current Agent"},
			)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("beginUserInputContinuation() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPrepareUserInputContinuationRestoresAgentRunConfig(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	agentID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	legacyProvider := "legacy-provider"
	message := waitingUserInputTestMessage(messageID, conversationID, "ask-1")
	message.ModelProvider = &legacyProvider
	message.ModelName = "legacy-model"
	continuation := &UserInputContinuation{
		Conversation: &runtimemodel.Conversation{
			ID:             conversationID,
			OrganizationID: organizationID,
			AccountID:      accountID,
			Metadata: map[string]interface{}{
				"surface": aiChatSurfaceExternalPageChat,
			},
		},
		Message:  message,
		Request:  governanceMapFromAny(message.Metadata["user_input_request"]),
		Response: map[string]interface{}{"request_id": "ask-1", "answers": []interface{}{}},
	}
	config := RunConfig{
		SystemPrompt:        "Agent system prompt",
		SystemPromptVersion: "agent.draft",
		ModelProvider:       "configured-provider",
		Model:               "configured-model",
		ModelParameters:     map[string]interface{}{"temperature": 0.25},
		EnabledSkillIDs:     []string{skills.SkillCalculator},
		KnowledgeDatasetIDs: []string{"dataset-1"},
		AgentMemoryEnabled:  true,
		AgentMemorySlots: []AgentMemorySlotConfig{{
			Key: "preference", Enabled: true, MaxChars: 800,
		}},
		AgentMemoryUserScope: "account",
		BillingAppID:         agentID.String(),
		BillingAppType:       runtimemodel.ConversationCallerAgent,
	}
	caller := Caller{Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole}
	svc := &service{
		skillRuntime:      skills.NewRuntime(nil, nil),
		modelSpecResolver: userInputContinuationModelSpecResolver{},
	}

	prepared, err := svc.prepareUserInputContinuationChat(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		caller,
		config,
		continuation,
		runtimedto.UserInputContinuationRequest{Surface: aiChatSurfaceWorkChat},
	)
	if err != nil {
		t.Fatalf("prepareUserInputContinuationChat() error = %v", err)
	}
	if prepared.Message.ID != messageID || !prepared.Continuation {
		t.Fatalf("prepared continuation = %#v, want same message %s", prepared, messageID)
	}
	if prepared.Caller.Type != caller.Type || prepared.Caller.ID == nil || *prepared.Caller.ID != agentID {
		t.Fatalf("prepared caller = %#v, want %#v", prepared.Caller, caller)
	}
	if prepared.parts.Surface != aiChatSurfaceExternalPageChat {
		t.Fatalf("surface = %q, want persisted %q", prepared.parts.Surface, aiChatSurfaceExternalPageChat)
	}
	if prepared.LLMRequest.Model != config.Model || prepared.LLMRequest.Provider != config.ModelProvider {
		t.Fatalf("LLM model = %s/%s, want %s/%s", prepared.LLMRequest.Provider, prepared.LLMRequest.Model, config.ModelProvider, config.Model)
	}
	if prepared.LLMRequest.Temperature == nil || *prepared.LLMRequest.Temperature != 0.25 {
		t.Fatalf("temperature = %v, want 0.25", prepared.LLMRequest.Temperature)
	}
	if len(prepared.LLMRequest.Messages) == 0 || !strings.Contains(stringFromAny(prepared.LLMRequest.Messages[0].Content), config.SystemPrompt) {
		t.Fatalf("system message = %#v, want configured prompt", prepared.LLMRequest.Messages)
	}
	if !prepared.parts.ProtocolToolsEnabled {
		t.Fatalf("protocol tools disabled for verified Agent model capability: parts=%#v", prepared.parts)
	}
	if !prepared.toolLoopEnabled() {
		t.Fatalf("configured skill tool loop disabled: parts=%#v", prepared.parts)
	}
	if !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillCalculator) {
		t.Fatalf("SkillIDs = %#v, want calculator", prepared.parts.SkillIDs)
	}
	if len(prepared.parts.KnowledgeDatasetIDs) != 1 || prepared.parts.KnowledgeDatasetIDs[0] != "dataset-1" {
		t.Fatalf("KnowledgeDatasetIDs = %#v, want dataset-1", prepared.parts.KnowledgeDatasetIDs)
	}
	if !prepared.parts.AgentMemoryEnabled || len(prepared.parts.AgentMemorySlots) != 1 || prepared.parts.AgentMemoryUserScope != "account" {
		t.Fatalf("agent memory config = enabled:%v slots:%#v scope:%q", prepared.parts.AgentMemoryEnabled, prepared.parts.AgentMemorySlots, prepared.parts.AgentMemoryUserScope)
	}
	if prepared.parts.UseMemory {
		t.Fatal("Agent continuation unexpectedly enabled AIChat user memory")
	}
}

func TestPrepareUserInputContinuationKeepsAIChatMetadataSkills(t *testing.T) {
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	message := waitingUserInputTestMessage(messageID, conversationID, "ask-1")
	message.ModelName = "chat-model"
	message.Metadata["configured_skill_ids"] = []string{skills.SkillCalculator}
	continuation := &UserInputContinuation{
		Conversation: &runtimemodel.Conversation{
			ID:             conversationID,
			OrganizationID: organizationID,
			AccountID:      accountID,
			Metadata: map[string]interface{}{
				"surface": aiChatSurfaceContextualSidebar,
			},
		},
		Message:  message,
		Request:  governanceMapFromAny(message.Metadata["user_input_request"]),
		Response: map[string]interface{}{"request_id": "ask-1", "answers": []interface{}{}},
	}
	svc := &service{
		skillRuntime:      skills.NewRuntime(nil, nil),
		modelSpecResolver: userInputContinuationModelSpecResolver{},
		repos: &repository.Repositories{
			SkillConfig: emptyUserInputSkillConfigRepo{},
		},
	}

	prepared, err := svc.prepareUserInputContinuationChat(
		context.Background(),
		Scope{OrganizationID: organizationID, AccountID: accountID},
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		RunConfig{},
		continuation,
		runtimedto.UserInputContinuationRequest{Surface: aiChatSurfaceWorkChat},
	)
	if err != nil {
		t.Fatalf("prepareUserInputContinuationChat() error = %v", err)
	}
	if prepared.parts.ProtocolToolsEnabled {
		t.Fatal("AIChat continuation unexpectedly enabled Agent protocol-only policy")
	}
	if prepared.parts.Surface != aiChatSurfaceContextualSidebar {
		t.Fatalf("surface = %q, want persisted %q", prepared.parts.Surface, aiChatSurfaceContextualSidebar)
	}
	if !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillCalculator) {
		t.Fatalf("SkillIDs = %#v, want restored AIChat calculator skill", prepared.parts.SkillIDs)
	}
	if !skillIDEnabled(prepared.parts.SkillIDs, skills.SkillConsoleNavigator) {
		t.Fatalf("SkillIDs = %#v, want contextual console navigator", prepared.parts.SkillIDs)
	}
	if !prepared.toolLoopEnabled() {
		t.Fatal("AIChat skill continuation no longer enters the tool loop")
	}
}

type userInputContinuationModelSpecResolver struct{}

func (userInputContinuationModelSpecResolver) Resolve(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
	return ModelSpec{SupportsToolCall: true}, true, nil
}

func waitingUserInputTestMessage(messageID, conversationID uuid.UUID, requestID string) *runtimemodel.Message {
	return &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Status:         runtimemodel.MessageStatusWaitingQuestion,
		Query:          "Update the Agent after clarifying the target",
		Metadata: map[string]interface{}{
			"user_input_request": map[string]interface{}{
				"request_id": requestID,
				"message":    "Choose the target.",
				"questions": []interface{}{
					map[string]interface{}{"id": "target", "question": "Which target?"},
				},
			},
		},
	}
}

type fixedUserInputConversationRepo struct {
	repository.ConversationRepository
	conversation *runtimemodel.Conversation
	callerType   string
	callerID     *uuid.UUID
}

func (r *fixedUserInputConversationRepo) GetByCallerScoped(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ uuid.UUID, callerType string, callerID *uuid.UUID) (*runtimemodel.Conversation, error) {
	r.callerType = callerType
	r.callerID = normalizeCallerID(callerID)
	return r.conversation, nil
}

type fixedUserInputMessageRepo struct {
	repository.MessageRepository
	message *runtimemodel.Message
}

type emptyUserInputSkillConfigRepo struct {
	repository.OrganizationSkillConfigRepository
}

func (emptyUserInputSkillConfigRepo) ListByOrganization(context.Context, uuid.UUID) ([]*runtimemodel.OrganizationSkillConfig, error) {
	return nil, nil
}

func (r fixedUserInputMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}
