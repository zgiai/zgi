package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRuntimeInputRepositorySubmitIsIdempotentAndOrdered(t *testing.T) {
	db, conversation, targetMessageID := openRuntimeInputRepositoryTestDB(t)
	repo := NewRuntimeInputRepository(db)
	ctx := context.Background()
	clientInputID := uuid.New()

	first, created, err := repo.Submit(ctx, newRuntimeInput(conversation, targetMessageID, clientInputID, "first"))
	if err != nil {
		t.Fatalf("submit first runtime input: %v", err)
	}
	if !created || first.Sequence != 1 {
		t.Fatalf("first input created=%v sequence=%d, want true/1", created, first.Sequence)
	}

	retry, created, err := repo.Submit(ctx, newRuntimeInput(conversation, targetMessageID, clientInputID, "duplicate body"))
	if err != nil {
		t.Fatalf("retry runtime input: %v", err)
	}
	if created || retry.ID != first.ID || retry.Content != "first" {
		t.Fatalf("retry created=%v id=%s content=%q, want existing %s", created, retry.ID, retry.Content, first.ID)
	}

	second, created, err := repo.Submit(ctx, newRuntimeInput(conversation, targetMessageID, uuid.New(), "second"))
	if err != nil {
		t.Fatalf("submit second runtime input: %v", err)
	}
	if !created || second.Sequence != 2 {
		t.Fatalf("second input created=%v sequence=%d, want true/2", created, second.Sequence)
	}

	items, err := repo.ListByConversation(ctx, conversation.ID, conversation.OrganizationID, conversation.AccountID)
	if err != nil {
		t.Fatalf("list runtime inputs: %v", err)
	}
	if len(items) != 2 || items[0].ID != first.ID || items[1].ID != second.ID {
		t.Fatalf("runtime inputs are not stable and ordered: %#v", items)
	}
}

func TestRuntimeInputRepositoryClaimConsumeAndFollowUpReservation(t *testing.T) {
	db, conversation, targetMessageID := openRuntimeInputRepositoryTestDB(t)
	repo := NewRuntimeInputRepository(db)
	ctx := context.Background()

	for _, content := range []string{"first", "second"} {
		if _, _, err := repo.Submit(ctx, newRuntimeInput(conversation, targetMessageID, uuid.New(), content)); err != nil {
			t.Fatalf("submit %q: %v", content, err)
		}
	}

	batchID := uuid.New()
	now := time.Now().UTC()
	claimed, err := repo.ClaimPending(ctx, targetMessageID, runtimemodel.RuntimeInputConsumptionFollowUp, &batchID, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim runtime inputs: %v", err)
	}
	if len(claimed) != 2 || claimed[0].Sequence != 1 || claimed[1].Sequence != 2 {
		t.Fatalf("claimed inputs = %#v, want two ordered items", claimed)
	}

	pending, err := repo.HasPendingFollowUp(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("check pending follow-up: %v", err)
	}
	if !pending {
		t.Fatal("expected claimed follow-up to reserve the conversation")
	}

	consumedMessageID := uuid.New()
	if err := repo.MarkBatchConsumed(ctx, batchID, consumedMessageID, now.Add(time.Second)); err != nil {
		t.Fatalf("consume runtime input batch: %v", err)
	}
	pending, err = repo.HasPendingFollowUp(ctx, conversation.ID)
	if err != nil {
		t.Fatalf("recheck pending follow-up: %v", err)
	}
	if pending {
		t.Fatal("consumed follow-up must release the conversation reservation")
	}

	items, err := repo.ListByBatch(ctx, batchID)
	if err != nil {
		t.Fatalf("list consumed batch: %v", err)
	}
	for _, item := range items {
		if item.Status != runtimemodel.RuntimeInputStatusConsumed || item.ConsumedMessageID == nil || *item.ConsumedMessageID != consumedMessageID {
			t.Fatalf("consumed input not projected correctly: %#v", item)
		}
	}
}

func TestRuntimeInputRepositoryCurrentRunClaimLeavesFollowUpOnlyPending(t *testing.T) {
	db, conversation, targetMessageID := openRuntimeInputRepositoryTestDB(t)
	repo := NewRuntimeInputRepository(db)
	ctx := context.Background()

	current := newRuntimeInput(conversation, targetMessageID, uuid.New(), "same sender supplement")
	if _, _, err := repo.Submit(ctx, current); err != nil {
		t.Fatalf("submit current-run eligible input: %v", err)
	}
	followUp := newRuntimeInput(conversation, targetMessageID, uuid.New(), "queued next turn")
	mode := runtimemodel.RuntimeInputConsumptionFollowUp
	followUp.ConsumptionMode = &mode
	if _, _, err := repo.Submit(ctx, followUp); err != nil {
		t.Fatalf("submit follow-up-only input: %v", err)
	}

	now := time.Now().UTC()
	currentBatchID := uuid.New()
	claimed, err := repo.ClaimPending(ctx, targetMessageID, runtimemodel.RuntimeInputConsumptionCurrentRun, &currentBatchID, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim current-run inputs: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Content != current.Content {
		t.Fatalf("current-run claim = %#v, want only eligible input", claimed)
	}

	followUpBatchID := uuid.New()
	queued, err := repo.ClaimPending(ctx, targetMessageID, runtimemodel.RuntimeInputConsumptionFollowUp, &followUpBatchID, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim follow-up inputs: %v", err)
	}
	if len(queued) != 1 || queued[0].Content != followUp.Content {
		t.Fatalf("follow-up claim = %#v, want only queued input", queued)
	}
}

func TestRuntimeInputRepositoryReleasesRejectedCurrentRunBatch(t *testing.T) {
	db, conversation, targetMessageID := openRuntimeInputRepositoryTestDB(t)
	repo := NewRuntimeInputRepository(db)
	ctx := context.Background()
	input := newRuntimeInput(conversation, targetMessageID, uuid.New(), "retry this supplement")
	if _, _, err := repo.Submit(ctx, input); err != nil {
		t.Fatalf("submit runtime input: %v", err)
	}

	batchID := uuid.New()
	now := time.Now().UTC()
	claimed, err := repo.ClaimPending(ctx, targetMessageID, runtimemodel.RuntimeInputConsumptionCurrentRun, &batchID, now, now.Add(time.Minute))
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim current-run input: items=%d error=%v", len(claimed), err)
	}
	if err := repo.ReleaseCurrentRunBatch(ctx, batchID, now.Add(time.Second)); err != nil {
		t.Fatalf("release current-run batch: %v", err)
	}

	reclaimedBatchID := uuid.New()
	reclaimed, err := repo.ClaimPending(ctx, targetMessageID, runtimemodel.RuntimeInputConsumptionCurrentRun, &reclaimedBatchID, now.Add(2*time.Second), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("reclaim current-run input: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].ID != input.ID {
		t.Fatalf("reclaimed inputs = %#v, want %s", reclaimed, input.ID)
	}
}

func TestRuntimeInputRepositoryCancelPendingFollowUpAndRecoverCurrentRunClaim(t *testing.T) {
	db, conversation, targetMessageID := openRuntimeInputRepositoryTestDB(t)
	repo := NewRuntimeInputRepository(db)
	ctx := context.Background()

	if _, _, err := repo.Submit(ctx, newRuntimeInput(conversation, targetMessageID, uuid.New(), "follow up")); err != nil {
		t.Fatalf("submit follow-up: %v", err)
	}
	batchID := uuid.New()
	now := time.Now().UTC()
	if _, err := repo.ClaimPending(ctx, targetMessageID, runtimemodel.RuntimeInputConsumptionFollowUp, &batchID, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("claim follow-up: %v", err)
	}
	canceled, err := repo.CancelPendingFollowUps(ctx, conversation.ID, now.Add(time.Second))
	if err != nil {
		t.Fatalf("cancel pending follow-up: %v", err)
	}
	if len(canceled) != 1 || canceled[0].Status != runtimemodel.RuntimeInputStatusCanceled {
		t.Fatalf("canceled inputs = %#v, want one canceled input", canceled)
	}

	if _, _, err := repo.Submit(ctx, newRuntimeInput(conversation, targetMessageID, uuid.New(), "current run")); err != nil {
		t.Fatalf("submit current-run input: %v", err)
	}
	currentBatchID := uuid.New()
	expiredAt := now.Add(-time.Minute)
	if _, err := repo.ClaimPending(ctx, targetMessageID, runtimemodel.RuntimeInputConsumptionCurrentRun, &currentBatchID, now.Add(-2*time.Minute), expiredAt); err != nil {
		t.Fatalf("claim current-run input: %v", err)
	}
	recovered, err := repo.RecoverStaleClaims(ctx, now)
	if err != nil {
		t.Fatalf("recover stale claims: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered %d claims, want 1", recovered)
	}

	items, err := repo.ListByConversation(ctx, conversation.ID, conversation.OrganizationID, conversation.AccountID)
	if err != nil {
		t.Fatalf("list recovered inputs: %v", err)
	}
	last := items[len(items)-1]
	if last.Status != runtimemodel.RuntimeInputStatusPending || last.ClaimBatchID != nil || last.ConsumptionMode != nil {
		t.Fatalf("stale claim was not restored to pending: %#v", last)
	}
}

func openRuntimeInputRepositoryTestDB(t *testing.T) (*gorm.DB, *runtimemodel.Conversation, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&runtimemodel.Conversation{}, &runtimemodel.Message{}, &runtimemodel.RuntimeInput{}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatal(err)
	}

	targetMessageID := uuid.New()
	conversation := &runtimemodel.Conversation{
		ID:               uuid.New(),
		OrganizationID:   uuid.New(),
		AccountID:        uuid.New(),
		CallerType:       runtimemodel.ConversationCallerAgent,
		ConversationType: runtimemodel.ConversationTypeChat,
		Title:            "runtime input test",
		Status:           runtimemodel.ConversationStatusNormal,
		RuntimeStatus:    runtimemodel.ConversationRuntimeStatusStreaming,
		ActiveMessageID:  &targetMessageID,
		Source:           runtimemodel.ConversationSourceConsole,
		Metadata:         map[string]interface{}{},
	}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := db.Create(&runtimemodel.Message{
		ID:              targetMessageID,
		ConversationID:  conversation.ID,
		Query:           "initial request",
		Status:          runtimemodel.MessageStatusStreaming,
		ModelName:       "test-model",
		ModelParameters: map[string]interface{}{},
		Metadata:        map[string]interface{}{},
	}).Error; err != nil {
		t.Fatalf("create target message: %v", err)
	}
	return db, conversation, targetMessageID
}

func newRuntimeInput(conversation *runtimemodel.Conversation, targetMessageID, clientInputID uuid.UUID, content string) *runtimemodel.RuntimeInput {
	return &runtimemodel.RuntimeInput{
		OrganizationID:  conversation.OrganizationID,
		WorkspaceID:     conversation.WorkspaceID,
		AccountID:       conversation.AccountID,
		ConversationID:  conversation.ID,
		TargetMessageID: targetMessageID,
		ClientInputID:   clientInputID,
		Content:         content,
	}
}
