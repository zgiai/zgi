package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentruntime"
)

var (
	_ agentruntime.CleanupRepository = (*Store)(nil)
	_ agentruntime.LeaseRepository   = (*Store)(nil)
)

const agentRuntimeCleanupColumns = `
	id, resource_type, resource_id, backend, backend_ref, action, status,
	attempts, last_error, not_before, lease_owner, lease_expires_at,
	created_at, updated_at`

func (s *Store) ClaimCleanup(ctx context.Context, ownerID string, now time.Time, leaseExpiresAt time.Time) (agentruntime.CleanupTask, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id
			FROM agent_runtime_cleanup_tasks
			WHERE ((status IN ('pending', 'retrying') AND not_before <= $1)
			    OR (status = 'running' AND lease_expires_at <= $1))
			ORDER BY not_before ASC, created_at ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE agent_runtime_cleanup_tasks
		SET status = 'running',
		    attempts = attempts + 1,
		    lease_owner = $2,
		    lease_expires_at = $3,
		    updated_at = $1
		WHERE id = (SELECT id FROM candidate)
		RETURNING `+agentRuntimeCleanupColumns,
		now.UTC(), ownerID, leaseExpiresAt.UTC(),
	)
	task, err := scanAgentRuntimeCleanup(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.CleanupTask{}, false, nil
	}
	return task, err == nil, err
}

func (s *Store) CompleteCleanup(ctx context.Context, taskID string, ownerID string, succeeded bool, lastError string, notBefore time.Time, now time.Time) error {
	status := "retrying"
	if succeeded {
		status = "completed"
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_runtime_cleanup_tasks
		SET status = $3,
		    last_error = $4,
		    not_before = $5,
		    lease_owner = '',
		    lease_expires_at = NULL,
		    updated_at = $6
		WHERE id = $1
		  AND lease_owner = $2
		  AND status = 'running'
		  AND lease_expires_at > $6
	`, taskID, ownerID, status, lastError, notBefore.UTC(), now.UTC())
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return agentruntime.ErrCleanupClaimLost
	}
	return nil
}

func (s *Store) AcquireLease(ctx context.Context, lease agentruntime.AdmissionLease, now time.Time) (agentruntime.AdmissionLease, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO agent_runtime_admission_leases (
			id, binding_id, owner_id, generation, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (binding_id) DO UPDATE SET
			owner_id = EXCLUDED.owner_id,
			generation = agent_runtime_admission_leases.generation + 1,
			expires_at = EXCLUDED.expires_at,
			updated_at = EXCLUDED.updated_at
		WHERE agent_runtime_admission_leases.owner_id = EXCLUDED.owner_id
		   OR agent_runtime_admission_leases.expires_at <= $8
		RETURNING id, binding_id, owner_id, generation, expires_at, created_at, updated_at
	`, lease.ID, lease.BindingID, lease.OwnerID, lease.Generation, lease.ExpiresAt.UTC(), lease.CreatedAt.UTC(), lease.UpdatedAt.UTC(), now.UTC())
	acquired, err := scanAgentRuntimeLease(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.AdmissionLease{}, agentruntime.ErrLeaseHeld
	}
	return acquired, err
}

func (s *Store) RenewLease(ctx context.Context, leaseID string, ownerID string, expectedGeneration int64, now time.Time, expiresAt time.Time) (agentruntime.AdmissionLease, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE agent_runtime_admission_leases
		SET generation = generation + 1,
		    expires_at = $4,
		    updated_at = $3
		WHERE id = $1
		  AND owner_id = $2
		  AND generation = $5
		  AND expires_at > $3
		RETURNING id, binding_id, owner_id, generation, expires_at, created_at, updated_at
	`, leaseID, ownerID, now.UTC(), expiresAt.UTC(), expectedGeneration)
	renewed, err := scanAgentRuntimeLease(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.AdmissionLease{}, agentruntime.ErrGenerationConflict
	}
	return renewed, err
}

func (s *Store) ReleaseLease(ctx context.Context, leaseID string, ownerID string, expectedGeneration int64) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM agent_runtime_admission_leases
		WHERE id = $1 AND owner_id = $2 AND generation = $3
	`, leaseID, ownerID, expectedGeneration)
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

func scanAgentRuntimeCleanup(scanner rowScanner) (agentruntime.CleanupTask, error) {
	var task agentruntime.CleanupTask
	var leaseExpiresAt sql.NullTime
	err := scanner.Scan(
		&task.ID,
		&task.ResourceType,
		&task.ResourceID,
		&task.Backend,
		&task.BackendRef,
		&task.Action,
		&task.Status,
		&task.Attempts,
		&task.LastError,
		&task.NotBefore,
		&task.LeaseOwner,
		&leaseExpiresAt,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if leaseExpiresAt.Valid {
		task.LeaseExpiresAt = &leaseExpiresAt.Time
	}
	return task, err
}

func scanAgentRuntimeLease(scanner rowScanner) (agentruntime.AdmissionLease, error) {
	var lease agentruntime.AdmissionLease
	err := scanner.Scan(
		&lease.ID,
		&lease.BindingID,
		&lease.OwnerID,
		&lease.Generation,
		&lease.ExpiresAt,
		&lease.CreatedAt,
		&lease.UpdatedAt,
	)
	return lease, err
}
