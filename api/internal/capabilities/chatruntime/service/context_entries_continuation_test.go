package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func storedTurn(t *testing.T, svc *service, messageID uuid.UUID) []string {
	t.Helper()
	entries, err := svc.repos.TranscriptEntry.ListPreviewsByMessage(context.Background(), messageID)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		label := entry.Kind + ":" + messageContentText(entry.Preview["content"])
		if id := messageContentText(entry.Preview["tool_call_id"]); id != "" {
			label = entry.Kind + ":" + id
		}
		if calls, ok := entry.Preview["tool_calls"].([]interface{}); ok && len(calls) > 0 {
			label = entry.Kind + ":call"
		}
		out = append(out, label)
	}
	return out
}

func requireTurn(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Fatalf("stored turn =\n  %s\nwant\n  %s", strings.Join(got, " | "), strings.Join(want, " | "))
	}
}

func TestContinuationDoesNotRestoreThePausedPartialAnswer(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-partial-answer")
	scope := Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}
	// The paused run stored its question and the partial answer it had.
	message.Answer = "partial"
	svc.persistTurnTranscriptEntries(context.Background(), &PreparedChat{Message: message, Conversation: conversation, Scope: scope}, nil)

	resumed := &PreparedChat{Message: message, Conversation: conversation, Scope: scope, Continuation: true}
	resumed.LLMRequest = &adapter.ChatRequest{}
	resumed.resumeWith(adapter.Message{Role: "user", Content: "continue with more budget"})
	call := adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "lookup"}}}}
	result := adapter.Message{Role: "tool", ToolCallID: "call-1", Content: "result"}

	// Message.Answer still holds the paused partial answer during the run.
	svc.persistTurnTranscriptEntries(context.Background(), resumed, []adapter.Message{call})
	bound := svc.persistTurnTranscriptEntries(context.Background(), resumed, []adapter.Message{call, result})
	message.Answer = "final"
	svc.persistTurnTranscriptEntries(context.Background(), resumed, []adapter.Message{call, result, {Role: "assistant", Content: "final"}})

	requireTurn(t, storedTurn(t, svc, message.ID),
		"user:帮我退款", "assistant:partial", "user:continue with more budget", "assistant:call", "tool:call-1", "assistant:final")
	entries, _ := svc.repos.TranscriptEntry.ListPreviewsByMessage(context.Background(), message.ID)
	if len(bound) != 2 || bound[1].EntryID != entries[4].ID.String() {
		t.Fatalf("tool result bound to %q, want the stored tool entry %s", bound[1].EntryID, entries[4].ID)
	}
}

func TestContinuationWithoutStoredEntriesKeepsWhatResumedIt(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-legacy-continuation")
	scope := Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}
	// A message paused before transcript entries existed has only metadata.
	message.Metadata = mergeAgentTranscriptMetadata(nil, []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "ask_user"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "waiting for the user"},
	}, "")
	resumed := &PreparedChat{Message: message, Conversation: conversation, Scope: scope, Continuation: true}
	resumed.LLMRequest = &adapter.ChatRequest{}
	resumed.resumeWith(adapter.Message{Role: "user", Content: "The user answered: yes"})

	// The answer is set when the resumed run finishes, after the writer
	// captured what resumed the turn.
	message.Answer = "done"
	svc.persistTurnTranscriptEntries(context.Background(), resumed, []adapter.Message{{Role: "assistant", Content: "done"}})
	svc.persistTurnTranscriptEntries(context.Background(), resumed, []adapter.Message{{Role: "assistant", Content: "done"}})

	requireTurn(t, storedTurn(t, svc, message.ID),
		"user:帮我退款", "assistant:call", "tool:call-1", "user:The user answered: yes", "assistant:done")
}

func TestSystemResumeMessagesAreStored(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-system-resume")
	scope := Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}
	svc.persistTurnTranscriptEntries(context.Background(), &PreparedChat{Message: message, Conversation: conversation, Scope: scope}, []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "send_mail"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "approval required"},
	})
	resumed := &PreparedChat{Message: message, Conversation: conversation, Scope: scope, Continuation: true}
	resumed.LLMRequest = &adapter.ChatRequest{}
	resumed.resumeWith(adapter.Message{Role: "system", Content: "The user approved send_mail."})
	if got := resumed.LLMRequest.Messages[0].Role; got != "system" {
		t.Fatalf("request role = %q, the model must still receive the original message", got)
	}
	message.Answer = "sent"
	svc.persistTurnTranscriptEntries(context.Background(), resumed, []adapter.Message{{Role: "assistant", Content: "sent"}})

	requireTurn(t, storedTurn(t, svc, message.ID),
		"user:帮我退款", "assistant:call", "tool:call-1", "user:The user approved send_mail.", "assistant:sent")
}

func TestLegacyBootstrapStaysAlignedAcrossCheckpoints(t *testing.T) {
	svc, parent, conversation := newContextEntryTestService(t, "context-entry-bootstrap-checkpoints")
	parent.Status = runtimemodel.MessageStatusCompleted
	parent.Query = "old question"
	parent.Answer = "old answer"
	if err := svc.repos.DB.Save(parent).Error; err != nil {
		t.Fatal(err)
	}
	child := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation.ID, ParentID: &parent.ID, Query: "new question", Status: runtimemodel.MessageStatusStreaming, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}}
	if err := svc.repos.Message.Create(context.Background(), child); err != nil {
		t.Fatal(err)
	}
	prepared := &PreparedChat{Message: child, Conversation: conversation, Scope: Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}}
	call := adapter.Message{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "lookup"}}}}
	svc.persistTurnTranscriptEntries(context.Background(), prepared, []adapter.Message{call})
	svc.persistTurnTranscriptEntries(context.Background(), prepared, []adapter.Message{call, {Role: "tool", ToolCallID: "call-1", Content: "result"}})

	requireTurn(t, storedTurn(t, svc, child.ID),
		"user:old question", "assistant:old answer", "user:new question", "assistant:call", "tool:call-1")
}
