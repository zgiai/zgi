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
	"github.com/zgiai/zgi-sandbox/internal/policy"
	"github.com/zgiai/zgi-sandbox/internal/sandbox"
)

type Store struct {
	db                    *sql.DB
	observerRetentionDays int
	observerMaxEvents     int
}

type DependencyBuildRequestRecord struct {
	BuildID               string          `json:"build_id"`
	Fingerprint           string          `json:"fingerprint"`
	Status                string          `json:"status"`
	OrganizationID        string          `json:"organization_id,omitempty"`
	ProfileName           string          `json:"profile_name"`
	DependencyRequestJSON json.RawMessage `json:"dependency_request_json"`
	PackagesJSON          json.RawMessage `json:"packages_json"`
	SourcesJSON           json.RawMessage `json:"sources_json"`
	WarningsJSON          json.RawMessage `json:"warnings_json"`
	PackageCount          int             `json:"package_count"`
	ArtifactChecksum      string          `json:"artifact_checksum,omitempty"`
	SizeBytes             int64           `json:"size_bytes,omitempty"`
	Error                 string          `json:"error,omitempty"`
	CreatedAt             time.Time       `json:"created_at"`
	UpdatedAt             time.Time       `json:"updated_at"`
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
			organization_id TEXT NOT NULL DEFAULT '',
			workspace_id TEXT NOT NULL DEFAULT '',
			app_id TEXT NOT NULL DEFAULT '',
			workflow_run_id TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			network_enabled BOOLEAN NOT NULL,
			network_policy TEXT NOT NULL,
			dependency_profile TEXT NOT NULL,
			dependency_artifact_checksum TEXT NOT NULL DEFAULT '',
			workspace_binding TEXT NOT NULL DEFAULT '',
			ttl_seconds INTEGER NOT NULL,
			worker_id TEXT NOT NULL,
			worker_addr TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sandboxes_status_expires ON sandboxes(status, expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_sandboxes_ownership ON sandboxes(organization_id, workspace_id, workflow_run_id);`,
		`ALTER TABLE sandboxes ADD COLUMN IF NOT EXISTS dependency_artifact_checksum TEXT NOT NULL DEFAULT '';`,
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
		`CREATE INDEX IF NOT EXISTS idx_observer_events_ownership ON observer_events ((metadata_json->>'organization_id'), (metadata_json->>'workspace_id'), (metadata_json->>'workflow_run_id'), created_at DESC);`,
		`CREATE TABLE IF NOT EXISTS dependency_profiles (
			name TEXT PRIMARY KEY,
			version TEXT NOT NULL,
			status TEXT NOT NULL,
			enabled BOOLEAN NOT NULL,
			owner_scope TEXT NOT NULL,
			languages_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			packages_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			base_runtime TEXT NOT NULL,
			checksum TEXT NOT NULL,
			size_bytes BIGINT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dependency_profiles_status_enabled ON dependency_profiles(status, enabled);`,
		`CREATE TABLE IF NOT EXISTS runtime_artifacts (
			checksum TEXT PRIMARY KEY,
			size_bytes BIGINT NOT NULL,
			storage_path TEXT NOT NULL DEFAULT '',
			languages_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			packages_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			base_runtime TEXT NOT NULL,
			public_reusable BOOLEAN NOT NULL DEFAULT false,
			security_status TEXT NOT NULL DEFAULT 'verified',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			last_used_at TIMESTAMPTZ
		);`,
		`CREATE TABLE IF NOT EXISTS dependency_profile_records (
			scope TEXT NOT NULL,
			organization_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			status TEXT NOT NULL,
			enabled BOOLEAN NOT NULL,
			owner_scope TEXT NOT NULL,
			languages_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			packages_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			base_runtime TEXT NOT NULL,
			checksum TEXT NOT NULL,
			artifact_checksum TEXT NOT NULL,
			size_bytes BIGINT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			public_reusable BOOLEAN NOT NULL DEFAULT false,
			pinned BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL,
			last_used_at TIMESTAMPTZ,
			PRIMARY KEY (scope, organization_id, name)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_dependency_profile_records_status_enabled ON dependency_profile_records(status, enabled);`,
		`CREATE INDEX IF NOT EXISTS idx_dependency_profile_records_artifact ON dependency_profile_records(artifact_checksum);`,
		`CREATE TABLE IF NOT EXISTS dependency_build_requests (
			fingerprint TEXT PRIMARY KEY,
			build_id TEXT NOT NULL,
			status TEXT NOT NULL,
			organization_id TEXT NOT NULL DEFAULT '',
			profile_name TEXT NOT NULL,
			dependency_request_json JSONB NOT NULL,
			packages_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			sources_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			warnings_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			package_count INTEGER NOT NULL DEFAULT 0,
			artifact_checksum TEXT NOT NULL DEFAULT '',
			size_bytes BIGINT NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		);`,
		`ALTER TABLE dependency_build_requests ADD COLUMN IF NOT EXISTS artifact_checksum TEXT NOT NULL DEFAULT '';`,
		`ALTER TABLE dependency_build_requests ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;`,
		`CREATE INDEX IF NOT EXISTS idx_dependency_build_requests_status ON dependency_build_requests(status, updated_at DESC);`,
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
			metadata_json, organization_id, workspace_id, app_id, workflow_run_id, user_id,
			network_enabled, network_policy, dependency_profile, dependency_artifact_checksum,
			workspace_binding, ttl_seconds, worker_id, worker_addr
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8::jsonb, $9, $10, $11, $12, $13,
			$14, $15, $16, $17,
			$18, $19, $20, $21
		)
		ON CONFLICT(id) DO UPDATE SET
			runtime_profile = EXCLUDED.runtime_profile,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at,
			expires_at = EXCLUDED.expires_at,
			root_path = EXCLUDED.root_path,
			metadata_json = EXCLUDED.metadata_json,
			organization_id = EXCLUDED.organization_id,
			workspace_id = EXCLUDED.workspace_id,
			app_id = EXCLUDED.app_id,
			workflow_run_id = EXCLUDED.workflow_run_id,
			user_id = EXCLUDED.user_id,
			network_enabled = EXCLUDED.network_enabled,
			network_policy = EXCLUDED.network_policy,
			dependency_profile = EXCLUDED.dependency_profile,
			dependency_artifact_checksum = EXCLUDED.dependency_artifact_checksum,
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
		box.OrganizationID,
		box.WorkspaceID,
		box.AppID,
		box.WorkflowRunID,
		box.UserID,
		box.NetworkEnabled,
		box.NetworkPolicy,
		box.DependencyProfile,
		box.DependencyArtifactChecksum,
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
		       metadata_json, organization_id, workspace_id, app_id, workflow_run_id, user_id,
		       network_enabled, network_policy, dependency_profile, dependency_artifact_checksum, workspace_binding,
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
		       metadata_json, organization_id, workspace_id, app_id, workflow_run_id, user_id,
		       network_enabled, network_policy, dependency_profile, dependency_artifact_checksum, workspace_binding,
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

func (s *Store) CountActiveByOrganization(organizationID string, now time.Time) (int, error) {
	row := s.db.QueryRow(`
		SELECT COUNT(1)
		FROM sandboxes
		WHERE status = $1 AND expires_at > $2 AND organization_id = $3
	`, string(sandbox.StatusActive), now.UTC(), organizationID)

	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ListActiveDependencyProfilesByOrganization(organizationID string, now time.Time) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT dependency_profile
		FROM sandboxes
		WHERE status = $1
		  AND expires_at > $2
		  AND organization_id = $3
		  AND dependency_profile <> ''
		ORDER BY dependency_profile
	`, string(sandbox.StatusActive), now.UTC(), organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var profile string
		if err := rows.Scan(&profile); err != nil {
			return nil, err
		}
		items = append(items, profile)
	}
	return items, rows.Err()
}

func (s *Store) UpsertDependencyBuildRequest(record DependencyBuildRequestRecord) (*DependencyBuildRequestRecord, error) {
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	row := s.db.QueryRow(`
		INSERT INTO dependency_build_requests (
			fingerprint, build_id, status, organization_id, profile_name,
			dependency_request_json, packages_json, sources_json, warnings_json,
			package_count, artifact_checksum, size_bytes, error, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6::jsonb, $7::jsonb, $8::jsonb, $9::jsonb,
			$10, $11, $12, $13, $14, $15
		)
		ON CONFLICT(fingerprint) DO UPDATE SET
			updated_at = EXCLUDED.updated_at
		RETURNING
			build_id, fingerprint, status, organization_id, profile_name,
			dependency_request_json, packages_json, sources_json, warnings_json,
			package_count, artifact_checksum, size_bytes, error, created_at, updated_at
	`,
		record.Fingerprint,
		record.BuildID,
		record.Status,
		record.OrganizationID,
		record.ProfileName,
		string(defaultRawJSON(record.DependencyRequestJSON, `{}`)),
		string(defaultRawJSON(record.PackagesJSON, `[]`)),
		string(defaultRawJSON(record.SourcesJSON, `[]`)),
		string(defaultRawJSON(record.WarningsJSON, `[]`)),
		record.PackageCount,
		record.ArtifactChecksum,
		record.SizeBytes,
		record.Error,
		record.CreatedAt,
		record.UpdatedAt,
	)
	return scanDependencyBuildRequest(rowScan{row: row})
}

func (s *Store) GetDependencyBuildRequest(fingerprint string) (*DependencyBuildRequestRecord, error) {
	row := s.db.QueryRow(`
		SELECT build_id, fingerprint, status, organization_id, profile_name,
		       dependency_request_json, packages_json, sources_json, warnings_json,
		       package_count, artifact_checksum, size_bytes, error, created_at, updated_at
		FROM dependency_build_requests
		WHERE fingerprint = $1
	`, fingerprint)
	record, err := scanDependencyBuildRequest(rowScan{row: row})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("dependency build request not found")
		}
		return nil, err
	}
	return record, nil
}

func (s *Store) UpdateDependencyBuildRequestStatus(fingerprint string, status string, artifactChecksum string, sizeBytes int64, message string) (*DependencyBuildRequestRecord, error) {
	row := s.db.QueryRow(`
		UPDATE dependency_build_requests
		SET status = $2,
		    artifact_checksum = $3,
		    size_bytes = $4,
		    error = $5,
		    updated_at = $6
		WHERE fingerprint = $1
		RETURNING
			build_id, fingerprint, status, organization_id, profile_name,
			dependency_request_json, packages_json, sources_json, warnings_json,
			package_count, artifact_checksum, size_bytes, error, created_at, updated_at
	`, fingerprint, status, artifactChecksum, sizeBytes, message, time.Now().UTC())
	record, err := scanDependencyBuildRequest(rowScan{row: row})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("dependency build request not found")
		}
		return nil, err
	}
	return record, nil
}

func (s *Store) SaveDependencyProfile(profile policy.DependencyProfile) error {
	languages, err := json.Marshal(defaultStringSlice(profile.Languages))
	if err != nil {
		return err
	}
	packages, err := json.Marshal(defaultDependencyPackages(profile.Packages))
	if err != nil {
		return err
	}
	scope := defaultString(profile.Scope, "global")
	organizationID := profile.OrganizationID
	if scope == "global" {
		organizationID = ""
	}
	artifactChecksum := defaultString(profile.ArtifactChecksum, profile.Checksum)

	now := time.Now().UTC()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec(`
		INSERT INTO runtime_artifacts (
			checksum, size_bytes, languages_json, packages_json, base_runtime,
			public_reusable, security_status, created_at, updated_at
		) VALUES (
			$1, $2, $3::jsonb, $4::jsonb, $5,
			$6, 'verified', $7, $8
		)
		ON CONFLICT(checksum) DO UPDATE SET
			size_bytes = EXCLUDED.size_bytes,
			languages_json = EXCLUDED.languages_json,
			packages_json = EXCLUDED.packages_json,
			base_runtime = EXCLUDED.base_runtime,
			public_reusable = runtime_artifacts.public_reusable OR EXCLUDED.public_reusable,
			updated_at = EXCLUDED.updated_at
	`, artifactChecksum, profile.SizeBytes, string(languages), string(packages), profile.BaseRuntime, profile.PublicReusable || scope == "global", now, now); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO dependency_profile_records (
			scope, organization_id, name, version, status, enabled, owner_scope,
			languages_json, packages_json, base_runtime, checksum, artifact_checksum,
			size_bytes, description, public_reusable, pinned, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8::jsonb, $9::jsonb, $10, $11, $12,
			$13, $14, $15, $16, $17, $18
		)
		ON CONFLICT(scope, organization_id, name) DO UPDATE SET
			version = EXCLUDED.version,
			status = EXCLUDED.status,
			enabled = EXCLUDED.enabled,
			owner_scope = EXCLUDED.owner_scope,
			languages_json = EXCLUDED.languages_json,
			packages_json = EXCLUDED.packages_json,
			base_runtime = EXCLUDED.base_runtime,
			checksum = EXCLUDED.checksum,
			artifact_checksum = EXCLUDED.artifact_checksum,
			size_bytes = EXCLUDED.size_bytes,
			description = EXCLUDED.description,
			public_reusable = EXCLUDED.public_reusable,
			pinned = EXCLUDED.pinned,
			updated_at = EXCLUDED.updated_at
	`, scope, organizationID, profile.Name, profile.Version, profile.Status, profile.Enabled, defaultString(profile.OwnerScope, scope), string(languages), string(packages), profile.BaseRuntime, profile.Checksum, artifactChecksum, profile.SizeBytes, profile.Description, profile.PublicReusable || scope == "global", profile.Pinned, now, now); err != nil {
		return err
	}

	if scope != "global" {
		return tx.Commit()
	}
	_, err = tx.Exec(`
		INSERT INTO dependency_profiles (
			name, version, status, enabled, owner_scope, languages_json, packages_json,
			base_runtime, checksum, size_bytes, description, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6::jsonb, $7::jsonb,
			$8, $9, $10, $11, $12, $13
		)
		ON CONFLICT(name) DO UPDATE SET
			version = EXCLUDED.version,
			status = EXCLUDED.status,
			enabled = EXCLUDED.enabled,
			owner_scope = EXCLUDED.owner_scope,
			languages_json = EXCLUDED.languages_json,
			packages_json = EXCLUDED.packages_json,
			base_runtime = EXCLUDED.base_runtime,
			checksum = EXCLUDED.checksum,
			size_bytes = EXCLUDED.size_bytes,
			description = EXCLUDED.description,
			updated_at = EXCLUDED.updated_at
	`,
		profile.Name,
		profile.Version,
		profile.Status,
		profile.Enabled,
		profile.OwnerScope,
		string(languages),
		string(packages),
		profile.BaseRuntime,
		profile.Checksum,
		profile.SizeBytes,
		profile.Description,
		now,
		now,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListDependencyProfiles() ([]policy.DependencyProfile, error) {
	rows, err := s.db.Query(`
		SELECT name, version, status, enabled, owner_scope, scope, organization_id,
		       languages_json, packages_json, base_runtime, checksum, artifact_checksum,
		       size_bytes, description, public_reusable, pinned
		FROM dependency_profile_records
		ORDER BY created_at ASC, scope ASC, organization_id ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]policy.DependencyProfile, 0)
	for rows.Next() {
		profile, err := scanDependencyProfile(rowsScan{rows: rows})
		if err != nil {
			return nil, err
		}
		items = append(items, *profile)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	legacy, err := s.listLegacyDependencyProfiles()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[dependencyProfileStorageKey(item)] = true
	}
	for _, item := range legacy {
		if seen[dependencyProfileStorageKey(item)] {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) listLegacyDependencyProfiles() ([]policy.DependencyProfile, error) {
	rows, err := s.db.Query(`
		SELECT name, version, status, enabled, owner_scope, languages_json, packages_json,
		       base_runtime, checksum, size_bytes, description
		FROM dependency_profiles
		ORDER BY created_at ASC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]policy.DependencyProfile, 0)
	for rows.Next() {
		profile, err := scanLegacyDependencyProfile(rowsScan{rows: rows})
		if err != nil {
			return nil, err
		}
		items = append(items, *profile)
	}
	return items, rows.Err()
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
		  AND ($5 = '' OR type LIKE $6)
		  AND ($7::timestamptz IS NULL OR created_at > $7)
		  AND ($8::timestamptz IS NULL OR created_at < $8)
		  AND ($9 = '' OR metadata_json->>'organization_id' = $10)
		  AND ($11 = '' OR metadata_json->>'workspace_id' = $12)
		  AND ($13 = '' OR metadata_json->>'app_id' = $14)
		  AND ($15 = '' OR metadata_json->>'workflow_run_id' = $16)
		  AND ($17 = '' OR metadata_json->>'user_id' = $18)
		  AND ($19 = '' OR metadata_json->>'request_id' = $20)
		ORDER BY created_at DESC
	`
	var after any
	if !query.After.IsZero() {
		after = query.After.UTC()
	}
	var before any
	if !query.Before.IsZero() {
		before = query.Before.UTC()
	}
	args := []any{
		query.SandboxID, query.SandboxID,
		query.Type, query.Type,
		query.TypePrefix, query.TypePrefix + "%",
		after,
		before,
		query.OrganizationID, query.OrganizationID,
		query.WorkspaceID, query.WorkspaceID,
		query.AppID, query.AppID,
		query.WorkflowRunID, query.WorkflowRunID,
		query.UserID, query.UserID,
		query.RequestID, query.RequestID,
	}
	if query.Limit > 0 {
		statement += ` LIMIT $21`
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
		&box.OrganizationID,
		&box.WorkspaceID,
		&box.AppID,
		&box.WorkflowRunID,
		&box.UserID,
		&box.NetworkEnabled,
		&box.NetworkPolicy,
		&box.DependencyProfile,
		&box.DependencyArtifactChecksum,
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
	box.DependencyProfileVersion = metadata["dependency_profile_version"]
	return &box, nil
}

func scanDependencyProfile(scanner rowScanner) (*policy.DependencyProfile, error) {
	var profile policy.DependencyProfile
	var languagesJSON []byte
	var packagesJSON []byte
	if err := scanner.Scan(
		&profile.Name,
		&profile.Version,
		&profile.Status,
		&profile.Enabled,
		&profile.OwnerScope,
		&profile.Scope,
		&profile.OrganizationID,
		&languagesJSON,
		&packagesJSON,
		&profile.BaseRuntime,
		&profile.Checksum,
		&profile.ArtifactChecksum,
		&profile.SizeBytes,
		&profile.Description,
		&profile.PublicReusable,
		&profile.Pinned,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(languagesJSON, &profile.Languages); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(packagesJSON, &profile.Packages); err != nil {
		return nil, err
	}
	return &profile, nil
}

func scanLegacyDependencyProfile(scanner rowScanner) (*policy.DependencyProfile, error) {
	var profile policy.DependencyProfile
	var languagesJSON []byte
	var packagesJSON []byte
	if err := scanner.Scan(
		&profile.Name,
		&profile.Version,
		&profile.Status,
		&profile.Enabled,
		&profile.OwnerScope,
		&languagesJSON,
		&packagesJSON,
		&profile.BaseRuntime,
		&profile.Checksum,
		&profile.SizeBytes,
		&profile.Description,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(languagesJSON, &profile.Languages); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(packagesJSON, &profile.Packages); err != nil {
		return nil, err
	}
	profile.Scope = "global"
	profile.ArtifactChecksum = profile.Checksum
	profile.PublicReusable = true
	return &profile, nil
}

func scanDependencyBuildRequest(scanner rowScanner) (*DependencyBuildRequestRecord, error) {
	var record DependencyBuildRequestRecord
	if err := scanner.Scan(
		&record.BuildID,
		&record.Fingerprint,
		&record.Status,
		&record.OrganizationID,
		&record.ProfileName,
		&record.DependencyRequestJSON,
		&record.PackagesJSON,
		&record.SourcesJSON,
		&record.WarningsJSON,
		&record.PackageCount,
		&record.ArtifactChecksum,
		&record.SizeBytes,
		&record.Error,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &record, nil
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

func defaultStringSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

func defaultDependencyPackages(value []policy.DependencyPackage) []policy.DependencyPackage {
	if value == nil {
		return []policy.DependencyPackage{}
	}
	return value
}

func defaultRawJSON(value json.RawMessage, fallback string) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(fallback)
	}
	return value
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func dependencyProfileStorageKey(profile policy.DependencyProfile) string {
	scope := defaultString(profile.Scope, "global")
	return scope + "\x00" + profile.OrganizationID + "\x00" + profile.Name
}

func max(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
