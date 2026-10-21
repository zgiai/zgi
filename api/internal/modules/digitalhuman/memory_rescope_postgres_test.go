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
	write := func(key string, memoryType memoryruntime.MemoryType) *DigitalHumanMemory {
		t.Helper()
		result, err := service.ApplyMemoryProposal(t.Context(), scope, digitalHumanID, MemoryProposalRequest{
			OperationID: uuid.New(), Key: key, Action: memoryruntime.MutationActionUpsert, MemoryClass: memoryruntime.MemoryClassSelf,
			MemoryType: memoryType, Content: key + " content", ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
			Evidence:   []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "operator:" + key}},
			Visibility: MemoryVisibilityWorkspace, Confidence: 1, OccurredAt: fixedNow, AdminApproved: true,
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
	workspace := &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeWorkspace, ID: "ignored-client-value"}

	original := write("release-window", memoryruntime.MemoryTypeFact)
	moved, err := rescope(original, memoryruntime.MemoryClassBusiness, workspace)
	if err != nil {
		t.Fatalf("rescope to workspace: %v", err)
	}
	if moved.Memory.MemoryClass != memoryruntime.MemoryClassBusiness || moved.Memory.ScopeType != memoryruntime.MemoryScopeWorkspace ||
		moved.Memory.ScopeID != workspaceID.String() || moved.Memory.Content != original.Content || moved.Memory.SupersedesID != nil {
		t.Fatalf("moved memory = %#v", moved.Memory)
	}
	old, err := NewRepository(tx).MemoryByIDForUpdate(t.Context(), scope, digitalHumanID, original.ID)
	if err != nil || old.Status != MemoryStatusInvalidated {
		t.Fatalf("original after rescope = %#v, %v", old, err)
	}

	// The move is an ordinary memory operation and can be undone.
	if _, err := service.UndoMemory(t.Context(), scope, digitalHumanID, moved.Event.OperationID, MemoryUndoRequest{ExpectedRevision: moved.Memory.Revision}); err != nil {
		t.Fatalf("undo rescope: %v", err)
	}
	restored, err := NewRepository(tx).MemoryByIDForUpdate(t.Context(), scope, digitalHumanID, original.ID)
	if err != nil || restored.Status != MemoryStatusActive {
		t.Fatalf("original after undo = %#v, %v", restored, err)
	}

	// Identity stays self memory, and a move to the current scope is refused.
	identity := write("who-i-am", memoryruntime.MemoryTypeIdentity)
	if _, err := rescope(identity, memoryruntime.MemoryClassBusiness, workspace); !errors.Is(err, ErrMemoryPolicyRejected) {
		t.Fatalf("identity rescope error = %v, want policy rejected", err)
	}
	if _, err := rescope(restored, memoryruntime.MemoryClassSelf, nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("same-scope rescope error = %v, want invalid request", err)
	}
	if _, err := rescope(restored, memoryruntime.MemoryClassBusiness, &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeChannel, ID: "group"}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("channel rescope error = %v, want invalid request", err)
	}

	// A client proposal still cannot supersede across scopes.
	_, err = service.ApplyMemoryProposal(t.Context(), scope, digitalHumanID, MemoryProposalRequest{
		OperationID: uuid.New(), Key: restored.MemoryKey, Action: memoryruntime.MutationActionUpsert, MemoryClass: memoryruntime.MemoryClassBusiness,
		Scope:      &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeWorkspace, ID: workspaceID.String()},
		MemoryType: restored.MemoryType, Content: restored.Content, SupersedesID: &restored.ID, ExpectedSupersededRevision: restored.Revision,
		ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
		Evidence:   []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "operator:cross-scope"}},
		Visibility: MemoryVisibilityWorkspace, Confidence: 1, OccurredAt: fixedNow, AdminApproved: true,
		SourceContext: MemorySourceContext{PromotedByAuthorizedActor: true},
	})
	if !errors.Is(err, ErrMemoryConflict) {
		t.Fatalf("cross-scope proposal error = %v, want conflict", err)
	}
}
