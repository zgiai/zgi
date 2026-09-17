package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTranscriptEntryTestDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&runtimemodel.Message{}, &runtimemodel.TranscriptEntry{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func seedTranscriptMessage(t *testing.T, db *gorm.DB, conversationID, messageID, organizationID uuid.UUID) {
	t.Helper()
	message := &runtimemodel.Message{
		ID:              messageID,
		ConversationID:  conversationID,
		Query:           "query",
		Answer:          "",
		Status:          runtimemodel.MessageStatusStreaming,
		ModelName:       "test-model",
		ModelParameters: map[string]interface{}{},
		Metadata:        map[string]interface{}{},
	}
	if err := db.Create(message).Error; err != nil {
		t.Fatalf("seed message: %v", err)
	}
	_ = organizationID
}

func transcriptEntry(id uuid.UUID, conversationID, messageID, organizationID uuid.UUID, seq int, cseq int64, kind, content string) *runtimemodel.TranscriptEntry {
	message := map[string]interface{}{"role": kind, "content": content}
	return &runtimemodel.TranscriptEntry{
		ID:             id,
		MessageID:      messageID,
		ConversationID: conversationID,
		OrganizationID: organizationID,
		CSeq:           cseq,
		Seq:            seq,
		Kind:           kind,
		Preview:        message,
		Payload:        message,
		ByteSize:       len(content),
	}
}

func TestTranscriptEntryAppendAssignsOrderAndRoundTripsPreviews(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-append")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversationID, messageID, organizationID := uuid.New(), uuid.New(), uuid.New()
	seedTranscriptMessage(t, db, conversationID, messageID, organizationID)

	seq, cseq, err := repo.NextPositions(ctx, conversationID, messageID)
	if err != nil {
		t.Fatalf("NextPositions: %v", err)
	}
	if seq != 1 || cseq != 1 {
		t.Fatalf("first positions = (%d, %d), want (1, 1)", seq, cseq)
	}

	first := transcriptEntry(uuid.New(), conversationID, messageID, organizationID, 1, 1, runtimemodel.TranscriptEntryKindUser, "帮我退款")
	second := transcriptEntry(uuid.New(), conversationID, messageID, organizationID, 2, 2, runtimemodel.TranscriptEntryKindTool, "订单 12345")
	if err := repo.Append(ctx, []*runtimemodel.TranscriptEntry{first, second}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	seq, cseq, err = repo.NextPositions(ctx, conversationID, messageID)
	if err != nil {
		t.Fatalf("NextPositions after append: %v", err)
	}
	if seq != 3 || cseq != 3 {
		t.Fatalf("positions after append = (%d, %d), want (3, 3)", seq, cseq)
	}

	previews, err := repo.ListPreviewsByConversation(ctx, conversationID, 0, 0)
	if err != nil {
		t.Fatalf("ListPreviewsByConversation: %v", err)
	}
	if len(previews) != 2 {
		t.Fatalf("preview rows = %d, want 2", len(previews))
	}
	if previews[0].CSeq != 1 || previews[1].CSeq != 2 {
		t.Fatalf("previews out of order: %d, %d", previews[0].CSeq, previews[1].CSeq)
	}
	if previews[0].Preview["content"] != "帮我退款" {
		t.Fatalf("preview content = %v, want 帮我退款", previews[0].Preview["content"])
	}
	if previews[0].MessageID != messageID || previews[1].Kind != runtimemodel.TranscriptEntryKindTool {
		t.Fatalf("preview scope mismatch: %+v", previews[0])
	}
}

func TestTranscriptEntryAppendRejectsDuplicatePositions(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-duplicate")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversationID, messageID, organizationID := uuid.New(), uuid.New(), uuid.New()
	seedTranscriptMessage(t, db, conversationID, messageID, organizationID)

	entry := transcriptEntry(uuid.New(), conversationID, messageID, organizationID, 1, 1, runtimemodel.TranscriptEntryKindUser, "hello")
	if err := repo.Append(ctx, []*runtimemodel.TranscriptEntry{entry}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	duplicate := transcriptEntry(uuid.New(), conversationID, messageID, organizationID, 1, 1, runtimemodel.TranscriptEntryKindUser, "hello again")
	if err := repo.Append(ctx, []*runtimemodel.TranscriptEntry{duplicate}); err == nil {
		t.Fatal("expected duplicate (message_id, seq) to be rejected")
	}
}

func TestTranscriptEntryUpdatePreviewKeepsPayloadAndFiltersByCSeq(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-update-preview")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversationID, messageID, organizationID := uuid.New(), uuid.New(), uuid.New()
	seedTranscriptMessage(t, db, conversationID, messageID, organizationID)

	entry := transcriptEntry(uuid.New(), conversationID, messageID, organizationID, 1, 1, runtimemodel.TranscriptEntryKindTool, "900KB 原始工具结果")
	if err := repo.Append(ctx, []*runtimemodel.TranscriptEntry{entry}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	pruned := map[string]interface{}{"role": "tool", "content": "[工具结果已归档] ref=" + entry.ID.String()}
	if err := repo.UpdatePreview(ctx, entry.ID, pruned, 42); err != nil {
		t.Fatalf("UpdatePreview: %v", err)
	}

	payloads, err := repo.GetPayload(ctx, entry.ID, organizationID)
	if err != nil {
		t.Fatalf("GetPayload: %v", err)
	}
	if payloads.Payload["content"] != "900KB 原始工具结果" {
		t.Fatalf("payload was modified: %v", payloads.Payload["content"])
	}

	previews, err := repo.ListPreviewsByConversation(ctx, conversationID, 0, 0)
	if err != nil {
		t.Fatalf("ListPreviewsByConversation: %v", err)
	}
	if len(previews) != 1 || previews[0].Preview["content"] != pruned["content"] || previews[0].ByteSize != 42 {
		t.Fatalf("preview not updated: %+v", previews[0])
	}

	after, err := repo.ListPreviewsByConversation(ctx, conversationID, 1, 0)
	if err != nil {
		t.Fatalf("ListPreviewsByConversation after boundary: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("cseq boundary filter returned %d rows, want 0", len(after))
	}
}

func TestTranscriptEntryGetPayloadIsScopedToOrganization(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-org-scope")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversationID, messageID, organizationID := uuid.New(), uuid.New(), uuid.New()
	seedTranscriptMessage(t, db, conversationID, messageID, organizationID)

	entry := transcriptEntry(uuid.New(), conversationID, messageID, organizationID, 1, 1, runtimemodel.TranscriptEntryKindTool, "secret")
	if err := repo.Append(ctx, []*runtimemodel.TranscriptEntry{entry}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	if _, err := repo.GetPayload(ctx, entry.ID, uuid.New()); err == nil {
		t.Fatal("expected cross-organization lookup to fail")
	}
}

func TestTranscriptEntryDeleteByConversationRemovesFacts(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-delete-conversation")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversationID, messageID, organizationID := uuid.New(), uuid.New(), uuid.New()
	seedTranscriptMessage(t, db, conversationID, messageID, organizationID)

	entry := transcriptEntry(uuid.New(), conversationID, messageID, organizationID, 1, 1, runtimemodel.TranscriptEntryKindUser, "hello")
	if err := repo.Append(ctx, []*runtimemodel.TranscriptEntry{entry}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	deleted, err := repo.DeleteByConversation(ctx, conversationID)
	if err != nil {
		t.Fatalf("DeleteByConversation: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted rows = %d, want 1", deleted)
	}
	remaining, err := repo.ListPreviewsByConversation(ctx, conversationID, 0, 0)
	if err != nil {
		t.Fatalf("ListPreviewsByConversation: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("conversation still has %d entries", len(remaining))
	}
}

func TestTranscriptEntryPreviewQueryNeverSelectsPayload(t *testing.T) {
	db, mock := newConversationRepositoryMockDB(t)
	repo := NewTranscriptEntryRepository(db)
	conversationID := uuid.New()

	// The whitelist is the guard: if a projection/list query ever selects
	// payload again, this expectation stops matching and the test fails.
	mock.ExpectQuery(`(?s)SELECT id, message_id, cseq, seq, kind, preview, byte_size FROM "chat_runtime_message_transcript_entries" WHERE conversation_id = \$1 AND cseq > \$2 ORDER BY cseq ASC`).
		WithArgs(conversationID, int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "message_id", "cseq", "seq", "kind", "preview", "byte_size"}))

	if _, err := repo.ListPreviewsByConversation(context.Background(), conversationID, 7, 0); err != nil {
		t.Fatalf("ListPreviewsByConversation: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMessageContextSummaryRoundTripAndBoundaryFilter(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "message-context-summary")
	repo := NewMessageRepository(db)
	ctx := context.Background()
	conversationID, firstMessageID, secondMessageID, organizationID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	seedTranscriptMessage(t, db, conversationID, firstMessageID, organizationID)
	seedTranscriptMessage(t, db, conversationID, secondMessageID, organizationID)

	if summary, err := repo.ContextSummaryAt(ctx, conversationID, 100); err != nil || summary != nil {
		t.Fatalf("empty conversation summary = (%v, %v), want (nil, nil)", summary, err)
	}

	if err := repo.UpdateContextSummary(ctx, firstMessageID, "第一段摘要", 40); err != nil {
		t.Fatalf("UpdateContextSummary: %v", err)
	}
	if err := repo.UpdateContextSummary(ctx, secondMessageID, "第二段摘要", 90); err != nil {
		t.Fatalf("UpdateContextSummary: %v", err)
	}

	summary, err := repo.ContextSummaryAt(ctx, conversationID, 95)
	if err != nil {
		t.Fatalf("ContextSummaryAt: %v", err)
	}
	if summary == nil || summary.ThroughSeq != 90 || summary.Summary != "第二段摘要" || summary.MessageID != secondMessageID {
		t.Fatalf("latest applicable summary = %+v", summary)
	}

	older, err := repo.ContextSummaryAt(ctx, conversationID, 40)
	if err != nil {
		t.Fatalf("ContextSummaryAt older: %v", err)
	}
	if older == nil || older.ThroughSeq != 40 || older.MessageID != firstMessageID {
		t.Fatalf("older applicable summary = %+v", older)
	}

	if none, err := repo.ContextSummaryAt(ctx, conversationID, 10); err != nil || none != nil {
		t.Fatalf("summary before first boundary = (%v, %v), want (nil, nil)", none, err)
	}
}
