package migrations

import (
	"encoding/json"
	"fmt"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

const migrationRepairWorkspaceRoleTemplateLocalizationID = "20260809130000_repair_workspace_role_template_localization"

const repairWorkspaceRoleTemplateNameLocalizationSQL = `
UPDATE public.roles
SET
	name_i18n = ?::jsonb,
	name_customized = false
WHERE template_origin = 'system_default'
  AND system_key = ?
  AND BTRIM(name) IN (BTRIM(?), BTRIM(?))
`

const repairWorkspaceRoleTemplateDescriptionLocalizationSQL = `
UPDATE public.roles
SET
	description_i18n = ?::jsonb,
	description_customized = false
WHERE template_origin = 'system_default'
  AND system_key = ?
  AND BTRIM(COALESCE(description, '')) IN (BTRIM(?), BTRIM(?))
`

func init() {
	registerSchemaMigration(
		migrationRepairWorkspaceRoleTemplateLocalizationID,
		upRepairWorkspaceRoleTemplateLocalization,
		nil,
	)
}

func upRepairWorkspaceRoleTemplateLocalization(schema *mschema.Builder) error {
	return schema.DataFix("repair workspace role template localization state", func(db *gorm.DB) error {
		for _, definition := range workspace_model.DefaultWorkspaceRoleTemplateDefinitions() {
			nameI18n, err := json.Marshal(map[string]string{
				"zh_Hans": definition.NameZhHans,
				"en_US":   definition.NameEnUS,
			})
			if err != nil {
				return fmt.Errorf("encode default workspace role name localization for %s: %w", definition.SystemKey, err)
			}
			if err := db.Exec(
				repairWorkspaceRoleTemplateNameLocalizationSQL,
				string(nameI18n),
				definition.SystemKey,
				definition.NameZhHans,
				definition.NameEnUS,
			).Error; err != nil {
				return fmt.Errorf("repair default workspace role name localization for %s: %w", definition.SystemKey, err)
			}

			descriptionI18n, err := json.Marshal(map[string]string{
				"zh_Hans": definition.DescZhHans,
				"en_US":   definition.DescEnUS,
			})
			if err != nil {
				return fmt.Errorf("encode default workspace role description localization for %s: %w", definition.SystemKey, err)
			}
			if err := db.Exec(
				repairWorkspaceRoleTemplateDescriptionLocalizationSQL,
				string(descriptionI18n),
				definition.SystemKey,
				definition.DescZhHans,
				definition.DescEnUS,
			).Error; err != nil {
				return fmt.Errorf("repair default workspace role description localization for %s: %w", definition.SystemKey, err)
			}
		}
		return nil
	})
}
