package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IdentityWorkspace is the workspace end-user identities derive from.
func (d *Definition) IdentityWorkspace() uuid.UUID {
	if d != nil && d.IdentityWorkspaceID != nil && *d.IdentityWorkspaceID != uuid.Nil {
		return *d.IdentityWorkspaceID
	}
	if d == nil {
		return uuid.Nil
	}
	return d.WorkspaceID
}

// WorkspaceMoveAssetType names DigitalHumans in workspace moves.
const WorkspaceMoveAssetType = "digital_human"

// workspaceMoveDetachedBindings are binding types whose resources belong to
// the source workspace; a move detaches them. Skills, Connectors, and
// integration connections are organization-wide and stay bound.
var workspaceMoveDetachedBindings = []string{"knowledge_dataset", "database", "database_table", "workflow"}

// WorkspaceMover moves a DigitalHuman and everything it owns to another
// workspace of its organization, for the workspace asset move.
//
//   - Its history, memory, tasks, reports, conversations, artifacts, API
//     keys, files, end-user instances, and runtime surfaces move with it.
//   - Bindings to source-workspace datasets, databases, and workflows are
//     detached; the preview warns about them.
//   - It does not move while it has work in flight or bound channels, whose
//     installations belong to the source workspace.
//   - End-user identities stay stable: they keep deriving from the
//     workspace the DigitalHuman was created in (identity_workspace_id).
type WorkspaceMover struct {
	now func() time.Time
}

// NewWorkspaceMover builds the DigitalHuman workspace mover.
func NewWorkspaceMover() *WorkspaceMover {
	return &WorkspaceMover{now: func() time.Time { return time.Now().UTC() }}
}

func (m *WorkspaceMover) AssetType() string { return WorkspaceMoveAssetType }

func (m *WorkspaceMover) Permission() workspacemodel.WorkspacePermissionCode {
	return workspacemodel.WorkspacePermissionDigitalHumanUpdate
}

func (m *WorkspaceMover) SourceWorkspace(ctx context.Context, db *gorm.DB, organizationID, id string) (string, error) {
	var row struct{ WorkspaceID string }
	if _, err := uuid.Parse(id); err != nil {
		return "", gorm.ErrRecordNotFound
	}
	if err := db.WithContext(ctx).Table("digital_humans").Select("workspace_id").
		Where("id = ? AND deleted_at IS NULL", id).Take(&row).Error; err != nil {
		return "", err
	}
	return row.WorkspaceID, nil
}

func (m *WorkspaceMover) Lock(ctx context.Context, tx *gorm.DB, organizationID, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return nil
	}
	var row struct{ ID string }
	err := tx.WithContext(ctx).Table("digital_humans").Select("id").Where("id = ? AND deleted_at IS NULL", id).
		Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func (m *WorkspaceMover) Preview(ctx context.Context, db *gorm.DB, organizationID, id string, item *dto.WorkspaceAssetMovePreviewItem) error {
	count := func(query *gorm.DB) (int64, error) {
		var n int64
		err := query.Count(&n).Error
		return n, err
	}
	busy, err := count(db.WithContext(ctx).Table("digital_human_runs").
		Where("digital_human_id = ? AND status IN ?", id, []RunStatus{RunStatusQueued, RunStatusRunning, RunStatusCanceling}))
	if err != nil {
		return fmt.Errorf("count DigitalHuman runs in flight: %w", err)
	}
	if busy > 0 {
		item.Blockers = append(item.Blockers, "the digital human has runs in progress; wait for them to finish")
	}
	channels, err := count(db.WithContext(ctx).Table("channel_bindings").Where("subject_type = ? AND subject_id = ?", "digital_human", id))
	if err != nil {
		return fmt.Errorf("count DigitalHuman channel bindings: %w", err)
	}
	if channels > 0 {
		item.Blockers = append(item.Blockers, "remove the digital human's channel bindings first; channel installations belong to the source workspace")
	}
	detached, err := count(db.WithContext(ctx).Table("digital_human_resource_bindings").
		Where("digital_human_id = ? AND binding_type IN ?", id, workspaceMoveDetachedBindings).Distinct("binding_type", "resource_id"))
	if err != nil {
		return fmt.Errorf("count DigitalHuman workspace bindings: %w", err)
	}
	if detached > 0 {
		item.Warnings = append(item.Warnings, fmt.Sprintf("%d bound knowledge bases, databases, or workflows stay in the source workspace and will be unbound from the digital human", detached))
	}
	return nil
}

// workspaceMoveTables are rewritten to the target workspace, keyed by the
// DigitalHuman. Order matters for the scope triggers: the DigitalHuman row
// first, then versions, then everything that references them.
var workspaceMoveTables = []string{
	"digital_human_versions", "digital_human_resource_bindings",
	"digital_human_runs", "digital_human_memory_states", "digital_human_memories", "digital_human_memory_events",
	"digital_human_memory_undo_records", "digital_human_memory_jobs", "digital_human_action_ledger",
	"digital_human_api_keys", "digital_human_external_files", "digital_human_access_events", "digital_human_end_user_instances",
	"digital_human_channel_task_policies", "digital_human_direct_skill_credentials", "digital_human_channel_approval_jobs",
	"digital_human_task_reports",
}

func (m *WorkspaceMover) Move(ctx context.Context, tx *gorm.DB, organizationID, accountID string, item dto.WorkspaceAssetMovePreviewItem) error {
	id, err := uuid.Parse(item.ID)
	if err != nil {
		return err
	}
	source, err := uuid.Parse(item.FromWorkspaceID)
	if err != nil {
		return err
	}
	target, err := uuid.Parse(item.TargetWorkspaceID)
	if err != nil {
		return err
	}
	db := tx.WithContext(ctx)
	// Pin the identity workspace before anything moves, under the normal
	// guards.
	if err := db.Exec("UPDATE digital_humans SET identity_workspace_id = workspace_id WHERE id = ? AND identity_workspace_id IS NULL", id).Error; err != nil {
		return fmt.Errorf("pin DigitalHuman identity workspace: %w", err)
	}
	// The guards accept workspace-only updates of this DigitalHuman for the
	// rest of the transaction.
	if err := db.Exec("SELECT set_config('zgi.digital_human_workspace_move', ?, true)", id.String()).Error; err != nil {
		return fmt.Errorf("start DigitalHuman workspace move: %w", err)
	}
	if err := db.Exec("DELETE FROM digital_human_resource_bindings WHERE digital_human_id = ? AND binding_type IN ?", id, workspaceMoveDetachedBindings).Error; err != nil {
		return fmt.Errorf("detach DigitalHuman workspace bindings: %w", err)
	}
	now := m.now()
	if err := db.Exec("UPDATE digital_humans SET workspace_id = ?, updated_at = ? WHERE id = ? AND workspace_id = ?", target, now, id, source).Error; err != nil {
		return fmt.Errorf("move DigitalHuman: %w", err)
	}
	for _, table := range workspaceMoveTables {
		if err := db.Exec(fmt.Sprintf("UPDATE %s SET workspace_id = ? WHERE digital_human_id = ? AND workspace_id = ?", table), target, id, source).Error; err != nil {
			return fmt.Errorf("move DigitalHuman %s: %w", table, err)
		}
	}
	statements := []struct {
		name  string
		query string
		args  []any
	}{
		{"run outbox", "UPDATE digital_human_run_outbox SET workspace_id = ? WHERE workspace_id = ? AND run_id IN (SELECT id FROM digital_human_runs WHERE digital_human_id = ?)", []any{target, source, id}},
		{"tasks", "UPDATE automation_tasks SET workspace_id = ? WHERE workspace_id = ? AND subject_type = ? AND subject_id = ?", []any{target, source, "digital_human", id.String()}},
		{"conversations", "UPDATE chat_runtime_conversations SET workspace_id = ? WHERE workspace_id = ? AND caller_type = ? AND caller_id = ?", []any{target, source, "digital_human", id}},
		// The team's artifacts of this DigitalHuman follow it to its new team.
		{"workspace artifacts", "UPDATE artifacts SET owner_id = ? WHERE application_type = ? AND application_id = ? AND owner_kind = ? AND owner_id = ?", []any{target, "digital_human", id, "workspace", source}},
		{"artifacts", "UPDATE artifacts SET workspace_id = ? WHERE workspace_id = ? AND application_type = ? AND application_id = ?", []any{target, source, "digital_human", id}},
		{"notification channels", "UPDATE notification_channels SET workspace_id = ? WHERE workspace_id = ? AND owner_kind = ? AND owner_id = ?", []any{target, source, "digital_human", id}},
		{"relationship memory", "UPDATE user_memory_relationships SET workspace_id = ? WHERE workspace_id = ? AND consumer_type = ? AND consumer_id = ?", []any{target, source, "digital_human", id}},
		{"execution uploads", "UPDATE agent_execution_uploads SET workspace_id = ? WHERE workspace_id = ? AND application_type = ? AND application_id = ?", []any{target, source, "digital_human", id}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			return fmt.Errorf("move DigitalHuman %s: %w", statement.name, err)
		}
	}
	if err := runtimeauth.NewStore(tx).RelocateResourceWorkspace(ctx, runtimeauth.PublishedRuntimeResourceDigitalHuman, id, source, target); err != nil {
		return fmt.Errorf("move DigitalHuman runtime surfaces: %w", err)
	}
	return db.Exec("SELECT set_config('zgi.digital_human_workspace_move', '', true)").Error
}
