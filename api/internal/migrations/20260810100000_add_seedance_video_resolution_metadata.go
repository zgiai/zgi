package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddSeedanceVideoResolutionMetadataID = "20260810100000_add_seedance_video_resolution_metadata"

func init() {
	registerSchemaMigration(
		migrationAddSeedanceVideoResolutionMetadataID,
		upAddSeedanceVideoResolutionMetadata,
		nil,
	)
}

func upAddSeedanceVideoResolutionMetadata(schema *mschema.Builder) error {
	return schema.Raw(`
		UPDATE public.llm_models
		SET
			default_parameters = jsonb_set(
				COALESCE(default_parameters, '{}'::jsonb),
				'{capabilities}',
				COALESCE(default_parameters->'capabilities', '{}'::jsonb)
					|| jsonb_build_object(
						'video',
						COALESCE(default_parameters#>'{capabilities,video}', '{}'::jsonb)
							|| jsonb_build_object(
								'resolutions',
								CASE name
									WHEN 'doubao-seedance-2-0-260128' THEN '["480p","720p","1080p","4k"]'::jsonb
									ELSE '["480p","720p"]'::jsonb
								END
							)
					),
				true
			),
			updated_at = NOW()
		WHERE provider = 'doubao'
			AND name IN (
				'doubao-seedance-2-0-260128',
				'doubao-seedance-2-0-fast-260128',
				'doubao-seedance-2-0-mini-260615'
			)
			AND deleted_at IS NULL
	`)
}
