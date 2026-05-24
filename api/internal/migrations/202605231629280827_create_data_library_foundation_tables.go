package migrations

import (
	"fmt"
	"strings"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration202605231629280827ID = "202605231629280827_create_data_library_foundation_tables"

func init() {
	registerSchemaMigration(migration202605231629280827ID, upCreateDataLibraryFoundationTables, downCreateDataLibraryFoundationTables)
}

func upCreateDataLibraryFoundationTables(schema *mschema.Builder) error {
	statements := []string{
		`
		CREATE TABLE IF NOT EXISTS content_parse_chunk_artifact_sets (
			id UUID PRIMARY KEY,
			parse_artifact_id UUID NULL REFERENCES content_parse_artifacts(id) ON DELETE SET NULL,
			parse_run_id UUID NULL REFERENCES content_parse_runs(id) ON DELETE SET NULL,
			source_content_hash VARCHAR(255) NOT NULL,
			use_case VARCHAR(32) NOT NULL,
			planner_name VARCHAR(64) NOT NULL,
			parent_mode VARCHAR(64) NULL,
			segmentation VARCHAR(64) NULL,
			chunker_version VARCHAR(64) NOT NULL,
			signature VARCHAR(255) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'succeeded',
			unit_count INTEGER NOT NULL DEFAULT 0,
			content_hash VARCHAR(255) NOT NULL,
			quality_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			artifact_storage_key TEXT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_content_parse_chunk_artifact_sets_signature
		ON content_parse_chunk_artifact_sets (signature)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_content_parse_chunk_artifact_sets_parse_artifact
		ON content_parse_chunk_artifact_sets (parse_artifact_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_content_parse_chunk_artifact_sets_parse_run
		ON content_parse_chunk_artifact_sets (parse_run_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_content_parse_chunk_artifact_sets_source_hash
		ON content_parse_chunk_artifact_sets (source_content_hash, use_case, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_content_parse_chunk_artifact_sets_deleted_at
		ON content_parse_chunk_artifact_sets (deleted_at)
		`,
		`
		ALTER TABLE content_parse_chunking_runs
		ADD COLUMN IF NOT EXISTS chunk_artifact_set_id UUID NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_content_parse_chunking_artifact_set
		ON content_parse_chunking_runs (chunk_artifact_set_id)
		`,
		`
		CREATE TABLE IF NOT EXISTS data_library_document_assets (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			title TEXT NOT NULL,
			source_file_id VARCHAR(255) NOT NULL,
			current_version_id UUID NULL,
			content_hash VARCHAR(255) NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'archived',
			processing_level VARCHAR(32) NOT NULL DEFAULT 'archive',
			quality_score DOUBLE PRECISION NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			permission_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by VARCHAR(255) NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE TABLE IF NOT EXISTS data_library_document_versions (
			id UUID PRIMARY KEY,
			asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
			version_no INTEGER NOT NULL,
			source_file_id VARCHAR(255) NOT NULL,
			content_hash VARCHAR(255) NULL,
			file_name TEXT NULL,
			file_size BIGINT NOT NULL DEFAULT 0,
			mime_type VARCHAR(255) NULL,
			parse_artifact_id UUID NULL REFERENCES content_parse_artifacts(id) ON DELETE SET NULL,
			chunk_artifact_set_id UUID NULL REFERENCES content_parse_chunk_artifact_sets(id) ON DELETE SET NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'archived',
			quality_score DOUBLE PRECISION NULL,
			uploaded_by VARCHAR(255) NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_data_library_document_versions_asset_version
		ON data_library_document_versions (asset_id, version_no)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_org_workspace_status
		ON data_library_document_assets (organization_id, workspace_id, status, updated_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_source_file
		ON data_library_document_assets (source_file_id)
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_data_library_assets_org_source_file_active
		ON data_library_document_assets (organization_id, source_file_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_content_hash
		ON data_library_document_assets (content_hash)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_assets_deleted_at
		ON data_library_document_assets (deleted_at)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_versions_asset_created
		ON data_library_document_versions (asset_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_versions_source_file
		ON data_library_document_versions (source_file_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_versions_content_hash
		ON data_library_document_versions (content_hash)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_versions_parse_artifact
		ON data_library_document_versions (parse_artifact_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_versions_chunk_artifact
		ON data_library_document_versions (chunk_artifact_set_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_versions_deleted_at
		ON data_library_document_versions (deleted_at)
		`,
		`
		CREATE TABLE IF NOT EXISTS data_library_reuse_events (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
			version_id UUID NULL REFERENCES data_library_document_versions(id) ON DELETE SET NULL,
			artifact_type VARCHAR(64) NOT NULL,
			artifact_id UUID NULL,
			consumer_type VARCHAR(64) NOT NULL,
			consumer_id VARCHAR(255) NOT NULL,
			consumer_version VARCHAR(255) NULL,
			saved_seconds BIGINT NOT NULL DEFAULT 0,
			saved_cost_micros BIGINT NOT NULL DEFAULT 0,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by VARCHAR(255) NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_org_consumer
		ON data_library_reuse_events (organization_id, consumer_type, consumer_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_asset_created
		ON data_library_reuse_events (asset_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_version
		ON data_library_reuse_events (version_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_artifact
		ON data_library_reuse_events (artifact_type, artifact_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_workspace
		ON data_library_reuse_events (workspace_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_reuse_events_deleted_at
		ON data_library_reuse_events (deleted_at)
		`,
		`
		CREATE TABLE IF NOT EXISTS data_library_processing_requests (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
			target_level VARCHAR(32) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'planned',
			requested_by VARCHAR(255) NULL,
			force BOOLEAN NOT NULL DEFAULT FALSE,
			plan_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			request_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			execution_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
			executor_key VARCHAR(255) NULL,
			error_code VARCHAR(128) NULL,
			error_message TEXT NULL,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			queued_at TIMESTAMPTZ NULL,
			started_at TIMESTAMPTZ NULL,
			completed_at TIMESTAMPTZ NULL,
			failed_at TIMESTAMPTZ NULL,
			cancelled_at TIMESTAMPTZ NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_org_status
		ON data_library_processing_requests (organization_id, status, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_asset_created
		ON data_library_processing_requests (asset_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_workspace
		ON data_library_processing_requests (workspace_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_deleted_at
		ON data_library_processing_requests (deleted_at)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_processing_requests_executor_status
		ON data_library_processing_requests (executor_key, status, created_at DESC)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE TABLE IF NOT EXISTS data_library_vector_artifacts (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
			version_id UUID NOT NULL REFERENCES data_library_document_versions(id) ON DELETE CASCADE,
			chunk_artifact_set_id UUID NOT NULL REFERENCES content_parse_chunk_artifact_sets(id) ON DELETE RESTRICT,
			embedding_provider VARCHAR(128) NOT NULL,
			embedding_model VARCHAR(255) NOT NULL,
			embedding_dimension INTEGER NOT NULL DEFAULT 0,
			vector_collection VARCHAR(255) NOT NULL,
			vector_namespace VARCHAR(255) NULL,
			vector_count BIGINT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			content_hash VARCHAR(255) NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by VARCHAR(255) NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_org_status
		ON data_library_vector_artifacts (organization_id, status, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_asset_created
		ON data_library_vector_artifacts (asset_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_version
		ON data_library_vector_artifacts (version_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_chunk_set
		ON data_library_vector_artifacts (chunk_artifact_set_id, created_at DESC)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_workspace
		ON data_library_vector_artifacts (workspace_id)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_content_hash
		ON data_library_vector_artifacts (content_hash)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_vector_artifacts_deleted_at
		ON data_library_vector_artifacts (deleted_at)
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS uq_data_library_vector_artifacts_active_model
		ON data_library_vector_artifacts (
			organization_id,
			chunk_artifact_set_id,
			embedding_provider,
			embedding_model,
			vector_collection,
			COALESCE(vector_namespace, '')
		)
		WHERE deleted_at IS NULL AND status IN ('pending', 'ready')
		`,
		`
		CREATE TABLE IF NOT EXISTS data_library_knowledge_base_asset_refs (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			dataset_id UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
			asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
			version_id UUID NOT NULL REFERENCES data_library_document_versions(id) ON DELETE CASCADE,
			chunk_artifact_set_id UUID NULL REFERENCES content_parse_chunk_artifact_sets(id) ON DELETE SET NULL,
			vector_artifact_id UUID NULL REFERENCES data_library_vector_artifacts(id) ON DELETE SET NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by VARCHAR(255) NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_org_dataset
		ON data_library_knowledge_base_asset_refs (organization_id, dataset_id, status)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_asset
		ON data_library_knowledge_base_asset_refs (asset_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_version
		ON data_library_knowledge_base_asset_refs (version_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_chunk_set
		ON data_library_knowledge_base_asset_refs (chunk_artifact_set_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_vector
		ON data_library_knowledge_base_asset_refs (vector_artifact_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_kb_asset_refs_workspace
		ON data_library_knowledge_base_asset_refs (workspace_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_data_library_kb_asset_refs_active
		ON data_library_knowledge_base_asset_refs (organization_id, dataset_id, asset_id, version_id)
		WHERE deleted_at IS NULL AND status = 'active'
		`,
		`
		CREATE TABLE IF NOT EXISTS data_library_database_asset_refs (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			data_source_id UUID NOT NULL REFERENCES data_sources(id) ON DELETE CASCADE,
			table_id UUID NULL REFERENCES data_source_tables(id) ON DELETE SET NULL,
			asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
			version_id UUID NOT NULL REFERENCES data_library_document_versions(id) ON DELETE CASCADE,
			parse_artifact_id UUID NULL REFERENCES content_parse_artifacts(id) ON DELETE SET NULL,
			extraction_artifact_id UUID NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by VARCHAR(255) NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_org_source
		ON data_library_database_asset_refs (organization_id, data_source_id, status)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_table
		ON data_library_database_asset_refs (table_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_asset
		ON data_library_database_asset_refs (asset_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_version
		ON data_library_database_asset_refs (version_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_parse
		ON data_library_database_asset_refs (parse_artifact_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_extraction
		ON data_library_database_asset_refs (extraction_artifact_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_db_asset_refs_workspace
		ON data_library_database_asset_refs (workspace_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_data_library_db_asset_refs_active_source
		ON data_library_database_asset_refs (organization_id, data_source_id, asset_id, version_id)
		WHERE deleted_at IS NULL AND status = 'active' AND table_id IS NULL
		`,
		`
		CREATE UNIQUE INDEX IF NOT EXISTS uniq_data_library_db_asset_refs_active_table
		ON data_library_database_asset_refs (organization_id, data_source_id, table_id, asset_id, version_id)
		WHERE deleted_at IS NULL AND status = 'active' AND table_id IS NOT NULL
		`,
		`
		CREATE TABLE IF NOT EXISTS data_library_extraction_artifacts (
			id UUID PRIMARY KEY,
			organization_id VARCHAR(255) NOT NULL,
			workspace_id VARCHAR(255) NULL,
			asset_id UUID NOT NULL REFERENCES data_library_document_assets(id) ON DELETE CASCADE,
			version_id UUID NOT NULL REFERENCES data_library_document_versions(id) ON DELETE CASCADE,
			parse_artifact_id UUID NULL REFERENCES content_parse_artifacts(id) ON DELETE SET NULL,
			data_source_id UUID NULL REFERENCES data_sources(id) ON DELETE SET NULL,
			table_id UUID NULL REFERENCES data_source_tables(id) ON DELETE SET NULL,
			schema_name VARCHAR(255) NULL,
			schema_hash VARCHAR(255) NULL,
			extractor_provider VARCHAR(128) NULL,
			extractor_model VARCHAR(255) NULL,
			record_count BIGINT NOT NULL DEFAULT 0,
			field_count BIGINT NOT NULL DEFAULT 0,
			evidence_count BIGINT NOT NULL DEFAULT 0,
			status VARCHAR(32) NOT NULL DEFAULT 'pending',
			quality_score DOUBLE PRECISION NULL,
			content_hash VARCHAR(255) NULL,
			output_uri TEXT NULL,
			metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_by VARCHAR(255) NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ NULL
		)
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_org_status
		ON data_library_extraction_artifacts (organization_id, status, created_at DESC)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_asset_created
		ON data_library_extraction_artifacts (asset_id, created_at DESC)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_version
		ON data_library_extraction_artifacts (version_id, created_at DESC)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_parse
		ON data_library_extraction_artifacts (parse_artifact_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_source
		ON data_library_extraction_artifacts (data_source_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_table
		ON data_library_extraction_artifacts (table_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_workspace
		ON data_library_extraction_artifacts (workspace_id)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_schema_hash
		ON data_library_extraction_artifacts (schema_hash)
		WHERE deleted_at IS NULL
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_data_library_extraction_artifacts_content_hash
		ON data_library_extraction_artifacts (content_hash)
		WHERE deleted_at IS NULL
		`,
		`
		ALTER TABLE data_library_database_asset_refs
		ADD CONSTRAINT fk_data_library_db_asset_refs_extraction_artifact
		FOREIGN KEY (extraction_artifact_id)
		REFERENCES data_library_extraction_artifacts(id)
		ON DELETE SET NULL
		`,
	}
	for _, statement := range statements {
		if err := schema.Raw(statement); err != nil {
			if isDataLibraryDuplicateConstraintError(err) {
				continue
			}
			return fmt.Errorf("create data library foundation tables: %w", err)
		}
	}
	return nil
}

func downCreateDataLibraryFoundationTables(schema *mschema.Builder) error {
	return fmt.Errorf("rollback for migration %s is not supported", migration202605231629280827ID)
}

func isDataLibraryDuplicateConstraintError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "SQLSTATE 42710") || strings.Contains(message, "already exists")
}
