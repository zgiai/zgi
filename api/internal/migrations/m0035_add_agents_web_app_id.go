package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0035_add_agents_web_app_id adds web_app_id column to agents table
// This column is used to uniquely identify web applications for each agent
func M0035_add_agents_web_app_id() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000035",
		Migrate: func(tx *gorm.DB) error {
			// Add web_app_id column to agents table
			if err := tx.Exec(`
				ALTER TABLE agents
				ADD COLUMN IF NOT EXISTS web_app_id UUID NOT NULL DEFAULT uuid_generate_v4()
			`).Error; err != nil {
				return err
			}

			// Create unique index on web_app_id
			if err := tx.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_web_app_id ON agents(web_app_id)
			`).Error; err != nil {
				return err
			}

			// Add web_app_id column to agents_conversations table
			if err := tx.Exec(`
				ALTER TABLE agents_conversations
				ADD COLUMN IF NOT EXISTS web_app_id UUID
			`).Error; err != nil {
				return err
			}

			// Create index on web_app_id for agents_conversations
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_conversations_web_app_id ON agents_conversations(web_app_id)
			`).Error; err != nil {
				return err
			}

			// Add web_app_id column to agents_messages table
			if err := tx.Exec(`
				ALTER TABLE agents_messages
				ADD COLUMN IF NOT EXISTS web_app_id UUID
			`).Error; err != nil {
				return err
			}

			// Create index on web_app_id for agents_messages
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_messages_web_app_id ON agents_messages(web_app_id)
			`).Error; err != nil {
				return err
			}

			// Add web_app_id column to workflow_run_logs table
			if err := tx.Exec(`
				ALTER TABLE workflow_run_logs
				ADD COLUMN IF NOT EXISTS web_app_id UUID
			`).Error; err != nil {
				return err
			}

			// Add web_app_id column to workflow_node_runtime_logs table
			if err := tx.Exec(`
				ALTER TABLE workflow_node_runtime_logs
				ADD COLUMN IF NOT EXISTS web_app_id UUID
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			sqls := []string{
				`DROP INDEX IF EXISTS idx_agents_web_app_id`,
				`ALTER TABLE agents DROP COLUMN IF EXISTS web_app_id`,
				`DROP INDEX IF EXISTS idx_agents_conversations_web_app_id`,
				`ALTER TABLE agents_conversations DROP COLUMN IF EXISTS web_app_id`,
				`DROP INDEX IF EXISTS idx_agents_messages_web_app_id`,
				`ALTER TABLE agents_messages DROP COLUMN IF EXISTS web_app_id`,
				`ALTER TABLE workflow_run_logs DROP COLUMN IF EXISTS web_app_id`,
				`ALTER TABLE workflow_node_runtime_logs DROP COLUMN IF EXISTS web_app_id`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
