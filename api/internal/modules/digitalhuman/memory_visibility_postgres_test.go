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
	write := func(key string, memoryType memoryruntime.MemoryType, class memoryruntime.MemoryClass, memoryScope *memoryruntime.MemoryScopeRef) *DigitalHumanMemory {
		t.Helper()
		result, err := service.ApplyMemoryProposal(t.Context(), scope, digitalHumanID, MemoryProposalRequest{
			OperationID: uuid.New(), Key: key, Action: memoryruntime.MutationActionUpsert, MemoryClass: class, Scope: memoryScope,
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
	public := write("price-list", memoryruntime.MemoryTypeFact, memoryruntime.MemoryClassSelf, nil)
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
}
