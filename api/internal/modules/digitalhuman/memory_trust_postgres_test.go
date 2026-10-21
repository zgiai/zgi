package digitalhuman

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDigitalHumanMemoryTrustPostgresChannelScopeAndRecall(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman memory trust PostgreSQL integration")
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
		{"INSERT INTO organizations (id, name) VALUES (?, ?)", []any{organizationID, "Memory trust integration"}},
		{"INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", []any{workspaceID, organizationID, "Memory trust integration"}},
		{"INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", []any{actorID, "Memory operator", "memory-trust-" + actorID.String() + "@example.invalid"}},
	} {
		if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	repository := NewRepository(tx)
	service := NewService(repository)
	fixedNow := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
	detail, err := service.Create(t.Context(), scope, CreateRequest{
		Name: "Channel memory subject", Persona: "Keep conversation context separate.", DefaultModel: "test-model",
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
	state, err := service.memoryState(t.Context(), scope, detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}

	revision := "sha256:" + strings.Repeat("c", 64)
	createRun := func(trust SourceTrust, surface string, event invocation.Event) *Run {
		t.Helper()
		event.ID, event.OccurredAt = uuid.NewString(), fixedNow
		event.Target = runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: detail.Definition.ID, OrganizationID: organizationID, WorkspaceID: workspaceID}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		runID := uuid.New()
		run := &Run{
			ID: runID, OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: detail.Definition.ID,
			DigitalHumanVersionID: detail.Published.ID, ActorID: &actorID, Surface: surface, SourceTrust: sourceTrustPointer(trust),
			IdempotencyKey: "memory-trust-" + runID.String(), InvocationEvent: encoded, RunSpec: datatypes.JSON(`{}`), RunSpecRevision: revision,
			ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionSnapshotRevision: revision, ExecutionTrace: datatypes.JSON(`{}`),
			Query: "Remember this for the group.", Status: RunStatusRunning, CreatedAt: fixedNow, UpdatedAt: fixedNow,
		}
		if err := repository.CreateRun(t.Context(), run); err != nil {
			t.Fatal(err)
		}
		return run
	}
	channelA := channelEvent("group-a")
	channelA.Type, channelA.Sender = invocation.EventTypeChannelMessage, invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "member"}
	channelRun := createRun(SourceTrustChannelParticipant, SurfaceChannel, channelA)
	stored, err := repository.RunByID(t.Context(), Scope{OrganizationID: organizationID, WorkspaceID: workspaceID}, detail.Definition.ID, channelRun.ID)
	if err != nil || RunSourceTrust(stored) != SourceTrustChannelParticipant {
		t.Fatalf("stored channel run trust = %v, %v", stored, err)
	}

	admission := admitMemoryWrite(channelRun, channelA, memoryWriteExplicit, true)
	if !admission.Allowed {
		t.Fatalf("channel admission = %+v", admission)
	}
	write := func(run *Run, key string, class memoryruntime.MemoryClass, memoryScope *memoryruntime.MemoryScopeRef) *DigitalHumanMemory {
		t.Helper()
		result, err := service.ApplyAutonomousMemoryProposal(t.Context(), runScope(run), detail.Definition.ID, MemoryProposalRequest{
			OperationID: uuid.New(), Key: key, Action: memoryruntime.MutationActionUpsert, MemoryClass: class, Scope: memoryScope,
			MemoryType: memoryruntime.MemoryTypeDecision, Content: key + " content", ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
			Evidence:   []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeRun, ID: run.ID.String(), Digest: run.RunSpecRevision}},
			Visibility: MemoryVisibilityWorkspace, Confidence: 1, RunID: &run.ID, OccurredAt: fixedNow,
			SourceContext: MemorySourceContext{SourceTrust: RunSourceTrust(run)},
		})
		if err != nil {
			t.Fatalf("write %s: %v", key, err)
		}
		return result.Memory
	}
	channelMemory := write(channelRun, "group-a-standup", admission.Class, admission.Scope)
	if channelMemory.MemoryClass != memoryruntime.MemoryClassBusiness || channelMemory.ScopeType != memoryruntime.MemoryScopeChannel || channelMemory.ScopeID != admission.Scope.ID {
		t.Fatalf("channel memory = %#v", channelMemory)
	}
	operatorRun := createRun(SourceTrustAuthorizedOperator, SurfaceConsole, invocation.Event{Type: invocation.EventTypeManual, Origin: invocation.Origin{Kind: invocation.OriginManual}, Sender: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: actorID.String()}})
	write(operatorRun, "self-decision", memoryruntime.MemoryClassSelf, nil)
	write(operatorRun, "workspace-fact", memoryruntime.MemoryClassBusiness, &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeWorkspace, ID: workspaceID.String()})

	recall := func(run *Run, event invocation.Event) map[string]bool {
		t.Helper()
		readScope := memoryReadScopeForRun(run, event)
		items, err := repository.ActiveMemories(t.Context(), Scope{OrganizationID: organizationID, WorkspaceID: workspaceID}, detail.Definition.ID, 50, &readScope)
		if err != nil {
			t.Fatal(err)
		}
		keys := make(map[string]bool, len(items))
		for _, item := range items {
			keys[item.MemoryKey] = true
		}
		return keys
	}
	expect := func(label string, got map[string]bool, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s recalled %v, want %v", label, got, want)
		}
		for _, key := range want {
			if !got[key] {
				t.Fatalf("%s recalled %v, want %v", label, got, want)
			}
		}
	}
	expect("own channel", recall(channelRun, channelA), "group-a-standup", "self-decision", "workspace-fact")
	channelB := channelEvent("group-b")
	expect("other channel", recall(channelRun, channelB), "self-decision", "workspace-fact")
	externalRun := &Run{OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: detail.Definition.ID, Surface: SurfaceAPI, SourceTrust: sourceTrustPointer(SourceTrustExternalEndUser)}
	expect("external end user", recall(externalRun, invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginAPI}}), "self-decision")
}
