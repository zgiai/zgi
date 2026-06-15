package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestRunToolGovernanceDecisionStreamRejectsWithoutTools(t *testing.T) {
	ctx := context.Background()
	organizationID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	provider := "deepseek"
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

	conversation := &runtimemodel.Conversation{
		ID:                   conversationID,
		OrganizationID:       organizationID,
		AccountID:            accountID,
		CallerType:           runtimemodel.ConversationCallerAIChat,
		Title:                "Files",
		Status:               runtimemodel.ConversationStatusNormal,
		RuntimeStatus:        runtimemodel.ConversationRuntimeStatusIdle,
		CurrentLeafMessageID: &messageID,
		Metadata:             map[string]interface{}{},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "Delete the selected report file",
		Status:          runtimemodel.MessageStatusWaitingApproval,
		ModelProvider:   &provider,
		ModelName:       "deepseek-chat",
		ModelParameters: map[string]interface{}{"temperature": 0.2},
		Metadata:        pendingToolGovernanceDecisionMetadata("corr-1"),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	messageRepo := &toolGovernanceStreamMessageRepo{message: message}
	conversationRepo := &toolGovernanceStreamConversationRepo{conversation: conversation}
	llm := &toolGovernanceStreamLLM{
		streamChunks: []string{"I kept the file. ", "No deletion was performed."},
	}
	svc := NewService(&repository.Repositories{
		Access:       toolGovernanceStreamAccessRepo{},
		Conversation: conversationRepo,
		Message:      messageRepo,
	}, llm).(*service)
	svc.events = newStreamEventStore(nil)

	var events []StreamEvent
	result, err := svc.RunToolGovernanceDecisionStream(
		ctx,
		Scope{OrganizationID: organizationID, AccountID: accountID},
		conversationID,
		messageID,
		"corr-1",
		runtimedto.ToolGovernanceDecisionRequest{Action: "reject", Reason: "keep it for audit"},
		func(event StreamEvent) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("RunToolGovernanceDecisionStream() error = %v", err)
	}
	if result.Status != runtimemodel.MessageStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if result.Answer != "I kept the file. No deletion was performed." {
		t.Fatalf("result answer = %q", result.Answer)
	}
	if !messageRepo.updateMetadataAnyStatusCalled {
		t.Fatal("UpdateMetadataAnyStatus was not called for the reject decision")
	}
	if !messageRepo.updateCompletedCalled {
		t.Fatal("UpdateCompleted was not called")
	}
	if !conversationRepo.updateAfterMessageCalled {
		t.Fatal("UpdateAfterMessage was not called")
	}
	if message.Status != runtimemodel.MessageStatusCompleted || message.Answer != result.Answer {
		t.Fatalf("message = status %q answer %q, want completed result answer", message.Status, message.Answer)
	}
	if conversation.RuntimeStatus != runtimemodel.ConversationRuntimeStatusIdle || conversation.ActiveMessageID != nil {
		t.Fatalf("conversation runtime = %q active %#v, want idle with no active message", conversation.RuntimeStatus, conversation.ActiveMessageID)
	}
	if conversation.CurrentLeafMessageID == nil || *conversation.CurrentLeafMessageID != messageID {
		t.Fatalf("conversation current leaf = %#v, want %s", conversation.CurrentLeafMessageID, messageID)
	}

	if len(llm.streamRequests) != 1 {
		t.Fatalf("AppChatStream calls = %d, want 1", len(llm.streamRequests))
	}
	streamReq := llm.streamRequests[0]
	if !streamReq.Stream {
		t.Fatal("reject continuation request is not streaming")
	}
	if len(streamReq.Tools) != 0 || len(streamReq.Functions) != 0 {
		t.Fatalf("reject continuation request tools = %#v functions = %#v, want none", streamReq.Tools, streamReq.Functions)
	}
	if streamReq.Provider != "deepseek" || streamReq.Model != "deepseek-chat" {
		t.Fatalf("request identity = provider %q model %q", streamReq.Provider, streamReq.Model)
	}
	if streamReq.Temperature == nil || *streamReq.Temperature != 0.2 {
		t.Fatalf("request temperature = %#v, want 0.2", streamReq.Temperature)
	}
	requestText := toolGovernanceStreamRequestText(streamReq)
	for _, want := range []string{
		"Do not execute or claim the rejected action",
		"Delete the selected report file",
		"keep it for audit",
		"corr-1",
		"file.delete",
	} {
		if !strings.Contains(requestText, want) {
			t.Fatalf("reject continuation request missing %q in %q", want, requestText)
		}
	}

	metadataEvent, ok := toolGovernanceDecisionEventFromMetadata(message.Metadata, "corr-1")
	if !ok {
		t.Fatalf("tool governance decision not persisted in metadata: %#v", message.Metadata)
	}
	if metadataEvent["approval_status"] != "rejected" {
		t.Fatalf("approval_status = %#v, want rejected", metadataEvent["approval_status"])
	}
	governance := governanceMapFromAny(metadataEvent["governance"])
	if governance["approval_status"] != "rejected" || governance["requires_approval"] != false {
		t.Fatalf("governance = %#v, want rejected and no longer requiring approval", governance)
	}
	approvalResult := governanceMapFromAny(governance["approval_result"])
	modelFeedback := governanceMapFromAny(approvalResult["model_feedback"])
	if modelFeedback["status"] != "user_rejected" {
		t.Fatalf("model feedback = %#v, want user_rejected", modelFeedback)
	}
	if grants := mapSliceFromAny(message.Metadata["tool_governance_one_shot_grants"]); len(grants) != 0 {
		t.Fatalf("one-shot grants = %#v, want none for rejection", grants)
	}
	if grants := mapSliceFromAny(conversation.Metadata["tool_governance_session_grants"]); len(grants) != 0 {
		t.Fatalf("session grants = %#v, want none for rejection", grants)
	}

	assertToolGovernanceStreamEvents(t, events)
}

func assertToolGovernanceStreamEvents(t *testing.T, events []StreamEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("stream events = none")
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.EventType] = true
		if event.EventType == streamEventToolGovernanceDecision {
			if event.Payload["approval_status"] != "rejected" {
				t.Fatalf("decision event approval_status = %#v, want rejected", event.Payload["approval_status"])
			}
			governance := governanceMapFromAny(event.Payload["governance"])
			if governance["requires_approval"] != false {
				t.Fatalf("decision event governance = %#v, want no longer requiring approval", governance)
			}
		}
		if event.EventType == streamEventMessageEnd {
			if event.Payload["status"] != runtimemodel.MessageStatusCompleted {
				t.Fatalf("message_end status = %#v, want completed", event.Payload["status"])
			}
		}
	}
	for _, want := range []string{
		streamEventMessageStart,
		streamEventToolGovernanceDecision,
		streamEventMessage,
		streamEventMessageEnd,
	} {
		if !seen[want] {
			t.Fatalf("stream events missing %q in %#v", want, events)
		}
	}
}

func toolGovernanceStreamRequestText(req *adapter.ChatRequest) string {
	if req == nil {
		return ""
	}
	parts := make([]string, 0, len(req.Messages))
	for _, message := range req.Messages {
		parts = append(parts, messageContentText(message.Content))
	}
	return strings.Join(parts, "\n")
}

type toolGovernanceStreamAccessRepo struct {
	repository.AccessRepository
}

func (toolGovernanceStreamAccessRepo) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

type toolGovernanceStreamConversationRepo struct {
	repository.ConversationRepository
	conversation             *runtimemodel.Conversation
	updateAfterMessageCalled bool
	updateMetadataCalled     bool
}

func (r *toolGovernanceStreamConversationRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Conversation, error) {
	return r.conversation, nil
}

func (r *toolGovernanceStreamConversationRepo) UpdateAfterMessage(_ context.Context, _ uuid.UUID, leafMessageID uuid.UUID) error {
	r.updateAfterMessageCalled = true
	r.conversation.CurrentLeafMessageID = &leafMessageID
	r.conversation.RuntimeStatus = runtimemodel.ConversationRuntimeStatusIdle
	r.conversation.ActiveMessageID = nil
	r.conversation.UpdatedAt = time.Now()
	return nil
}

func (r *toolGovernanceStreamConversationRepo) UpdateMetadata(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updateMetadataCalled = true
	r.conversation.Metadata = copyStringAnyMap(metadata)
	return nil
}

type toolGovernanceStreamMessageRepo struct {
	repository.MessageRepository
	message                       *runtimemodel.Message
	updateMetadataAnyStatusCalled bool
	updateCompletedCalled         bool
	updateErrorCalled             bool
}

func (r *toolGovernanceStreamMessageRepo) GetScoped(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*runtimemodel.Message, error) {
	return r.message, nil
}

func (r *toolGovernanceStreamMessageRepo) ListBranch(context.Context, uuid.UUID, int) ([]*runtimemodel.Message, error) {
	return []*runtimemodel.Message{}, nil
}

func (r *toolGovernanceStreamMessageRepo) UpdateMetadataAnyStatus(_ context.Context, _ uuid.UUID, metadata map[string]interface{}) error {
	r.updateMetadataAnyStatusCalled = true
	r.message.Metadata = copyStringAnyMap(metadata)
	r.message.UpdatedAt = time.Now()
	return nil
}

func (r *toolGovernanceStreamMessageRepo) UpdateCompleted(_ context.Context, _ uuid.UUID, answer string, metadata map[string]interface{}) error {
	r.updateCompletedCalled = true
	r.message.Answer = answer
	r.message.Status = runtimemodel.MessageStatusCompleted
	r.message.Error = nil
	r.message.Metadata = copyStringAnyMap(metadata)
	r.message.UpdatedAt = time.Now()
	return nil
}

func (r *toolGovernanceStreamMessageRepo) UpdateError(_ context.Context, _ uuid.UUID, message string) error {
	r.updateErrorCalled = true
	r.message.Status = runtimemodel.MessageStatusError
	r.message.Error = &message
	r.message.UpdatedAt = time.Now()
	return nil
}

type toolGovernanceStreamLLM struct {
	streamChunks   []string
	streamRequests []*adapter.ChatRequest
	appContexts    []*llmclient.AppContext
}

func (f *toolGovernanceStreamLLM) Chat(context.Context, string, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("unexpected Chat call")
}

func (f *toolGovernanceStreamLLM) ChatStream(context.Context, string, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("unexpected ChatStream call")
}

func (f *toolGovernanceStreamLLM) CreateResponse(context.Context, string, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected CreateResponse call")
}

func (f *toolGovernanceStreamLLM) Embed(context.Context, string, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected Embed call")
}

func (f *toolGovernanceStreamLLM) CreateImage(context.Context, string, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected CreateImage call")
}

func (f *toolGovernanceStreamLLM) Rerank(context.Context, string, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected Rerank call")
}

func (f *toolGovernanceStreamLLM) AppChat(context.Context, *llmclient.AppContext, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("unexpected AppChat call")
}

func (f *toolGovernanceStreamLLM) AppChatStream(_ context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	f.appContexts = append(f.appContexts, appCtx)
	f.streamRequests = append(f.streamRequests, cloneChatRequest(req))
	ch := make(chan adapter.StreamResponse, len(f.streamChunks)+1)
	for _, chunk := range f.streamChunks {
		ch <- adapter.StreamResponse{
			Choices: []adapter.StreamChoice{
				{Delta: adapter.Message{Content: chunk}},
			},
		}
	}
	ch <- adapter.StreamResponse{
		Usage: &adapter.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		Done: true,
	}
	close(ch)
	return ch, nil
}

func (f *toolGovernanceStreamLLM) AppCreateResponse(context.Context, *llmclient.AppContext, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("unexpected AppCreateResponse call")
}

func (f *toolGovernanceStreamLLM) AppEmbed(context.Context, *llmclient.AppContext, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("unexpected AppEmbed call")
}

func (f *toolGovernanceStreamLLM) AppCreateImage(context.Context, *llmclient.AppContext, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("unexpected AppCreateImage call")
}

func (f *toolGovernanceStreamLLM) AppRerank(context.Context, *llmclient.AppContext, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("unexpected AppRerank call")
}
