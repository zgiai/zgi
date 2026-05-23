package memory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestMemoryToolUpdateAcceptsIDAlias(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	created, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "Remember the old value.",
		Category: CategoryFact,
	})
	if err != nil {
		t.Fatalf("CreateEntry() error = %v", err)
	}

	tool := aiChatMemoryTool(newUpdateMemoryTool(svc))
	_, err = tool.Invoke(ctx, accountID.String(), map[string]interface{}{
		"id":      created.ID,
		"content": "Remember the updated value.",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke(update with id alias) error = %v", err)
	}

	state, err := svc.GetMe(ctx, accountID)
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if got := state.Entries[0].Content; got != "Remember the updated value." {
		t.Fatalf("entry content = %q", got)
	}
}

func TestMemoryToolListsExpiredTemporaryMemories(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	expiredAt := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)

	created, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:    "The user needed to bring the briefcase yesterday.",
		MemoryType: MemoryTypeTemporary,
		ExpiresAt:  expiredAt,
	})
	if err != nil {
		t.Fatalf("CreateEntry() error = %v", err)
	}

	messages, err := aiChatMemoryTool(newListTemporaryMemoriesTool(svc)).Invoke(ctx, accountID.String(), map[string]interface{}{
		"status": memoryStatusExpired,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke(list temporary) error = %v", err)
	}
	var payload struct {
		Entries []struct {
			ID         string `json:"id"`
			MemoryType string `json:"memory_type"`
			Status     string `json:"status"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(messages[0].Text), &payload); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if len(payload.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(payload.Entries))
	}
	if payload.Entries[0].ID != created.ID || payload.Entries[0].MemoryType != MemoryTypeTemporary || payload.Entries[0].Status != memoryStatusExpired {
		t.Fatalf("entry = %#v, want expired temporary entry %s", payload.Entries[0], created.ID)
	}
}

func TestMemoryToolAddAcceptsMemoryAlias(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	tool := aiChatMemoryTool(newAddMemoryTool(svc))
	_, err := tool.Invoke(ctx, accountID.String(), map[string]interface{}{
		"memory":   "The user's birthday is May 24.",
		"category": CategoryProfile,
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke(add with memory alias) error = %v", err)
	}

	state, err := svc.GetMe(ctx, accountID)
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if len(state.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(state.Entries))
	}
	if got := state.Entries[0].Content; got != "The user's birthday is May 24." {
		t.Fatalf("entry content = %q", got)
	}
}

func TestMemoryToolReadReturnsEntryIDAlias(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	created, err := svc.CreateEntry(ctx, accountID, CreateEntryRequest{
		Content:  "Prefers short answers.",
		Category: CategoryPreference,
	})
	if err != nil {
		t.Fatalf("CreateEntry() error = %v", err)
	}

	messages, err := aiChatMemoryTool(newReadMemoryTool(svc)).Invoke(ctx, accountID.String(), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke(read) error = %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("len(messages) = %d, want 1", len(messages))
	}
	if !strings.Contains(messages[0].Text, `"entry_id"`) {
		t.Fatalf("tool response %q does not expose entry_id", messages[0].Text)
	}

	var payload struct {
		Entries []struct {
			ID      string `json:"id"`
			EntryID string `json:"entry_id"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(messages[0].Text), &payload); err != nil {
		t.Fatalf("decode tool response: %v", err)
	}
	if len(payload.Entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(payload.Entries))
	}
	if payload.Entries[0].ID != created.ID || payload.Entries[0].EntryID != created.ID {
		t.Fatalf("ids = (%q, %q), want %q", payload.Entries[0].ID, payload.Entries[0].EntryID, created.ID)
	}
}

func TestMemoryToolRejectsWhenMemoryDisabled(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, false); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	_, err := aiChatMemoryTool(newReadMemoryTool(svc)).Invoke(ctx, accountID.String(), nil, nil, nil, nil)
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("Invoke(read disabled) error = %v, want ErrDisabled", err)
	}
}

func TestMemoryToolRejectsWorkflowRuntime(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	tool := newReadMemoryTool(svc).ForkToolRuntime(&tools.ToolRuntime{
		InvokeFrom: tools.ToolInvokeFromWorkflow,
	})
	_, err := tool.Invoke(ctx, accountID.String(), nil, nil, nil, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Invoke(read workflow) error = %v, want ErrUnauthorized", err)
	}
}

func TestMemoryToolRejectsMissingRuntime(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	_, err := newReadMemoryTool(svc).Invoke(ctx, accountID.String(), nil, nil, nil, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Invoke(read without runtime) error = %v, want ErrUnauthorized", err)
	}
}

func TestMemoryToolAuditSourceUsesRuntime(t *testing.T) {
	ctx := context.Background()
	svc, store := newTestService()
	accountID := uuid.New()
	if _, err := svc.SetEnabled(ctx, accountID, true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	_, err := aiChatMemoryTool(newAddMemoryTool(svc)).Invoke(ctx, accountID.String(), map[string]interface{}{
		"content": "Prefers concise answers.",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke(add) error = %v", err)
	}
	if len(store.events) < 2 {
		t.Fatalf("len(events) = %d, want at least 2", len(store.events))
	}
	if got := store.events[len(store.events)-1].Source; got != EventSourceAIChat {
		t.Fatalf("event source = %s, want %s", got, EventSourceAIChat)
	}
}

func aiChatMemoryTool(tool tools.Tool) tools.Tool {
	return tool.ForkToolRuntime(&tools.ToolRuntime{
		InvokeFrom: tools.ToolInvokeFromAIChat,
	})
}
