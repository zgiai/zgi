package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationRemoveRuntimeManagedChatSkillConfigsID = "20260707090000_remove_runtime_managed_chat_skill_configs"

func init() {
	registerSchemaMigration(
		migrationRemoveRuntimeManagedChatSkillConfigsID,
		upRemoveRuntimeManagedChatSkillConfigs,
		nil,
	)
}

func upRemoveRuntimeManagedChatSkillConfigs(schema *mschema.Builder) error {
	return schema.DataFix("remove runtime-managed chat skill config rows", func(db *gorm.DB) error {
		return db.Exec(`
			DELETE FROM public.chat_runtime_organization_skill_configs
			WHERE skill_id IN (
				'agent-database',
				'agent-knowledge',
				'agent-management',
				'agent-memory',
				'agent-workflow',
				'console-navigator',
				'file-manager',
				'intent-router',
				'user-memory'
			)
		`).Error
	})
}
