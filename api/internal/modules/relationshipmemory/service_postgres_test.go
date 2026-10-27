package relationshipmemory

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRelationshipMemoryPostgresIsolationCASAndEpochFence(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run relationship memory PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	service := NewService(tx)
	organizationID, workspaceID, agentID := uuid.New(), uuid.New(), uuid.New()
	userA, userB := usersubject.AccountSubject(uuid.New()), usersubject.AccountSubject(uuid.New())
	relationshipA, err := service.EnsureRelationship(t.Context(), EnsureRelationshipRequest{
		OrganizationID: organizationID, WorkspaceID: workspaceID, Subject: userA,
		ConsumerType: ConsumerAgent, ConsumerID: agentID, ConsumerNameSnapshot: "Support",
	})
	if err != nil {
		t.Fatal(err)
	}
	relationshipAAgain, err := service.EnsureRelationship(t.Context(), EnsureRelationshipRequest{
		OrganizationID: organizationID, WorkspaceID: workspaceID, Subject: userA,
		ConsumerType: ConsumerAgent, ConsumerID: agentID, ConsumerNameSnapshot: "Support",
	})
	if err != nil {
		t.Fatalf("ensure existing relationship: %v", err)
	}
	if relationshipAAgain.ID != relationshipA.ID {
		t.Fatalf("idempotent relationship ID = %s, want %s", relationshipAAgain.ID, relationshipA.ID)
	}
	relationshipB, err := service.EnsureRelationship(t.Context(), EnsureRelationshipRequest{
		OrganizationID: organizationID, WorkspaceID: workspaceID, Subject: userB,
		ConsumerType: ConsumerAgent, ConsumerID: agentID, ConsumerNameSnapshot: "Support",
	})
	if err != nil {
		t.Fatal(err)
	}
	contract := ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", Definitions: []MemoryDefinition{{Key: "preference", Enabled: true, MaxChars: 2000}}, RenderBudgetChars: 4000}
	createOperationID := uuid.New()
	created, err := service.PutItem(t.Context(), relationshipA.ID, PutItemRequest{
		Key: "preference", Content: "Prefers concise answers.", ExpectedRevision: 0, Contract: contract,
		Metadata: MutationMetadata{OperationID: createOperationID, SourceKind: SourceExplicit, EvidenceFromUserInput: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.PutItem(t.Context(), relationshipA.ID, PutItemRequest{
		Key: "preference", Content: "Prefers concise answers.", ExpectedRevision: 0, Contract: contract,
		Metadata: MutationMetadata{OperationID: createOperationID, SourceKind: SourceExplicit, EvidenceFromUserInput: true},
	})
	if err != nil || replayed.Revision != created.Revision {
		t.Fatalf("idempotent replay = %#v, err = %v", replayed, err)
	}
	if _, err := service.PutItem(t.Context(), relationshipA.ID, PutItemRequest{
		Key: "preference", Content: "Stale overwrite.", ExpectedRevision: 0, Contract: contract,
		Metadata: MutationMetadata{OperationID: uuid.New(), SourceKind: SourceExplicit, EvidenceFromUserInput: true},
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale write error = %v", err)
	}
	if rendered, items, err := service.RenderContext(t.Context(), relationshipB.ID, contract); err != nil || rendered != "" || len(items) != 0 {
		t.Fatalf("second user received first user's memory: rendered=%q items=%#v err=%v", rendered, items, err)
	}
	deleted, err := service.PutItem(t.Context(), relationshipB.ID, PutItemRequest{
		Key: "preference", DisplayName: "Preference", Content: "Prefers detailed answers.", ExpectedRevision: 0, Contract: contract,
		Metadata: MutationMetadata{OperationID: uuid.New(), SourceKind: SourceExplicit, EvidenceFromUserInput: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	deleteOperationID := uuid.New()
	deleteResult, err := service.DeleteItem(t.Context(), relationshipB.ID, "preference", deleted.Revision, MutationMetadata{OperationID: deleteOperationID, SourceKind: SourceManager})
	if err != nil {
		t.Fatal(err)
	}
	if !deleteResult.Deleted || deleteResult.UndoableUntil == nil {
		t.Fatalf("delete result = %#v", deleteResult)
	}
	restored, err := service.Undo(t.Context(), relationshipB.ID, deleteOperationID)
	if err != nil {
		t.Fatal(err)
	}
	if restored == nil || restored.ID != deleted.ID || restored.ContentPayload != "Prefers detailed answers." || restored.Revision != deleted.Revision+2 {
		t.Fatalf("restored deletion = %#v", restored)
	}
	if _, err := service.DeleteItem(t.Context(), relationshipB.ID, "preference", deleted.Revision, MutationMetadata{OperationID: deleteOperationID, SourceKind: SourceManager}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("replayed delete after undo error = %v", err)
	}

	job, err := service.ScheduleExtraction(t.Context(), ScheduleRequest{
		RelationshipID: relationshipA.ID, ConversationID: uuid.New(), MessageWatermarkID: uuid.New(),
		MemoryEpoch: relationshipA.MemoryEpoch, ConfigRevision: "published-v1", Definitions: contract.Definitions,
		ExtractorVersion: "relationship-memory-test", IdempotencyKey: uuid.NewString(),
		ScheduledAt: time.Now(), ForceAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ClearItems(t.Context(), relationshipA.ID); err != nil {
		t.Fatal(err)
	}
	var storedJob UserMemoryExtractionJob
	if err := tx.First(&storedJob, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedJob.Status != "cancelled" {
		t.Fatalf("job status = %q", storedJob.Status)
	}
	oldEpoch := relationshipA.MemoryEpoch
	if _, err := service.PutItem(t.Context(), relationshipA.ID, PutItemRequest{
		Key: "preference", Content: "Should not be resurrected.", ExpectedRevision: 0, Contract: contract,
		Metadata: MutationMetadata{OperationID: uuid.New(), SourceKind: SourceAutomatic, ExpectedMemoryEpoch: &oldEpoch, EvidenceFromUserInput: true},
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("old epoch write error = %v", err)
	}
	if created.ContentPayload != "Prefers concise answers." {
		t.Fatalf("decoded content = %q", created.ContentPayload)
	}
}
