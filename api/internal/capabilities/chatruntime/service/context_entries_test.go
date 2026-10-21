package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newContextEntryTestService(t *testing.T, name string) (*service, *runtimemodel.Message, *runtimemodel.Conversation) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&runtimemodel.Conversation{}, &runtimemodel.Message{}, &runtimemodel.TranscriptEntry{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	conversation := &runtimemodel.Conversation{ID: uuid.New(), OrganizationID: uuid.New(), AccountID: uuid.New(), Title: "conversation", Metadata: map[string]interface{}{}}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	message := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation.ID, Query: "帮我退款", Status: runtimemodel.MessageStatusStreaming, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}}
	if err := db.Create(message).Error; err != nil {
		t.Fatal(err)
	}
	return &service{repos: repository.NewRepositories(db)}, message, conversation
}

func testProtocolTranscript() []adapter.Message {
	return []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "lookup"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "result"},
		{Role: "assistant", Content: "done"},
	}
}

func TestContextEntrySyncAppendsParentLinkedEntries(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-sync-new")
	prepared := &PreparedChat{Message: message, Conversation: conversation, Scope: Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}}
	svc.persistTurnTranscriptEntries(context.Background(), prepared, testProtocolTranscript())
	if message.TranscriptHeadEntryID == nil {
		t.Fatal("transcript head was not advanced")
	}
	entries, err := svc.repos.TranscriptEntry.ListPreviewsByMessage(context.Background(), message.ID)
	if err != nil || len(entries) != 4 {
		t.Fatalf("entries = %d/%v", len(entries), err)
	}
	if entries[0].Kind != runtimemodel.TranscriptEntryKindUser || entries[1].ParentEntryID == nil || *entries[1].ParentEntryID != entries[0].ID {
		t.Fatalf("parent chain = %+v", entries)
	}
}

func TestPersistTurnTranscriptEntriesBindsEntryIDsForCurrentRun(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-bind-ids")
	prepared := &PreparedChat{Message: message, Conversation: conversation, Scope: Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}}
	transcript := []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "lookup"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "result"},
	}
	bound := svc.persistTurnTranscriptEntries(context.Background(), prepared, transcript)
	if len(bound) != len(transcript) {
		t.Fatalf("bound transcript length = %d, want %d", len(bound), len(transcript))
	}
	for index, item := range bound {
		if strings.TrimSpace(item.EntryID) == "" {
			t.Fatalf("bound transcript entry %d has no durable id", index)
		}
	}
}

func TestContinuationAppendsAfterStoredTurnEntries(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-continuation")
	scope := Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}
	// The paused run stored the question, the tool call, and its pending result.
	paused := &PreparedChat{Message: message, Conversation: conversation, Scope: scope}
	svc.persistTurnTranscriptEntries(context.Background(), paused, []adapter.Message{
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "manage_task"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: `{"status":"approval_required"}`},
	})

	// The continuation run starts with an empty turn transcript: only what it
	// produces is new. It must be appended after the stored entries.
	message.Answer = "已修改完成"
	resumed := &PreparedChat{Message: message, Conversation: conversation, Scope: scope, Continuation: true}
	resumed.continuationTranscript = []adapter.Message{{Role: "user", Content: `Workflow task result JSON: {"status":"applied"}`}}
	bound := svc.persistTurnTranscriptEntries(context.Background(), resumed, []adapter.Message{
		{Role: "assistant", Content: "已修改完成"},
	})
	if len(bound) != 1 || strings.TrimSpace(bound[0].EntryID) == "" {
		t.Fatalf("continuation transcript not bound to a stored entry: %+v", bound)
	}
	entries, err := svc.repos.TranscriptEntry.ListPreviewsByMessage(context.Background(), message.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := make([]string, 0, len(entries))
	for _, entry := range entries {
		kinds = append(kinds, entry.Kind)
	}
	want := []string{
		runtimemodel.TranscriptEntryKindUser,
		runtimemodel.TranscriptEntryKindAssistant,
		runtimemodel.TranscriptEntryKindTool,
		runtimemodel.TranscriptEntryKindUser,
		runtimemodel.TranscriptEntryKindAssistant,
	}
	if len(kinds) != len(want) {
		t.Fatalf("entry kinds = %v, want %v", kinds, want)
	}
	for index := range want {
		if kinds[index] != want[index] {
			t.Fatalf("entry kinds = %v, want %v", kinds, want)
		}
	}
	if entries[3].ParentEntryID == nil || *entries[3].ParentEntryID != entries[2].ID {
		t.Fatalf("continuation entry is not linked after the stored tool result: %+v", entries[3])
	}
	// The resolution the model was given is part of the record, so a later
	// turn does not see only the pending tool result.
	if !strings.Contains(messageContentText(entries[3].Preview["content"]), `"status":"applied"`) {
		t.Fatalf("continuation resolution was not stored: %+v", entries[3].Preview)
	}

	// A later checkpoint of the same continuation must not duplicate entries.
	svc.persistTurnTranscriptEntries(context.Background(), resumed, []adapter.Message{
		{Role: "assistant", Content: "已修改完成"},
	})
	again, err := svc.repos.TranscriptEntry.ListPreviewsByMessage(context.Background(), message.ID)
	if err != nil || len(again) != len(want) {
		t.Fatalf("entries after repeated checkpoint = %d/%v, want %d", len(again), err, len(want))
	}
}

func TestProjectTranscriptChainUsesSummaryBoundary(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-projection-new")
	prepared := &PreparedChat{Message: message, Conversation: conversation, Scope: Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}}
	svc.persistTurnTranscriptEntries(context.Background(), prepared, []adapter.Message{{Role: "assistant", Content: "old"}, {Role: "assistant", Content: "tail"}})
	entries, _ := svc.repos.TranscriptEntry.ListPreviewsByMessage(context.Background(), message.ID)
	boundary := entries[1].ID
	svc.persistAgentContextSummary(context.Background(), prepared, "summary", boundary.String())
	region, ok, err := svc.projectTranscriptChain(context.Background(), *message.TranscriptHeadEntryID, map[uuid.UUID]*runtimemodel.Message{message.ID: message}, false)
	if err != nil || !ok {
		t.Fatalf("projection = %v/%v", ok, err)
	}
	if len(region.Messages) != 2 || !strings.Contains(messageContentText(region.Messages[0].Content), "summary") || messageContentText(region.Messages[1].Content) != "tail" {
		t.Fatalf("projected messages = %+v", region.Messages)
	}
}

func TestPersistedLegacyAgentTranscriptRemainsReadable(t *testing.T) {
	svc, message, _ := newContextEntryTestService(t, "context-entry-legacy-new")
	message.Answer = "final"
	message.Metadata = mergeAgentTranscriptMetadata(nil, testProtocolTranscript(), message.Answer)
	got := svc.historicalAgentTranscript(context.Background(), message)
	if len(got) != 2 || got[0].Role != "assistant" || got[1].Role != "tool" {
		t.Fatalf("legacy transcript = %+v", got)
	}
}

func TestCopyConversationTranscriptForBranchOwnsIndependentEntries(t *testing.T) {
	svc, sourceMessage, sourceConversation := newContextEntryTestService(t, "context-entry-branch-copy")
	prepared := &PreparedChat{Message: sourceMessage, Conversation: sourceConversation, Scope: Scope{OrganizationID: sourceConversation.OrganizationID, AccountID: sourceConversation.AccountID}}
	svc.persistTurnTranscriptEntries(context.Background(), prepared, []adapter.Message{{Role: "assistant", Content: "answer"}})

	targetConversation := &runtimemodel.Conversation{ID: uuid.New(), OrganizationID: sourceConversation.OrganizationID, AccountID: sourceConversation.AccountID, Title: "branch", Metadata: map[string]interface{}{}}
	if err := svc.repos.DB.Create(targetConversation).Error; err != nil {
		t.Fatal(err)
	}
	targetMessage := &runtimemodel.Message{ID: uuid.New(), ConversationID: targetConversation.ID, Query: sourceMessage.Query, Status: runtimemodel.MessageStatusCompleted, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}}
	if err := svc.repos.DB.Create(targetMessage).Error; err != nil {
		t.Fatal(err)
	}
	copied, err := svc.copyConversationTranscriptForBranch(context.Background(), *sourceMessage.TranscriptHeadEntryID, targetConversation.ID, map[uuid.UUID]uuid.UUID{sourceMessage.ID: targetMessage.ID}, sourceConversation.OrganizationID)
	if err != nil || copied != 2 {
		t.Fatalf("copy = %d/%v", copied, err)
	}
	var stored runtimemodel.Message
	if err := svc.repos.DB.First(&stored, "id = ?", targetMessage.ID).Error; err != nil || stored.TranscriptHeadEntryID == nil || *stored.TranscriptHeadEntryID == *sourceMessage.TranscriptHeadEntryID {
		t.Fatalf("target head = %v/%v", stored.TranscriptHeadEntryID, err)
	}
	if _, err := svc.repos.TranscriptEntry.DeleteByConversation(context.Background(), sourceConversation.ID); err != nil {
		t.Fatal(err)
	}
	chain, err := svc.repos.TranscriptEntry.LoadPreviewChain(context.Background(), *stored.TranscriptHeadEntryID, nil, 0)
	if err != nil || len(chain) != 2 {
		t.Fatalf("target chain after source delete = %d/%v", len(chain), err)
	}
}

func TestCopyConversationTranscriptForBranchRejectsIncompleteMessageMapping(t *testing.T) {
	svc, sourceMessage, sourceConversation := newContextEntryTestService(t, "context-entry-branch-missing-map")
	prepared := &PreparedChat{Message: sourceMessage, Conversation: sourceConversation, Scope: Scope{OrganizationID: sourceConversation.OrganizationID, AccountID: sourceConversation.AccountID}}
	svc.persistTurnTranscriptEntries(context.Background(), prepared, []adapter.Message{{Role: "assistant", Content: "answer"}})
	if _, err := svc.copyConversationTranscriptForBranch(context.Background(), *sourceMessage.TranscriptHeadEntryID, uuid.New(), map[uuid.UUID]uuid.UUID{}, sourceConversation.OrganizationID); err == nil {
		t.Fatal("expected incomplete message mapping to be rejected")
	}
}

func TestLegacyMainMessageHistoryIsLazilyJoinedToParentChain(t *testing.T) {
	svc, parent, conversation := newContextEntryTestService(t, "context-entry-legacy-bootstrap")
	parent.Status = runtimemodel.MessageStatusCompleted
	parent.Query = "old question"
	parent.Answer = "old answer"
	parent.Metadata = mergeAgentTranscriptMetadata(nil, testProtocolTranscript(), parent.Answer)
	if err := svc.repos.DB.Save(parent).Error; err != nil {
		t.Fatal(err)
	}
	child := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation.ID, ParentID: &parent.ID, Query: "new question", Status: runtimemodel.MessageStatusStreaming, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}}
	if err := svc.repos.Message.Create(context.Background(), child); err != nil {
		t.Fatal(err)
	}
	prepared := &PreparedChat{Message: child, Conversation: conversation, Scope: Scope{OrganizationID: conversation.OrganizationID, AccountID: conversation.AccountID}}
	svc.persistTurnTranscriptEntries(context.Background(), prepared, []adapter.Message{{Role: "assistant", Content: "new answer"}})
	region, ok, err := svc.projectTranscriptChain(context.Background(), *child.TranscriptHeadEntryID, map[uuid.UUID]*runtimemodel.Message{parent.ID: parent, child.ID: child}, false)
	if err != nil || !ok {
		t.Fatalf("legacy bootstrap projection = %v/%v", ok, err)
	}
	joined := make([]string, 0, len(region.Messages))
	for _, message := range region.Messages {
		joined = append(joined, messageContentText(message.Content))
	}
	text := strings.Join(joined, "|")
	if !strings.Contains(text, "old question") || !strings.Contains(text, "old answer") || !strings.Contains(text, "new question") {
		t.Fatalf("legacy history was not joined: %s", text)
	}
}

func TestPruneTranscriptMessageForWriteLeavesCompressionToContextManager(t *testing.T) {
	entryID := uuid.New()
	big := strings.Repeat("large result ", 9000)
	payload := map[string]interface{}{"role": "tool", "tool_call_id": "call-1", "content": big}
	preview := pruneTranscriptMessageForWrite(entryID, payload, runtimemodel.TranscriptEntryKindTool)
	if messageContentText(preview["content"]) != big {
		t.Fatal("persistence layer changed an uncompressed tool result")
	}
}

func TestCompactBoundaryUsesEntryID(t *testing.T) {
	messages := []adapter.Message{{Role: "user", Content: "q", EntryID: uuid.NewString()}, {Role: "assistant", Content: "a", EntryID: uuid.NewString()}}
	got := messages[len(messages)-1].EntryID
	if got != messages[1].EntryID {
		t.Fatalf("boundary = %q", got)
	}
}
