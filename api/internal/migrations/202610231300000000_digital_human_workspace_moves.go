package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

// DigitalHumans can move to another workspace of their organization. Their
// rows are guarded by scope and immutability triggers, so a move sets
// zgi.digital_human_workspace_move to the moving DigitalHuman for its
// transaction; the guards then accept an update that changes nothing but the
// workspace, and the removal of the bindings the move detaches. Everything
// else stays rejected. identity_workspace_id keeps the workspace end-user
// identities were derived in, so they survive the move.
func init() {
	registerSchemaMigration("202610231300000000_digital_human_workspace_moves", upDigitalHumanWorkspaceMoves, downDigitalHumanWorkspaceMoves)
}

const digitalHumanWorkspaceMoveSQL = `
ALTER TABLE public.digital_humans ADD COLUMN IF NOT EXISTS identity_workspace_id uuid;

CREATE OR REPLACE FUNCTION public.digital_human_workspace_move_allows(moving uuid, old_row jsonb, new_row jsonb)
RETURNS boolean LANGUAGE sql STABLE AS $$
	SELECT moving IS NOT NULL
		AND current_setting('zgi.digital_human_workspace_move', true) = moving::text
		AND (old_row - 'workspace_id' - 'updated_at') = (new_row - 'workspace_id' - 'updated_at');
$$;

CREATE OR REPLACE FUNCTION public.digital_human_memory_event_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.digital_human_id, to_jsonb(OLD), to_jsonb(NEW)) THEN
			RETURN NEW;
		END IF;
		RAISE EXCEPTION 'digital human memory events are immutable' USING ERRCODE = '23514';
	END;
$$;

CREATE OR REPLACE FUNCTION public.prevent_published_digital_human_version_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF OLD.state = 'published' THEN
			IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.digital_human_id, to_jsonb(OLD), to_jsonb(NEW)) THEN
				RETURN NEW;
			END IF;
			RAISE EXCEPTION 'published digital human versions are immutable' USING ERRCODE = '23514';
		END IF;
		RETURN COALESCE(NEW, OLD);
	END;
$$;

CREATE OR REPLACE FUNCTION public.enforce_dh_binding_scope_and_immutability() RETURNS trigger LANGUAGE plpgsql AS $$
	DECLARE owner_state varchar(16);
	BEGIN
		IF TG_OP = 'DELETE' AND current_setting('zgi.digital_human_workspace_move', true) = OLD.digital_human_id::text THEN
			RETURN OLD;
		END IF;
		IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.digital_human_id, to_jsonb(OLD), to_jsonb(NEW)) THEN
			IF NOT EXISTS (
				SELECT 1 FROM public.digital_human_versions version
				WHERE version.id = NEW.digital_human_version_id AND version.digital_human_id = NEW.digital_human_id
					AND version.organization_id = NEW.organization_id AND version.workspace_id = NEW.workspace_id
			) THEN RAISE EXCEPTION 'digital human resource binding scope is invalid' USING ERRCODE = '23514'; END IF;
			RETURN NEW;
		END IF;
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

CREATE OR REPLACE FUNCTION public.enforce_digital_human_memory_supersedes_scope() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		-- A move rewrites every memory of the DigitalHuman, so a superseded
		-- row may not have moved yet when its successor is checked.
		IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.digital_human_id, to_jsonb(OLD), to_jsonb(NEW)) THEN
			RETURN NEW;
		END IF;
		IF NEW.supersedes_id IS NOT NULL AND NOT EXISTS (
			SELECT 1
			FROM public.digital_human_memories prior
			WHERE prior.id = NEW.supersedes_id
			  AND prior.id <> NEW.id
			  AND prior.organization_id = NEW.organization_id
			  AND prior.workspace_id = NEW.workspace_id
			  AND prior.digital_human_id = NEW.digital_human_id
			  AND prior.memory_class = NEW.memory_class
			  AND prior.scope_type = NEW.scope_type
			  AND prior.scope_id = NEW.scope_id
		) THEN
			RAISE EXCEPTION 'digital human superseded memory scope mismatch' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
$$;

CREATE OR REPLACE FUNCTION public.validate_digital_human_version_pointers() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		-- A move rewrites the DigitalHuman before its versions follow.
		IF TG_OP = 'UPDATE' AND public.digital_human_workspace_move_allows(OLD.id, to_jsonb(OLD), to_jsonb(NEW)) THEN
			RETURN NEW;
		END IF;
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

CREATE OR REPLACE FUNCTION public.preserve_execution_upload_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF OLD.application_type = 'digital_human' THEN
		IF public.digital_human_workspace_move_allows(CAST(OLD.application_id AS uuid), to_jsonb(OLD), to_jsonb(NEW)) THEN
			RETURN NEW;
		END IF;
	END IF;
	IF OLD IS DISTINCT FROM NEW THEN
		RAISE EXCEPTION 'execution upload ownership is immutable';
	END IF;
	RETURN NEW;
END
$$;
`

const digitalHumanWorkspaceMoveDownSQL = `
CREATE OR REPLACE FUNCTION public.digital_human_memory_event_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		RAISE EXCEPTION 'digital human memory events are immutable' USING ERRCODE = '23514';
	END;
$$;

CREATE OR REPLACE FUNCTION public.prevent_published_digital_human_version_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF OLD.state = 'published' THEN
			RAISE EXCEPTION 'published digital human versions are immutable' USING ERRCODE = '23514';
		END IF;
		RETURN COALESCE(NEW, OLD);
	END;
$$;

CREATE OR REPLACE FUNCTION public.enforce_dh_binding_scope_and_immutability() RETURNS trigger LANGUAGE plpgsql AS $$
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

CREATE OR REPLACE FUNCTION public.enforce_digital_human_memory_supersedes_scope() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN
		IF NEW.supersedes_id IS NOT NULL AND NOT EXISTS (
			SELECT 1
			FROM public.digital_human_memories prior
			WHERE prior.id = NEW.supersedes_id
			  AND prior.id <> NEW.id
			  AND prior.organization_id = NEW.organization_id
			  AND prior.workspace_id = NEW.workspace_id
			  AND prior.digital_human_id = NEW.digital_human_id
			  AND prior.memory_class = NEW.memory_class
			  AND prior.scope_type = NEW.scope_type
			  AND prior.scope_id = NEW.scope_id
		) THEN
			RAISE EXCEPTION 'digital human superseded memory scope mismatch' USING ERRCODE = '23514';
		END IF;
		RETURN NEW;
	END;
$$;

CREATE OR REPLACE FUNCTION public.validate_digital_human_version_pointers() RETURNS trigger LANGUAGE plpgsql AS $$
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

CREATE OR REPLACE FUNCTION public.preserve_execution_upload_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF OLD IS DISTINCT FROM NEW THEN
		RAISE EXCEPTION 'execution upload ownership is immutable';
	END IF;
	RETURN NEW;
END
$$;

DROP FUNCTION IF EXISTS public.digital_human_workspace_move_allows(uuid, jsonb, jsonb);
ALTER TABLE public.digital_humans DROP COLUMN IF EXISTS identity_workspace_id;
`

func upDigitalHumanWorkspaceMoves(schema *mschema.Builder) error {
	return schema.Raw(digitalHumanWorkspaceMoveSQL)
}

func downDigitalHumanWorkspaceMoves(schema *mschema.Builder) error {
	return schema.Raw(digitalHumanWorkspaceMoveDownSQL)
}
