package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/zgiai/zgi-sandbox/internal/config"
	"github.com/zgiai/zgi-sandbox/internal/observer"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

type Store struct {
	db                    *sql.DB
	observerRetentionDays int
	observerMaxEvents     int
}

func Open(cfg config.Config) (*Store, error) {
	if cfg.DatabaseURL == "" {
		return nil, errors.New("database url is required")
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres store: %w", err)
	}

	db.SetMaxOpenConns(max(8, cfg.MaxWorkers*4))
	db.SetMaxIdleConns(max(4, cfg.MaxWorkers))
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres store: %w", err)
	}

	store := &Store{
		db:                    db,
		observerRetentionDays: cfg.ObserverRetentionDays,
		observerMaxEvents:     cfg.ObserverMaxEvents,
	}
	if err := store.prepare(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) prepare(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS sandboxes (
			id TEXT PRIMARY KEY,
			runtime_profile TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL,
			root_path TEXT NOT NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			tenant_id TEXT NOT NULL DEFAULT '',
			workspace_id TEXT NOT NULL DEFAULT '',
			app_id TEXT NOT NULL DEFAULT '',
			workflow_run_id TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			network_enabled BOOLEAN NOT NULL,
			network_policy TEXT NOT NULL,
			dependency_profile TEXT NOT NULL,
			workspace_binding TEXT NOT NULL DEFAULT '',
			ttl_seconds INTEGER NOT NULL,
			worker_id TEXT NOT NULL,
			worker_addr TEXT NOT NULL
		);`,
		`ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS workspace_id TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS app_id TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS workflow_run_id TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS user_id TEXT NOT NULL DEFAULT '';`,
		`CREATE INDEX IF NOT EXISTS idx_sandboxes_status_expires ON sandboxes(status, expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_sandboxes_ownership ON sandboxes(tenant_id, workspace_id, workflow_run_id);`,
		`CREATE TABLE IF NOT EXISTS sandbox_endpoints (
			sandbox_id TEXT NOT NULL,
			port TEXT NOT NULL,
			url TEXT NOT NULL,
			status TEXT NOT NULL,
			target_host TEXT NOT NULL,
			target_port INTEGER NOT NULL,
			scheme TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (sandbox_id, port)
		);`,
		`CREATE TABLE IF NOT EXISTS observer_events (
			id TEXT PRIMARY KEY,
			sandbox_id TEXT,
			type TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb
		);`,
		`CREATE INDEX IF NOT EXISTS idx_observer_events_scope ON observer_events(sandbox_id, type, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_observer_events_created_at ON observer_events(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_observer_events_ownership ON observer_events ((metadata_json->>'tenant_id'), (metadata_json->>'workspace_id'), (metadata_json->>'workflow_run_id'), created_at DESC);`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("prepare postgres schema: %w", err)
		}
	}
	return nil
}

func (s *Store) SaveSandbox(box sandbox.Sandbox) error {
	metadata, err := json.Marshal(defaultMap(box.Metadata))
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO sandboxes (
			id, runtime_profile, status, created_at, updated_at, expires_at, root_path,
			metadata_json, tenant_id, workspace_id, app_id, workflow_run_id, user_id,
			network_enabled, network_policy, dependency_profile, workspace_binding,
			ttl_seconds, worker_id, worker_addr
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8::jsonb, $9, $10, $11, $12, $13,
			$14, $15, $16, $17,
			$18, $19, $20
		)
		ON CONFLICT(id) DO UPDATE SET
			runtime_profile = EXCLUDED.runtime_profile,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at,
			expires_at = EXCLUDED.expires_at,
			root_path = EXCLUDED.root_path,
			metadata_json = EXCLUDED.metadata_json,
			tenant_id = EXCLUDED.tenant_id,
			workspace_id = EXCLUDED.workspace_id,
			app_id = EXCLUDED.app_id,
			workflow_run_id = EXCLUDED.workflow_run_id,
			user_id = EXCLUDED.user_id,
			network_enabled = EXCLUDED.network_enabled,
			network_policy = EXCLUDED.network_policy,
			dependency_profile = EXCLUDED.dependency_profile,
			workspace_binding = EXCLUDED.workspace_binding,
			ttl_seconds = EXCLUDED.ttl_seconds,
			worker_id = EXCLUDED.worker_id,
			worker_addr = EXCLUDED.worker_addr
	`,
		box.ID,
		string(box.RuntimeProfile),
		string(box.Status),
		box.CreatedAt.UTC(),
		box.UpdatedAt.UTC(),
		box.ExpiresAt.UTC(),
		box.RootPath,
		string(metadata),
		box.TenantID,
		box.WorkspaceID,
		box.AppID,
		box.WorkflowRunID,
		box.UserID,
		box.NetworkEnabled,
		box.NetworkPolicy,
		box.DependencyProfile,
		box.WorkspaceBinding,
		box.TTLSeconds,
		box.WorkerID,
		box.WorkerAddr,
	)
	return err
}

func (s *Store) GetSandbox(id string) (*sandbox.Sandbox, error) {
	row := s.db.QueryRow(`
		SELECT id, runtime_profile, status, created_at, updated_at, expires_at, root_path,
		       metadata_json, tenant_id, workspace_id, app_id, workflow_run_id, user_id,
		       network_enabled, network_policy, dependency_profile, workspace_binding,
		       ttl_seconds, worker_id, worker_addr
		FROM sandboxes
		WHERE id = $1
	`, id)

	box, err := scanSandbox(rowScan{row: row})
	if err != nil {
		return nil, err
	}
	return box, nil
}

func (s *Store) ListSandboxes() ([]sandbox.Sandbox, error) {
	rows, err := s.db.Query(`
		SELECT id, runtime_profile, status, created_at, updated_at, expires_at, root_path,
		       metadata_json, tenant_id, workspace_id, app_id, workflow_run_id, user_id,
		       network_enabled, network_policy, dependency_profile, workspace_binding,
		       ttl_seconds, worker_id, worker_addr
		FROM sandboxes
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]sandbox.Sandbox, 0)
	for rows.Next() {
		box, err := scanSandbox(rowsScan{rows: rows})
		if err != nil {
			return nil, err
		}
		items = append(items, *box)
	}
	return items, rows.Err()
}

func (s *Store) CountActive(workerID string, now time.Time) (int, error) {
	row := s.db.QueryRow(`
		SELECT COUNT(1)
		FROM sandboxes
		WHERE status = $1 AND expires_at > $2 AND worker_id = $3
	`, string(sandbox.StatusActive), now.UTC(), workerID)

	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountActiveByTenant(tenantID string, now time.Time) (int, error) {
	row := s.db.QueryRow(`
		SELECT COUNT(1)
		FROM sandboxes
		WHERE status = $1 AND expires_at > $2 AND tenant_id = $3
	`, string(sandbox.StatusActive), now.UTC(), tenantID)

	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) SaveEndpoint(endpoint sandbox.Endpoint) error {
	now := endpoint.UpdatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	createdAt := endpoint.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	_, err := s.db.Exec(`
		INSERT INTO sandbox_endpoints (
			sandbox_id, port, url, status, target_host, target_port, scheme, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(sandbox_id, port) DO UPDATE SET
			url = EXCLUDED.url,
			status = EXCLUDED.status,
			target_host = EXCLUDED.target_host,
			target_port = EXCLUDED.target_port,
			scheme = EXCLUDED.scheme,
			updated_at = EXCLUDED.updated_at
	`,
		endpoint.SandboxID,
		endpoint.Port,
		endpoint.URL,
		endpoint.Status,
		defaultString(endpoint.TargetHost, "127.0.0.1"),
		endpoint.TargetPort,
		defaultString(endpoint.Scheme, "http"),
		createdAt.UTC(),
		now.UTC(),
	)
	return err
}

func (s *Store) GetEndpoint(sandboxID string, port string) (*sandbox.Endpoint, error) {
	row := s.db.QueryRow(`
		SELECT sandbox_id, port, url, status, target_host, target_port, scheme, created_at, updated_at
		FROM sandbox_endpoints
		WHERE sandbox_id = $1 AND port = $2
	`, sandboxID, port)

	var endpoint sandbox.Endpoint
	if err := row.Scan(
		&endpoint.SandboxID,
		&endpoint.Port,
		&endpoint.URL,
		&endpoint.Status,
		&endpoint.TargetHost,
		&endpoint.TargetPort,
		&endpoint.Scheme,
		&endpoint.CreatedAt,
		&endpoint.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("sandbox endpoint not found")
		}
		return nil, err
	}

	return &endpoint, nil
}

func (s *Store) AppendEvent(event observer.Event) error {
	metadata, err := json.Marshal(defaultAnyMap(event.Metadata))
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO observer_events (id, sandbox_id, type, message, created_at, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb)
	`, event.ID, event.SandboxID, event.Type, event.Message, event.CreatedAt.UTC(), string(metadata))
	if err != nil {
		return err
	}
	return s.PruneObserverEvents()
}

func (s *Store) PruneObserverEvents() error {
	if s.observerRetentionDays > 0 {
		cutoff := time.Now().UTC().Add(-time.Duration(s.observerRetentionDays) * 24 * time.Hour)
		if _, err := s.db.Exec(`DELETE FROM observer_events WHERE created_at < $1`, cutoff); err != nil {
			return err
		}
	}

	if s.observerMaxEvents > 0 {
		_, err := s.db.Exec(`
			DELETE FROM observer_events
			WHERE id IN (
				SELECT id
				FROM observer_events
				ORDER BY created_at DESC, id DESC
				OFFSET $1
			)
		`, s.observerMaxEvents)
		return err
	}
	return nil
}

func (s *Store) QueryEvents(query observer.Query) ([]observer.Event, error) {
	statement := `
		SELECT id, sandbox_id, type, message, created_at, metadata_json
		FROM observer_events
		WHERE ($1 = '' OR sandbox_id = $2)
		  AND ($3 = '' OR type = $4)
		  AND ($5::timestamptz IS NULL OR created_at < $5)
		  AND ($6 = '' OR metadata_json->>'tenant_id' = $7)
		  AND ($8 = '' OR metadata_json->>'workspace_id' = $9)
		  AND ($10 = '' OR metadata_json->>'app_id' = $11)
		  AND ($12 = '' OR metadata_json->>'workflow_run_id' = $13)
		  AND ($14 = '' OR metadata_json->>'user_id' = $15)
		ORDER BY created_at DESC
	`
	var before any
	if !query.Before.IsZero() {
		before = query.Before.UTC()
	}
	args := []any{
		query.SandboxID, query.SandboxID,
		query.Type, query.Type,
		before,
		query.TenantID, query.TenantID,
		query.WorkspaceID, query.WorkspaceID,
		query.AppID, query.AppID,
		query.WorkflowRunID, query.WorkflowRunID,
		query.UserID, query.UserID,
	}
	if query.Limit > 0 {
		statement += ` LIMIT $16`
		args = append(args, query.Limit)
	}

	rows, err := s.db.Query(statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]observer.Event, 0)
	for rows.Next() {
		var event observer.Event
		var metadataJSON []byte
		if err := rows.Scan(&event.ID, &event.SandboxID, &event.Type, &event.Message, &event.CreatedAt, &metadataJSON); err != nil {
			return nil, err
		}

		metadata := map[string]any{}
		if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
			return nil, err
		}
		event.Metadata = metadata
		events = append(events, event)
	}

	return events, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

type rowScan struct {
	row *sql.Row
}

func (r rowScan) Scan(dest ...any) error {
	return r.row.Scan(dest...)
}

type rowsScan struct {
	rows *sql.Rows
}

func (r rowsScan) Scan(dest ...any) error {
	return r.rows.Scan(dest...)
}

func scanSandbox(scanner rowScanner) (*sandbox.Sandbox, error) {
	var box sandbox.Sandbox
	var metadataJSON []byte
	if err := scanner.Scan(
		&box.ID,
		&box.RuntimeProfile,
		&box.Status,
		&box.CreatedAt,
		&box.UpdatedAt,
		&box.ExpiresAt,
		&box.RootPath,
		&metadataJSON,
		&box.TenantID,
		&box.WorkspaceID,
		&box.AppID,
		&box.WorkflowRunID,
		&box.UserID,
		&box.NetworkEnabled,
		&box.NetworkPolicy,
		&box.DependencyProfile,
		&box.WorkspaceBinding,
		&box.TTLSeconds,
		&box.WorkerID,
		&box.WorkerAddr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("sandbox not found")
		}
		return nil, err
	}

	metadata := map[string]string{}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, err
	}
	if len(metadata) > 0 {
		box.Metadata = metadata
	}
	return &box, nil
}

func defaultMap(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	return value
}

func defaultAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func max(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
