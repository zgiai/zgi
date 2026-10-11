package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateDigitalHumanExternalFilesID = "202609101000000000_create_digital_human_external_files"

func init() {
	registerSchemaMigration(
		migrationCreateDigitalHumanExternalFilesID,
		upCreateDigitalHumanExternalFiles,
		downCreateDigitalHumanExternalFiles,
	)
}

func upCreateDigitalHumanExternalFiles(schema *mschema.Builder) error {
	return schema.Raw(`
		CREATE TABLE IF NOT EXISTS public.digital_human_external_files (
			file_id uuid PRIMARY KEY REFERENCES public.upload_files(id) ON DELETE CASCADE,
			digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE CASCADE,
			organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
			workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
			user_subject_kind varchar(32) NOT NULL,
			user_subject_id uuid NOT NULL,
			surface varchar(32) NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT ck_digital_human_external_file_surface
				CHECK (surface IN ('digital_human_api', 'digital_human_webapp')),
			CONSTRAINT ck_digital_human_external_file_subject_kind
				CHECK (user_subject_kind IN ('account', 'api_end_user'))
		);
		CREATE INDEX IF NOT EXISTS idx_digital_human_external_files_owner
			ON public.digital_human_external_files
			(digital_human_id, user_subject_id, surface, created_at DESC);
	`)
}

func downCreateDigitalHumanExternalFiles(schema *mschema.Builder) error {
	return schema.DropIfExists("digital_human_external_files")
}
