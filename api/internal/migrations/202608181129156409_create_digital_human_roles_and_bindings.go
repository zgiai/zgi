package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migration202608181129156409ID = "202608181129156409_create_digital_human_roles_and_bindings"

const createDigitalHumanRolesAndBindingsSQL = `
	CREATE TABLE IF NOT EXISTS public.digital_human_role_templates (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		name varchar(80) NOT NULL,
		description text NOT NULL DEFAULT '',
		draft_version_id uuid,
		published_version_id uuid,
		revision bigint NOT NULL DEFAULT 1,
		created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at timestamptz,
		CONSTRAINT digital_human_role_templates_revision_check CHECK (revision > 0)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_dh_role_templates_scope_name
		ON public.digital_human_role_templates (organization_id, workspace_id, lower(name))
		WHERE deleted_at IS NULL;

	CREATE TABLE IF NOT EXISTS public.digital_human_role_template_versions (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		role_template_id uuid NOT NULL REFERENCES public.digital_human_role_templates(id) ON DELETE RESTRICT,
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		state varchar(16) NOT NULL,
		version_number integer NOT NULL DEFAULT 0,
		goals jsonb NOT NULL DEFAULT '[]'::jsonb,
		responsibilities jsonb NOT NULL DEFAULT '[]'::jsonb,
		capabilities jsonb NOT NULL DEFAULT '[]'::jsonb,
		sop_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
		knowledge_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
		policy jsonb NOT NULL DEFAULT '{}'::jsonb,
		evaluation_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
		revision varchar(71) NOT NULL,
		lock_version bigint NOT NULL DEFAULT 1,
		published_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		published_at timestamptz,
		created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT dh_role_template_versions_state_check CHECK (state IN ('draft', 'published')),
		CONSTRAINT dh_role_template_versions_number_check CHECK (
			(state = 'draft' AND version_number = 0 AND published_at IS NULL)
			OR (state = 'published' AND version_number > 0 AND published_at IS NOT NULL)
		),
		CONSTRAINT dh_role_template_versions_lock_check CHECK (lock_version > 0)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_dh_role_template_versions_one_draft
		ON public.digital_human_role_template_versions (role_template_id) WHERE state = 'draft';
	CREATE UNIQUE INDEX IF NOT EXISTS idx_dh_role_template_versions_number
		ON public.digital_human_role_template_versions (role_template_id, version_number) WHERE state = 'published';

	ALTER TABLE public.digital_human_role_templates
		ADD CONSTRAINT fk_dh_role_templates_draft_version
		FOREIGN KEY (draft_version_id) REFERENCES public.digital_human_role_template_versions(id) ON DELETE SET NULL;
	ALTER TABLE public.digital_human_role_templates
		ADD CONSTRAINT fk_dh_role_templates_published_version
		FOREIGN KEY (published_version_id) REFERENCES public.digital_human_role_template_versions(id) ON DELETE SET NULL;

	CREATE TABLE IF NOT EXISTS public.digital_human_role_assignments (
		id uuid PRIMARY KEY DEFAULT public.uuid_generate_v4(),
		organization_id uuid NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
		workspace_id uuid NOT NULL REFERENCES public.workspaces(id) ON DELETE CASCADE,
		digital_human_id uuid NOT NULL REFERENCES public.digital_humans(id) ON DELETE RESTRICT,
		digital_human_version_id uuid NOT NULL REFERENCES public.digital_human_versions(id) ON DELETE RESTRICT,
		role_template_id uuid NOT NULL REFERENCES public.digital_human_role_templates(id) ON DELETE RESTRICT,
		role_template_version_id uuid NOT NULL REFERENCES public.digital_human_role_template_versions(id) ON DELETE RESTRICT,
		parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
		override_policy jsonb NOT NULL DEFAULT '{}'::jsonb,
		effective_from timestamptz,
		effective_to timestamptz,
		revision varchar(71) NOT NULL,
		created_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		updated_by uuid REFERENCES public.accounts(id) ON DELETE SET NULL,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		CONSTRAINT dh_role_assignments_effective_range_check CHECK (
			effective_to IS NULL OR effective_from IS NULL OR effective_to > effective_from
		)
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_dh_role_assignments_version_role
		ON public.digital_human_role_assignments (digital_human_version_id, role_template_version_id);
	CREATE INDEX IF NOT EXISTS idx_dh_role_assignments_scope
		ON public.digital_human_role_assignments (organization_id, workspace_id, digital_human_id, digital_human_version_id);

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

	CREATE OR REPLACE FUNCTION public.enforce_dh_role_template_version_scope()
	RETURNS trigger LANGUAGE plpgsql AS $$
	DECLARE parent_organization_id uuid; parent_workspace_id uuid;
	BEGIN
		SELECT organization_id, workspace_id INTO parent_organization_id, parent_workspace_id
		FROM public.digital_human_role_templates
		WHERE id = NEW.role_template_id AND deleted_at IS NULL;
		IF parent_organization_id IS NULL OR parent_organization_id <> NEW.organization_id OR parent_workspace_id <> NEW.workspace_id THEN
			RAISE EXCEPTION 'digital human role template version scope does not match its owner' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
	$$;
	CREATE TRIGGER trg_dh_role_template_version_scope
		BEFORE INSERT OR UPDATE ON public.digital_human_role_template_versions
		FOR EACH ROW EXECUTE FUNCTION public.enforce_dh_role_template_version_scope();

	CREATE OR REPLACE FUNCTION public.prevent_published_dh_role_template_version_mutation()
	RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF OLD.state = 'published' THEN
			RAISE EXCEPTION 'published digital human role template versions are immutable' USING ERRCODE = '23514';
		END IF;
		RETURN COALESCE(NEW, OLD);
	END;
	$$;
	CREATE TRIGGER trg_published_dh_role_template_version_immutable
		BEFORE UPDATE OR DELETE ON public.digital_human_role_template_versions
		FOR EACH ROW EXECUTE FUNCTION public.prevent_published_dh_role_template_version_mutation();

	CREATE OR REPLACE FUNCTION public.validate_dh_role_template_version_pointers()
	RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF NEW.draft_version_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM public.digital_human_role_template_versions version
			WHERE version.id = NEW.draft_version_id AND version.role_template_id = NEW.id
				AND version.organization_id = NEW.organization_id AND version.workspace_id = NEW.workspace_id AND version.state = 'draft'
		) THEN RAISE EXCEPTION 'digital human role template draft pointer is invalid' USING ERRCODE = '23514'; END IF;
		IF NEW.published_version_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM public.digital_human_role_template_versions version
			WHERE version.id = NEW.published_version_id AND version.role_template_id = NEW.id
				AND version.organization_id = NEW.organization_id AND version.workspace_id = NEW.workspace_id AND version.state = 'published'
		) THEN RAISE EXCEPTION 'digital human role template published pointer is invalid' USING ERRCODE = '23514'; END IF;
		RETURN NEW;
	END;
	$$;
	CREATE TRIGGER trg_dh_role_template_version_pointers
		BEFORE INSERT OR UPDATE OF draft_version_id, published_version_id ON public.digital_human_role_templates
		FOR EACH ROW EXECUTE FUNCTION public.validate_dh_role_template_version_pointers();

	CREATE OR REPLACE FUNCTION public.enforce_dh_assignment_scope_and_immutability()
	RETURNS trigger LANGUAGE plpgsql AS $$
	DECLARE owner_state varchar(16);
	BEGIN
		IF TG_OP IN ('UPDATE', 'DELETE') THEN
			SELECT state INTO owner_state FROM public.digital_human_versions WHERE id = OLD.digital_human_version_id;
			IF owner_state = 'published' THEN
				RAISE EXCEPTION 'published digital human role assignments are immutable' USING ERRCODE = '23514';
			END IF;
		END IF;
		IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
		SELECT state INTO owner_state FROM public.digital_human_versions WHERE id = NEW.digital_human_version_id;
		IF owner_state <> 'draft' THEN
			RAISE EXCEPTION 'new digital human role assignments require a draft owner' USING ERRCODE = '23514';
		END IF;
		IF NOT EXISTS (
			SELECT 1 FROM public.digital_human_versions version
			WHERE version.id = NEW.digital_human_version_id AND version.digital_human_id = NEW.digital_human_id
				AND version.organization_id = NEW.organization_id AND version.workspace_id = NEW.workspace_id
		) OR NOT EXISTS (
			SELECT 1 FROM public.digital_human_role_template_versions version
			WHERE version.id = NEW.role_template_version_id AND version.role_template_id = NEW.role_template_id
				AND version.organization_id = NEW.organization_id AND version.workspace_id = NEW.workspace_id AND version.state = 'published'
		) THEN RAISE EXCEPTION 'digital human role assignment scope is invalid' USING ERRCODE = '23514'; END IF;
		RETURN NEW;
	END;
	$$;
	CREATE TRIGGER trg_dh_assignment_scope_and_immutability
		BEFORE INSERT OR UPDATE OR DELETE ON public.digital_human_role_assignments
		FOR EACH ROW EXECUTE FUNCTION public.enforce_dh_assignment_scope_and_immutability();

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
	return schema.Raw(createDigitalHumanRolesAndBindingsSQL)
}

func down202608181129156409(schema *mschema.Builder) error {
	if err := schema.Raw(`
		DROP TRIGGER IF EXISTS trg_dh_binding_scope_and_immutability ON public.digital_human_resource_bindings;
		DROP TRIGGER IF EXISTS trg_dh_assignment_scope_and_immutability ON public.digital_human_role_assignments;
		DROP TRIGGER IF EXISTS trg_dh_role_template_version_pointers ON public.digital_human_role_templates;
		DROP TRIGGER IF EXISTS trg_published_dh_role_template_version_immutable ON public.digital_human_role_template_versions;
		DROP TRIGGER IF EXISTS trg_dh_role_template_version_scope ON public.digital_human_role_template_versions;
		ALTER TABLE IF EXISTS public.digital_human_role_templates
			DROP CONSTRAINT IF EXISTS fk_dh_role_templates_draft_version,
			DROP CONSTRAINT IF EXISTS fk_dh_role_templates_published_version;
		DROP FUNCTION IF EXISTS public.enforce_dh_binding_scope_and_immutability();
		DROP FUNCTION IF EXISTS public.enforce_dh_assignment_scope_and_immutability();
		DROP FUNCTION IF EXISTS public.validate_dh_role_template_version_pointers();
		DROP FUNCTION IF EXISTS public.prevent_published_dh_role_template_version_mutation();
		DROP FUNCTION IF EXISTS public.enforce_dh_role_template_version_scope()
	`); err != nil {
		return err
	}
	for _, table := range []string{
		"digital_human_resource_bindings",
		"digital_human_role_assignments",
		"digital_human_role_template_versions",
		"digital_human_role_templates",
	} {
		if err := schema.DropIfExists(table); err != nil {
			return err
		}
	}
	return nil
}
