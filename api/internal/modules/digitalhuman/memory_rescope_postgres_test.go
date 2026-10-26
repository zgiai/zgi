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

func TestDigitalHumanMemoryRescopePostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman memory rescope PostgreSQL integration")
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
	rescope := func(memory *DigitalHumanMemory, class memoryruntime.MemoryClass, memoryScope *memoryruntime.MemoryScopeRef) (*MemoryMutationResult, error) {
		return service.RescopeMemory(t.Context(), scope, digitalHumanID, memory.ID, MemoryRescopeRequest{
			OperationID: uuid.New(), ExpectedRevision: memory.Revision, ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
			MemoryClass: class, Scope: memoryScope,
		})
	}
	instance := &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeInstance, ID: uuid.NewString()}

	// An operator promotes an end user's note into the DigitalHuman's own
	// memory; it starts internal (ADR 0015).
	note := write("release-window", memoryruntime.MemoryTypeFact, memoryruntime.MemoryClassBusiness, instance)
	promoted, err := rescope(note, memoryruntime.MemoryClassSelf, nil)
	if err != nil {
		t.Fatalf("promote note: %v", err)
	}
	if promoted.Memory.MemoryClass != memoryruntime.MemoryClassSelf || promoted.Memory.ScopeType != "" || promoted.Memory.Visibility != MemoryVisibilityInternal ||
		promoted.Memory.Content != note.Content || promoted.Memory.SupersedesID != nil {
		t.Fatalf("promoted memory = %#v", promoted.Memory)
	}
	old, err := NewRepository(tx).MemoryByIDForUpdate(t.Context(), scope, digitalHumanID, note.ID)
	if err != nil || old.Status != MemoryStatusInvalidated {
		t.Fatalf("note after promotion = %#v, %v", old, err)
	}

	// A retry with the same operation replays the committed move.
	retried, err := service.RescopeMemory(t.Context(), scope, digitalHumanID, note.ID, MemoryRescopeRequest{
		OperationID: promoted.Event.OperationID, ExpectedRevision: note.Revision, ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
		MemoryClass: memoryruntime.MemoryClassSelf,
	})
	if err != nil || !retried.Replayed || retried.Memory.ID != promoted.Memory.ID {
		t.Fatalf("retried promotion = %#v, %v", retried, err)
	}

	// The move is an ordinary memory operation and can be undone.
	if _, err := service.UndoMemory(t.Context(), scope, digitalHumanID, promoted.Event.OperationID, MemoryUndoRequest{ExpectedRevision: promoted.Memory.Revision}); err != nil {
		t.Fatalf("undo promotion: %v", err)
	}
	restored, err := NewRepository(tx).MemoryByIDForUpdate(t.Context(), scope, digitalHumanID, note.ID)
	if err != nil || restored.Status != MemoryStatusActive {
		t.Fatalf("note after undo = %#v, %v", restored, err)
	}

	// The DigitalHuman's own memory is the only target.
	if _, err := rescope(restored, memoryruntime.MemoryClassBusiness, instance); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("rescope to a business scope error = %v, want invalid request", err)
	}
	identity := write("who-i-am", memoryruntime.MemoryTypeIdentity, memoryruntime.MemoryClassSelf, nil)
	if _, err := rescope(identity, memoryruntime.MemoryClassSelf, nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("same-scope rescope error = %v, want invalid request", err)
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
