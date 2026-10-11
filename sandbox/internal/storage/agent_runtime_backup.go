package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/checkpointbackup"
)

// ReadCheckpointBindings takes a bounded, read-only consistent DB snapshot. The
// operator must stop admission/retention for the export window: storage and DB
// snapshots cannot otherwise be atomic. A concurrent object removal fails export.
func ReadCheckpointBindings(ctx context.Context, dsn, backend string) ([]agentexecution.Binding, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var busy bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_runtime_jobs WHERE backend=$1 AND status IN ('queued','running'))`, backend).Scan(&busy); err != nil {
		return nil, err
	}
	if busy {
		return nil, errors.New("backup requires terminal Jobs and suspended bindings")
	}
	bindings, err := auditRows(ctx, tx, `SELECT `+agentRuntimeBindingColumns+` FROM agent_runtime_bindings WHERE backend=$1 AND status <> 'deleted' ORDER BY id LIMIT 10001`, backend, scanAgentRuntimeBinding)
	if err != nil {
		return nil, err
	}
	for i := range bindings {
		// Paused provider resources remain owned by the source deployment, never
		// by a recovery target. Do not carry diagnostic text into a recovery set.
		bindings[i].BackendRef = ""
		bindings[i].WorkspaceRef = ""
		bindings[i].FailureReason = ""
	}
	if err := checkpointbackup.ValidateBindings(bindings, backend, time.Now()); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return bindings, nil
}

var recoveryTables = []string{"agent_runtime_bindings", "agent_runtime_jobs", "agent_runtime_job_output_chunks", "agent_runtime_admission_leases", "agent_runtime_cleanup_tasks", "agent_runtime_workspaces", "agent_runtime_home_snapshots"}

// CheckRecoveryTarget rejects existing runtime data before checkpoint uploads or
// migrations. Configuration registries are not user execution data.
func CheckRecoveryTarget(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, table := range recoveryTables {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			continue
		}
		var populated bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+`)`).Scan(&populated); err != nil {
			return err
		}
		if populated {
			return errors.New("recovery target contains runtime data; refusing overwrite")
		}
	}
	return nil
}

// RestoreCheckpointBindings atomically imports a trusted, verified recovery set
// into an empty initialized target. It neither restores Jobs/cleanup tasks nor
// connects to original provider resources. Objects must already be verified.
func (s *Store) RestoreCheckpointBindings(ctx context.Context, backend string, bindings []agentexecution.Binding) error {
	if err := checkpointbackup.ValidateBindings(bindings, backend, time.Now()); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, agentAdmissionLockID); err != nil {
		return err
	}
	for _, table := range recoveryTables {
		if _, err := tx.ExecContext(ctx, `LOCK TABLE `+table+` IN SHARE ROW EXCLUSIVE MODE`); err != nil {
			return err
		}
		var populated bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+`)`).Scan(&populated); err != nil {
			return err
		}
		if populated {
			return errors.New("recovery target contains runtime data; refusing overwrite")
		}
	}
	for _, b := range bindings {
		identity, err := encodeBindingIdentity(b.Identity)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_runtime_bindings (`+agentRuntimeBindingColumns+`) VALUES (
		$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'','',$12,'',$13,$14,'suspended',$15,'',$16,$17,$18,$19,$20,$21,$22)`,
			b.ID, b.OrganizationID, b.SubjectID, b.AgentID, b.ConversationID, b.ProfileID, b.NetworkPolicy, b.NetworkMode, b.HomeSnapshotID, b.WorkspaceID, b.Backend, b.CheckpointKey, b.IdempotencyKey, b.RequestHash, b.Generation+1, b.CreatedAt.UTC(), time.Now().UTC(), b.LastActivityAt.UTC(), b.ExpiresAt.UTC(), b.RetentionExpiresAt.UTC(), b.HomeSnapshotRef, identity)
		if err != nil {
			return fmt.Errorf("import recovery binding: %w", err)
		}
	}
	return tx.Commit()
}
