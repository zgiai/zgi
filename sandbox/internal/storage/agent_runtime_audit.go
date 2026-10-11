package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	runtime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

// ReadRuntimeAudit does not call Open/prepare, run migrations, renew leases or
// wake provider resources. The caller owns the observations and timeout.
func ReadRuntimeAudit(ctx context.Context, dsn, backend string, resources []runtime.ResourceObservation) (runtime.RuntimeAudit, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return runtime.RuntimeAudit{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return runtime.RuntimeAudit{}, err
	}
	defer func() { _ = tx.Rollback() }()
	bindings, err := auditRows(ctx, tx, `SELECT `+agentRuntimeBindingColumns+` FROM agent_runtime_bindings WHERE backend = $1 ORDER BY id LIMIT 10001`, backend, scanAgentRuntimeBinding)
	if err != nil {
		return runtime.RuntimeAudit{}, err
	}
	jobs, err := auditRows(ctx, tx, `SELECT `+agentExecutionJobColumns+` FROM agent_runtime_jobs WHERE backend = $1 AND status IN ('queued', 'running') ORDER BY id LIMIT 10001`, backend, scanAgentExecutionJob)
	if err != nil {
		return runtime.RuntimeAudit{}, err
	}
	cleanup, err := auditRows(ctx, tx, `SELECT id, binding_id, not_before, lease_expires_at
		FROM agent_runtime_cleanup_tasks WHERE backend = $1 AND status <> 'completed' ORDER BY id LIMIT 10001`, backend,
		func(row rowScanner) (runtime.CleanupTask, error) {
			var task runtime.CleanupTask
			err := row.Scan(&task.ID, &task.BindingID, &task.NotBefore, &task.LeaseExpiresAt)
			return task, err
		})
	if err != nil {
		return runtime.RuntimeAudit{}, err
	}
	if err := tx.Commit(); err != nil {
		return runtime.RuntimeAudit{}, err
	}
	return runtime.AuditInventory(backend, time.Now(), bindings, jobs, cleanup, resources), nil
}

func auditRows[T any](ctx context.Context, tx *sql.Tx, query, backend string, scan func(rowScanner) (T, error)) ([]T, error) {
	rows, err := tx.QueryContext(ctx, query, backend)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
		if len(result) > 10000 {
			return nil, fmt.Errorf("runtime audit exceeds 10000 rows; narrow the deployment database")
		}
	}
	return result, rows.Err()
}
