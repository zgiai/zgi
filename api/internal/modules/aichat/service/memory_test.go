package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	aichatdto "github.com/zgiai/zgi/api/internal/modules/aichat/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
)

type fakeUserMemoryService struct {
	rendered string
	enabled  bool
}

func (f fakeUserMemoryService) IsEnabled(ctx context.Context, accountID uuid.UUID) (bool, error) {
	return f.enabled, nil
}

func (f fakeUserMemoryService) RenderContext(ctx context.Context, accountID uuid.UUID, budget int) (string, error) {
	return f.rendered, nil
}

func TestNormalizeChatRequestPreservesUseMemory(t *testing.T) {
	parts, err := normalizeChatRequest(aichatRequestForMemory(true))
	if err != nil {
		t.Fatalf("normalizeChatRequest() error = %v", err)
	}
	if !parts.UseMemory {
		t.Fatal("parts.UseMemory = false, want true")
	}
}

func TestBuildUpstreamMessagesInjectsUserMemoryContext(t *testing.T) {
	svc := &service{
		memoryService: fakeUserMemoryService{enabled: true, rendered: "User memory for this account.\n- [preference] Call the user Captain."},
	}
	parts, err := normalizeChatRequest(aichatRequestForMemory(true))
	if err != nil {
		t.Fatalf("normalizeChatRequest() error = %v", err)
	}
	result, err := svc.buildUpstreamMessages(context.Background(), Scope{AccountID: uuid.New()}, nil, parts)
	if err != nil {
		t.Fatalf("buildUpstreamMessages() error = %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("messages are empty")
	}
	systemPrompt, _ := result.Messages[0].Content.(string)
	if !strings.Contains(systemPrompt, "Call the user Captain.") {
		t.Fatalf("system prompt = %q, want injected memory", systemPrompt)
	}
	if result.Metadata == nil || result.Metadata["user_memory"] == nil {
		t.Fatalf("metadata = %#v, want user_memory metadata", result.Metadata)
	}
}

func TestBuildUpstreamMessagesSkipsUserMemoryWhenSettingDisabled(t *testing.T) {
	svc := &service{
		memoryService: fakeUserMemoryService{enabled: false, rendered: "User memory for this account.\n- [preference] Call the user Captain."},
	}
	parts, err := normalizeChatRequest(aichatRequestForMemory(true))
	if err != nil {
		t.Fatalf("normalizeChatRequest() error = %v", err)
	}
	result, err := svc.buildUpstreamMessages(context.Background(), Scope{AccountID: uuid.New()}, nil, parts)
	if err != nil {
		t.Fatalf("buildUpstreamMessages() error = %v", err)
	}
	systemPrompt, _ := result.Messages[0].Content.(string)
	if strings.Contains(systemPrompt, "Call the user Captain.") {
		t.Fatalf("system prompt = %q, want disabled memory omitted", systemPrompt)
	}
	userMemory, ok := result.Metadata["user_memory"].(map[string]interface{})
	if !ok {
		t.Fatalf("metadata = %#v, want user_memory metadata", result.Metadata)
	}
	if userMemory["enabled"] != false {
		t.Fatalf("user_memory metadata = %#v, want enabled=false", userMemory)
	}
}

func TestNormalizeRegenerateRequestInheritsUseMemory(t *testing.T) {
	message := &aichatmodel.Message{
		Query:           "hello",
		ModelName:       "gpt-4o",
		ModelParameters: map[string]interface{}{},
		Metadata:        map[string]interface{}{"use_memory": true},
	}
	parts, err := normalizeRegenerateRequest(aichatdto.RegenerateMessageRequest{}, message)
	if err != nil {
		t.Fatalf("normalizeRegenerateRequest() error = %v", err)
	}
	if !parts.UseMemory {
		t.Fatal("parts.UseMemory = false, want inherited true")
	}
}

func TestNormalizeRegenerateRequestAllowsUseMemoryOverride(t *testing.T) {
	disabled := false
	message := &aichatmodel.Message{
		Query:           "hello",
		ModelName:       "gpt-4o",
		ModelParameters: map[string]interface{}{},
		Metadata:        map[string]interface{}{"use_memory": true},
	}
	parts, err := normalizeRegenerateRequest(aichatdto.RegenerateMessageRequest{UseMemory: &disabled}, message)
	if err != nil {
		t.Fatalf("normalizeRegenerateRequest() error = %v", err)
	}
	if parts.UseMemory {
		t.Fatal("parts.UseMemory = true, want explicit false override")
	}
}

func aichatRequestForMemory(useMemory bool) aichatdto.ChatRequest {
	return aichatdto.ChatRequest{
		Query:     "hello",
		Model:     "gpt-4o",
		UseMemory: useMemory,
	}
}
