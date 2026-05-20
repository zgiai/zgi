package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0030_create_aichat_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2CreateAIChatTablesID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE IF NOT EXISTS public.aichat_conversations (
					id UUID PRIMARY KEY,
					organization_id UUID NOT NULL,
					workspace_id UUID,
					account_id UUID NOT NULL,
					title VARCHAR(255) NOT NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'normal',
					runtime_status VARCHAR(32) NOT NULL DEFAULT 'idle',
					current_leaf_message_id UUID,
					active_message_id UUID,
					dialogue_count INTEGER NOT NULL DEFAULT 0,
					source VARCHAR(32) NOT NULL DEFAULT 'console',
					source_conversation_id UUID,
					source_web_app_id UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ
				)`,
				`CREATE TABLE IF NOT EXISTS public.aichat_messages (
					id UUID PRIMARY KEY,
					conversation_id UUID NOT NULL,
					parent_id UUID,
					query TEXT NOT NULL,
					answer TEXT NOT NULL DEFAULT '',
					status VARCHAR(32) NOT NULL DEFAULT 'pending',
					error TEXT,
					model_provider VARCHAR(255),
					model_name VARCHAR(255) NOT NULL,
					billing_reason_source VARCHAR(64),
					model_parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
					metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
					source_message_id UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ
				)`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_owner_updated
					ON public.aichat_conversations(organization_id, account_id, updated_at DESC)
					WHERE deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_workspace
					ON public.aichat_conversations(workspace_id)
					WHERE deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_runtime_status
					ON public.aichat_conversations(runtime_status)
					WHERE deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_active_message
					ON public.aichat_conversations(active_message_id)
					WHERE active_message_id IS NOT NULL AND deleted_at IS NULL`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_aichat_conversations_source_conversation
					ON public.aichat_conversations(source_conversation_id)
					WHERE source_conversation_id IS NOT NULL AND deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_conversations_source_web_app
					ON public.aichat_conversations(source_web_app_id)
					WHERE source_web_app_id IS NOT NULL AND deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_messages_conversation_created
					ON public.aichat_messages(conversation_id, created_at ASC)
					WHERE deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_messages_parent
					ON public.aichat_messages(parent_id)
					WHERE parent_id IS NOT NULL AND deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_messages_billing_reason_source
					ON public.aichat_messages(billing_reason_source)
					WHERE deleted_at IS NULL`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_aichat_messages_source_message
					ON public.aichat_messages(source_message_id)
					WHERE source_message_id IS NOT NULL AND deleted_at IS NULL`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("create aichat tables: %w", err)
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP INDEX IF EXISTS public.idx_aichat_messages_source_message`,
				`DROP INDEX IF EXISTS public.idx_aichat_messages_billing_reason_source`,
				`DROP INDEX IF EXISTS public.idx_aichat_messages_parent`,
				`DROP INDEX IF EXISTS public.idx_aichat_messages_conversation_created`,
				`DROP INDEX IF EXISTS public.idx_aichat_conversations_source_web_app`,
				`DROP INDEX IF EXISTS public.idx_aichat_conversations_source_conversation`,
				`DROP INDEX IF EXISTS public.idx_aichat_conversations_active_message`,
				`DROP INDEX IF EXISTS public.idx_aichat_conversations_runtime_status`,
				`DROP INDEX IF EXISTS public.idx_aichat_conversations_workspace`,
				`DROP INDEX IF EXISTS public.idx_aichat_conversations_owner_updated`,
				`DROP TABLE IF EXISTS public.aichat_messages`,
				`DROP TABLE IF EXISTS public.aichat_conversations`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("rollback aichat tables: %w", err)
				}
			}
			return nil
		},
	}
}
