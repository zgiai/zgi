package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/migrations"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestWorkspaceAssetMoveMovesDigitalHumansPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ZGI_MIGRATION_TEST_DSN"))
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the DigitalHuman workspace move PostgreSQL integration")
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
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	organizationID, sourceID, targetID, accountID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, 'DigitalHuman move')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Mover', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, 'Source')", sourceID, organizationID)
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, 'Target')", targetID, organizationID)

	service := digitalhuman.NewService(digitalhuman.NewRepository(tx))
	scope := digitalhuman.Scope{OrganizationID: organizationID, WorkspaceID: sourceID, ActorID: accountID}
	detail, err := service.Create(ctx, scope, digitalhuman.CreateRequest{Name: "Movable", Persona: "Helps the team.", Rules: []string{"Be brief."}, DefaultModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Publish(ctx, scope, detail.Definition.ID, digitalhuman.PublishRequest{ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	digitalHumanID := detail.Definition.ID
	state, err := service.UpdateMemoryState(ctx, scope, digitalHumanID, digitalhuman.UpdateMemoryStateRequest{ExpectedMemoryEpoch: 0, AutoExtractionEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ApplyMemoryProposal(ctx, scope, digitalHumanID, digitalhuman.MemoryProposalRequest{
		OperationID: uuid.New(), Key: "launch", Action: memoryruntime.MutationActionUpsert, MemoryClass: memoryruntime.MemoryClassSelf,
		MemoryType: memoryruntime.MemoryTypeFact, Content: "The launch is on Monday.", ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch,
		Evidence: []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "note"}}, Visibility: digitalhuman.MemoryVisibilityWorkspace,
		Confidence: 1, OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO automation_tasks (id, organization_id, workspace_id, name, task_kind, subject_type, subject_id, owner_kind, origin_surface, status, schedule_type, timezone, schedule_config, source_type, created_by, updated_by, created_at, updated_at) VALUES (?, ?, ?, 'Weekly', 'digital_human', 'digital_human', ?, 'digital_human', 'console', 'active', 'manual', 'UTC', '{}', 'manual', ?, ?, now(), now())",
		uuid.New(), organizationID, sourceID, digitalHumanID.String(), accountID, accountID)
	// The hidden dataset projecting the memory, as retrieval creates it.
	createdBy := accountID.String()
	projection := &datasetmodel.Dataset{ID: uuid.New().String(), OrganizationID: organizationID.String(), WorkspaceID: sourceID.String(), Name: "DigitalHuman memory projection",
		Provider: datasetmodel.ProviderDigitalHumanMemory, Permission: string(datasetmodel.DatasetPermissionOnlyMe), CreatedBy: createdBy, Owner: &createdBy}
	if err := tx.Create(projection).Error; err != nil {
		t.Fatal(err)
	}
	exec("UPDATE digital_human_memory_states SET memory_dataset_id = ? WHERE digital_human_id = ?", projection.ID, digitalHumanID)

	authorization := &stubAssetMoveOrgService{allowed: true}
	mover := NewWorkspaceAssetMoveService(tx, authorization, nil).RegisterAssetMover(digitalhuman.NewWorkspaceMover())
	request := dto.WorkspaceAssetMoveRequest{TargetWorkspaceID: targetID.String(), Items: []dto.WorkspaceAssetMoveItem{{Type: digitalhuman.WorkspaceMoveAssetType, ID: digitalHumanID.String()}}}

	// Work in flight blocks the move.
	runID := uuid.New()
	exec("INSERT INTO digital_human_runs (id, organization_id, workspace_id, digital_human_id, digital_human_version_id, idempotency_key, invocation_event, run_spec, run_spec_revision, execution_snapshot, execution_snapshot_revision, execution_trace, query, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, '{}', '{}', 'sha256:x', '{}', 'sha256:x', '{}', 'q', 'running', now(), now())",
		runID, organizationID, sourceID, digitalHumanID, detail.Published.ID, "move-"+runID.String())
	preview, err := mover.Preview(ctx, organizationID.String(), accountID.String(), request)
	if err != nil || preview.Movable || len(preview.Items) != 1 || len(preview.Items[0].Blockers) == 0 {
		t.Fatalf("preview with a run in progress = %#v, %v", preview, err)
	}
	if _, err := mover.Move(ctx, organizationID.String(), accountID.String(), request); !errors.Is(err, ErrAssetMoveBlocked) {
		t.Fatalf("move with a run in progress = %v", err)
	}
	exec("UPDATE digital_human_runs SET status = 'succeeded' WHERE id = ?", runID)

	moved, err := mover.Move(ctx, organizationID.String(), accountID.String(), request)
	if err != nil || !moved.Moved {
		t.Fatalf("move = %#v, %v", moved, err)
	}
	count := func(query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.Raw(query, args...).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"digital_human_versions", "digital_human_memories", "digital_human_memory_events", "digital_human_memory_states", "digital_human_runs"} {
		if left := count("SELECT count(*) FROM "+table+" WHERE digital_human_id = ? AND workspace_id = ?", digitalHumanID, sourceID); left != 0 {
			t.Fatalf("%s kept %d rows in the source workspace", table, left)
		}
		if moved := count("SELECT count(*) FROM "+table+" WHERE digital_human_id = ? AND workspace_id = ?", digitalHumanID, targetID); moved == 0 {
			t.Fatalf("%s has no rows in the target workspace", table)
		}
	}
	if n := count("SELECT count(*) FROM datasets WHERE id = ? AND workspace_id = ?", projection.ID, targetID); n != 1 {
		t.Fatal("the memory projection dataset stayed in the source workspace")
	}
	if n := count("SELECT count(*) FROM digital_humans WHERE id = ? AND workspace_id = ? AND identity_workspace_id = ?", digitalHumanID, targetID, sourceID); n != 1 {
		t.Fatal("the DigitalHuman did not move with its identity workspace pinned")
	}
	if n := count("SELECT count(*) FROM automation_tasks WHERE subject_id = ? AND workspace_id = ?", digitalHumanID.String(), targetID); n != 1 {
		t.Fatal("the DigitalHuman's task did not move")
	}
	if n := count("SELECT count(*) FROM workspace_asset_move_events WHERE asset_type = ? AND asset_id = ?", digitalhuman.WorkspaceMoveAssetType, digitalHumanID.String()); n != 1 {
		t.Fatal("the move was not audited")
	}
	if set := count("SELECT count(*) WHERE coalesce(current_setting('zgi.digital_human_workspace_move', true), '') <> ''"); set != 0 {
		t.Fatal("the move bypass outlived the move")
	}

	// The guards still refuse ordinary changes to published versions and
	// memory events once the move is over.
	if err := tx.Exec("SAVEPOINT guards").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("UPDATE digital_human_memory_events SET workspace_id = ? WHERE digital_human_id = ?", sourceID, digitalHumanID).Error; err == nil {
		t.Fatal("a memory event was rewritten outside a move")
	}
	if err := tx.Exec("ROLLBACK TO SAVEPOINT guards").Error; err != nil {
		t.Fatal(err)
	}
	moveAgain, err := mover.Preview(ctx, organizationID.String(), accountID.String(), request)
	if err != nil || moveAgain.Movable {
		t.Fatalf("a DigitalHuman already in the target was movable again: %#v %v", moveAgain, err)
	}
}
