package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

const migrationCreateChatRuntimeAndAgentVersionsID = "20260525090000_create_chat_runtime_and_agent_versions"

func init() {
	registerSchemaMigration(migrationCreateChatRuntimeAndAgentVersionsID, upCreateChatRuntimeAndAgentVersions, nil)
}

func upCreateChatRuntimeAndAgentVersions(schema *mschema.Builder) error {
	statements := []string{
		`
		DO $$
		BEGIN
			IF to_regclass('public.aichat_conversations') IS NOT NULL
				AND to_regclass('public.chat_runtime_conversations') IS NULL THEN
				ALTER TABLE public.aichat_conversations RENAME TO chat_runtime_conversations;
			END IF;
			IF to_regclass('public.aichat_messages') IS NOT NULL
				AND to_regclass('public.chat_runtime_messages') IS NULL THEN
				ALTER TABLE public.aichat_messages RENAME TO chat_runtime_messages;
			END IF;
			IF to_regclass('public.aichat_custom_skills') IS NOT NULL
				AND to_regclass('public.chat_runtime_custom_skills') IS NULL THEN
				ALTER TABLE public.aichat_custom_skills RENAME TO chat_runtime_custom_skills;
			END IF;
			IF to_regclass('public.aichat_organization_skill_configs') IS NOT NULL
				AND to_regclass('public.chat_runtime_organization_skill_configs') IS NULL THEN
				ALTER TABLE public.aichat_organization_skill_configs RENAME TO chat_runtime_organization_skill_configs;
			END IF;
		END $$;
		`,
		`
		CREATE TABLE IF NOT EXISTS chat_runtime_conversations (
			id uuid NOT NULL,
			organization_id uuid NOT NULL,
			workspace_id uuid,
			account_id uuid NOT NULL,
			caller_type character varying(32) NOT NULL DEFAULT 'aichat',
			caller_id uuid,
			title character varying(255) NOT NULL,
			status character varying(32) NOT NULL DEFAULT 'normal',
			runtime_status character varying(32) NOT NULL DEFAULT 'idle',
			current_leaf_message_id uuid,
			active_message_id uuid,
			dialogue_count integer NOT NULL DEFAULT 0,
			source character varying(32) NOT NULL DEFAULT 'console',
			source_conversation_id uuid,
			source_web_app_id uuid,
			created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at timestamp with time zone,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			PRIMARY KEY (id)
		)
		`,
		`ALTER TABLE chat_runtime_conversations ADD COLUMN IF NOT EXISTS caller_type character varying(32) NOT NULL DEFAULT 'aichat'`,
		`ALTER TABLE chat_runtime_conversations ADD COLUMN IF NOT EXISTS caller_id uuid`,
		`
		CREATE TABLE IF NOT EXISTS chat_runtime_messages (
			id uuid NOT NULL,
			conversation_id uuid NOT NULL,
			parent_id uuid,
			query text NOT NULL,
			answer text NOT NULL DEFAULT '',
			status character varying(32) NOT NULL DEFAULT 'pending',
			error text,
			model_provider character varying(255),
			model_name character varying(255) NOT NULL,
			billing_reason_source character varying(64),
			model_parameters jsonb NOT NULL DEFAULT '{}'::jsonb,
			metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
			source_message_id uuid,
			created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at timestamp with time zone,
			PRIMARY KEY (id)
		)
		`,
		`
		CREATE TABLE IF NOT EXISTS chat_runtime_custom_skills (
			id uuid NOT NULL,
			organization_id uuid NOT NULL,
			skill_id character varying(128) NOT NULL,
			name character varying(128) NOT NULL,
			description text NOT NULL,
			when_to_use text NOT NULL,
			runtime_type character varying(32) NOT NULL DEFAULT 'prompt',
			display jsonb NOT NULL DEFAULT '{}'::jsonb,
			storage_path text NOT NULL,
			manifest jsonb NOT NULL DEFAULT '{}'::jsonb,
			status character varying(32) NOT NULL DEFAULT 'active',
			validation_error text,
			created_by uuid NOT NULL,
			created_at timestamp with time zone NOT NULL DEFAULT now(),
			updated_at timestamp with time zone NOT NULL DEFAULT now(),
			deleted_at timestamp with time zone,
			PRIMARY KEY (id)
		)
		`,
		`
		CREATE TABLE IF NOT EXISTS chat_runtime_organization_skill_configs (
			organization_id uuid NOT NULL,
			skill_id character varying(128) NOT NULL,
			enabled boolean NOT NULL DEFAULT true,
			created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (organization_id, skill_id)
		)
		`,
		`
		CREATE TABLE IF NOT EXISTS agent_published_versions (
			id uuid DEFAULT uuid_generate_v4() NOT NULL PRIMARY KEY,
			agent_id uuid NOT NULL,
			workspace_id uuid NOT NULL,
			version character varying(255) NOT NULL,
			version_uuid uuid NOT NULL,
			config_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
			description text NOT NULL DEFAULT '',
			created_by uuid,
			created_at timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at timestamp with time zone
		)
		`,
		`CREATE INDEX IF NOT EXISTS idx_chat_runtime_conversations_owner_updated ON chat_runtime_conversations (organization_id, account_id, updated_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_chat_runtime_conversations_caller_updated ON chat_runtime_conversations (organization_id, account_id, caller_type, caller_id, updated_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_chat_runtime_messages_conversation_created ON chat_runtime_messages (conversation_id, created_at) WHERE deleted_at IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_runtime_custom_skills_org_skill_active ON chat_runtime_custom_skills (organization_id, skill_id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_agent_published_versions_agent_created ON agent_published_versions (agent_id, created_at DESC) WHERE deleted_at IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_published_versions_version_uuid ON agent_published_versions (version_uuid) WHERE deleted_at IS NULL`,
	}
	for _, statement := range statements {
		if err := schema.Raw(statement); err != nil {
			return err
		}
	}
	return nil
}
