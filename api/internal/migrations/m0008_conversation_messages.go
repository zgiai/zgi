package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0008_conversation_messages creates conversation and message related tables
func M0008_conversation_messages() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100700",
		Migrate: func(tx *gorm.DB) error {
			// Create conversation_group table
			if err := tx.Exec(`
				CREATE TABLE conversation_group (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id VARCHAR(36) NOT NULL,
					group_id VARCHAR(36) NOT NULL,
					conversation_id VARCHAR(36),
					from_account_id VARCHAR(36) NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					name VARCHAR(255) NOT NULL DEFAULT '',
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create conversations table
			if err := tx.Exec(`
				CREATE TABLE conversations (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id UUID NOT NULL,
					app_model_config_id UUID,
					model_provider VARCHAR(255),
					override_model_configs TEXT,
					model_id VARCHAR(255),
					mode VARCHAR(255) NOT NULL,
					name VARCHAR(255) NOT NULL,
					summary TEXT,
					inputs JSON NOT NULL,
					introduction TEXT,
					system_instruction TEXT,
					system_instruction_tokens INTEGER NOT NULL DEFAULT 0,
					status VARCHAR(255) NOT NULL,
					from_source VARCHAR(255) NOT NULL,
					from_end_user_id UUID,
					from_account_id UUID,
					read_at TIMESTAMPTZ,
					read_account_id UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					is_deleted BOOLEAN NOT NULL DEFAULT false,
					invoke_from VARCHAR(255),
					dialogue_count INTEGER NOT NULL DEFAULT 0,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create message_agent_thoughts table
			if err := tx.Exec(`
				CREATE TABLE message_agent_thoughts (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					message_id UUID NOT NULL,
					message_chain_id UUID,
					position INTEGER NOT NULL,
					thought TEXT,
					tool TEXT,
					tool_input TEXT,
					observation TEXT,
					tool_process_data TEXT,
					message TEXT,
					message_token INTEGER,
					message_unit_price NUMERIC,
					answer TEXT,
					answer_token INTEGER,
					answer_unit_price NUMERIC,
					tokens INTEGER,
					total_price NUMERIC,
					currency VARCHAR,
					latency FLOAT8,
					created_by_role VARCHAR NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					message_price_unit NUMERIC(10,7) NOT NULL DEFAULT 0.001,
					answer_price_unit NUMERIC(10,7) NOT NULL DEFAULT 0.001,
					message_files TEXT,
					tool_labels_str TEXT NOT NULL DEFAULT '{}',
					tool_meta_str TEXT NOT NULL DEFAULT '{}',
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create message_annotations table
			if err := tx.Exec(`
				CREATE TABLE message_annotations (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id UUID NOT NULL,
					conversation_id UUID,
					message_id UUID,
					content TEXT NOT NULL,
					account_id UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					question TEXT,
					hit_count INTEGER NOT NULL DEFAULT 0,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create message_feedbacks table
			if err := tx.Exec(`
				CREATE TABLE message_feedbacks (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id UUID NOT NULL,
					conversation_id UUID NOT NULL,
					message_id UUID NOT NULL,
					rating VARCHAR(255) NOT NULL,
					content TEXT,
					from_source VARCHAR(255) NOT NULL,
					from_end_user_id UUID,
					from_account_id UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create message_files table
			if err := tx.Exec(`
				CREATE TABLE message_files (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					message_id UUID NOT NULL,
					type VARCHAR(255) NOT NULL,
					transfer_method VARCHAR(255) NOT NULL,
					url TEXT,
					upload_file_id UUID,
					created_by_role VARCHAR(255) NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					belongs_to VARCHAR(255),
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create messages table
			if err := tx.Exec(`
				CREATE TABLE messages (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id UUID NOT NULL,
					model_provider VARCHAR(255),
					model_id VARCHAR(255),
					override_model_configs TEXT,
					conversation_id UUID NOT NULL,
					inputs JSON NOT NULL,
					query TEXT NOT NULL,
					message JSON NOT NULL,
					message_tokens INTEGER NOT NULL DEFAULT 0,
					message_unit_price NUMERIC(10,4) NOT NULL,
					answer TEXT NOT NULL,
					answer_tokens INTEGER NOT NULL DEFAULT 0,
					answer_unit_price NUMERIC(10,4) NOT NULL,
					provider_response_latency FLOAT8 NOT NULL DEFAULT 0,
					total_price NUMERIC(10,7),
					currency VARCHAR(255) NOT NULL,
					from_source VARCHAR(255) NOT NULL,
					from_end_user_id UUID,
					from_account_id UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					agent_based BOOLEAN NOT NULL DEFAULT false,
					message_price_unit NUMERIC(10,7) NOT NULL DEFAULT 0.001,
					answer_price_unit NUMERIC(10,7) NOT NULL DEFAULT 0.001,
					workflow_run_id UUID,
					status VARCHAR(255) NOT NULL DEFAULT 'normal',
					error TEXT,
					message_metadata TEXT,
					invoke_from VARCHAR(255),
					parent_message_id UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes
			indexes := []string{
				`CREATE INDEX conversation_group_app_id_idx ON conversation_group(app_id)`,
				`CREATE INDEX conversation_app_id_idx ON conversations(app_id)`,
				`CREATE INDEX conversation_app_model_config_id_idx ON conversations(app_model_config_id)`,
				`CREATE INDEX message_agent_thoughts_message_id_idx ON message_agent_thoughts(message_id)`,
				`CREATE INDEX message_agent_thoughts_message_chain_id_idx ON message_agent_thoughts(message_chain_id)`,
				`CREATE INDEX message_annotations_app_id_idx ON message_annotations(app_id)`,
				`CREATE INDEX message_annotations_conversation_id_idx ON message_annotations(conversation_id)`,
				`CREATE INDEX message_annotations_message_id_idx ON message_annotations(message_id)`,
				`CREATE INDEX message_feedbacks_app_id_idx ON message_feedbacks(app_id)`,
				`CREATE INDEX message_feedbacks_conversation_id_idx ON message_feedbacks(conversation_id)`,
				`CREATE INDEX message_feedbacks_message_id_idx ON message_feedbacks(message_id)`,
				`CREATE INDEX message_files_message_id_idx ON message_files(message_id)`,
				`CREATE INDEX message_files_created_by_idx ON message_files(created_by)`,
				`CREATE INDEX messages_app_id_idx ON messages(app_id)`,
				`CREATE INDEX messages_conversation_id_idx ON messages(conversation_id)`,
				`CREATE INDEX messages_created_at_idx ON messages(created_at)`,
				`CREATE INDEX messages_end_user_idx ON messages(app_id, from_source, from_end_user_id)`,
				`CREATE INDEX messages_workflow_run_id_idx ON messages(conversation_id, workflow_run_id)`,
			}

			for _, indexSQL := range indexes {
				if err := tx.Exec(indexSQL).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(
				"conversation_group",
				"conversations",
				"message_agent_thoughts",
				"message_annotations",
				"message_feedbacks",
				"message_files",
				"messages",
			)
		},
	}
}
