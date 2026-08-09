package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationFixWorkspaceRoleTemplateLifecycleID = "20260809123000_fix_workspace_role_template_lifecycle"

const dropWorkspaceRoleNameConstraintSQL = `
ALTER TABLE public.roles
	DROP CONSTRAINT IF EXISTS uk_roles_group_name
`

const dropWorkspaceRoleNameIndexSQL = `DROP INDEX IF EXISTS public.uk_roles_group_name`

const addActiveWorkspaceRoleNameIndexSQL = `
CREATE UNIQUE INDEX uk_roles_group_name
ON public.roles (group_id, name)
WHERE status != 'deleted'
`

const dropWorkspaceRoleSystemKeyIndexSQL = `DROP INDEX IF EXISTS public.uk_roles_group_system_key`

const addActiveWorkspaceRoleSystemKeyIndexSQL = `
CREATE UNIQUE INDEX uk_roles_group_system_key
ON public.roles (group_id, system_key)
WHERE system_key IS NOT NULL AND status != 'deleted'
`

const clearStaleWorkspaceRoleNameI18nSQL = `
UPDATE public.roles AS role
SET name_i18n = '{}'::jsonb
FROM public.accounts AS creator
WHERE creator.id = role.created_by
  AND role.name_i18n != '{}'::jsonb
  AND BTRIM(role.name) != BTRIM(COALESCE(
	role.name_i18n ->> CASE
		WHEN LOWER(BTRIM(COALESCE(creator.interface_language, ''))) LIKE 'en%' THEN 'en_US'
		ELSE 'zh_Hans'
	END,
	''
  ))
`

const clearStaleWorkspaceRoleDescriptionI18nSQL = `
UPDATE public.roles AS role
SET description_i18n = '{}'::jsonb
FROM public.accounts AS creator
WHERE creator.id = role.created_by
  AND role.description_i18n != '{}'::jsonb
  AND BTRIM(COALESCE(role.description, '')) != BTRIM(COALESCE(
	role.description_i18n ->> CASE
		WHEN LOWER(BTRIM(COALESCE(creator.interface_language, ''))) LIKE 'en%' THEN 'en_US'
		ELSE 'zh_Hans'
	END,
	''
  ))
`

func init() {
	registerSchemaMigration(
		migrationFixWorkspaceRoleTemplateLifecycleID,
		upFixWorkspaceRoleTemplateLifecycle,
		nil,
	)
}

func upFixWorkspaceRoleTemplateLifecycle(schema *mschema.Builder) error {
	for _, statement := range []string{
		dropWorkspaceRoleNameConstraintSQL,
		dropWorkspaceRoleNameIndexSQL,
		addActiveWorkspaceRoleNameIndexSQL,
		dropWorkspaceRoleSystemKeyIndexSQL,
		addActiveWorkspaceRoleSystemKeyIndexSQL,
	} {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	if err := schema.DataFix("clear stale workspace role template localization", func(db *gorm.DB) error {
		for _, statement := range []string{
			clearStaleWorkspaceRoleNameI18nSQL,
			clearStaleWorkspaceRoleDescriptionI18nSQL,
		} {
			if err := db.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return seedWorkspaceDefaultRoleTemplates(schema)
}
