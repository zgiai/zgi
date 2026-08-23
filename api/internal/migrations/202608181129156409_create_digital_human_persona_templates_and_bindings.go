package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migration202608181129156409ID = "202608181129156409_create_digital_human_persona_templates_and_bindings"

const createDigitalHumanPersonaTemplatesAndBindingsSQL = `
	CREATE TABLE IF NOT EXISTS public.digital_human_persona_templates (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		name varchar(80) NOT NULL,
		persona text NOT NULL,
		goals jsonb NOT NULL DEFAULT '[]'::jsonb,
		rules jsonb NOT NULL DEFAULT '[]'::jsonb,
		revision bigint NOT NULL DEFAULT 1,
		created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at timestamptz,
		CONSTRAINT digital_human_persona_templates_revision_check CHECK (revision > 0),
		CONSTRAINT digital_human_persona_templates_persona_check CHECK (btrim(persona) <> ''),
		CONSTRAINT digital_human_persona_templates_goals_check CHECK (jsonb_typeof(goals) = 'array'),
		CONSTRAINT digital_human_persona_templates_rules_check CHECK (jsonb_typeof(rules) = 'array')
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_dh_persona_templates_scope_name
		ON public.digital_human_persona_templates (organization_id, workspace_id, lower(name))
		WHERE deleted_at IS NULL;

	CREATE TABLE IF NOT EXISTS public.digital_human_resource_bindings (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		digital_human_version_id uuid NOT NULL REFERENCES public.digital_human_versions(id) ON DELETE RESTRICT,
		binding_type varchar(32) NOT NULL,
		resource_id varchar(255) NOT NULL,
		parent_resource_id varchar(255) NOT NULL DEFAULT '',
		display_name varchar(255) NOT NULL DEFAULT '',
		access_mode varchar(16) NOT NULL,
		resource_revision varchar(255) NOT NULL,
		binding_revision varchar(71) NOT NULL,
		credential_ref varchar(512) NOT NULL DEFAULT '',
		metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
		authorized_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		authorized_at timestamptz,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT dh_resource_bindings_type_check CHECK (binding_type IN (
			'skill', 'knowledge_dataset', 'database', 'database_table', 'workflow', 'integration_connection'
		)),
		CONSTRAINT dh_resource_bindings_access_check CHECK (access_mode IN ('read', 'write', 'execute')),
		CONSTRAINT dh_resource_bindings_revision_check CHECK (resource_revision <> '' AND binding_revision <> '')
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_dh_resource_bindings_version_resource
		ON public.digital_human_resource_bindings (
			digital_human_version_id, binding_type, parent_resource_id, resource_id, access_mode
		);
	CREATE INDEX IF NOT EXISTS idx_dh_resource_bindings_scope
		ON public.digital_human_resource_bindings (organization_id, workspace_id, digital_human_id, digital_human_version_id);

	CREATE OR REPLACE FUNCTION public.enforce_dh_binding_scope_and_immutability()
	RETURNS trigger LANGUAGE plpgsql AS $$
	DECLARE owner_state varchar(16);
	BEGIN
		IF TG_OP IN ('UPDATE', 'DELETE') THEN
			SELECT state INTO owner_state FROM public.digital_human_versions WHERE id = OLD.digital_human_version_id;
			IF owner_state = 'published' THEN
				RAISE EXCEPTION 'published digital human resource bindings are immutable' USING ERRCODE = '23514';
			END IF;
		END IF;
		IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
		SELECT state INTO owner_state FROM public.digital_human_versions WHERE id = NEW.digital_human_version_id;
		IF owner_state <> 'draft' THEN
			RAISE EXCEPTION 'new digital human resource bindings require a draft owner' USING ERRCODE = '23514';
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM public.digital_human_versions version
			WHERE version.id = NEW.digital_human_version_id AND version.digital_human_id = NEW.digital_human_id
				AND version.organization_id = NEW.organization_id AND version.workspace_id = NEW.workspace_id
		) THEN RAISE EXCEPTION 'digital human resource binding scope is invalid' USING ERRCODE = '23514'; END IF;
		RETURN NEW;
	END;
	$$;
	CREATE TRIGGER trg_dh_binding_scope_and_immutability
		BEFORE INSERT OR UPDATE OR DELETE ON public.digital_human_resource_bindings
		FOR EACH ROW EXECUTE FUNCTION public.enforce_dh_binding_scope_and_immutability()
`

func init() {
	registerSchemaMigration(migration202608181129156409ID, up202608181129156409, down202608181129156409)
}

func up202608181129156409(schema *mschema.Builder) error {
	return schema.Raw(createDigitalHumanPersonaTemplatesAndBindingsSQL)
}

func down202608181129156409(schema *mschema.Builder) error {
	if err := schema.Raw(`
		DROP TRIGGER IF EXISTS trg_dh_binding_scope_and_immutability ON public.digital_human_resource_bindings;
		DROP FUNCTION IF EXISTS public.enforce_dh_binding_scope_and_immutability()
	`); err != nil {
		return err
	}
	for _, table := range []string{"digital_human_resource_bindings", "digital_human_persona_templates"} {
		if err := schema.DropIfExists(table); err != nil {
			return err
		}
	}
	return nil
}
