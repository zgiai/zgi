package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	agentruntime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

var _ agentruntime.Repository = (*Store)(nil)

const agentRuntimeBindingColumns = `
	id, organization_id, subject_id, agent_id, conversation_id, profile_id,
	home_snapshot_id, workspace_id, backend, backend_ref, workspace_ref,
	idempotency_key, request_hash, status, generation, failure_reason,
	created_at, updated_at, last_activity_at, expires_at`

func (s *Store) ReserveBinding(ctx context.Context, binding agentruntime.Binding) (agentruntime.Binding, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO agent_runtime_bindings (
			id, organization_id, subject_id, agent_id, conversation_id, profile_id,
			home_snapshot_id, workspace_id, backend, backend_ref, workspace_ref,
			idempotency_key, request_hash, status, generation, failure_reason,
			created_at, updated_at, last_activity_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15, $16,
			$17, $18, $19, $20
		)
		ON CONFLICT (organization_id, subject_id, idempotency_key) DO NOTHING
		RETURNING `+agentRuntimeBindingColumns,
		binding.ID,
		binding.OrganizationID,
		binding.SubjectID,
		binding.AgentID,
		binding.ConversationID,
		binding.ProfileID,
		binding.HomeSnapshotID,
		binding.WorkspaceID,
		binding.Backend,
		binding.BackendRef,
		binding.WorkspaceRef,
		binding.IdempotencyKey,
		binding.RequestHash,
		binding.Status,
		binding.Generation,
		binding.FailureReason,
		binding.CreatedAt.UTC(),
		binding.UpdatedAt.UTC(),
		binding.LastActivityAt.UTC(),
		binding.ExpiresAt.UTC(),
	)
	created, err := scanAgentRuntimeBinding(row)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Binding{}, false, err
	}
	row = s.db.QueryRowContext(ctx, `
		SELECT `+agentRuntimeBindingColumns+`
		FROM agent_runtime_bindings
		WHERE organization_id = $1 AND subject_id = $2 AND idempotency_key = $3
	`, binding.OrganizationID, binding.SubjectID, binding.IdempotencyKey)
	existing, err := scanAgentRuntimeBinding(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return agentruntime.Binding{}, false, agentruntime.ErrNotFound
		}
		return agentruntime.Binding{}, false, err
	}
	return existing, false, nil
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

func (s *Store) UpdateBinding(ctx context.Context, binding agentruntime.Binding, expectedGeneration int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_runtime_bindings
		SET backend_ref = $2,
		    workspace_ref = $3,
		    status = $4,
		    generation = $5,
		    failure_reason = $6,
		    updated_at = $7,
		    last_activity_at = $8,
		    expires_at = $9
		WHERE id = $1 AND generation = $10
	`,
		binding.ID,
		binding.BackendRef,
		binding.WorkspaceRef,
		binding.Status,
		binding.Generation,
		binding.FailureReason,
		binding.UpdatedAt.UTC(),
		binding.LastActivityAt.UTC(),
		binding.ExpiresAt.UTC(),
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
	return nil
}

func (s *Store) EnqueueCleanup(ctx context.Context, task agentruntime.CleanupTask) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_runtime_cleanup_tasks (
			id, resource_type, resource_id, backend, backend_ref, workspace_ref, action,
			status, attempts, last_error, not_before, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13
		)
		ON CONFLICT (resource_type, resource_id, action) DO UPDATE SET
			backend = EXCLUDED.backend,
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
	err := scanner.Scan(
		&binding.ID,
		&binding.OrganizationID,
		&binding.SubjectID,
		&binding.AgentID,
		&binding.ConversationID,
		&binding.ProfileID,
		&binding.HomeSnapshotID,
		&binding.WorkspaceID,
		&binding.Backend,
		&binding.BackendRef,
		&binding.WorkspaceRef,
		&binding.IdempotencyKey,
		&binding.RequestHash,
		&binding.Status,
		&binding.Generation,
		&binding.FailureReason,
		&binding.CreatedAt,
		&binding.UpdatedAt,
		&binding.LastActivityAt,
		&binding.ExpiresAt,
	)
	return binding, err
}
