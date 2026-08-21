package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608201932016586ID = "202608201932016586_add_default_subject_to_channel_installations"

const addDefaultSubjectToChannelInstallationsSQL = `
	ALTER TABLE public.channel_installations
		ADD COLUMN IF NOT EXISTS default_subject_type varchar(32),
		ADD COLUMN IF NOT EXISTS default_subject_id uuid;

	ALTER TABLE public.channel_installations
		DROP CONSTRAINT IF EXISTS channel_installations_default_subject_check;
	ALTER TABLE public.channel_installations
		ADD CONSTRAINT channel_installations_default_subject_check CHECK (
			(default_subject_type IS NULL AND default_subject_id IS NULL)
			OR (default_subject_type = 'digital_human' AND default_subject_id IS NOT NULL)
		);

	CREATE INDEX IF NOT EXISTS idx_channel_installations_default_subject
		ON public.channel_installations (organization_id, workspace_id, default_subject_type, default_subject_id)
		WHERE deleted_at IS NULL AND default_subject_id IS NOT NULL;

	CREATE OR REPLACE FUNCTION public.enforce_channel_installation_default_subject()
	RETURNS trigger AS $$
	BEGIN
		IF NEW.default_subject_id IS NOT NULL AND NOT EXISTS (
			SELECT 1
			FROM public.digital_humans subject
			WHERE subject.id = NEW.default_subject_id
			  AND subject.organization_id = NEW.organization_id
			  AND subject.workspace_id = NEW.workspace_id
			  AND subject.deleted_at IS NULL
		) THEN
			RAISE EXCEPTION 'channel installation default subject scope mismatch' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;

	DROP TRIGGER IF EXISTS trg_channel_installation_default_subject ON public.channel_installations;
	CREATE TRIGGER trg_channel_installation_default_subject
		BEFORE INSERT OR UPDATE OF organization_id, workspace_id, default_subject_type, default_subject_id
		ON public.channel_installations
		FOR EACH ROW EXECUTE FUNCTION public.enforce_channel_installation_default_subject();
`

func init() {
	registerSchemaMigration(migration202608201932016586ID, up202608201932016586, down202608201932016586)
}

func up202608201932016586(schema *mschema.Builder) error {
	return schema.Raw(addDefaultSubjectToChannelInstallationsSQL)
}

func down202608201932016586(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TRIGGER IF EXISTS trg_channel_installation_default_subject ON public.channel_installations;
		DROP FUNCTION IF EXISTS public.enforce_channel_installation_default_subject();
		DROP INDEX IF EXISTS public.idx_channel_installations_default_subject;
		ALTER TABLE public.channel_installations
			DROP CONSTRAINT IF EXISTS channel_installations_default_subject_check,
			DROP COLUMN IF EXISTS default_subject_id,
			DROP COLUMN IF EXISTS default_subject_type;
	`)
}
