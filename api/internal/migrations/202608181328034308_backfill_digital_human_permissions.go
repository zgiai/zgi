package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migration202608181328034308ID = "202608181328034308_backfill_digital_human_permissions"

const backfillDigitalHumanPermissionsSQL = `
WITH target_roles AS (
	SELECT
		id,
		CASE system_key
			WHEN 'default_advanced' THEN '["digital_human.view","digital_human.create","digital_human.update","digital_human.delete","digital_human.publish","digital_human.run","digital_human.logs.view","digital_human.memory.view","digital_human.memory.manage"]'::jsonb
			WHEN 'default_basic' THEN '["digital_human.view","digital_human.create","digital_human.update","digital_human.run","digital_human.logs.view","digital_human.memory.view","digital_human.memory.manage"]'::jsonb
			WHEN 'default_readonly' THEN '["digital_human.view","digital_human.logs.view","digital_human.memory.view"]'::jsonb
		END AS additions
	FROM public.roles
	WHERE template_origin = 'system_default'
		AND system_key IN ('default_advanced', 'default_basic', 'default_readonly')
)
UPDATE public.roles AS role
SET permissions = (
		SELECT COALESCE(jsonb_agg(DISTINCT expanded.permission ORDER BY expanded.permission), '[]'::jsonb)
		FROM jsonb_array_elements_text(COALESCE(role.permissions, '[]'::jsonb) || target.additions) AS expanded(permission)
	),
	updated_at = CURRENT_TIMESTAMP
FROM target_roles AS target
WHERE role.id = target.id;

WITH target_members AS (
	SELECT
		member.id,
		CASE
			WHEN member.role_id = '00000000-0000-0000-0000-000000000002'::uuid
				THEN '["digital_human.view","digital_human.create","digital_human.update","digital_human.delete","digital_human.publish","digital_human.run","digital_human.logs.view","digital_human.memory.view","digital_human.memory.manage"]'::jsonb
			WHEN member.role_id = '00000000-0000-0000-0000-000000000003'::uuid
				THEN '["digital_human.view","digital_human.run","digital_human.logs.view","digital_human.memory.view"]'::jsonb
			WHEN member.role_id = '00000000-0000-0000-0000-000000000004'::uuid
				THEN '["digital_human.view","digital_human.logs.view","digital_human.memory.view"]'::jsonb
			WHEN template.system_key = 'default_advanced'
				THEN '["digital_human.view","digital_human.create","digital_human.update","digital_human.delete","digital_human.publish","digital_human.run","digital_human.logs.view","digital_human.memory.view","digital_human.memory.manage"]'::jsonb
			WHEN template.system_key = 'default_basic'
				THEN '["digital_human.view","digital_human.create","digital_human.update","digital_human.run","digital_human.logs.view","digital_human.memory.view","digital_human.memory.manage"]'::jsonb
			WHEN template.system_key = 'default_readonly'
				THEN '["digital_human.view","digital_human.logs.view","digital_human.memory.view"]'::jsonb
		END AS additions
	FROM public.workspace_members AS member
	LEFT JOIN public.roles AS template
		ON template.id = member.permission_template_role_id
		AND template.template_origin = 'system_default'
	WHERE member.permission_source IN ('role_template', 'legacy_role')
		AND (
			member.role_id IN (
				'00000000-0000-0000-0000-000000000002'::uuid,
				'00000000-0000-0000-0000-000000000003'::uuid,
				'00000000-0000-0000-0000-000000000004'::uuid
			)
			OR template.system_key IN ('default_advanced', 'default_basic', 'default_readonly')
		)
)
UPDATE public.workspace_members AS member
SET permissions = (
		SELECT COALESCE(jsonb_agg(DISTINCT expanded.permission ORDER BY expanded.permission), '[]'::jsonb)
		FROM jsonb_array_elements_text(COALESCE(member.permissions, '[]'::jsonb) || target.additions) AS expanded(permission)
	),
	updated_at = CURRENT_TIMESTAMP
FROM target_members AS target
WHERE member.id = target.id
`

const removeBackfilledDigitalHumanPermissionsSQL = `
UPDATE public.roles AS role
SET permissions = (
		SELECT COALESCE(jsonb_agg(expanded.permission ORDER BY expanded.permission), '[]'::jsonb)
		FROM jsonb_array_elements_text(COALESCE(role.permissions, '[]'::jsonb)) AS expanded(permission)
		WHERE expanded.permission NOT LIKE 'digital_human.%'
	),
	updated_at = CURRENT_TIMESTAMP
WHERE role.template_origin = 'system_default'
	AND role.system_key IN ('default_advanced', 'default_basic', 'default_readonly');

UPDATE public.workspace_members AS member
SET permissions = (
		SELECT COALESCE(jsonb_agg(expanded.permission ORDER BY expanded.permission), '[]'::jsonb)
		FROM jsonb_array_elements_text(COALESCE(member.permissions, '[]'::jsonb)) AS expanded(permission)
		WHERE expanded.permission NOT LIKE 'digital_human.%'
	),
	updated_at = CURRENT_TIMESTAMP
WHERE member.permission_source IN ('role_template', 'legacy_role')
	AND (
		member.role_id IN (
			'00000000-0000-0000-0000-000000000002'::uuid,
			'00000000-0000-0000-0000-000000000003'::uuid,
			'00000000-0000-0000-0000-000000000004'::uuid
		)
		OR EXISTS (
			SELECT 1
			FROM public.roles AS template
			WHERE template.id = member.permission_template_role_id
				AND template.template_origin = 'system_default'
				AND template.system_key IN ('default_advanced', 'default_basic', 'default_readonly')
		)
	)
`

func init() {
	registerSchemaMigration(
		migration202608181328034308ID,
		up202608181328034308,
		down202608181328034308,
	)
}

func up202608181328034308(schema *mschema.Builder) error {
	return schema.DataFix("backfill DigitalHuman permissions into system role snapshots", func(db *gorm.DB) error {
		return db.Exec(backfillDigitalHumanPermissionsSQL).Error
	})
}

func down202608181328034308(schema *mschema.Builder) error {
	return schema.DataFix("remove backfilled DigitalHuman permissions from system role snapshots", func(db *gorm.DB) error {
		return db.Exec(removeBackfilledDigitalHumanPermissionsSQL).Error
	})
}
