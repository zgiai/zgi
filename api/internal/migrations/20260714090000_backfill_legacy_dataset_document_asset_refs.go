package migrations

import (
	"fmt"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationBackfillLegacyDatasetDocumentAssetRefsID = "20260714090000_backfill_legacy_dataset_document_asset_refs"

const legacyDatasetDocumentBackfillSource = "legacy_dataset_document_backfill"

const createLegacyDatasetDocumentBackfillPlanSQL = `
CREATE TEMP TABLE legacy_dataset_document_asset_ref_plan ON COMMIT DROP AS
WITH document_sources AS (
	SELECT
		d.id AS document_id,
		d.organization_id::text AS organization_id,
		d.dataset_id,
		ds.workspace_id::text AS workspace_id,
		COALESCE(NULLIF(BTRIM(d.name), ''), 'Untitled document') AS document_name,
		d.created_by::text AS created_by,
		d.created_at,
		d.updated_at,
		d.completed_at,
		COALESCE(
			NULLIF(BTRIM(d.file_id), ''),
			NULLIF(BTRIM(d.doc_metadata ->> 'source_file_id'), ''),
			NULLIF(BTRIM(SUBSTRING(
				COALESCE(d.data_source_info, '')
				FROM '"upload_file_id"[[:space:]]*:[[:space:]]*"([^"]+)"'
			)), ''),
			NULLIF(BTRIM(SUBSTRING(
				COALESCE(d.data_source_info, '')
				FROM '"source_file_id"[[:space:]]*:[[:space:]]*"([^"]+)"'
			)), '')
		) AS original_source_file_id
	FROM public.documents AS d
	JOIN public.datasets AS ds ON ds.id = d.dataset_id
	WHERE NOT EXISTS (
		SELECT 1
		FROM public.data_library_knowledge_base_asset_refs AS existing_document_ref
		JOIN public.data_library_document_assets AS existing_document_asset
			ON existing_document_asset.id = existing_document_ref.asset_id
			AND existing_document_asset.deleted_at IS NULL
		WHERE existing_document_ref.dataset_document_id = d.id
		  AND existing_document_ref.deleted_at IS NULL
		  AND existing_document_ref.organization_id = d.organization_id::text
		  AND existing_document_ref.dataset_id = d.dataset_id
		  AND existing_document_asset.organization_id = d.organization_id::text
		  AND existing_document_asset.workspace_id IS NOT DISTINCT FROM ds.workspace_id::text
	)
)
SELECT
	document_sources.*,
	upload_files.id::text AS matched_source_file_id,
	COALESCE(NULLIF(upload_files.name, ''), document_sources.document_name) AS file_name,
	COALESCE(upload_files.hash, '') AS file_hash,
	upload_files.workspace_id::text AS source_workspace_id,
	COALESCE(upload_files.is_archived, false) AS source_file_archived
FROM document_sources
LEFT JOIN public.upload_files AS upload_files
	ON upload_files.id::text = document_sources.original_source_file_id
	AND upload_files.organization_id::text = document_sources.organization_id
`

const createLegacyDatasetDocumentBackfillPlanIndexSQL = `
CREATE UNIQUE INDEX legacy_dataset_document_asset_ref_plan_document
ON legacy_dataset_document_asset_ref_plan (document_id)
`

const insertLegacyDatasetDocumentCanonicalAssetsSQL = `
WITH ranked_sources AS (
	SELECT
		plan.*,
		ROW_NUMBER() OVER (
			PARTITION BY plan.organization_id, plan.matched_source_file_id
			ORDER BY
				CASE
					WHEN plan.workspace_id IS NOT DISTINCT FROM plan.source_workspace_id THEN 0
					ELSE 1
				END,
				plan.created_at,
				plan.document_id
		) AS source_rank
	FROM legacy_dataset_document_asset_ref_plan AS plan
	WHERE plan.matched_source_file_id IS NOT NULL
),
canonical_sources AS (
	SELECT *
	FROM ranked_sources
	WHERE source_rank = 1
)
INSERT INTO public.data_library_document_assets (
	id,
	organization_id,
	workspace_id,
	title,
	source_file_id,
	content_hash,
	status,
	processing_level,
	product_status,
	processing_progress,
	generation_no,
	chunk_count,
	vector_status,
	metadata_json,
	permission_policy,
	created_by,
	created_at,
	updated_at
)
SELECT
	public.uuid_generate_v4(),
	canonical_sources.organization_id,
	COALESCE(canonical_sources.source_workspace_id, canonical_sources.workspace_id),
	canonical_sources.file_name,
	canonical_sources.matched_source_file_id,
	NULLIF(canonical_sources.file_hash, ''),
	'archived',
	'archive',
	'stored_only',
	0,
	0,
	0,
	'none',
	jsonb_build_object(
		'source', '` + legacyDatasetDocumentBackfillSource + `',
		'legacy_backfill', true,
		'source_file_available', true
	),
	'{}'::jsonb,
	canonical_sources.created_by,
	canonical_sources.created_at,
	canonical_sources.updated_at
FROM canonical_sources
WHERE NOT EXISTS (
	SELECT 1
	FROM public.data_library_document_assets AS existing_asset
	WHERE existing_asset.organization_id = canonical_sources.organization_id
	  AND existing_asset.source_file_id = canonical_sources.matched_source_file_id
	  AND existing_asset.deleted_at IS NULL
)
ON CONFLICT DO NOTHING
`

const createLegacyDatasetDocumentResolvedPlanSQL = `
CREATE TEMP TABLE legacy_dataset_document_asset_ref_resolved ON COMMIT DROP AS
WITH canonical_candidates AS (
	SELECT
		plan.*,
		assets.id AS canonical_asset_id,
		ROW_NUMBER() OVER (
			PARTITION BY plan.organization_id, plan.dataset_id, assets.id
			ORDER BY plan.created_at, plan.document_id
		) AS dataset_asset_rank,
		EXISTS (
			SELECT 1
			FROM public.data_library_knowledge_base_asset_refs AS existing_asset_ref
			WHERE existing_asset_ref.organization_id = plan.organization_id
			  AND existing_asset_ref.dataset_id = plan.dataset_id
			  AND existing_asset_ref.asset_id = assets.id
			  AND existing_asset_ref.deleted_at IS NULL
		) AS asset_already_referenced
	FROM legacy_dataset_document_asset_ref_plan AS plan
	LEFT JOIN public.data_library_document_assets AS assets
		ON assets.organization_id = plan.organization_id
		AND assets.source_file_id = plan.matched_source_file_id
		AND assets.deleted_at IS NULL
		AND assets.workspace_id IS NOT DISTINCT FROM plan.workspace_id
)
SELECT
	canonical_candidates.*,
	CASE
		WHEN canonical_asset_id IS NOT NULL
		 AND dataset_asset_rank = 1
		 AND NOT asset_already_referenced
		THEN canonical_asset_id
		ELSE public.uuid_generate_v5(
			'79f2cf2c-29a4-4d3a-9f3b-72a78bc0912a'::uuid,
			'legacy-dataset-document-asset:' || document_id::text
		)
	END AS resolved_asset_id,
	CASE
		WHEN canonical_asset_id IS NOT NULL
		 AND dataset_asset_rank = 1
		 AND NOT asset_already_referenced
		THEN true
		ELSE false
	END AS uses_source_asset,
	CASE
		WHEN matched_source_file_id IS NULL THEN 'missing_source_file'
		WHEN canonical_asset_id IS NULL THEN 'workspace_mismatch'
		WHEN asset_already_referenced THEN 'existing_dataset_asset_ref'
		WHEN dataset_asset_rank > 1 THEN 'duplicate_dataset_file'
		ELSE 'compatibility_fallback'
	END AS placeholder_reason
FROM canonical_candidates
`

const createLegacyDatasetDocumentResolvedPlanIndexSQL = `
CREATE UNIQUE INDEX legacy_dataset_document_asset_ref_resolved_document
ON legacy_dataset_document_asset_ref_resolved (document_id)
`

const insertLegacyDatasetDocumentPlaceholderAssetsSQL = `
INSERT INTO public.data_library_document_assets (
	id,
	organization_id,
	workspace_id,
	title,
	source_file_id,
	content_hash,
	status,
	processing_level,
	product_status,
	processing_progress,
	generation_no,
	chunk_count,
	vector_status,
	metadata_json,
	permission_policy,
	created_by,
	created_at,
	updated_at
)
SELECT
	resolved.resolved_asset_id,
	resolved.organization_id,
	resolved.workspace_id,
	resolved.file_name,
	resolved.resolved_asset_id::text,
	NULLIF(resolved.file_hash, ''),
	'archived',
	'archive',
	'stored_only',
	0,
	0,
	0,
	'none',
	jsonb_strip_nulls(jsonb_build_object(
		'source', '` + legacyDatasetDocumentBackfillSource + `',
		'legacy_backfill', true,
		'legacy_placeholder', true,
		'source_file_available', false,
		'legacy_document_id', resolved.document_id::text,
		'original_source_file_id', resolved.original_source_file_id,
		'placeholder_reason', resolved.placeholder_reason
	)),
	'{}'::jsonb,
	resolved.created_by,
	resolved.created_at,
	resolved.updated_at
FROM legacy_dataset_document_asset_ref_resolved AS resolved
WHERE NOT resolved.uses_source_asset
ON CONFLICT DO NOTHING
`

const insertLegacyDatasetDocumentAssetRefsSQL = `
INSERT INTO public.data_library_knowledge_base_asset_refs (
	id,
	organization_id,
	workspace_id,
	dataset_id,
	asset_id,
	version_id,
	dataset_document_id,
	status,
	sync_status,
	synced_generation_no,
	sync_run_id,
	last_synced_at,
	metadata_json,
	created_by,
	created_at,
	updated_at
)
SELECT
	public.uuid_generate_v5(
		'24a6498e-ff2c-4697-89dd-db3a7d03f2ec'::uuid,
		'legacy-dataset-document-ref:' || resolved.document_id::text
	),
	resolved.organization_id,
	resolved.workspace_id,
	resolved.dataset_id,
	resolved.resolved_asset_id,
	NULL,
	resolved.document_id,
	'active',
	'synced',
	NULL,
	NULL,
	COALESCE(resolved.completed_at, resolved.updated_at, resolved.created_at),
	jsonb_strip_nulls(jsonb_build_object(
		'source', '` + legacyDatasetDocumentBackfillSource + `',
		'legacy_backfill', true,
		'legacy_document_id', resolved.document_id::text,
		'legacy_placeholder', NOT resolved.uses_source_asset,
		'original_source_file_id', resolved.original_source_file_id,
		'placeholder_reason', CASE
			WHEN resolved.uses_source_asset THEN NULL
			ELSE resolved.placeholder_reason
		END
	)),
	resolved.created_by,
	resolved.created_at,
	resolved.updated_at
FROM legacy_dataset_document_asset_ref_resolved AS resolved
JOIN public.data_library_document_assets AS assets
	ON assets.id = resolved.resolved_asset_id
	AND assets.deleted_at IS NULL
	AND (
		resolved.uses_source_asset
		OR (
			assets.metadata_json ->> 'source' = '` + legacyDatasetDocumentBackfillSource + `'
			AND assets.metadata_json ->> 'legacy_document_id' = resolved.document_id::text
		)
	)
ON CONFLICT DO NOTHING
`

const countUnlinkedLegacyDatasetDocumentsSQL = `
SELECT COUNT(*)
FROM legacy_dataset_document_asset_ref_resolved AS resolved
WHERE (
	SELECT COUNT(*)
	FROM public.data_library_knowledge_base_asset_refs AS refs
	JOIN public.data_library_document_assets AS assets
		ON assets.id = refs.asset_id
		AND assets.deleted_at IS NULL
	WHERE refs.dataset_document_id = resolved.document_id
	  AND refs.deleted_at IS NULL
	  AND refs.organization_id = resolved.organization_id
	  AND refs.dataset_id = resolved.dataset_id
	  AND assets.organization_id = resolved.organization_id
	  AND assets.workspace_id IS NOT DISTINCT FROM resolved.workspace_id
) <> 1
`

func init() {
	registerSchemaMigration(
		migrationBackfillLegacyDatasetDocumentAssetRefsID,
		upBackfillLegacyDatasetDocumentAssetRefs,
		downBackfillLegacyDatasetDocumentAssetRefs,
	)
}

func upBackfillLegacyDatasetDocumentAssetRefs(schema *mschema.Builder) error {
	return schema.DataFix("backfill legacy dataset documents into file assets and knowledge base refs", func(db *gorm.DB) error {
		return db.Transaction(func(tx *gorm.DB) error {
			statements := []struct {
				description string
				sql         string
			}{
				{"create legacy dataset document backfill plan", createLegacyDatasetDocumentBackfillPlanSQL},
				{"index legacy dataset document backfill plan", createLegacyDatasetDocumentBackfillPlanIndexSQL},
				{"insert canonical legacy document assets", insertLegacyDatasetDocumentCanonicalAssetsSQL},
				{"resolve legacy dataset document asset mappings", createLegacyDatasetDocumentResolvedPlanSQL},
				{"index resolved legacy dataset document mappings", createLegacyDatasetDocumentResolvedPlanIndexSQL},
				{"insert placeholder legacy document assets", insertLegacyDatasetDocumentPlaceholderAssetsSQL},
				{"insert legacy knowledge base asset refs", insertLegacyDatasetDocumentAssetRefsSQL},
			}

			for _, statement := range statements {
				if err := tx.Exec(statement.sql).Error; err != nil {
					return fmt.Errorf("%s: %w", statement.description, err)
				}
			}

			var unlinked int64
			if err := tx.Raw(countUnlinkedLegacyDatasetDocumentsSQL).Scan(&unlinked).Error; err != nil {
				return fmt.Errorf("verify legacy dataset document refs: %w", err)
			}
			if unlinked != 0 {
				return fmt.Errorf("legacy dataset document backfill left %d documents without exactly one active ref", unlinked)
			}
			return nil
		})
	})
}

func downBackfillLegacyDatasetDocumentAssetRefs(_ *mschema.Builder) error {
	// Keep the data fix irreversible: removing refs after users start managing the
	// restored documents would hide them again and could orphan later file work.
	return nil
}
