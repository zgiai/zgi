package storage

import (
	"context"
	"database/sql"
	"fmt"
)

const migrationLockID int64 = 9120260929

type schemaMigration struct {
	version     int64
	description string
	statements  []string
}

var schemaMigrations = []schemaMigration{
	{
		version:     2026092901,
		description: "agent runtime control plane",
		statements: []string{
			`CREATE TABLE agent_runtime_profiles (
				id TEXT PRIMARY KEY,
				organization_id TEXT NOT NULL DEFAULT '',
				name TEXT NOT NULL,
				backend TEXT NOT NULL,
				status TEXT NOT NULL,
				capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				UNIQUE (organization_id, name)
			);`,
			`CREATE TABLE agent_runtime_home_snapshots (
				id TEXT PRIMARY KEY,
				organization_id TEXT NOT NULL,
				subject_id TEXT NOT NULL,
				profile_id TEXT NOT NULL,
				backend TEXT NOT NULL,
				backend_ref TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL,
				generation BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL
			);`,
			`CREATE TABLE agent_runtime_workspaces (
				id TEXT PRIMARY KEY,
				organization_id TEXT NOT NULL,
				subject_id TEXT NOT NULL,
				backend TEXT NOT NULL,
				backend_ref TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL,
				generation BIGINT NOT NULL DEFAULT 1,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL
			);`,
			`CREATE TABLE agent_runtime_bindings (
				id TEXT PRIMARY KEY,
				organization_id TEXT NOT NULL,
				subject_id TEXT NOT NULL,
				agent_id TEXT NOT NULL,
				conversation_id TEXT NOT NULL,
				profile_id TEXT NOT NULL,
				home_snapshot_id TEXT NOT NULL DEFAULT '',
				workspace_id TEXT NOT NULL,
				backend TEXT NOT NULL,
				backend_ref TEXT NOT NULL DEFAULT '',
				workspace_ref TEXT NOT NULL DEFAULT '',
				idempotency_key TEXT NOT NULL,
				request_hash TEXT NOT NULL,
				status TEXT NOT NULL,
				generation BIGINT NOT NULL DEFAULT 1,
				failure_reason TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				last_activity_at TIMESTAMPTZ NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL,
				UNIQUE (organization_id, subject_id, idempotency_key)
			);`,
			`CREATE INDEX idx_agent_runtime_bindings_owner_status ON agent_runtime_bindings (organization_id, subject_id, status);`,
			`CREATE INDEX idx_agent_runtime_bindings_expiry ON agent_runtime_bindings (status, expires_at);`,
			`CREATE TABLE agent_runtime_jobs (
				id TEXT PRIMARY KEY,
				binding_id TEXT NOT NULL,
				organization_id TEXT NOT NULL,
				subject_id TEXT NOT NULL,
				status TEXT NOT NULL,
				generation BIGINT NOT NULL DEFAULT 1,
				command_json JSONB NOT NULL,
				exit_code INTEGER,
				failure_reason TEXT NOT NULL DEFAULT '',
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				started_at TIMESTAMPTZ,
				finished_at TIMESTAMPTZ
			);`,
			`CREATE INDEX idx_agent_runtime_jobs_binding_created ON agent_runtime_jobs (binding_id, created_at DESC);`,
			`CREATE TABLE agent_runtime_admission_leases (
				id TEXT PRIMARY KEY,
				binding_id TEXT NOT NULL UNIQUE,
				owner_id TEXT NOT NULL,
				generation BIGINT NOT NULL DEFAULT 1,
				expires_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL
			);`,
			`CREATE INDEX idx_agent_runtime_leases_expiry ON agent_runtime_admission_leases (expires_at);`,
			`CREATE TABLE agent_runtime_cleanup_tasks (
				id TEXT PRIMARY KEY,
				resource_type TEXT NOT NULL,
				resource_id TEXT NOT NULL,
				backend TEXT NOT NULL,
				backend_ref TEXT NOT NULL DEFAULT '',
				action TEXT NOT NULL,
				status TEXT NOT NULL,
				attempts INTEGER NOT NULL DEFAULT 0,
				last_error TEXT NOT NULL DEFAULT '',
				not_before TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				UNIQUE (resource_type, resource_id, action)
			);`,
			`CREATE INDEX idx_agent_runtime_cleanup_pending ON agent_runtime_cleanup_tasks (status, not_before);`,
		},
	},
	{
		version:     2026092902,
		description: "agent runtime cleanup claim leases",
		statements: []string{
			`ALTER TABLE agent_runtime_cleanup_tasks ADD COLUMN lease_owner TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_cleanup_tasks ADD COLUMN lease_expires_at TIMESTAMPTZ;`,
			`CREATE INDEX idx_agent_runtime_cleanup_claims ON agent_runtime_cleanup_tasks (status, lease_expires_at, not_before);`,
		},
	},
	{
		version:     2026092903,
		description: "agent execution asynchronous jobs",
		statements: []string{
			`ALTER TABLE agent_runtime_jobs ADD COLUMN backend TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN backend_ref TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN workspace_ref TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN idempotency_key TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN request_hash TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN command TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN args_json JSONB NOT NULL DEFAULT '[]'::jsonb;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN environment_json JSONB NOT NULL DEFAULT '{}'::jsonb;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN work_dir TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN pty BOOLEAN NOT NULL DEFAULT false;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN pty_columns INTEGER NOT NULL DEFAULT 0;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN pty_rows INTEGER NOT NULL DEFAULT 0;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN timeout_ms BIGINT NOT NULL DEFAULT 0;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN cancel_requested BOOLEAN NOT NULL DEFAULT false;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN claim_owner TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN claim_expires_at TIMESTAMPTZ;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN stdout_bytes BIGINT NOT NULL DEFAULT 0;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN stderr_bytes BIGINT NOT NULL DEFAULT 0;`,
			`ALTER TABLE agent_runtime_jobs ADD COLUMN next_output_sequence BIGINT NOT NULL DEFAULT 0;`,
			`CREATE UNIQUE INDEX idx_agent_runtime_jobs_idempotency ON agent_runtime_jobs (binding_id, idempotency_key) WHERE idempotency_key <> '';`,
			`CREATE INDEX idx_agent_runtime_jobs_claim ON agent_runtime_jobs (status, claim_expires_at, created_at);`,
			`CREATE TABLE agent_runtime_job_output_chunks (
				job_id TEXT NOT NULL,
				sequence BIGINT NOT NULL,
				stream TEXT NOT NULL,
				data BYTEA NOT NULL,
				created_at TIMESTAMPTZ NOT NULL,
				PRIMARY KEY (job_id, sequence)
			);`,
		},
	},
	{
		version:     2026092904,
		description: "agent runtime cleanup workspace handles",
		statements: []string{
			`ALTER TABLE agent_runtime_cleanup_tasks ADD COLUMN workspace_ref TEXT NOT NULL DEFAULT '';`,
		},
	},
	{
		version:     2026092905,
		description: "agent runtime workspace checkpoint reference",
		statements: []string{
			`ALTER TABLE agent_runtime_bindings ADD COLUMN checkpoint_key TEXT NOT NULL DEFAULT '';`,
			`ALTER TABLE agent_runtime_bindings ADD COLUMN pending_checkpoint_key TEXT NOT NULL DEFAULT '';`,
		},
	},
	{
		version:     2026093001,
		description: "agent runtime seven-day workspace retention deadline",
		statements: []string{
			`ALTER TABLE agent_runtime_bindings ADD COLUMN retention_expires_at TIMESTAMPTZ;`,
			`UPDATE agent_runtime_bindings SET retention_expires_at = last_activity_at + INTERVAL '7 days';`,
			`ALTER TABLE agent_runtime_bindings ALTER COLUMN retention_expires_at SET NOT NULL;`,
			`CREATE INDEX idx_agent_runtime_bindings_retention ON agent_runtime_bindings (retention_expires_at, id) WHERE status IN ('suspended', 'deleting');`,
		},
	},
	{
		version:     2026093002,
		description: "agent runtime allocation attempt cleanup ownership",
		statements: []string{
			`ALTER TABLE agent_runtime_cleanup_tasks ADD COLUMN binding_id TEXT NOT NULL DEFAULT '';`,
		},
	},
	{
		version:     2026093003,
		description: "agent runtime immutable home template reference",
		statements: []string{
			`ALTER TABLE agent_runtime_bindings ADD COLUMN home_snapshot_ref TEXT NOT NULL DEFAULT '';`,
			`CREATE FUNCTION prevent_approved_home_snapshot_mutation() RETURNS trigger AS $$
			BEGIN
				IF OLD.status = 'approved' AND (
					NEW.organization_id IS DISTINCT FROM OLD.organization_id OR
					NEW.subject_id IS DISTINCT FROM OLD.subject_id OR
					NEW.profile_id IS DISTINCT FROM OLD.profile_id OR
					NEW.backend IS DISTINCT FROM OLD.backend OR
					NEW.backend_ref IS DISTINCT FROM OLD.backend_ref
				) THEN
					RAISE EXCEPTION 'approved home snapshot identity and artifact are immutable';
				END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql;`,
			`CREATE TRIGGER trg_approved_home_snapshot_immutable
			BEFORE UPDATE ON agent_runtime_home_snapshots
			FOR EACH ROW EXECUTE FUNCTION prevent_approved_home_snapshot_mutation();`,
		},
	},
	{
		version:     2026093004,
		description: "agent runtime allocation attempt generation fence",
		statements: []string{
			`ALTER TABLE agent_runtime_cleanup_tasks ADD COLUMN binding_generation BIGINT NOT NULL DEFAULT 0;`,
		},
	},
	{
		version:     2026093005,
		description: "retain home templates referenced by live bindings",
		statements: []string{
			`CREATE INDEX idx_agent_runtime_bindings_home_snapshot_live
			ON agent_runtime_bindings (home_snapshot_id)
			WHERE home_snapshot_id <> '' AND status NOT IN ('deleted', 'failed');`,
			`CREATE FUNCTION validate_home_snapshot_binding_reference() RETURNS trigger AS $$
			BEGIN
				IF NEW.home_snapshot_id <> '' THEN
					PERFORM 1 FROM agent_runtime_home_snapshots
					WHERE id = NEW.home_snapshot_id
					  AND organization_id = NEW.organization_id
					  AND subject_id = ''
					  AND profile_id = NEW.profile_id
					  AND backend = NEW.backend
					  AND status = 'approved'
					FOR KEY SHARE;
					IF NOT FOUND THEN
						RAISE EXCEPTION 'binding references an unavailable home snapshot';
					END IF;
				END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql;`,
			`CREATE TRIGGER trg_validate_home_snapshot_binding_reference
			BEFORE INSERT OR UPDATE OF home_snapshot_id ON agent_runtime_bindings
			FOR EACH ROW EXECUTE FUNCTION validate_home_snapshot_binding_reference();`,
			`CREATE FUNCTION prevent_referenced_home_snapshot_delete() RETURNS trigger AS $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM agent_runtime_bindings
					WHERE home_snapshot_id = OLD.id AND status NOT IN ('deleted', 'failed')
				) THEN
					RAISE EXCEPTION 'home snapshot is still referenced by a live binding';
				END IF;
				RETURN OLD;
			END;
			$$ LANGUAGE plpgsql;`,
			`CREATE TRIGGER trg_referenced_home_snapshot_delete
			BEFORE DELETE ON agent_runtime_home_snapshots
			FOR EACH ROW EXECUTE FUNCTION prevent_referenced_home_snapshot_delete();`,
		},
	},
}

func (s *Store) applyMigrations(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin schema migrations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sandbox_schema_migrations (
		version BIGINT PRIMARY KEY,
		description TEXT NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`); err != nil {
		return fmt.Errorf("prepare schema migration ledger: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("lock schema migrations: %w", err)
	}
	for _, migration := range schemaMigrations {
		var applied bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sandbox_schema_migrations WHERE version = $1)`, migration.version).Scan(&applied); err != nil {
			return fmt.Errorf("check schema migration %d: %w", migration.version, err)
		}
		if applied {
			continue
		}
		for _, statement := range migration.statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply schema migration %d: %w", migration.version, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO sandbox_schema_migrations (version, description) VALUES ($1, $2)`, migration.version, migration.description); err != nil {
			return fmt.Errorf("record schema migration %d: %w", migration.version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migrations: %w", err)
	}
	return nil
}
