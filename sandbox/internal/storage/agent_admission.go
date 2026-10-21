package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	runtime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// All admission transactions acquire this lock BEFORE any binding row locks.
// The lock is shared by worker processes using this database. Cleanup still
// acquires the lock for ordering but is never denied by a policy mismatch.
const agentAdmissionLockID int64 = 0x5a474941444d

func (s *Store) lockAgentAdmission(ctx context.Context, tx *sql.Tx, backend string) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, agentAdmissionLockID); err != nil {
		return fmt.Errorf("lock agent admission: %w", err)
	}
	var existing []byte
	err := tx.QueryRowContext(ctx, `SELECT limits_json FROM agent_runtime_admission_policies WHERE backend = $1`, backend).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if s.admission == nil {
		// A process with no configured limits cannot bypass a registered policy.
		if err == nil {
			return runtime.ErrAdmissionPolicy
		}
		return nil
	}
	if err == nil {
		var limits runtime.AdmissionLimits
		if json.Unmarshal(existing, &limits) != nil || limits.WithDefaults() != *s.admission {
			return runtime.ErrAdmissionPolicy
		}
		return nil
	}
	encoded, err := json.Marshal(s.admission)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO agent_runtime_admission_policies (backend, limits_json) VALUES ($1, $2::jsonb)`, backend, string(encoded))
	return err
}

func (s *Store) checkBindingCapacity(ctx context.Context, tx *sql.Tx, binding runtime.Binding) error {
	if err := s.checkEndUserBindingCapacity(ctx, tx, binding); err != nil {
		return err
	}
	if s.admission == nil {
		return nil
	}
	var total, org int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE organization_id = $2)
		FROM agent_runtime_bindings WHERE backend = $1 AND status NOT IN ('suspended', 'deleted')`, binding.Backend, binding.OrganizationID).Scan(&total, &org)
	if err != nil {
		return err
	}
	if total >= s.admission.Bindings || org >= s.admission.BindingsPerOrg {
		return runtime.ErrCapacityExceeded
	}
	return nil
}

func (s *Store) checkJobCapacity(ctx context.Context, tx *sql.Tx, job runtime.Job) error {
	if err := s.checkEndUserJobCapacity(ctx, tx, job); err != nil {
		return err
	}
	if s.admission == nil {
		return nil
	}
	if job.Timeout <= 0 || job.Timeout > s.admission.JobTimeout() {
		return fmt.Errorf("%w: Job timeout exceeds admission policy", runtime.ErrInvalidRequest)
	}
	var total, org, binding int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*), COUNT(*) FILTER (WHERE organization_id = $2),
		COUNT(*) FILTER (WHERE binding_id = $3) FROM agent_runtime_jobs
		WHERE backend = $1 AND status IN ('queued', 'running')`, job.Backend, job.OrganizationID, job.BindingID).Scan(&total, &org, &binding)
	if err != nil {
		return err
	}
	if total >= s.admission.Jobs || org >= s.admission.JobsPerOrg || binding >= s.admission.JobsPerBinding {
		return runtime.ErrCapacityExceeded
	}
	return nil
}

// ReplaceAdmissionPolicy is an explicit maintenance action. It requires a
// drained backend, keeps suspended files, and fences stale worker settings at
// the next admission. It never changes existing resources or file retention.
func ReplaceAdmissionPolicy(ctx context.Context, dsn, backend string, limits runtime.AdmissionLimits) error {
	if err := limits.Validate(); err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, agentAdmissionLockID); err != nil {
		return err
	}
	var busy bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM agent_runtime_bindings WHERE backend = $1 AND status NOT IN ('suspended', 'deleted'))
		OR EXISTS (SELECT 1 FROM agent_runtime_jobs WHERE backend = $1 AND status IN ('queued', 'running'))`, backend).Scan(&busy); err != nil {
		return err
	}
	if busy {
		return fmt.Errorf("%w: drain backend before changing admission limits", runtime.ErrInvalidTransition)
	}
	encoded, err := json.Marshal(limits)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_runtime_admission_policies (backend, limits_json) VALUES ($1, $2::jsonb)
		ON CONFLICT (backend) DO UPDATE SET limits_json = EXCLUDED.limits_json`, backend, string(encoded)); err != nil {
		return err
	}
	return tx.Commit()
}
