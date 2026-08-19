package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDigitalHumanMemoryPostgresOwnershipFencesAndUndo(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman memory PostgreSQL integration")
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

	organizationID, workspaceID := uuid.New(), uuid.New()
	actorA, actorB := uuid.New(), uuid.New()
	if err := tx.Exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "DigitalHuman memory integration").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "Memory integration").Error; err != nil {
		t.Fatal(err)
	}
	for index, actorID := range []uuid.UUID{actorA, actorB} {
		if err := tx.Exec("INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", actorID, "Memory actor", "memory-actor-"+actorID.String()+"@example.invalid").Error; err != nil {
			t.Fatalf("insert actor %d: %v", index, err)
		}
	}

	repository := NewRepository(tx)
	service := NewService(repository)
	fixedNow := time.Date(2026, 8, 19, 7, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }
	scopeA := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorA}
	scopeB := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorB}
	detail, err := service.Create(t.Context(), scopeA, CreateRequest{
		Name: "Shared memory subject", Persona: "Maintain evidence-backed operational continuity.",
		Rules: []string{"Never store credentials."}, DefaultModel: "test-model",
		MemoryPolicy: map[string]any{"automatic_extraction": false}, TriggerPolicy: map[string]any{"manual": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Publish(t.Context(), scopeA, detail.Definition.ID, PublishRequest{
		ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.memoryState(t.Context(), scopeA, detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	guardRunID := uuid.New()
	guardEvent := invocation.Event{
		ID: uuid.NewString(), Type: invocation.EventTypeManual,
		Origin: invocation.Origin{Kind: invocation.OriginManual},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: detail.Definition.ID,
			OrganizationID: organizationID, WorkspaceID: workspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: actorA.String()},
		OccurredAt: fixedNow,
	}
	guardEventJSON, err := json.Marshal(guardEvent)
	if err != nil {
		t.Fatal(err)
	}
	revision := "sha256:" + strings.Repeat("b", 64)
	guardRun := &Run{
		ID: guardRunID, OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: detail.Definition.ID,
		DigitalHumanVersionID: detail.Published.ID, ActorID: &actorA, IdempotencyKey: "runtime-scope-guard-" + guardRunID.String(),
		InvocationEvent: guardEventJSON, RunSpec: datatypes.JSON(`{}`), RunSpecRevision: revision,
		ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionSnapshotRevision: revision, ExecutionTrace: datatypes.JSON(`{}`),
		Query: "Save this to memory.", Status: RunStatusRunning, CreatedAt: fixedNow, UpdatedAt: fixedNow,
	}
	if err := repository.CreateRun(t.Context(), guardRun); err != nil {
		t.Fatal(err)
	}
	provider := NewMemoryRuntimeProvider(service)
	tool := provider.memoryMutationTool(guardRun, guardEvent, state, []memoryruntime.MemoryScopeRef{{
		Type: memoryruntime.MemoryScopeWorkspace, ID: workspaceID.String(),
	}}, true)
	toolResult := tool.Handler(t.Context(), adapter.ToolCall{
		ID: "unauthorized-project-scope",
		Function: adapter.FunctionCall{
			Name:      digitalHumanMemoryToolName,
			Arguments: `{"action":"upsert","mode":"explicit","evidence":"Save this to memory.","key":"unauthorized-project","memory_class":"business","memory_type":"fact","scope_type":"project","scope_id":"project-not-authorized","content":"must not commit","expected_revision":0,"confidence":1}`,
		},
	})
	if toolResult.Status != "failed" || toolResult.Result["error_code"] != AppCodeMemoryPolicyRejected.String() {
		t.Fatalf("unauthorized runtime tool result = %#v", toolResult)
	}
	var unauthorizedCount int64
	if err := tx.Model(&DigitalHumanMemory{}).
		Where("digital_human_id = ? AND memory_key = ?", detail.Definition.ID, "unauthorized-project").
		Count(&unauthorizedCount).Error; err != nil {
		t.Fatal(err)
	}
	if unauthorizedCount != 0 {
		t.Fatalf("unauthorized runtime memory count = %d, want 0", unauthorizedCount)
	}
	var rejectedEvent MemoryEvent
	if err := tx.Where("run_id = ? AND action = ?", guardRunID, "reject").Take(&rejectedEvent).Error; err != nil {
		t.Fatal(err)
	}
	if rejectedEvent.Result != "rejected" || rejectedEvent.ReasonCode != "memory_policy_rejected" {
		t.Fatalf("unauthorized runtime event = %#v", rejectedEvent)
	}
	var rejectedLedger ActionLedgerEntry
	if err := tx.Where("run_id = ? AND action_type = ?", guardRunID, "memory.proposal").Take(&rejectedLedger).Error; err != nil {
		t.Fatal(err)
	}
	if rejectedLedger.Status != "rejected" || rejectedLedger.MemoryOperationID == nil || *rejectedLedger.MemoryOperationID != rejectedEvent.OperationID {
		t.Fatalf("unauthorized runtime ledger = %#v, event operation = %s", rejectedLedger, rejectedEvent.OperationID)
	}
	projectScope := memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeProject, ID: "project-7"}
	operationID := uuid.New()
	proposal := MemoryProposalRequest{
		OperationID: operationID, Key: "launch-date", Action: memoryruntime.MutationActionUpsert,
		MemoryClass: memoryruntime.MemoryClassBusiness, MemoryType: memoryruntime.MemoryTypeFact,
		Scope: &projectScope, Content: "The approved launch date is September 8.",
		ExpectedRevision: 0, ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
		Evidence:   []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "approval:launch-date"}},
		Visibility: MemoryVisibilityWorkspace, Confidence: 1, OccurredAt: fixedNow,
	}
	created, err := service.ApplyMemoryProposal(t.Context(), scopeA, detail.Definition.ID, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if created.Memory == nil || created.Memory.Revision != 1 {
		t.Fatalf("created memory = %#v", created.Memory)
	}

	for name, scope := range map[string]Scope{"actor_a": scopeA, "actor_b": scopeB} {
		items, err := service.RetrieveMemories(t.Context(), scope, detail.Definition.ID, []memoryruntime.MemoryScopeRef{projectScope}, 10)
		if err != nil {
			t.Fatalf("retrieve for %s: %v", name, err)
		}
		if len(items) != 1 || items[0].ID != created.Memory.ID || items[0].Content != created.Memory.Content {
			t.Fatalf("retrieve for %s = %#v", name, items)
		}
	}
	otherScope := memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeProject, ID: "project-8"}
	items, err := service.RetrieveMemories(t.Context(), scopeB, detail.Definition.ID, []memoryruntime.MemoryScopeRef{otherScope}, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("unauthorized scope retrieval = %#v, err %v", items, err)
	}

	replayed, err := service.ApplyMemoryProposal(t.Context(), scopeB, detail.Definition.ID, proposal)
	if err != nil || !replayed.Replayed || replayed.Memory == nil || replayed.Memory.ID != created.Memory.ID {
		t.Fatalf("replay = %#v, err %v", replayed, err)
	}

	staleRevision := proposal
	staleRevision.OperationID = uuid.New()
	staleRevision.Content = "A stale overwrite."
	if _, err := service.ApplyMemoryProposal(t.Context(), scopeA, detail.Definition.ID, staleRevision); !errors.Is(err, ErrMemoryConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
	staleConfig := proposal
	staleConfig.OperationID, staleConfig.ExpectedRevision, staleConfig.ConfigRevision = uuid.New(), 1, "sha256:stale"
	if _, err := service.ApplyMemoryProposal(t.Context(), scopeA, detail.Definition.ID, staleConfig); !errors.Is(err, ErrMemoryConflict) {
		t.Fatalf("stale config error = %v", err)
	}
	updatedState, err := service.UpdateMemoryState(t.Context(), scopeA, detail.Definition.ID, UpdateMemoryStateRequest{
		ExpectedMemoryEpoch: state.MemoryEpoch, AutoExtractionEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	staleEpoch := proposal
	staleEpoch.OperationID, staleEpoch.ExpectedRevision = uuid.New(), 1
	if _, err := service.ApplyMemoryProposal(t.Context(), scopeA, detail.Definition.ID, staleEpoch); !errors.Is(err, ErrMemoryConflict) {
		t.Fatalf("stale epoch error = %v", err)
	}

	unsafe := proposal
	unsafe.OperationID, unsafe.ExpectedRevision = uuid.New(), 1
	unsafe.ConfigRevision, unsafe.MemoryEpoch = updatedState.ConfigRevision, updatedState.MemoryEpoch
	unsafe.Content = "api_key=must-not-store"
	if _, err := service.ApplyAutonomousMemoryProposal(t.Context(), scopeA, detail.Definition.ID, unsafe); !errors.Is(err, ErrMemoryPolicyRejected) {
		t.Fatalf("sensitive proposal error = %v", err)
	}
	if _, err := service.ApplyAutonomousMemoryProposal(t.Context(), scopeB, detail.Definition.ID, unsafe); !errors.Is(err, ErrMemoryPolicyRejected) {
		t.Fatalf("sensitive proposal replay error = %v", err)
	}
	var rejectedReplayEvents int64
	if err := tx.Model(&MemoryEvent{}).Where("digital_human_id = ? AND operation_id = ? AND result = ?", detail.Definition.ID, unsafe.OperationID, "rejected").Count(&rejectedReplayEvents).Error; err != nil {
		t.Fatal(err)
	}
	if rejectedReplayEvents != 1 {
		t.Fatalf("rejected replay event count = %d", rejectedReplayEvents)
	}
	unsafe.OperationID, unsafe.Content = uuid.New(), "Private conversation detail."
	unsafe.SourceContext.PrivateConversation = true
	if _, err := service.ApplyAutonomousMemoryProposal(t.Context(), scopeA, detail.Definition.ID, unsafe); !errors.Is(err, ErrMemoryPolicyRejected) {
		t.Fatalf("private proposal error = %v", err)
	}
	unsafe.OperationID, unsafe.Content, unsafe.SourceContext = uuid.New(), "No evidence proposal.", MemorySourceContext{}
	unsafe.Evidence = nil
	if _, err := service.ApplyAutonomousMemoryProposal(t.Context(), scopeA, detail.Definition.ID, unsafe); !errors.Is(err, ErrMemoryPolicyRejected) {
		t.Fatalf("evidence-free proposal error = %v", err)
	}

	var forbiddenColumns int64
	if err := tx.Raw(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name IN ('digital_human_memories', 'digital_human_memory_states')
		AND column_name IN ('agent_id', 'user_id', 'user_scope')`).Scan(&forbiddenColumns).Error; err != nil {
		t.Fatal(err)
	}
	if forbiddenColumns != 0 {
		t.Fatalf("forbidden ownership columns = %d", forbiddenColumns)
	}

	undone, err := service.UndoMemory(t.Context(), scopeB, detail.Definition.ID, operationID, MemoryUndoRequest{ExpectedRevision: 1})
	if err != nil || undone.Memory == nil || undone.Memory.Status != MemoryStatusInvalidated {
		t.Fatalf("undo = %#v, err %v", undone, err)
	}
	items, err = service.RetrieveMemories(t.Context(), scopeA, detail.Definition.ID, []memoryruntime.MemoryScopeRef{projectScope}, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("retrieval after undo = %#v, err %v", items, err)
	}
	listedAfterUndo, err := service.ListMemories(t.Context(), scopeA, detail.Definition.ID, MemoryListQuery{
		Page: 1, PageSize: 20, Status: MemoryStatusInvalidated,
	})
	if err != nil || len(listedAfterUndo.Items) != 1 || listedAfterUndo.Items[0].ID != created.Memory.ID {
		t.Fatalf("memory list after undo = %#v, err %v", listedAfterUndo, err)
	}
	if !listedAfterUndo.Items[0].UndoConsumed || listedAfterUndo.Items[0].UndoableUntil == nil {
		t.Fatalf("memory undo status after undo = %#v", listedAfterUndo.Items[0])
	}

	oldOperationID := uuid.New()
	oldProposal := proposal
	oldProposal.OperationID, oldProposal.Key = oldOperationID, "launch-window-v1"
	oldProposal.Content, oldProposal.ExpectedRevision = "The launch window is September 8.", 0
	oldProposal.ConfigRevision, oldProposal.MemoryEpoch = updatedState.ConfigRevision, updatedState.MemoryEpoch
	oldMemory, err := service.ApplyMemoryProposal(t.Context(), scopeA, detail.Definition.ID, oldProposal)
	if err != nil || oldMemory.Memory == nil {
		t.Fatalf("create superseded memory = %#v, err %v", oldMemory, err)
	}

	supersede := oldProposal
	supersede.OperationID, supersede.Key = uuid.New(), "launch-window-v2"
	supersede.Content = "The approved launch window moved to September 12."
	supersede.SupersedesID, supersede.ExpectedSupersededRevision = &oldMemory.Memory.ID, 2
	if _, err := service.ApplyMemoryProposal(t.Context(), scopeB, detail.Definition.ID, supersede); !errors.Is(err, ErrMemoryConflict) {
		t.Fatalf("stale superseded revision error = %v", err)
	}
	var unchangedOld DigitalHumanMemory
	if err := tx.Where("id = ?", oldMemory.Memory.ID).Take(&unchangedOld).Error; err != nil {
		t.Fatal(err)
	}
	if unchangedOld.Status != MemoryStatusActive || unchangedOld.Revision != 1 {
		t.Fatalf("stale supersede changed target = %#v", unchangedOld)
	}
	var replacementCount int64
	if err := tx.Model(&DigitalHumanMemory{}).Where("digital_human_id = ? AND memory_key = ?", detail.Definition.ID, supersede.Key).Count(&replacementCount).Error; err != nil {
		t.Fatal(err)
	}
	if replacementCount != 0 {
		t.Fatalf("stale supersede replacement count = %d", replacementCount)
	}

	supersede.OperationID, supersede.ExpectedSupersededRevision = uuid.New(), 1
	replacement, err := service.ApplyMemoryProposal(t.Context(), scopeB, detail.Definition.ID, supersede)
	if err != nil || replacement.Memory == nil || replacement.Memory.SupersedesID == nil || *replacement.Memory.SupersedesID != oldMemory.Memory.ID {
		t.Fatalf("supersede = %#v, err %v", replacement, err)
	}
	if err := tx.Where("id = ?", oldMemory.Memory.ID).Take(&unchangedOld).Error; err != nil {
		t.Fatal(err)
	}
	if unchangedOld.Status != MemoryStatusInvalidated || unchangedOld.Revision != 2 || unchangedOld.LastOperationID != supersede.OperationID {
		t.Fatalf("superseded target = %#v", unchangedOld)
	}
	var supersedeUndo MemoryUndoRecord
	if err := tx.Where("operation_id = ?", supersede.OperationID).Take(&supersedeUndo).Error; err != nil {
		t.Fatal(err)
	}
	if len(supersedeUndo.SupersededSnapshot) == 0 {
		t.Fatal("supersede undo snapshot is empty")
	}
	otherScopeProposal := oldProposal
	otherScopeProposal.OperationID, otherScopeProposal.Key, otherScopeProposal.Scope = uuid.New(), "other-project-fact", &otherScope
	otherScopeMemory, err := service.ApplyMemoryProposal(t.Context(), scopeA, detail.Definition.ID, otherScopeProposal)
	if err != nil || otherScopeMemory.Memory == nil {
		t.Fatalf("create other-scope memory = %#v, err %v", otherScopeMemory, err)
	}
	if err := tx.SavePoint("cross_scope_supersedes_guard").Error; err != nil {
		t.Fatal(err)
	}
	crossScopeErr := tx.Model(&DigitalHumanMemory{}).Where("id = ?", replacement.Memory.ID).
		Update("supersedes_id", otherScopeMemory.Memory.ID).Error
	if crossScopeErr == nil {
		t.Fatal("database accepted a cross-scope supersedes reference")
	}
	if err := tx.RollbackTo("cross_scope_supersedes_guard").Error; err != nil {
		t.Fatal(err)
	}
	items, err = service.RetrieveMemories(t.Context(), scopeA, detail.Definition.ID, []memoryruntime.MemoryScopeRef{projectScope}, 10)
	if err != nil || len(items) != 1 || items[0].ID != replacement.Memory.ID {
		t.Fatalf("retrieval after supersede = %#v, err %v", items, err)
	}

	restored, err := service.UndoMemory(t.Context(), scopeA, detail.Definition.ID, supersede.OperationID, MemoryUndoRequest{ExpectedRevision: 1})
	if err != nil || restored.Memory == nil || restored.Memory.ID != oldMemory.Memory.ID || restored.Memory.Status != MemoryStatusActive || restored.Memory.Revision != 3 {
		t.Fatalf("undo supersede = %#v, err %v", restored, err)
	}
	var invalidatedReplacement DigitalHumanMemory
	if err := tx.Where("id = ?", replacement.Memory.ID).Take(&invalidatedReplacement).Error; err != nil {
		t.Fatal(err)
	}
	if invalidatedReplacement.Status != MemoryStatusInvalidated || invalidatedReplacement.Revision != 2 {
		t.Fatalf("replacement after undo = %#v", invalidatedReplacement)
	}
	items, err = service.RetrieveMemories(t.Context(), scopeB, detail.Definition.ID, []memoryruntime.MemoryScopeRef{projectScope}, 10)
	if err != nil || len(items) != 1 || items[0].ID != oldMemory.Memory.ID || items[0].Content != oldProposal.Content {
		t.Fatalf("retrieval after supersede undo = %#v, err %v", items, err)
	}
}

func TestDigitalHumanMemoryPostgresExtractionJobCommitsFromReferences(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman memory PostgreSQL extraction integration")
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
	if err := tx.Exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "DigitalHuman extraction integration").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "Extraction integration").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", actorID, "Extraction actor", "extraction-"+actorID.String()+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(tx)
	service := NewService(repository)
	fixedNow := time.Date(2026, 8, 19, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return fixedNow }
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
	detail, err := service.Create(t.Context(), scope, CreateRequest{
		Name: "Extraction subject", Persona: "Learn only durable business facts.",
		Rules: []string{"Require evidence."}, DefaultModel: "test-model",
		MemoryPolicy: map[string]any{"automatic_extraction": false}, TriggerPolicy: map[string]any{"schedule": true},
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
	state, err = service.UpdateMemoryState(t.Context(), scope, detail.Definition.ID, UpdateMemoryStateRequest{
		ExpectedMemoryEpoch: state.MemoryEpoch, AutoExtractionEnabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	event, err := invocation.ValidateEvent(invocation.Event{
		ID: uuid.NewString(), Type: invocation.EventTypeSchedule,
		Origin: invocation.Origin{Kind: invocation.OriginSchedule},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: detail.Definition.ID,
			OrganizationID: organizationID, WorkspaceID: workspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "memory-integration"},
		OccurredAt: fixedNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	runID, conversationID, messageID := uuid.New(), uuid.New(), uuid.New()
	revision := "sha256:" + strings.Repeat("a", 64)
	completedAt := fixedNow.Add(time.Second)
	run := &Run{
		ID: runID, OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: detail.Definition.ID,
		DigitalHumanVersionID: detail.Published.ID, ActorID: &actorID, IdempotencyKey: event.IdempotencyKey,
		InvocationEvent: eventJSON, RunSpec: datatypes.JSON(`{}`), RunSpecRevision: revision,
		ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionSnapshotRevision: revision, ExecutionTrace: datatypes.JSON(`{}`),
		Query: "Review the approved launch plan.", Status: RunStatusSucceeded,
		ConversationID: &conversationID, MessageID: &messageID, ResultSummary: "The launch date is September 8.",
		CompletedAt: &completedAt, CreatedAt: fixedNow, UpdatedAt: completedAt,
	}
	if err := repository.CreateRun(t.Context(), run); err != nil {
		t.Fatal(err)
	}
	workspaceRef, digitalHumanRef := workspaceID, detail.Definition.ID
	conversation := &runtimemodel.Conversation{
		ID: conversationID, OrganizationID: organizationID, WorkspaceID: &workspaceRef, AccountID: actorID,
		CallerType: runtimemodel.ConversationCallerDigitalHuman, CallerID: &digitalHumanRef,
		ConversationType: runtimemodel.ConversationTypeChat, Title: "Extraction integration",
		Status: runtimemodel.ConversationStatusNormal, RuntimeStatus: runtimemodel.ConversationRuntimeStatusIdle,
		Source: runtimemodel.ConversationSourceConsole, CreatedAt: fixedNow, UpdatedAt: completedAt,
	}
	if err := tx.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	provider := "test-provider"
	message := &runtimemodel.Message{
		ID: messageID, ConversationID: conversationID, Query: run.Query, Answer: run.ResultSummary,
		Status: runtimemodel.MessageStatusCompleted, ModelProvider: &provider, ModelName: "test-model",
		SourceMessageID: &runID, CreatedAt: fixedNow, UpdatedAt: completedAt,
	}
	if err := tx.Create(message).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ScheduleMemoryExtraction(t.Context(), scope, run, conversationID, messageID, datatypes.JSON(`[]`)); err != nil {
		t.Fatal(err)
	}

	extractor := &recordingMemoryExtractor{drafts: []MemoryExtractionDraft{{
		Key: "launch-date", MemoryClass: memoryruntime.MemoryClassBusiness, MemoryType: memoryruntime.MemoryTypeFact,
		ScopeType: memoryruntime.MemoryScopeWorkspace, ScopeID: workspaceID.String(),
		Content: "The approved launch date is September 8.", Confidence: 0.95,
	}}}
	runner := NewMemoryExtractionRunner(repository, service, extractor)
	runner.now = func() time.Time { return fixedNow.Add(memoryExtractionScheduleDelay + time.Second) }
	if err := runner.ProcessDue(t.Context(), 10); err != nil {
		t.Fatal(err)
	}
	if extractor.calls != 1 || extractor.input.Source.Query != run.Query || extractor.input.Source.Answer != run.ResultSummary {
		t.Fatalf("extractor calls=%d input=%#v", extractor.calls, extractor.input.Source)
	}
	items, err := service.RetrieveMemories(t.Context(), scope, detail.Definition.ID, []memoryruntime.MemoryScopeRef{{
		Type: memoryruntime.MemoryScopeWorkspace, ID: workspaceID.String(),
	}}, 10)
	if err != nil || len(items) != 1 || items[0].MemoryKey != "launch-date" || items[0].Revision != 1 {
		t.Fatalf("extracted memory = %#v, err %v", items, err)
	}
	var job MemoryExtractionJob
	if err := tx.Where("digital_human_id = ? AND idempotency_key = ?", detail.Definition.ID, "run:"+runID.String()).Take(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != MemoryExtractionJobSucceeded || job.AttemptCount != 1 || string(job.ArtifactRefs) != "[]" {
		t.Fatalf("extraction job = %#v", job)
	}
	if job.ConfigRevision != state.ConfigRevision || job.MemoryEpoch != state.MemoryEpoch {
		t.Fatalf("job fence = %s/%d, want %s/%d", job.ConfigRevision, job.MemoryEpoch, state.ConfigRevision, state.MemoryEpoch)
	}
}

type recordingMemoryExtractor struct {
	drafts []MemoryExtractionDraft
	input  MemoryExtractionInput
	calls  int
}

func (e *recordingMemoryExtractor) Extract(_ context.Context, input MemoryExtractionInput) ([]MemoryExtractionDraft, error) {
	e.calls++
	e.input = input
	return append([]MemoryExtractionDraft(nil), e.drafts...), nil
}
