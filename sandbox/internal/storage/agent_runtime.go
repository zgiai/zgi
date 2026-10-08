package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agentruntime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

var _ agentruntime.Repository = (*Store)(nil)

const agentRuntimeBindingColumns = `
	id, organization_id, subject_id, agent_id, conversation_id, profile_id,
	network_policy, network_mode, home_snapshot_id, workspace_id, backend, backend_ref, workspace_ref, checkpoint_key, pending_checkpoint_key,
	idempotency_key, request_hash, status, generation, failure_reason,
	created_at, updated_at, last_activity_at, expires_at, retention_expires_at, home_snapshot_ref, execution_identity`

func (s *Store) ReserveBinding(ctx context.Context, binding agentruntime.Binding) (agentruntime.Binding, bool, error) {
	identity, err := encodeBindingIdentity(binding.Identity)
	if err != nil {
		return agentruntime.Binding{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return agentruntime.Binding{}, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.lockAgentAdmission(ctx, tx, binding.Backend); err != nil {
		return agentruntime.Binding{}, false, err
	}
	existing, err := scanAgentRuntimeBinding(tx.QueryRowContext(ctx, `SELECT `+agentRuntimeBindingColumns+`
		FROM agent_runtime_bindings WHERE organization_id = $1 AND subject_id = $2 AND idempotency_key = $3`, binding.OrganizationID, binding.SubjectID, binding.IdempotencyKey))
	if err == nil {
		return existing, false, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Binding{}, false, err
	}
	if agentruntime.BindingConsumesCapacity(binding.Status) {
		if err := s.checkBindingCapacity(ctx, tx, binding.Backend, binding.OrganizationID); err != nil {
			return agentruntime.Binding{}, false, err
		}
	}
	if strings.TrimSpace(binding.NetworkPolicy) == "" {
		binding.NetworkPolicy = "offline"
	}
	if strings.TrimSpace(binding.NetworkMode) == "" {
		binding.NetworkMode = "disabled"
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO agent_runtime_bindings (
			id, organization_id, subject_id, agent_id, conversation_id, profile_id,
			network_policy, network_mode, home_snapshot_id, workspace_id, backend, backend_ref, workspace_ref, checkpoint_key, pending_checkpoint_key,
			idempotency_key, request_hash, status, generation, failure_reason,
			created_at, updated_at, last_activity_at, expires_at, retention_expires_at, home_snapshot_ref, execution_identity
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19,
			$20, $21, $22, $23, $24, $25, $26, $27
		)
		ON CONFLICT (organization_id, subject_id, idempotency_key) DO NOTHING
		RETURNING `+agentRuntimeBindingColumns,
		binding.ID,
		binding.OrganizationID,
		binding.SubjectID,
		binding.AgentID,
		binding.ConversationID,
		binding.ProfileID,
		binding.NetworkPolicy,
		binding.NetworkMode,
		binding.HomeSnapshotID,
		binding.WorkspaceID,
		binding.Backend,
		binding.BackendRef,
		binding.WorkspaceRef,
		binding.CheckpointKey,
		binding.PendingCheckpointKey,
		binding.IdempotencyKey,
		binding.RequestHash,
		binding.Status,
		binding.Generation,
		binding.FailureReason,
		binding.CreatedAt.UTC(),
		binding.UpdatedAt.UTC(),
		binding.LastActivityAt.UTC(),
		binding.ExpiresAt.UTC(),
		binding.RetentionExpiresAt.UTC(),
		binding.HomeSnapshotRef,
		identity,
	)
	created, err := scanAgentRuntimeBinding(row)
	if err == nil {
		return created, true, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Binding{}, false, err
	}
	row = tx.QueryRowContext(ctx, `
		SELECT `+agentRuntimeBindingColumns+`
		FROM agent_runtime_bindings
		WHERE organization_id = $1 AND subject_id = $2 AND idempotency_key = $3
	`, binding.OrganizationID, binding.SubjectID, binding.IdempotencyKey)
	existing, err = scanAgentRuntimeBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agentruntime.Binding{}, false, agentruntime.ErrNotFound
		}
		return agentruntime.Binding{}, false, err
	}
	return existing, false, tx.Commit()
}

func (s *Store) Binding(ctx context.Context, id string) (agentruntime.Binding, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+agentRuntimeBindingColumns+`
		FROM agent_runtime_bindings
		WHERE id = $1
	`, id)
	binding, err := scanAgentRuntimeBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Binding{}, agentruntime.ErrNotFound
	}
	return binding, err
}

func (s *Store) BindingByIdempotency(ctx context.Context, scope agentruntime.Scope, key string) (agentruntime.Binding, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+agentRuntimeBindingColumns+`
		FROM agent_runtime_bindings
		WHERE organization_id = $1 AND subject_id = $2 AND idempotency_key = $3
	`, scope.OrganizationID, scope.SubjectID, key)
	binding, err := scanAgentRuntimeBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Binding{}, agentruntime.ErrNotFound
	}
	return binding, err
}

func (s *Store) TouchBinding(ctx context.Context, id string, expectedGeneration int64, lastActivityAt time.Time, expiresAt time.Time, retentionExpiresAt time.Time) (agentruntime.Binding, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE agent_runtime_bindings
		SET last_activity_at = $3,
		    expires_at = $4,
		    retention_expires_at = $5,
		    updated_at = $3,
		    generation = generation + 1
		WHERE id = $1 AND generation = $2 AND status = 'active'
		RETURNING `+agentRuntimeBindingColumns,
		id, expectedGeneration, lastActivityAt.UTC(), expiresAt.UTC(), retentionExpiresAt.UTC(),
	)
	binding, err := scanAgentRuntimeBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Binding{}, agentruntime.ErrGenerationConflict
	}
	return binding, err
}

func (s *Store) ExpiredRetainedBindings(ctx context.Context, now time.Time, limit int) ([]agentruntime.Binding, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+agentRuntimeBindingColumns+`
		FROM agent_runtime_bindings
		WHERE (status = 'suspended' AND retention_expires_at <= $1) OR status = 'deleting'
		ORDER BY retention_expires_at ASC, id ASC
		LIMIT $2
	`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := make([]agentruntime.Binding, 0)
	for rows.Next() {
		binding, err := scanAgentRuntimeBinding(rows)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (s *Store) ExpiredBindings(ctx context.Context, now time.Time, limit int) ([]agentruntime.Binding, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+agentRuntimeBindingColumns+`
		FROM agent_runtime_bindings
		WHERE expires_at <= $1
		  AND (status = 'active' OR (status = 'suspending' AND updated_at <= $3))
		ORDER BY expires_at ASC, id ASC
		LIMIT $2
	`, now.UTC(), limit, now.UTC().Add(-agentruntime.IdleSuspendRetryDelay))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	bindings := make([]agentruntime.Binding, 0)
	for rows.Next() {
		binding, err := scanAgentRuntimeBinding(rows)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (s *Store) UpdateBinding(ctx context.Context, binding agentruntime.Binding, expectedGeneration int64) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, agentAdmissionLockID); err != nil {
		return err
	}
	var previousCheckpointKey string
	var previousStatus agentruntime.BindingStatus
	if err := tx.QueryRowContext(ctx, `
		SELECT checkpoint_key, status FROM agent_runtime_bindings
		WHERE id = $1 AND generation = $2 FOR UPDATE
	`, binding.ID, expectedGeneration).Scan(&previousCheckpointKey, &previousStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agentruntime.ErrGenerationConflict
		}
		return err
	}
	if !agentruntime.BindingConsumesCapacity(previousStatus) && agentruntime.BindingConsumesCapacity(binding.Status) {
		if err := s.lockAgentAdmission(ctx, tx, binding.Backend); err != nil {
			return err
		}
		if err := s.checkBindingCapacity(ctx, tx, binding.Backend, binding.OrganizationID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE agent_runtime_bindings
		SET backend_ref = $2,
		    workspace_ref = $3,
		    checkpoint_key = $4,
		    pending_checkpoint_key = $5,
		    status = $6,
		    generation = $7,
		    failure_reason = $8,
		    updated_at = $9,
		    last_activity_at = $10,
		    expires_at = $11,
		    retention_expires_at = $12,
		    home_snapshot_ref = $13
		WHERE id = $1 AND generation = $14
	`,
		binding.ID,
		binding.BackendRef,
		binding.WorkspaceRef,
		binding.CheckpointKey,
		binding.PendingCheckpointKey,
		binding.Status,
		binding.Generation,
		binding.FailureReason,
		binding.UpdatedAt.UTC(),
		binding.LastActivityAt.UTC(),
		binding.ExpiresAt.UTC(),
		binding.RetentionExpiresAt.UTC(),
		binding.HomeSnapshotRef,
		expectedGeneration,
	)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return agentruntime.ErrGenerationConflict
	}
	if previousCheckpointKey != "" && previousCheckpointKey != binding.CheckpointKey && binding.Status != agentruntime.BindingStatusDeleted {
		// Commit the new checkpoint reference and the old-object cleanup task
		// together. A crash at either side of this transaction leaves a
		// recoverable reference, never an untracked deleted snapshot.
		_, err := tx.ExecContext(ctx, `
			INSERT INTO agent_runtime_cleanup_tasks (
				id, resource_type, resource_id, backend, action, status,
				last_error, not_before, created_at, updated_at
			) VALUES ($1, 'checkpoint', $2, '', 'delete', 'pending', '', $3, $3, $3)
			ON CONFLICT (resource_type, resource_id, action) DO UPDATE SET
				status = 'pending', not_before = EXCLUDED.not_before,
				last_error = '', updated_at = EXCLUDED.updated_at
		`, "arc_cp_"+previousCheckpointKey, previousCheckpointKey, binding.UpdatedAt.UTC())
		if err != nil {
			return fmt.Errorf("enqueue superseded checkpoint cleanup: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) EnqueueCleanup(ctx context.Context, task agentruntime.CleanupTask) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_runtime_cleanup_tasks (
			id, resource_type, resource_id, binding_id, binding_generation, backend, backend_ref, workspace_ref, action,
			status, attempts, last_error, not_before, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15
		)
		ON CONFLICT (resource_type, resource_id, action) DO UPDATE SET
			backend = EXCLUDED.backend,
			binding_id = EXCLUDED.binding_id,
			binding_generation = EXCLUDED.binding_generation,
			backend_ref = EXCLUDED.backend_ref,
			workspace_ref = EXCLUDED.workspace_ref,
			status = 'pending',
			last_error = EXCLUDED.last_error,
			not_before = LEAST(agent_runtime_cleanup_tasks.not_before, EXCLUDED.not_before),
			updated_at = EXCLUDED.updated_at
	`,
		task.ID,
		task.ResourceType,
		task.ResourceID,
		task.BindingID,
		task.BindingGeneration,
		task.Backend,
		task.BackendRef,
		task.WorkspaceRef,
		task.Action,
		task.Status,
		task.Attempts,
		task.LastError,
		task.NotBefore.UTC(),
		task.CreatedAt.UTC(),
		task.UpdatedAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("enqueue cleanup task: %w", err)
	}
	return nil
}

func scanAgentRuntimeBinding(scanner rowScanner) (agentruntime.Binding, error) {
	var binding agentruntime.Binding
	var identity []byte
	err := scanner.Scan(
		&binding.ID,
		&binding.OrganizationID,
		&binding.SubjectID,
		&binding.AgentID,
		&binding.ConversationID,
		&binding.ProfileID,
		&binding.NetworkPolicy,
		&binding.NetworkMode,
		&binding.HomeSnapshotID,
		&binding.WorkspaceID,
		&binding.Backend,
		&binding.BackendRef,
		&binding.WorkspaceRef,
		&binding.CheckpointKey,
		&binding.PendingCheckpointKey,
		&binding.IdempotencyKey,
		&binding.RequestHash,
		&binding.Status,
		&binding.Generation,
		&binding.FailureReason,
		&binding.CreatedAt,
		&binding.UpdatedAt,
		&binding.LastActivityAt,
		&binding.ExpiresAt,
		&binding.RetentionExpiresAt,
		&binding.HomeSnapshotRef,
		&identity,
	)
	if err == nil && string(identity) != "null" {
		binding.Identity, err = agentruntime.DecodeExecutionIdentity(string(identity))
	}
	return binding, err
}

func encodeBindingIdentity(identity *agentruntime.ExecutionIdentity) (string, error) {
	if identity != nil {
		if err := identity.Validate(); err != nil {
			return "", err
		}
	}
	value, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("encode execution identity: %w", err)
	}
	return string(value), nil
}
