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
	if err := db.AutoMigrate(&runtimemodel.Message{}, &runtimemodel.Conversation{}, &runtimemodel.TranscriptEntry{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	conversation := &runtimemodel.Conversation{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
		Title:          "conversation",
		Metadata:       map[string]interface{}{},
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	message := &runtimemodel.Message{
		ID:              uuid.New(),
		ConversationID:  conversation.ID,
		Query:           "帮我退款",
		Status:          runtimemodel.MessageStatusStreaming,
		ModelName:       "test-model",
		ModelParameters: map[string]interface{}{},
		Metadata:        map[string]interface{}{},
	}
	if err := db.Create(message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	svc := &service{repos: repository.NewRepositories(db)}
	return svc, message, conversation
}

func testProtocolTranscript() []adapter.Message {
	return []adapter.Message{
		{Role: "user", Content: "帮我退款"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1", Type: "function", Function: adapter.FunctionCall{Name: "refund_query"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "订单 12345：已支付 199 元"},
		{Role: "assistant", Content: "已为你提交退款"},
	}
}

func TestProjectContextMessagesUsesSummaryBoundaryWithoutGapOrOverlap(t *testing.T) {
	entries := []*runtimemodel.TranscriptEntryPreview{
		{ID: uuid.New(), CSeq: 1, Preview: map[string]interface{}{"role": "user", "content": "第一轮"}},
		{ID: uuid.New(), CSeq: 2, Preview: map[string]interface{}{"role": "assistant", "content": "第一轮回答"}},
		{ID: uuid.New(), CSeq: 3, Preview: map[string]interface{}{"role": "user", "content": "第二轮"}},
		{ID: uuid.New(), CSeq: 4, Preview: map[string]interface{}{"role": "assistant", "content": "第二轮回答"}},
	}
	summary := &runtimemodel.MessageContextSummary{MessageID: uuid.New(), Summary: "更早对话的摘要", ThroughSeq: 2}

	messages, err := projectContextMessages(summary, entries)
	if err != nil {
		t.Fatalf("projectContextMessages: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages = %d, want 3 (summary + two entries after the boundary)", len(messages))
	}
	if messages[0].Role != "user" || !strings.Contains(messageContentText(messages[0].Content), "更早对话的摘要") {
		t.Fatalf("summary message = %+v", messages[0])
	}
	if messageContentText(messages[1].Content) != "第二轮" || messageContentText(messages[2].Content) != "第二轮回答" {
		t.Fatalf("covered entries leaked into the projection: %+v", messages[1:])
	}

	withoutSummary, err := projectContextMessages(nil, entries)
	if err != nil {
		t.Fatalf("projectContextMessages without summary: %v", err)
	}
	if len(withoutSummary) != 4 {
		t.Fatalf("messages without summary = %d, want 4", len(withoutSummary))
	}
}

func TestPruneTranscriptMessageForWriteArchivesOversizedToolResults(t *testing.T) {
	entryID := uuid.New()
	big := strings.Repeat("很长的工具结果", 4000)
	payload := map[string]interface{}{"role": "tool", "tool_call_id": "call-1", "content": big}

	preview := pruneTranscriptMessageForWrite(entryID, payload, runtimemodel.TranscriptEntryKindTool)
	content := messageContentText(preview["content"])
	if !strings.Contains(content, "ref="+entryID.String()) {
		t.Fatalf("preview lost the ref: %s", content)
	}
	if !strings.Contains(content, "原始长度=") || !strings.Contains(content, "预览（前 200 字）") {
		t.Fatalf("preview is missing the archive header: %s", content)
	}
	if len([]rune(content)) >= len([]rune(big)) {
		t.Fatalf("preview was not shortened: %d vs %d", len([]rune(content)), len([]rune(big)))
	}
	if preview["tool_call_id"] != "call-1" {
		t.Fatalf("preview lost the tool_call_id: %+v", preview)
	}
	if payload["content"] != big {
		t.Fatal("payload was mutated by the preview computation")
	}

	small := map[string]interface{}{"role": "tool", "content": "短结果"}
	if got := pruneTranscriptMessageForWrite(entryID, small, runtimemodel.TranscriptEntryKindTool); got["content"] != "短结果" {
		t.Fatalf("small tool result should stay inline: %+v", got)
	}
	assistant := map[string]interface{}{"role": "assistant", "content": big}
	if got := pruneTranscriptMessageForWrite(entryID, assistant, runtimemodel.TranscriptEntryKindAssistant); got["content"] != big {
		t.Fatal("assistant messages must never be archived")
	}
}

func TestCloseDanglingToolCallsClosesUnmatchedBatch(t *testing.T) {
	messages := []adapter.Message{
		{Role: "user", Content: "帮我退款"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{
			{ID: "call-1", Function: adapter.FunctionCall{Name: "refund_query"}},
			{ID: "call-2", Function: adapter.FunctionCall{Name: "refund_submit"}},
		}},
		{Role: "tool", ToolCallID: "call-1", Content: "订单 12345"},
	}

	closed, synthesized := closeDanglingToolCalls(messages)
	if len(synthesized) != 1 {
		t.Fatalf("synthesized = %d, want 1", len(synthesized))
	}
	if synthesized[0].ToolCallID != "call-2" {
		t.Fatalf("synthesized the wrong call: %+v", synthesized[0])
	}
	content := messageContentText(synthesized[0].Content)
	if !strings.Contains(content, "结果未记录") || !strings.Contains(content, "不要盲目重试") {
		t.Fatalf("synthetic result must warn against blind retry: %s", content)
	}
	if len(closed) != 4 {
		t.Fatalf("closed messages = %d, want 4", len(closed))
	}
}

func TestSnapContextWindowStartKeepsToolBatchWhole(t *testing.T) {
	messages := []adapter.Message{
		{Role: "user", Content: "第一轮"},
		{Role: "assistant", ToolCalls: []adapter.ToolCall{{ID: "call-1"}}},
		{Role: "tool", ToolCallID: "call-1", Content: "结果"},
		{Role: "assistant", Content: "回答"},
	}
	if got := snapContextWindowStart(messages, 2); got != 1 {
		t.Fatalf("window start = %d, want 1 so the assistant tool call stays inside", got)
	}
	if got := snapContextWindowStart(messages, 3); got != 3 {
		t.Fatalf("window start = %d, want 3 (already a legal boundary)", got)
	}
	if got := snapContextWindowStart(messages, 0); got != 0 {
		t.Fatalf("window start = %d, want 0", got)
	}
}

func TestContextEntrySyncAppendsTranscriptAsIdempotentEntries(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-sync")
	ctx := context.Background()
	prepared := &PreparedChat{Message: message, Conversation: conversation}
	writer, err := svc.agentContextEntrySync(ctx, prepared)
	if err != nil {
		t.Fatalf("agentContextEntrySync: %v", err)
	}
	if writer == nil {
		t.Fatal("expected an entry writer")
	}

	transcript := testProtocolTranscript()
	if err := writer.sync(ctx, svc.repos, transcript); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if err := writer.sync(ctx, svc.repos, transcript); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	entries, err := svc.repos.TranscriptEntry.ListPreviewsByMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("ListPreviewsByMessage: %v", err)
	}
	if len(entries) != len(transcript) {
		t.Fatalf("entries = %d, want %d (repeat sync must not duplicate)", len(entries), len(transcript))
	}
	for index, entry := range entries {
		if entry.Seq != index+1 || entry.CSeq != int64(index+1) {
			t.Fatalf("entry %d positions = (seq %d, cseq %d)", index, entry.Seq, entry.CSeq)
		}
	}

	extended := append(testProtocolTranscript(), adapter.Message{Role: "user", Content: "还有一笔"})
	if err := writer.sync(ctx, svc.repos, extended); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	entries, err = svc.repos.TranscriptEntry.ListPreviewsByMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("ListPreviewsByMessage after append: %v", err)
	}
	if len(entries) != len(extended) {
		t.Fatalf("entries = %d, want %d", len(entries), len(extended))
	}

	boundary, ok := writer.boundaryForCompactInput(transcript[:3])
	if !ok {
		t.Fatal("boundary for the first three messages was not resolved")
	}
	if boundary != 3 {
		t.Fatalf("boundary = %d, want 3 (last summarised entry)", boundary)
	}
	if _, ok := writer.boundaryForCompactInput([]adapter.Message{{Role: "user", Content: "不在日志里的消息"}}); ok {
		t.Fatal("an unmatched compact input must not claim a boundary")
	}
}

func TestLoadProjectedConversationContextRebuildsFromEntriesThenFallsBack(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-load")
	ctx := context.Background()
	prepared := &PreparedChat{Message: message, Conversation: conversation}
	writer, err := svc.agentContextEntrySync(ctx, prepared)
	if err != nil {
		t.Fatalf("agentContextEntrySync: %v", err)
	}
	if err := writer.sync(ctx, svc.repos, testProtocolTranscript()[:3]); err != nil {
		t.Fatalf("sync: %v", err)
	}

	messages, ok, err := svc.loadProjectedConversationContext(ctx, conversation.ID, 0)
	if err != nil || !ok {
		t.Fatalf("loadProjectedConversationContext = (%v, %v)", ok, err)
	}
	if len(messages) != 3 {
		t.Fatalf("projected messages = %d, want 3", len(messages))
	}

	if err := svc.repos.Message.UpdateContextSummary(ctx, message.ID, "更早对话的摘要", 2); err != nil {
		t.Fatalf("UpdateContextSummary: %v", err)
	}
	messages, ok, err = svc.loadProjectedConversationContext(ctx, conversation.ID, 3)
	if err != nil || !ok {
		t.Fatalf("loadProjectedConversationContext with summary = (%v, %v)", ok, err)
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2 (summary + the entry after the boundary)", len(messages))
	}
	if !strings.Contains(messageContentText(messages[0].Content), "更早对话的摘要") {
		t.Fatalf("first message is not the summary: %+v", messages[0])
	}
	if messageContentText(messages[1].Content) != "订单 12345：已支付 199 元" {
		t.Fatalf("entry after the boundary is missing: %+v", messages[1])
	}

	empty, ok, err := svc.loadProjectedConversationContext(ctx, uuid.New(), 0)
	if err != nil || ok || empty != nil {
		t.Fatalf("conversation without entries must report a miss: (%v, %v, %v)", empty, ok, err)
	}
}

func TestHistoricalAgentTranscriptPrefersEntriesOverLegacyMetadata(t *testing.T) {
	svc, message, conversation := newContextEntryTestService(t, "context-entry-history")
	ctx := context.Background()
	message.Metadata = map[string]interface{}{
		agentTranscriptVersionMetadataKey: agentTranscriptSchemaVersion,
		agentTranscriptMetadataKey: []interface{}{
			map[string]interface{}{"role": "assistant", "content": "旧快照回答"},
		},
	}
	legacy := svc.historicalAgentTranscript(ctx, message)
	if len(legacy) != 1 || messageContentText(legacy[0].Content) != "旧快照回答" {
		t.Fatalf("legacy metadata fallback = %+v", legacy)
	}

	prepared := &PreparedChat{Message: message, Conversation: conversation}
	writer, err := svc.agentContextEntrySync(ctx, prepared)
	if err != nil {
		t.Fatalf("agentContextEntrySync: %v", err)
	}
	if err := writer.sync(ctx, svc.repos, testProtocolTranscript()); err != nil {
		t.Fatalf("sync: %v", err)
	}

	projected := svc.historicalAgentTranscript(ctx, message)
	if len(projected) != 4 {
		t.Fatalf("projected messages = %d, want the 4 stored entries", len(projected))
	}
	if messageContentText(projected[3].Content) != "已为你提交退款" {
		t.Fatalf("entries did not win over metadata: %+v", projected[3])
	}
}
