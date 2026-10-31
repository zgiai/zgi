package repository

import (
	"context"
	"testing"

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
	if err := db.AutoMigrate(&runtimemodel.Conversation{}, &runtimemodel.Message{}, &runtimemodel.TranscriptEntry{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

func seedTranscriptMessage(t *testing.T, db *gorm.DB) (*runtimemodel.Conversation, *runtimemodel.Message) {
	t.Helper()
	conversation := &runtimemodel.Conversation{ID: uuid.New(), OrganizationID: uuid.New(), AccountID: uuid.New(), Title: "test"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	message := &runtimemodel.Message{ID: uuid.New(), ConversationID: conversation.ID, Query: "query", Status: runtimemodel.MessageStatusStreaming, ModelName: "test", ModelParameters: map[string]interface{}{}, Metadata: map[string]interface{}{}}
	if err := db.Create(message).Error; err != nil {
		t.Fatalf("seed message: %v", err)
	}
	return conversation, message
}

func chainEntry(conversation *runtimemodel.Conversation, message *runtimemodel.Message, parent *uuid.UUID, kind, content, event string) *runtimemodel.TranscriptEntry {
	payload := map[string]interface{}{"role": kind, "content": content}
	return &runtimemodel.TranscriptEntry{ID: uuid.New(), MessageID: message.ID, ConversationID: conversation.ID, OrganizationID: conversation.OrganizationID, ParentEntryID: parent, Kind: kind, Preview: payload, Payload: payload, EventID: event, ByteSize: len(content)}
}

func TestTranscriptEntryAppendAndParentProjection(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-parent")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversation, message := seedTranscriptMessage(t, db)
	first := chainEntry(conversation, message, nil, runtimemodel.TranscriptEntryKindUser, "hello", "user")
	second := chainEntry(conversation, message, &first.ID, runtimemodel.TranscriptEntryKindAssistant, "world", "assistant")
	head, err := repo.AppendForMessage(ctx, message.ID, nil, []*runtimemodel.TranscriptEntry{first, second})
	if err != nil || head != second.ID {
		t.Fatalf("append head = %v, err = %v", head, err)
	}
	var stored runtimemodel.Message
	if err := db.First(&stored, "id = ?", message.ID).Error; err != nil || stored.TranscriptHeadEntryID == nil || *stored.TranscriptHeadEntryID != second.ID {
		t.Fatalf("stored head = %v, err = %v", stored.TranscriptHeadEntryID, err)
	}
	chain, err := repo.LoadPreviewChain(ctx, second.ID, nil, 0)
	if err != nil || len(chain) != 2 || chain[0].ID != second.ID || chain[1].ID != first.ID {
		t.Fatalf("reverse chain = %+v, err = %v", chain, err)
	}
}

func TestListPreviewsByMessageFiltersSharedParentChain(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-shared-message-chain")
	repos := NewRepositories(db)
	ctx := context.Background()
	conversation, parent := seedTranscriptMessage(t, db)
	parentEntry := chainEntry(conversation, parent, nil, runtimemodel.TranscriptEntryKindAssistant, "parent", "parent")
	parentHead, err := repos.TranscriptEntry.AppendForMessage(ctx, parent.ID, nil, []*runtimemodel.TranscriptEntry{parentEntry})
	if err != nil {
		t.Fatal(err)
	}
	child := &runtimemodel.Message{
		ID:              uuid.New(),
		ConversationID:  conversation.ID,
		ParentID:        &parent.ID,
		Query:           "child",
		Status:          runtimemodel.MessageStatusStreaming,
		ModelName:       "test",
		ModelParameters: map[string]interface{}{},
		Metadata:        map[string]interface{}{},
	}
	if err := repos.Message.Create(ctx, child); err != nil {
		t.Fatal(err)
	}
	childEntry := chainEntry(conversation, child, &parentHead, runtimemodel.TranscriptEntryKindAssistant, "child", "child")
	if _, err := repos.TranscriptEntry.AppendForMessage(ctx, child.ID, &parentHead, []*runtimemodel.TranscriptEntry{childEntry}); err != nil {
		t.Fatal(err)
	}
	parentPreview, err := repos.TranscriptEntry.ListPreviewsByMessage(ctx, parent.ID)
	if err != nil || len(parentPreview) != 1 || parentPreview[0].ID != parentEntry.ID {
		t.Fatalf("parent previews = %+v/%v", parentPreview, err)
	}
	childPreview, err := repos.TranscriptEntry.ListPreviewsByMessage(ctx, child.ID)
	if err != nil || len(childPreview) != 1 || childPreview[0].ID != childEntry.ID {
		t.Fatalf("child previews = %+v/%v", childPreview, err)
	}
}

func TestTranscriptSummaryIsIdempotentAndCopyReadsParentChain(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-summary-copy")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversation, message := seedTranscriptMessage(t, db)
	entry := chainEntry(conversation, message, nil, runtimemodel.TranscriptEntryKindUser, "hello", "user")
	head, err := repo.AppendForMessage(ctx, message.ID, nil, []*runtimemodel.TranscriptEntry{entry})
	if err != nil {
		t.Fatal(err)
	}
	summaryHead, err := repo.AppendSummaryForMessage(ctx, message.ID, &head, &entry.ID, "summary", "summary:event")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := repo.AppendSummaryForMessage(ctx, message.ID, &head, &entry.ID, "summary", "summary:event")
	if err != nil || replayed != summaryHead {
		t.Fatalf("summary replay = %v/%v, want %v", replayed, err, summaryHead)
	}
	copied, err := repo.ListForCopyByHead(ctx, summaryHead, nil)
	if err != nil || len(copied) != 2 {
		t.Fatalf("copy source = %d/%v", len(copied), err)
	}
}

func TestTranscriptEntryRejectsStaleHeadAndUnreachableSummaryBoundary(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-cas-boundary")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversation, message := seedTranscriptMessage(t, db)
	first := chainEntry(conversation, message, nil, runtimemodel.TranscriptEntryKindUser, "hello", "user")
	head, err := repo.AppendForMessage(ctx, message.ID, nil, []*runtimemodel.TranscriptEntry{first})
	if err != nil {
		t.Fatal(err)
	}
	stale := chainEntry(conversation, message, nil, runtimemodel.TranscriptEntryKindAssistant, "stale", "stale")
	if _, err := repo.AppendForMessage(ctx, message.ID, nil, []*runtimemodel.TranscriptEntry{stale}); err == nil {
		t.Fatal("expected stale head CAS to fail")
	}
	unreachable := uuid.New()
	if _, err := repo.AppendSummaryForMessage(ctx, message.ID, &head, &unreachable, "summary", "summary:bad"); err == nil {
		t.Fatal("expected unreachable summary boundary to fail")
	}
}

func TestTranscriptEntryLoadDetectsParentCycle(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-cycle")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversation, message := seedTranscriptMessage(t, db)
	first := chainEntry(conversation, message, nil, runtimemodel.TranscriptEntryKindUser, "one", "one")
	second := chainEntry(conversation, message, &first.ID, runtimemodel.TranscriptEntryKindAssistant, "two", "two")
	if _, err := repo.AppendForMessage(ctx, message.ID, nil, []*runtimemodel.TranscriptEntry{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&runtimemodel.TranscriptEntry{}).Where("id = ?", first.ID).Update("parent_entry_id", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadPreviewChain(ctx, second.ID, nil, 0); err == nil {
		t.Fatal("expected parent cycle to fail")
	}
}

// A preview equal to its payload is stored once and read back as the payload;
// the chain stops at stopAt and at the boundary of the newest summary, and a
// missing parent is an error (ADR 0040).
func TestTranscriptPreviewIsStoredOnceAndChainStops(t *testing.T) {
	db := newTranscriptEntryTestDB(t, "transcript-preview-once")
	repo := NewTranscriptEntryRepository(db)
	ctx := context.Background()
	conversation, message := seedTranscriptMessage(t, db)
	first := chainEntry(conversation, message, nil, runtimemodel.TranscriptEntryKindUser, "one", "one")
	second := chainEntry(conversation, message, &first.ID, runtimemodel.TranscriptEntryKindTool, "two", "two")
	second.Preview = map[string]interface{}{"role": "tool", "content": "receipt"}
	third := chainEntry(conversation, message, &second.ID, runtimemodel.TranscriptEntryKindAssistant, "three", "three")
	head, err := repo.AppendForMessage(ctx, message.ID, nil, []*runtimemodel.TranscriptEntry{first, second, third})
	if err != nil {
		t.Fatal(err)
	}
	if first.Preview == nil {
		t.Fatal("the caller's preview was not restored")
	}
	var stored int64
	db.Model(&runtimemodel.TranscriptEntry{}).Where("preview IS NULL").Count(&stored)
	if stored != 2 {
		t.Fatalf("entries with a NULL preview = %d, want 2", stored)
	}
	chain, err := repo.LoadPreviewChain(ctx, head, nil, 0)
	if err != nil || len(chain) != 3 || chain[2].Preview["content"] != "one" || chain[1].Preview["content"] != "receipt" {
		t.Fatalf("chain = %+v, err = %v", chain, err)
	}
	if chain, err = repo.LoadPreviewChain(ctx, head, &second.ID, 0); err != nil || len(chain) != 2 {
		t.Fatalf("chain to stopAt = %d, err = %v", len(chain), err)
	}
	if _, err = repo.LoadPreviewChain(ctx, head, nil, 2); err == nil {
		t.Fatal("expected a chain over the limit to fail")
	}

	summary, err := repo.AppendSummaryForMessage(ctx, message.ID, &head, &second.ID, "summary", "summary:event")
	if err != nil {
		t.Fatal(err)
	}
	chain, err = repo.LoadPreviewChain(ctx, summary, nil, 0)
	if err != nil || len(chain) != 3 || chain[0].Preview["content"] != "summary" || chain[2].ID != second.ID {
		t.Fatalf("chain through summary = %+v, err = %v", chain, err)
	}
	copied, err := repo.ListForCopyByHead(ctx, head, nil)
	if err != nil || len(copied) != 3 || copied[0].Preview["content"] != "one" {
		t.Fatalf("copy = %+v, err = %v", copied, err)
	}

	if err := db.Where("id = ?", first.ID).Delete(&runtimemodel.TranscriptEntry{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadPreviewChain(ctx, head, nil, 0); err == nil {
		t.Fatal("expected a missing parent to fail")
	}
}
