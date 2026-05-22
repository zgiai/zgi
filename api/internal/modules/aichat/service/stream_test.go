package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestFinalizePreparedErrorSetsFailedMessageAsCurrentLeaf(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	conversationRepo := &recordingConversationRepository{}
	messageRepo := &recordingMessageRepository{}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
		events: newStreamEventStore(nil),
	}

	svc.finalizePreparedError(context.Background(), &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: conversationID},
		Message:      &aichatmodel.Message{ID: messageID},
	}, errors.New("tool failed"))

	if messageRepo.updateErrorID != messageID || messageRepo.updateErrorMessage != "tool failed" {
		t.Fatalf("UpdateError(%s, %q), want (%s, %q)", messageRepo.updateErrorID, messageRepo.updateErrorMessage, messageID, "tool failed")
	}
	if conversationRepo.updateAfterConversationID != conversationID || conversationRepo.updateAfterLeafID != messageID {
		t.Fatalf("UpdateAfterMessage(%s, %s), want (%s, %s)", conversationRepo.updateAfterConversationID, conversationRepo.updateAfterLeafID, conversationID, messageID)
	}
	if conversationRepo.finishActiveCalls != 0 {
		t.Fatalf("FinishActiveMessage calls = %d, want 0", conversationRepo.finishActiveCalls)
	}
}

func TestFinalizePreparedErrorCompletesRootReplacement(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	conversationRepo := &recordingConversationRepository{}
	messageRepo := &recordingMessageRepository{}
	svc := &service{
		repos: &repository.Repositories{
			Conversation: conversationRepo,
			Message:      messageRepo,
		},
		events: newStreamEventStore(nil),
	}

	svc.finalizePreparedError(context.Background(), &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: conversationID},
		Message:      &aichatmodel.Message{ID: messageID},
		ReplaceRoot:  true,
	}, errors.New("model failed"))

	if conversationRepo.completeRootConversationID != conversationID || conversationRepo.completeRootMessageID != messageID {
		t.Fatalf("CompleteRootReplacement(%s, %s), want (%s, %s)", conversationRepo.completeRootConversationID, conversationRepo.completeRootMessageID, conversationID, messageID)
	}
	if conversationRepo.updateAfterConversationID != uuid.Nil {
		t.Fatalf("UpdateAfterMessage was called for root replacement")
	}
}

func TestHandleProgressiveSkillCallReturnsRecoverableArgumentError(t *testing.T) {
	svc := &service{}
	call := adapter.ToolCall{
		ID: "call_1",
		Function: adapter.FunctionCall{
			Name:      skills.MetaToolLoadSkill,
			Arguments: "{",
		},
	}

	result := svc.handleProgressiveSkillCall(context.Background(), &PreparedChat{}, nil, call, skills.ExecutionContext{}, 0, nil, map[string]struct{}{}, nil)

	if !result.recoverable || result.fatalErr != nil {
		t.Fatalf("result recoverable=%v fatalErr=%v, want recoverable without fatal", result.recoverable, result.fatalErr)
	}
	if result.trace.Status != "error" || !strings.Contains(result.trace.Error, "invalid tool arguments") {
		t.Fatalf("trace = %#v, want invalid arguments error", result.trace)
	}
	content, _ := result.toolMessage.Content.(string)
	if !strings.Contains(content, `"recoverable":true`) {
		t.Fatalf("tool message content = %q, want recoverable payload", content)
	}
}

func TestRunPreparedSkillStreamEmitsPlanningTextAsAnswer(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "brief-writer", `---
name: brief-writer
description: Help draft short writing briefs.
when_to_use: Use when writing a concise brief.
---

# Brief Writer

Write concise briefs.
`)
	conversationID := uuid.New()
	messageID := uuid.New()
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient: &fakeAgenticLLMClient{
			appChatResponses: []*adapter.ChatResponse{
				{
					Choices: []adapter.Choice{{
						Message: adapter.Message{Role: "assistant", Content: "我会直接给出结果。"},
					}},
					Usage: &adapter.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7},
				},
			},
		},
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             conversationID,
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       messageID,
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "写一个 brief"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"brief-writer"},
		},
	}
	chunks := make([]string, 0)

	answer, usage, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	}, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "我会直接给出结果。" {
		t.Fatalf("answer = %q, want planning text answer", answer)
	}
	if len(chunks) != 1 || chunks[0] != answer {
		t.Fatalf("chunks = %#v, want single answer chunk", chunks)
	}
	if usage == nil || usage.TotalTokens != 7 {
		t.Fatalf("usage = %#v, want aggregated usage", usage)
	}
}

func TestRunPreparedSkillStreamContinuesAfterRecoverableToolArgumentError(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "brief-writer", `---
name: brief-writer
description: Help draft short writing briefs.
when_to_use: Use when writing a concise brief.
---

# Brief Writer

Write concise briefs.
`)
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role: "assistant",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: "{",
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "参数已修正，结果完成。"},
				}},
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "写一个 brief"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"brief-writer"},
		},
	}

	answer, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, nil)
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "参数已修正，结果完成。" {
		t.Fatalf("answer = %q, want final answer after recovery", answer)
	}
	if fakeLLM.appChatCalls != 2 {
		t.Fatalf("AppChat calls = %d, want 2", fakeLLM.appChatCalls)
	}
}

func TestRunPreparedSkillStreamEmitsAgentProgressForContentWithToolCalls(t *testing.T) {
	catalogDir := t.TempDir()
	writeTestSkill(t, catalogDir, "brief-writer", `---
name: brief-writer
description: Help draft short writing briefs.
when_to_use: Use when writing a concise brief.
---

# Brief Writer

Write concise briefs.
`)
	fakeLLM := &fakeAgenticLLMClient{
		appChatResponses: []*adapter.ChatResponse{
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{
						Role:    "assistant",
						Content: "我会先加载 brief writer。",
						ToolCalls: []adapter.ToolCall{{
							ID:   "call_1",
							Type: "function",
							Function: adapter.FunctionCall{
								Name:      skills.MetaToolLoadSkill,
								Arguments: "{",
							},
						}},
					},
				}},
			},
			{
				Choices: []adapter.Choice{{
					Message: adapter.Message{Role: "assistant", Content: "最终结果。"},
				}},
			},
		},
	}
	svc := &service{
		repos: &repository.Repositories{
			Message:     &recordingMessageRepository{},
			CustomSkill: &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}},
		},
		llmClient:    fakeLLM,
		events:       newStreamEventStore(nil),
		skillRuntime: skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
	}
	prepared := &PreparedChat{
		Conversation: &aichatmodel.Conversation{
			ID:             uuid.New(),
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		Message: &aichatmodel.Message{
			ID:       uuid.New(),
			Metadata: map[string]interface{}{},
		},
		LLMRequest: &adapter.ChatRequest{
			Messages: []adapter.Message{{Role: "user", Content: "写一个 brief"}},
		},
		Scope: Scope{
			OrganizationID: uuid.New(),
			AccountID:      uuid.New(),
		},
		parts: &chatRequestParts{
			SkillMode: skillModeAuto,
			SkillIDs:  []string{"brief-writer"},
		},
	}
	events := make([]StreamEvent, 0)

	answer, _, err := svc.runPreparedSkillStream(context.Background(), context.Background(), prepared, nil, func(event StreamEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("runPreparedSkillStream() error = %v", err)
	}
	if answer != "最终结果。" {
		t.Fatalf("answer = %q, want final answer without progress text", answer)
	}
	var progressContent string
	for _, event := range events {
		if event.EventType == streamEventAgentProgress {
			progressContent, _ = event.Payload["content"].(string)
			break
		}
	}
	if progressContent != "我会先加载 brief writer。" {
		t.Fatalf("agent progress content = %q, want progress text", progressContent)
	}
}

type recordingConversationRepository struct {
	updateAfterConversationID  uuid.UUID
	updateAfterLeafID          uuid.UUID
	completeRootConversationID uuid.UUID
	completeRootMessageID      uuid.UUID
	finishActiveCalls          int
}

func (r *recordingConversationRepository) Create(ctx context.Context, conversation *aichatmodel.Conversation) error {
	return nil
}
func (r *recordingConversationRepository) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*aichatmodel.Conversation, error) {
	return nil, nil
}
func (r *recordingConversationRepository) GetBySourceConversation(ctx context.Context, sourceConversationID uuid.UUID) (*aichatmodel.Conversation, error) {
	return nil, nil
}
func (r *recordingConversationRepository) ListScoped(ctx context.Context, organizationID, accountID uuid.UUID, limit, offset int) ([]*aichatmodel.Conversation, int64, error) {
	return nil, 0, nil
}
func (r *recordingConversationRepository) UpdateScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, updates map[string]interface{}) error {
	return nil
}
func (r *recordingConversationRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingConversationRepository) DeleteScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) StartStreaming(ctx context.Context, id, organizationID, accountID, messageID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) ClearActiveMessage(ctx context.Context, id, messageID uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) FinishActiveMessage(ctx context.Context, id, messageID uuid.UUID) error {
	r.finishActiveCalls++
	return nil
}
func (r *recordingConversationRepository) ClearActiveMessages(ctx context.Context, messageIDs []uuid.UUID) error {
	return nil
}
func (r *recordingConversationRepository) CompleteRootReplacement(ctx context.Context, id, messageID uuid.UUID) error {
	r.completeRootConversationID = id
	r.completeRootMessageID = messageID
	return nil
}
func (r *recordingConversationRepository) UpdateAfterMessage(ctx context.Context, id uuid.UUID, leafMessageID uuid.UUID) error {
	r.updateAfterConversationID = id
	r.updateAfterLeafID = leafMessageID
	return nil
}
func (r *recordingConversationRepository) RefreshAfterMessageDelete(ctx context.Context, id uuid.UUID) error {
	return nil
}

type recordingMessageRepository struct {
	updateErrorID      uuid.UUID
	updateErrorMessage string
}

func (r *recordingMessageRepository) Create(ctx context.Context, message *aichatmodel.Message) error {
	return nil
}
func (r *recordingMessageRepository) GetScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) GetBySourceMessage(ctx context.Context, sourceMessageID uuid.UUID) (*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) ListByConversationScoped(ctx context.Context, conversationID, organizationID, accountID uuid.UUID, limit, offset int) ([]*aichatmodel.Message, int64, error) {
	return nil, 0, nil
}
func (r *recordingMessageRepository) ListBranch(ctx context.Context, leafID uuid.UUID, maxDepth int) ([]*aichatmodel.Message, error) {
	return nil, nil
}
func (r *recordingMessageRepository) CountByConversation(ctx context.Context, conversationID uuid.UUID) (int64, error) {
	return 0, nil
}
func (r *recordingMessageRepository) ReplaceRootForStreaming(ctx context.Context, message *aichatmodel.Message) error {
	return nil
}
func (r *recordingMessageRepository) UpdateCompleted(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) UpdateMetadata(ctx context.Context, id uuid.UUID, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) UpdateError(ctx context.Context, id uuid.UUID, message string) error {
	r.updateErrorID = id
	r.updateErrorMessage = message
	return nil
}
func (r *recordingMessageRepository) MarkStopped(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (r *recordingMessageRepository) UpdateStoppedAnswer(ctx context.Context, id uuid.UUID, answer string, metadata map[string]interface{}) error {
	return nil
}
func (r *recordingMessageRepository) DeleteSubtreeScoped(ctx context.Context, id, organizationID, accountID uuid.UUID) (*repository.MessageDeleteResult, error) {
	return nil, nil
}
func (r *recordingMessageRepository) ListStaleActiveIDs(ctx context.Context, cutoff time.Time) ([]uuid.UUID, error) {
	return nil, nil
}
func (r *recordingMessageRepository) MarkStaleActiveAsError(ctx context.Context, cutoff time.Time, message string) (int64, error) {
	return 0, nil
}

func writeTestSkill(t *testing.T, catalogDir string, skillID string, content string) {
	t.Helper()

	root := filepath.Join(catalogDir, skillID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir skill root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}

type fakeAgenticLLMClient struct {
	appChatResponses []*adapter.ChatResponse
	appChatCalls     int
}

func (f *fakeAgenticLLMClient) Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) ChatStream(ctx context.Context, organizationID string, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) CreateResponse(ctx context.Context, organizationID string, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) Embed(ctx context.Context, organizationID string, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) CreateImage(ctx context.Context, organizationID string, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) Rerank(ctx context.Context, organizationID string, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	if f.appChatCalls >= len(f.appChatResponses) {
		return nil, errors.New("unexpected AppChat call")
	}
	resp := f.appChatResponses[f.appChatCalls]
	f.appChatCalls++
	return resp, nil
}

func (f *fakeAgenticLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAgenticLLMClient) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, errors.New("not implemented")
}
