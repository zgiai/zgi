package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationHardenWorkspaceRoleTemplateLifecycleID = "20260809124500_harden_workspace_role_template_lifecycle"

const addWorkspaceRoleCustomizationStateSQL = `
ALTER TABLE public.roles
	ADD COLUMN IF NOT EXISTS name_customized boolean DEFAULT false NOT NULL,
	ADD COLUMN IF NOT EXISTS description_customized boolean DEFAULT false NOT NULL
`

const backfillWorkspaceRoleCustomizationStateSQL = `
UPDATE public.roles
SET
	name_customized = template_origin = 'custom'
		OR name_i18n = '{}'::jsonb
		OR NOT EXISTS (
			SELECT 1
			FROM jsonb_each_text(name_i18n) AS localized(key, value)
			WHERE BTRIM(localized.value) = BTRIM(name)
		),
	description_customized = template_origin = 'custom'
		OR description_i18n = '{}'::jsonb
		OR NOT EXISTS (
			SELECT 1
			FROM jsonb_each_text(description_i18n) AS localized(key, value)
			WHERE BTRIM(localized.value) = BTRIM(COALESCE(description, ''))
		);

UPDATE public.roles
SET name_i18n = '{}'::jsonb
WHERE name_customized;

UPDATE public.roles
SET description_i18n = '{}'::jsonb
WHERE description_customized
`

const enforceActiveWorkspaceRoleTemplateAssignmentFunctionSQL = `
CREATE OR REPLACE FUNCTION public.enforce_active_workspace_role_template_assignment()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
	referenced_role_id uuid;
BEGIN
	IF TG_OP = 'UPDATE' THEN
		IF NEW.workspace_id IS NOT DISTINCT FROM OLD.workspace_id
		   AND NEW.role_id IS NOT DISTINCT FROM OLD.role_id
		   AND NEW.permission_template_role_id IS NOT DISTINCT FROM OLD.permission_template_role_id THEN
			RETURN NEW;
		END IF;
	END IF;

	FOREACH referenced_role_id IN ARRAY ARRAY[NEW.role_id, NEW.permission_template_role_id]
	LOOP
		IF referenced_role_id IS NULL OR referenced_role_id = ANY(ARRAY[
			'00000000-0000-0000-0000-000000000001'::uuid,
			'00000000-0000-0000-0000-000000000002'::uuid,
			'00000000-0000-0000-0000-000000000003'::uuid,
			'00000000-0000-0000-0000-000000000004'::uuid
		]) THEN
			CONTINUE;
		END IF;

		PERFORM 1
		FROM public.roles AS role_template
		JOIN public.workspaces AS workspace
			ON workspace.organization_id = role_template.group_id
		WHERE workspace.id = NEW.workspace_id
		  AND role_template.id = referenced_role_id
		  AND role_template.status = 'active'
		FOR SHARE OF role_template;

		IF NOT FOUND THEN
			RAISE EXCEPTION 'workspace role template is unavailable'
				USING ERRCODE = '23503';
		END IF;
	END LOOP;

	RETURN NEW;
END
$$
`

const enforceActiveWorkspaceRoleTemplateAssignmentTriggerSQL = `
DROP TRIGGER IF EXISTS workspace_members_active_role_template_guard
	ON public.workspace_members;
CREATE TRIGGER workspace_members_active_role_template_guard
	BEFORE INSERT OR UPDATE OF workspace_id, role_id, permission_template_role_id
	ON public.workspace_members
	FOR EACH ROW
	EXECUTE FUNCTION public.enforce_active_workspace_role_template_assignment()
`

const enforceActiveWorkspaceRoleTemplateAssignmentSQL = enforceActiveWorkspaceRoleTemplateAssignmentFunctionSQL + ";\n" + enforceActiveWorkspaceRoleTemplateAssignmentTriggerSQL

func init() {
	registerSchemaMigration(
		migrationHardenWorkspaceRoleTemplateLifecycleID,
		upHardenWorkspaceRoleTemplateLifecycle,
		downHardenWorkspaceRoleTemplateLifecycle,
	)
}

func upHardenWorkspaceRoleTemplateLifecycle(schema *mschema.Builder) error {
	if err := schema.Raw(addWorkspaceRoleCustomizationStateSQL); err != nil {
		return err
	}
	if err := schema.DataFix("backfill workspace role template customization state", func(db *gorm.DB) error {
		return db.Exec(backfillWorkspaceRoleCustomizationStateSQL).Error
	}); err != nil {
		return err
	}
	return schema.Raw(enforceActiveWorkspaceRoleTemplateAssignmentSQL)
}

func downHardenWorkspaceRoleTemplateLifecycle(schema *mschema.Builder) error {
	return schema.Raw(`
		DROP TRIGGER IF EXISTS workspace_members_active_role_template_guard
			ON public.workspace_members;
		DROP FUNCTION IF EXISTS public.enforce_active_workspace_role_template_assignment()
	`)
}
