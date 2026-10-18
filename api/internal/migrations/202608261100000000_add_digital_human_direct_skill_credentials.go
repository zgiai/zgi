package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608261100000000ID = "202608261100000000_add_digital_human_direct_skill_credentials"

const addDigitalHumanDirectSkillCredentialsSQL = `
	CREATE TABLE IF NOT EXISTS public.digital_human_direct_skill_credentials (
		id uuid PRIMARY KEY,
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE CASCADE,
		skill_id varchar(128) NOT NULL,
		encrypted_credentials text NOT NULL,
		credential_version integer NOT NULL DEFAULT 1,
		enabled boolean NOT NULL DEFAULT true,
		revision bigint NOT NULL DEFAULT 1,
		created_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		updated_by uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_direct_skill_id_check CHECK (skill_id <> ''),
		CONSTRAINT digital_human_direct_skill_credential_version_check CHECK (credential_version > 0),
		CONSTRAINT digital_human_direct_skill_revision_check CHECK (revision > 0)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_direct_skill_identity
		ON public.digital_human_direct_skill_credentials (
			organization_id, workspace_id, digital_human_id, skill_id
		);
	CREATE INDEX IF NOT EXISTS idx_digital_human_direct_skill_subject
		ON public.digital_human_direct_skill_credentials (
			organization_id, workspace_id, digital_human_id, enabled
		);
`

func init() {
	registerSchemaMigration(migration202608261100000000ID, up202608261100000000, down202608261100000000)
}

func up202608261100000000(schema *mschema.Builder) error {
	return schema.Raw(addDigitalHumanDirectSkillCredentialsSQL)
}

func down202608261100000000(schema *mschema.Builder) error {
	return schema.Raw(`DROP TABLE IF EXISTS public.digital_human_direct_skill_credentials;`)
}
