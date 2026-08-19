package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration202608181104207370ID = "202608181104207370_create_digital_human_foundation"

const createDigitalHumanFoundationSQL = `
	CREATE TABLE IF NOT EXISTS public.digital_humans (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		name varchar(80) NOT NULL,
		avatar text NOT NULL DEFAULT '',
		description text NOT NULL DEFAULT '',
		status varchar(16) NOT NULL DEFAULT 'draft',
		autonomy_level varchar(16) NOT NULL DEFAULT 'supervised',
		visibility varchar(16) NOT NULL DEFAULT 'workspace',
		draft_version_id uuid,
		published_version_id uuid,
		revision bigint NOT NULL DEFAULT 1,
		created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at timestamptz,
		CONSTRAINT digital_humans_status_check
			CHECK (status IN ('draft', 'active', 'disabled')),
		CONSTRAINT digital_humans_autonomy_level_check
			CHECK (autonomy_level IN ('assistive', 'supervised', 'autonomous')),
		CONSTRAINT digital_humans_visibility_check
			CHECK (visibility IN ('private', 'workspace')),
		CONSTRAINT digital_humans_revision_check CHECK (revision > 0)
	);

	CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_humans_scope_name
		ON public.digital_humans (organization_id, workspace_id, lower(name))
		WHERE deleted_at IS NULL;
	CREATE INDEX IF NOT EXISTS idx_digital_humans_scope_updated
		ON public.digital_humans (organization_id, workspace_id, updated_at DESC, id DESC)
		WHERE deleted_at IS NULL;

	CREATE TABLE IF NOT EXISTS public.digital_human_versions (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		state varchar(16) NOT NULL,
		version_number integer NOT NULL DEFAULT 0,
		persona text NOT NULL DEFAULT '',
		rules jsonb NOT NULL DEFAULT '[]'::jsonb,
		default_model varchar(255) NOT NULL DEFAULT '',
		memory_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
		trigger_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
		identity_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
		revision varchar(71) NOT NULL,
		lock_version bigint NOT NULL DEFAULT 1,
		published_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		published_at timestamptz,
		created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT digital_human_versions_state_check
			CHECK (state IN ('draft', 'published')),
		CONSTRAINT digital_human_versions_number_check
			CHECK ((state = 'draft' AND version_number = 0 AND published_at IS NULL)
				OR (state = 'published' AND version_number > 0 AND published_at IS NOT NULL)),
		CONSTRAINT digital_human_versions_lock_check CHECK (lock_version > 0)
	);

	CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_versions_one_draft
		ON public.digital_human_versions (digital_human_id)
		WHERE state = 'draft';
	CREATE UNIQUE INDEX IF NOT EXISTS idx_digital_human_versions_number
		ON public.digital_human_versions (digital_human_id, version_number)
		WHERE state = 'published';
	CREATE INDEX IF NOT EXISTS idx_digital_human_versions_scope
		ON public.digital_human_versions (organization_id, workspace_id, digital_human_id, state);

	ALTER TABLE public.digital_humans
		ADD CONSTRAINT fk_digital_humans_draft_version
		FOREIGN KEY (draft_version_id) REFERENCES public.digital_human_versions(id) ON DELETE SET NULL;
	ALTER TABLE public.digital_humans
		ADD CONSTRAINT fk_digital_humans_published_version
		FOREIGN KEY (published_version_id) REFERENCES public.digital_human_versions(id) ON DELETE SET NULL;

	CREATE OR REPLACE FUNCTION public.enforce_digital_human_version_scope()
	RETURNS trigger
	LANGUAGE plpgsql
	AS $$
	DECLARE
		parent_organization_id uuid;
		parent_workspace_id uuid;
	BEGIN
		SELECT organization_id, workspace_id
		INTO parent_organization_id, parent_workspace_id
		FROM public.digital_humans
		WHERE id = NEW.digital_human_id AND deleted_at IS NULL;
		IF parent_organization_id IS NULL
			OR parent_organization_id <> NEW.organization_id
			OR parent_workspace_id <> NEW.workspace_id THEN
			RAISE EXCEPTION 'digital human version scope does not match its owner' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
	$$;
	CREATE TRIGGER trg_digital_human_version_scope
		BEFORE INSERT OR UPDATE ON public.digital_human_versions
		FOR EACH ROW EXECUTE FUNCTION public.enforce_digital_human_version_scope();

	CREATE OR REPLACE FUNCTION public.prevent_published_digital_human_version_mutation()
	RETURNS trigger
	LANGUAGE plpgsql
	AS $$
	BEGIN
		IF OLD.state = 'published' THEN
			RAISE EXCEPTION 'published digital human versions are immutable' USING ERRCODE = '23514';
		END IF;
		RETURN COALESCE(NEW, OLD);
	END;
	$$;
	CREATE TRIGGER trg_published_digital_human_version_immutable
		BEFORE UPDATE OR DELETE ON public.digital_human_versions
		FOR EACH ROW EXECUTE FUNCTION public.prevent_published_digital_human_version_mutation();

	CREATE OR REPLACE FUNCTION public.validate_digital_human_version_pointers()
	RETURNS trigger
	LANGUAGE plpgsql
	AS $$
	BEGIN
		IF NEW.draft_version_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM public.digital_human_versions version
			WHERE version.id = NEW.draft_version_id
				AND version.digital_human_id = NEW.id
				AND version.organization_id = NEW.organization_id
				AND version.workspace_id = NEW.workspace_id
				AND version.state = 'draft'
		) THEN
			RAISE EXCEPTION 'digital human draft pointer is invalid' USING ERRCODE = '23514';
		END IF;
		IF NEW.published_version_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM public.digital_human_versions version
			WHERE version.id = NEW.published_version_id
				AND version.digital_human_id = NEW.id
				AND version.organization_id = NEW.organization_id
				AND version.workspace_id = NEW.workspace_id
				AND version.state = 'published'
		) THEN
			RAISE EXCEPTION 'digital human published pointer is invalid' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
	$$;
	CREATE TRIGGER trg_digital_human_version_pointers
		BEFORE INSERT OR UPDATE OF draft_version_id, published_version_id ON public.digital_humans
		FOR EACH ROW EXECUTE FUNCTION public.validate_digital_human_version_pointers()
`

func init() {
	registerSchemaMigration(migration202608181104207370ID, up202608181104207370, down202608181104207370)
}

func up202608181104207370(schema *mschema.Builder) error {
	return schema.Raw(createDigitalHumanFoundationSQL)
}

func down202608181104207370(schema *mschema.Builder) error {
	if err := schema.Raw(`
		DROP TRIGGER IF EXISTS trg_digital_human_version_pointers ON public.digital_humans;
		DROP TRIGGER IF EXISTS trg_published_digital_human_version_immutable ON public.digital_human_versions;
		DROP TRIGGER IF EXISTS trg_digital_human_version_scope ON public.digital_human_versions;
		ALTER TABLE IF EXISTS public.digital_humans
			DROP CONSTRAINT IF EXISTS fk_digital_humans_draft_version,
			DROP CONSTRAINT IF EXISTS fk_digital_humans_published_version;
		DROP FUNCTION IF EXISTS public.validate_digital_human_version_pointers();
		DROP FUNCTION IF EXISTS public.prevent_published_digital_human_version_mutation();
		DROP FUNCTION IF EXISTS public.enforce_digital_human_version_scope()
	`); err != nil {
		return err
	}
	if err := schema.DropIfExists("digital_human_versions"); err != nil {
		return err
	}
	return schema.DropIfExists("digital_humans")
}
