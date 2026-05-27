package agentmemory

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestAgentMemoryToolRejectsNonAgentRuntime(t *testing.T) {
	svc := &Service{repo: newFakeStore(uuid.New())}
	tool := newReadAgentMemoryTool(svc).ForkToolRuntime(&tools.ToolRuntime{
		InvokeFrom: tools.ToolInvokeFromAIChat,
	})
	_, err := tool.Invoke(context.Background(), uuid.NewString(), nil, nil, nil, nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Invoke error = %v, want ErrUnauthorized", err)
	}
}

func TestAgentMemoryToolReadsCurrentAgentAndUser(t *testing.T) {
	workspaceID := uuid.New()
	store := newFakeStore(workspaceID)
	svc := &Service{repo: store}
	agentID := uuid.New()
	userID := uuid.New()
	otherUserID := uuid.New()
	slots, err := svc.ReplaceSlots(context.Background(), agentID, uuid.New(), ReplaceSlotsRequest{Slots: []SlotUpsertRequest{{Key: "profile"}}})
	if err != nil {
		t.Fatalf("ReplaceSlots error = %v", err)
	}
	store.values[valueKey(workspaceID, agentID, "profile", UserScopeAccount, userID)] = &AgentMemoryValue{
		ID: uuid.New(), WorkspaceID: workspaceID, AgentID: agentID, SlotKey: "profile", UserScope: UserScopeAccount, UserID: userID, Content: "current user",
	}
	store.values[valueKey(workspaceID, agentID, "profile", UserScopeAccount, otherUserID)] = &AgentMemoryValue{
		ID: uuid.New(), WorkspaceID: workspaceID, AgentID: agentID, SlotKey: "profile", UserScope: UserScopeAccount, UserID: otherUserID, Content: "other user",
	}
	appID := agentID.String()
	tool := newReadAgentMemoryTool(svc).ForkToolRuntime(&tools.ToolRuntime{
		InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"workspace_id":         workspaceID.String(),
			"agent_id":             agentID.String(),
			"user_scope":           UserScopeAccount,
			"agent_memory_slots":   []RuntimeSlot{{Key: slots[0].Key, MaxChars: slots[0].MaxChars, Enabled: true}},
			"agent_memory_enabled": true,
		},
	})

	messages, err := tool.Invoke(context.Background(), userID.String(), nil, nil, &appID, nil)
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if len(messages) != 1 || messages[0].Text == "" {
		t.Fatalf("messages = %#v, want JSON text", messages)
	}
	if got := messages[0].Text; !containsAll(got, "current user", "profile", "created_at_unix", "updated_at_iso", "updated_at_display") || containsAll(got, "other user") {
		t.Fatalf("message text = %s, want only current user memory", got)
	}
}

func TestAgentMemoryToolUpdateUnknownKeyFails(t *testing.T) {
	svc := &Service{repo: newFakeStore(uuid.New())}
	agentID := uuid.New()
	appID := agentID.String()
	tool := newUpdateAgentMemoryTool(svc).ForkToolRuntime(&tools.ToolRuntime{
		InvokeFrom: tools.ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{
			"workspace_id":         uuid.NewString(),
			"agent_id":             agentID.String(),
			"user_scope":           UserScopeAccount,
			"agent_memory_slots":   []RuntimeSlot{{Key: "other", MaxChars: 1000, Enabled: true}},
			"agent_memory_enabled": true,
		},
	})
	_, err := tool.Invoke(context.Background(), uuid.NewString(), map[string]interface{}{
		"key":     "profile",
		"content": "hello",
	}, nil, &appID, nil)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Invoke error = %v, want ErrInvalidInput", err)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if part == "" {
			continue
		}
		found := false
		for i := 0; i+len(part) <= len(value); i++ {
			if value[i:i+len(part)] == part {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
