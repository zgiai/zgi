package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	aichatdto "github.com/zgiai/zgi/api/internal/modules/aichat/dto"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

type fakeUserMemoryService struct {
	rendered string
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
		memoryService: fakeUserMemoryService{rendered: "User memory for this account.\n- [preference] Call the user Captain."},
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

func TestAppendUserMemorySkillOnlyWhenCatalogHasHiddenSkill(t *testing.T) {
	parts := &chatRequestParts{UseMemory: true}
	appendUserMemorySkill(context.Background(), parts, []skills.SkillDiscoveryMetadata{
		{ID: memory.SkillID, Status: skills.SkillStatusActive, RuntimeType: skills.SkillRuntimeTypeHybrid},
	})
	if len(parts.SkillIDs) != 1 || parts.SkillIDs[0] != memory.SkillID {
		t.Fatalf("SkillIDs = %#v, want user-memory", parts.SkillIDs)
	}
	if len(parts.ToolSkillIDs) != 1 || parts.ToolSkillIDs[0] != memory.SkillID {
		t.Fatalf("ToolSkillIDs = %#v, want user-memory", parts.ToolSkillIDs)
	}
}

func aichatRequestForMemory(useMemory bool) aichatdto.ChatRequest {
	return aichatdto.ChatRequest{
		Query:     "hello",
		Model:     "gpt-4o",
		UseMemory: useMemory,
	}
}
