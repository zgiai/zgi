package storage

import (
	"context"
	"database/sql"

	runtime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// These counts run under agentAdmissionLockID, before any new allocation or
// queued Job is persisted. They span backends, entry points and publications;
// changing any of those must not reset an application's end-user quota.
const endUserBindingPredicate = `organization_id = $1 AND subject_id = $2
	AND execution_identity->>'workspace_id' = $3
	AND execution_identity->>'application_type' = $4
	AND execution_identity->>'application_id' = $5
	AND execution_identity->>'subject_kind' = $6`

func (s *Store) endUserLimits() runtime.AdmissionLimits {
	if s.admission == nil {
		return (runtime.AdmissionLimits{}).WithDefaults()
	}
	return s.admission.WithDefaults()
}

func (s *Store) checkEndUserBindingCapacity(ctx context.Context, tx *sql.Tx, binding runtime.Binding) error {
	if binding.Identity == nil {
		return nil // Historical console behavior is deliberately unchanged.
	}
	i := binding.Identity
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runtime_bindings WHERE `+endUserBindingPredicate+
		` AND status NOT IN ('suspended', 'deleted')`, binding.OrganizationID, binding.SubjectID,
		i.WorkspaceID, i.ApplicationType, i.ApplicationID, i.SubjectKind).Scan(&count)
	if err != nil {
		return err
	}
	if count >= s.endUserLimits().BindingsPerEndUser {
		return runtime.ErrCapacityExceeded
	}
	return nil
}

func (s *Store) checkEndUserJobCapacity(ctx context.Context, tx *sql.Tx, job runtime.Job) error {
	binding, err := scanAgentRuntimeBinding(tx.QueryRowContext(ctx, `SELECT `+agentRuntimeBindingColumns+
		` FROM agent_runtime_bindings WHERE id = $1`, job.BindingID))
	if err == sql.ErrNoRows {
		return runtime.ErrNotFound
	}
	if err != nil {
		return err
	}
	if binding.OrganizationID != job.OrganizationID || binding.SubjectID != job.SubjectID || binding.Backend != job.Backend {
		return runtime.ErrNotFound
	}
	if binding.Identity == nil {
		return nil
	}
	i := binding.Identity
	var count int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_runtime_jobs WHERE status IN ('queued', 'running')
		AND binding_id IN (SELECT id FROM agent_runtime_bindings WHERE `+endUserBindingPredicate+`)`,
		binding.OrganizationID, binding.SubjectID, i.WorkspaceID, i.ApplicationType, i.ApplicationID, i.SubjectKind).Scan(&count)
	if err != nil {
		return err
	}
	if count >= s.endUserLimits().JobsPerEndUser {
		return runtime.ErrCapacityExceeded
	}
	return nil
}
