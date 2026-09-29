package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	agentruntime "github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

var _ agentruntime.JobRepository = (*Store)(nil)

const agentExecutionJobColumns = `
	id, binding_id, organization_id, subject_id, backend, backend_ref,
	workspace_ref, idempotency_key, request_hash, status, generation,
	command, args_json, environment_json, work_dir, pty, pty_columns,
	pty_rows, timeout_ms, exit_code, failure_reason, cancel_requested,
	claim_owner, claim_expires_at, attempts, stdout_bytes, stderr_bytes,
	created_at, updated_at, started_at, finished_at`

func (s *Store) CreateJob(ctx context.Context, job agentruntime.Job) (agentruntime.Job, bool, error) {
	argsJSON, environmentJSON, commandJSON, err := marshalJobCommand(job)
	if err != nil {
		return agentruntime.Job{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO agent_runtime_jobs (
			id, binding_id, organization_id, subject_id, backend, backend_ref,
			workspace_ref, idempotency_key, request_hash, status, generation,
			command, args_json, environment_json, command_json, work_dir,
			pty, pty_columns, pty_rows, timeout_ms, failure_reason,
			cancel_requested, claim_owner, attempts, stdout_bytes, stderr_bytes,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13::jsonb, $14::jsonb, $15::jsonb, $16,
			$17, $18, $19, $20, $21,
			$22, $23, $24, $25, $26,
			$27, $28
		)
		ON CONFLICT (binding_id, idempotency_key) WHERE idempotency_key <> '' DO NOTHING
		RETURNING `+agentExecutionJobColumns,
		job.ID, job.BindingID, job.OrganizationID, job.SubjectID, job.Backend, job.BackendRef,
		job.WorkspaceRef, job.IdempotencyKey, job.RequestHash, job.Status, job.Generation,
		job.Command, string(argsJSON), string(environmentJSON), string(commandJSON), job.WorkDir,
		job.PTY, job.PTYColumns, job.PTYRows, durationMilliseconds(job.Timeout), job.FailureReason,
		job.CancelRequested, job.ClaimOwner, job.Attempts, job.StdoutBytes, job.StderrBytes,
		job.CreatedAt.UTC(), job.UpdatedAt.UTC(),
	)
	created, err := scanAgentExecutionJob(row)
	if err == nil {
		return created, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Job{}, false, err
	}
	row = s.db.QueryRowContext(ctx, `
		SELECT `+agentExecutionJobColumns+`
		FROM agent_runtime_jobs
		WHERE binding_id = $1 AND idempotency_key = $2
	`, job.BindingID, job.IdempotencyKey)
	existing, err := scanAgentExecutionJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Job{}, false, agentruntime.ErrNotFound
	}
	return existing, false, err
}

func (s *Store) Job(ctx context.Context, id string) (agentruntime.Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+agentExecutionJobColumns+` FROM agent_runtime_jobs WHERE id = $1`, id)
	job, err := scanAgentExecutionJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Job{}, agentruntime.ErrNotFound
	}
	return job, err
}

func (s *Store) ClaimJob(ctx context.Context, ownerID string, now time.Time, leaseExpiresAt time.Time) (agentruntime.Job, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT id
			FROM agent_runtime_jobs
			WHERE cancel_requested = false
			  AND (status = 'queued' OR (status = 'running' AND claim_expires_at <= $1))
			ORDER BY created_at ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE agent_runtime_jobs
		SET status = 'running',
		    generation = generation + 1,
		    claim_owner = $2,
		    claim_expires_at = $3,
		    attempts = attempts + 1,
		    started_at = COALESCE(started_at, $1),
		    updated_at = $1
		WHERE id = (SELECT id FROM candidate)
		RETURNING `+agentExecutionJobColumns,
		now.UTC(), ownerID, leaseExpiresAt.UTC(),
	)
	job, err := scanAgentExecutionJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Job{}, false, nil
	}
	return job, err == nil, err
}

func (s *Store) RenewJobClaim(ctx context.Context, jobID string, ownerID string, now time.Time, leaseExpiresAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_runtime_jobs
		SET claim_expires_at = $4, updated_at = $3
		WHERE id = $1
		  AND claim_owner = $2
		  AND status = 'running'
		  AND claim_expires_at > $3
	`, jobID, ownerID, now.UTC(), leaseExpiresAt.UTC())
	if err != nil {
		return err
	}
	return requireAffected(result, agentruntime.ErrJobClaimLost)
}

func (s *Store) AppendJobOutput(ctx context.Context, jobID string, stream agentruntime.OutputStream, data []byte, maximumBytes int64, now time.Time) (agentruntime.OutputChunk, error) {
	if stream != agentruntime.OutputStreamStdout && stream != agentruntime.OutputStreamStderr {
		return agentruntime.OutputChunk{}, agentruntime.ErrInvalidRequest
	}
	stdoutIncrement := 0
	stderrIncrement := 0
	if stream == agentruntime.OutputStreamStdout {
		stdoutIncrement = len(data)
	} else {
		stderrIncrement = len(data)
	}
	row := s.db.QueryRowContext(ctx, `
		WITH updated AS (
			UPDATE agent_runtime_jobs
			SET next_output_sequence = next_output_sequence + 1,
			    stdout_bytes = stdout_bytes + $3,
			    stderr_bytes = stderr_bytes + $4,
			    updated_at = $7
			WHERE id = $1
			  AND status = 'running'
			  AND stdout_bytes + stderr_bytes + $5 <= $6
			RETURNING next_output_sequence - 1 AS sequence
		)
		INSERT INTO agent_runtime_job_output_chunks (job_id, sequence, stream, data, created_at)
		SELECT $1, sequence, $2, $8, $7 FROM updated
		RETURNING job_id, sequence, stream, data, created_at
	`, jobID, stream, stdoutIncrement, stderrIncrement, len(data), maximumBytes, now.UTC(), data)
	chunk, err := scanAgentExecutionOutput(row)
	if err == nil {
		return chunk, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return agentruntime.OutputChunk{}, err
	}
	job, loadErr := s.Job(ctx, jobID)
	if loadErr != nil {
		return agentruntime.OutputChunk{}, loadErr
	}
	if job.Status != agentruntime.JobStatusRunning {
		return agentruntime.OutputChunk{}, fmt.Errorf("%w: job is %s", agentruntime.ErrInvalidTransition, job.Status)
	}
	return agentruntime.OutputChunk{}, agentruntime.ErrOutputLimit
}

func (s *Store) JobCancellationRequested(ctx context.Context, jobID string) (bool, error) {
	var requested bool
	err := s.db.QueryRowContext(ctx, `SELECT cancel_requested FROM agent_runtime_jobs WHERE id = $1`, jobID).Scan(&requested)
	if errors.Is(err, sql.ErrNoRows) {
		return false, agentruntime.ErrNotFound
	}
	return requested, err
}

func (s *Store) CompleteJob(ctx context.Context, jobID string, ownerID string, result agentruntime.JobResult, now time.Time) (agentruntime.Job, error) {
	status := agentruntime.JobStatusSucceeded
	if result.Canceled {
		status = agentruntime.JobStatusCanceled
	} else if result.Error != "" || result.ExitCode != 0 {
		status = agentruntime.JobStatusFailed
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE agent_runtime_jobs
		SET status = CASE WHEN cancel_requested THEN 'canceled' ELSE $3 END,
		    generation = generation + 1,
		    exit_code = $4,
		    failure_reason = $5,
		    claim_owner = '',
		    claim_expires_at = NULL,
		    updated_at = $6,
		    finished_at = $6
		WHERE id = $1
		  AND claim_owner = $2
		  AND status = 'running'
		  AND claim_expires_at > $6
		RETURNING `+agentExecutionJobColumns,
		jobID, ownerID, status, result.ExitCode, result.Error, now.UTC())
	job, err := scanAgentExecutionJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Job{}, agentruntime.ErrJobClaimLost
	}
	return job, err
}

func (s *Store) RequestJobCancellation(ctx context.Context, jobID string, expectedGeneration int64, now time.Time) (agentruntime.Job, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE agent_runtime_jobs
		SET cancel_requested = true,
		    generation = generation + 1,
		    status = CASE WHEN status = 'queued' THEN 'canceled' ELSE status END,
		    finished_at = CASE WHEN status = 'queued' THEN $3 ELSE finished_at END,
		    updated_at = $3
		WHERE id = $1
		  AND generation = $2
		  AND status IN ('queued', 'running')
		RETURNING `+agentExecutionJobColumns,
		jobID, expectedGeneration, now.UTC())
	job, err := scanAgentExecutionJob(row)
	if err == nil {
		return job, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return agentruntime.Job{}, err
	}
	current, loadErr := s.Job(ctx, jobID)
	if loadErr != nil {
		return agentruntime.Job{}, loadErr
	}
	if current.Status == agentruntime.JobStatusSucceeded || current.Status == agentruntime.JobStatusFailed || current.Status == agentruntime.JobStatusCanceled {
		return current, nil
	}
	return agentruntime.Job{}, agentruntime.ErrGenerationConflict
}

func (s *Store) JobOutput(ctx context.Context, jobID string, afterSequence int64, limit int) ([]agentruntime.OutputChunk, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT job_id, sequence, stream, data, created_at
		FROM agent_runtime_job_output_chunks
		WHERE job_id = $1 AND sequence > $2
		ORDER BY sequence ASC
		LIMIT $3
	`, jobID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chunks := make([]agentruntime.OutputChunk, 0)
	for rows.Next() {
		chunk, err := scanAgentExecutionOutput(rowsScan{rows: rows})
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		if _, err := s.Job(ctx, jobID); err != nil {
			return nil, err
		}
	}
	return chunks, nil
}

func scanAgentExecutionJob(scanner rowScanner) (agentruntime.Job, error) {
	var job agentruntime.Job
	var argsJSON []byte
	var environmentJSON []byte
	var timeoutMilliseconds int64
	var exitCode sql.NullInt64
	var claimExpiresAt sql.NullTime
	var startedAt sql.NullTime
	var finishedAt sql.NullTime
	var ptyColumns int
	var ptyRows int
	err := scanner.Scan(
		&job.ID, &job.BindingID, &job.OrganizationID, &job.SubjectID, &job.Backend, &job.BackendRef,
		&job.WorkspaceRef, &job.IdempotencyKey, &job.RequestHash, &job.Status, &job.Generation,
		&job.Command, &argsJSON, &environmentJSON, &job.WorkDir, &job.PTY, &ptyColumns,
		&ptyRows, &timeoutMilliseconds, &exitCode, &job.FailureReason, &job.CancelRequested,
		&job.ClaimOwner, &claimExpiresAt, &job.Attempts, &job.StdoutBytes, &job.StderrBytes,
		&job.CreatedAt, &job.UpdatedAt, &startedAt, &finishedAt,
	)
	if err != nil {
		return agentruntime.Job{}, err
	}
	if err := json.Unmarshal(argsJSON, &job.Args); err != nil {
		return agentruntime.Job{}, err
	}
	if err := json.Unmarshal(environmentJSON, &job.Environment); err != nil {
		return agentruntime.Job{}, err
	}
	job.PTYColumns = uint16(ptyColumns)
	job.PTYRows = uint16(ptyRows)
	job.Timeout = time.Duration(timeoutMilliseconds) * time.Millisecond
	if exitCode.Valid {
		value := int(exitCode.Int64)
		job.ExitCode = &value
	}
	if claimExpiresAt.Valid {
		job.ClaimExpiresAt = &claimExpiresAt.Time
	}
	if startedAt.Valid {
		job.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		job.FinishedAt = &finishedAt.Time
	}
	return job, nil
}

func scanAgentExecutionOutput(scanner rowScanner) (agentruntime.OutputChunk, error) {
	var chunk agentruntime.OutputChunk
	err := scanner.Scan(&chunk.JobID, &chunk.Sequence, &chunk.Stream, &chunk.Data, &chunk.CreatedAt)
	return chunk, err
}

func marshalJobCommand(job agentruntime.Job) ([]byte, []byte, []byte, error) {
	argsJSON, err := json.Marshal(job.Args)
	if err != nil {
		return nil, nil, nil, err
	}
	environmentJSON, err := json.Marshal(job.Environment)
	if err != nil {
		return nil, nil, nil, err
	}
	commandJSON, err := json.Marshal(map[string]any{
		"command":     job.Command,
		"args":        job.Args,
		"environment": job.Environment,
		"work_dir":    job.WorkDir,
		"pty":         job.PTY,
		"pty_columns": job.PTYColumns,
		"pty_rows":    job.PTYRows,
		"timeout_ms":  durationMilliseconds(job.Timeout),
	})
	return argsJSON, environmentJSON, commandJSON, err
}

func durationMilliseconds(value time.Duration) int64 {
	if value <= 0 {
		return 0
	}
	milliseconds := value.Milliseconds()
	if milliseconds == 0 {
		return 1
	}
	return milliseconds
}

func requireAffected(result sql.Result, fallback error) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fallback
	}
	return nil
}
