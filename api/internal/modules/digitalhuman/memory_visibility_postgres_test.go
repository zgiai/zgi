package digitalhuman

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDigitalHumanMemoryVisibilityPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman memory visibility PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	organizationID, workspaceID, actorID := uuid.New(), uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO organizations (id, name) VALUES (?, ?)", []any{organizationID, "Memory rescope integration"}},
		{"INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", []any{workspaceID, organizationID, "Memory rescope integration"}},
		{"INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", []any{actorID, "Memory operator", "memory-rescope-" + actorID.String() + "@example.invalid"}},
	} {
		if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(NewRepository(tx))
	fixedNow := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
	detail, err := service.Create(t.Context(), scope, CreateRequest{
		Name: "Rescope subject", Persona: "Keep shared facts in business memory.", DefaultModel: "test-model",
		MemoryPolicy: map[string]any{"automatic_extraction": false}, TriggerPolicy: map[string]any{"manual": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	digitalHumanID := detail.Definition.ID
	state, err := service.memoryState(t.Context(), scope, digitalHumanID)
	if err != nil {
		t.Fatal(err)
	}
	write := func(key string, memoryType memoryruntime.MemoryType) *DigitalHumanMemory {
		t.Helper()
		result, err := service.ApplyMemoryProposal(t.Context(), scope, digitalHumanID, MemoryProposalRequest{
			OperationID: uuid.New(), Key: key, Action: memoryruntime.MutationActionUpsert,
			MemoryType: memoryType, Content: key + " content", ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
			Evidence:   []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "operator:" + key}},
			Confidence: 1, OccurredAt: fixedNow, AdminApproved: true,
			SourceContext: MemorySourceContext{PromotedByAuthorizedActor: true},
		})
		if err != nil {
			t.Fatalf("write %s: %v", key, err)
		}
		return result.Memory
	}
	// An operator switches who may see a memory; each switch is a recorded,
	// undoable change.
	public := write("price-list", memoryruntime.MemoryTypeFact)
	if public.Visibility != MemoryVisibilityEveryone || public.Origin != MemoryOriginOperator {
		t.Fatalf("operator memory = %#v", public)
	}
	setVisibility := func(memory *DigitalHumanMemory, visibility MemoryVisibility) (*MemoryMutationResult, error) {
		return service.SetMemoryVisibility(t.Context(), scope, digitalHumanID, memory.ID, MemoryVisibilityRequest{
			OperationID: uuid.New(), ExpectedRevision: memory.Revision, ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch, Visibility: visibility,
		})
	}
	hidden, err := setVisibility(public, MemoryVisibilityInternal)
	if err != nil || hidden.Memory.Visibility != MemoryVisibilityInternal || hidden.Memory.Revision != public.Revision+1 || hidden.Memory.Content != public.Content {
		t.Fatalf("make internal = %#v, %v", hidden, err)
	}
	if _, err := setVisibility(public, MemoryVisibilityEveryone); !errors.Is(err, ErrMemoryConflict) {
		t.Fatalf("stale visibility change error = %v, want conflict", err)
	}
	if _, err := setVisibility(hidden.Memory, "members"); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown visibility error = %v, want invalid request", err)
	}
	if _, err := service.UndoMemory(t.Context(), scope, digitalHumanID, hidden.Event.OperationID, MemoryUndoRequest{ExpectedRevision: hidden.Memory.Revision}); err != nil {
		t.Fatalf("undo visibility change: %v", err)
	}
	if again, err := NewRepository(tx).MemoryByIDForUpdate(t.Context(), scope, digitalHumanID, public.ID); err != nil || again.Visibility != MemoryVisibilityEveryone {
		t.Fatalf("memory after undo = %#v, %v", again, err)
	}

	// Bulk actions change each selected memory in its own mutation, skip
	// what cannot change, and replay when retried.
	second, third := write("support-hours", memoryruntime.MemoryTypeFact), write("persona", memoryruntime.MemoryTypeIdentity)
	bulk := MemoryBulkRequest{OperationID: uuid.New(), Action: MemoryBulkSetVisibility, Visibility: MemoryVisibilityInternal, MemoryIDs: []uuid.UUID{public.ID, second.ID, third.ID, uuid.New()}}
	result, err := service.ApplyMemoryBulk(t.Context(), scope, digitalHumanID, bulk)
	if err != nil || result.Updated != 2 || len(result.Skipped) != 2 || result.Skipped[0].Reason != memoryBulkSkipIdentity || result.Skipped[1].Reason != memoryBulkSkipNotFound {
		t.Fatalf("bulk visibility = %#v, %v", result, err)
	}
	if replay, err := service.ApplyMemoryBulk(t.Context(), scope, digitalHumanID, bulk); err != nil || replay.Updated != 0 || len(replay.Skipped) != 4 {
		t.Fatalf("replayed bulk visibility = %#v, %v", replay, err)
	}
	listed, err := service.ListMemories(t.Context(), scope, digitalHumanID, MemoryListQuery{Status: MemoryStatusActive, Visibility: MemoryVisibilityInternal})
	if err != nil || listed.Total != 2 || listed.Facets.Visibility["internal"] != 2 || listed.Facets.Visibility["everyone"] != 1 ||
		listed.Facets.Types["fact"] != 2 || listed.Facets.Types["identity"] != 1 || listed.Facets.Origins["operator"] != 3 {
		t.Fatalf("listed internal memory = %#v, %v", listed, err)
	}
	invalidated, err := service.ApplyMemoryBulk(t.Context(), scope, digitalHumanID, MemoryBulkRequest{OperationID: uuid.New(), Action: MemoryBulkInvalidate, MemoryIDs: []uuid.UUID{public.ID, second.ID}})
	if err != nil || invalidated.Updated != 2 {
		t.Fatalf("bulk invalidate = %#v, %v", invalidated, err)
	}
	if active, err := service.ListMemories(t.Context(), scope, digitalHumanID, MemoryListQuery{Status: MemoryStatusActive}); err != nil || active.Total != 1 || active.Facets.Types["fact"] != 0 {
		t.Fatalf("active memory after bulk invalidate = %#v, %v", active, err)
	}
	if _, err := service.ApplyMemoryBulk(t.Context(), scope, digitalHumanID, MemoryBulkRequest{OperationID: uuid.New(), Action: "delete", MemoryIDs: []uuid.UUID{third.ID}}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown bulk action error = %v", err)
	}
}
