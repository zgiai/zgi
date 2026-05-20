package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0007_app_agent_workflow creates app, agent and workflow related tables
func M0007_app_agent_workflow() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100600",
		Migrate: func(tx *gorm.DB) error {
			// Create agent_api_key_usage_logs table
			if err := tx.Exec(`
				CREATE TABLE agent_api_key_usage_logs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					api_key_id UUID NOT NULL,
					agent_id UUID NOT NULL,
					operation_log_id UUID,
					request_path VARCHAR(500) NOT NULL,
					request_ip VARCHAR(45) NOT NULL,
					user_agent TEXT,
					request_headers JSON,
					request_body_size BIGINT DEFAULT 0,
					response_status_code INTEGER NOT NULL,
					response_body_size BIGINT DEFAULT 0,
					response_time_ms INTEGER NOT NULL,
					tokens_used INTEGER DEFAULT 0,
					cost_amount NUMERIC(10,6) DEFAULT 0,
					currency VARCHAR(3) DEFAULT 'USD',
					error_message TEXT,
					metadata JSON,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					tenant_id UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create agent_api_keys table
			if err := tx.Exec(`
				CREATE TABLE agent_api_keys (
					id VARCHAR(36) NOT NULL,
					agent_id VARCHAR(36) NOT NULL,
					tenant_id VARCHAR(36) NOT NULL,
					key_hash VARCHAR(64) NOT NULL,
					key_prefix VARCHAR(12) NOT NULL,
					name VARCHAR(255) NOT NULL,
					status VARCHAR(20) DEFAULT 'active',
					expires_at TIMESTAMPTZ,
					usage_count BIGINT DEFAULT 0,
					last_used_at TIMESTAMPTZ,
					daily_quota BIGINT DEFAULT -1,
					monthly_quota BIGINT DEFAULT -1,
					daily_usage BIGINT DEFAULT 0,
					monthly_usage BIGINT DEFAULT 0,
					last_reset_date DATE DEFAULT CURRENT_DATE,
					created_at TIMESTAMPTZ DEFAULT now(),
					updated_at TIMESTAMPTZ DEFAULT now(),
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create agent_extensions table
			if err := tx.Exec(`
				CREATE TABLE agent_extensions (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					agent_id UUID NOT NULL,
					permission VARCHAR(32),
					extended_properties JSONB,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create agents table
			if err := tx.Exec(`
				CREATE TABLE agents (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					name VARCHAR(255) NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					agent_type VARCHAR(255) NOT NULL,
					icon_type VARCHAR(255),
					icon TEXT,
					agents_model_config_id UUID,
					workflow_id UUID,
					enable_api BOOLEAN NOT NULL,
					is_public BOOLEAN NOT NULL DEFAULT false,
					is_universal BOOLEAN NOT NULL DEFAULT false,
					created_by UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_by UUID,
					deleted_at TIMESTAMPTZ,
					workflow_config VARCHAR,
					internal BOOLEAN DEFAULT false,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create agents_configs table
			if err := tx.Exec(`
				CREATE TABLE agents_configs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					agents_id UUID NOT NULL,
					model_provider VARCHAR(255),
					model_version_id VARCHAR(255),
					configs JSON,
					created_by UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_by UUID,
					deleted_at TIMESTAMPTZ,
					greeting_message TEXT,
					user_input_form TEXT,
					dataset_query_variable VARCHAR(255),
					pre_prompt TEXT,
					agent_mode TEXT,
					sensitive_word_avoidance TEXT,
					retriever_resource TEXT,
					prompt_type VARCHAR(255) NOT NULL DEFAULT 'simple',
					chat_prompt_config TEXT,
					completion_prompt_config TEXT,
					dataset_configs TEXT,
					external_data_tools TEXT,
					file_upload TEXT,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create agents_conversations table
			if err := tx.Exec(`
				CREATE TABLE agents_conversations (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					agent_id UUID NOT NULL,
					agent_config_id UUID,
					model_provider VARCHAR(255),
					override_model_configs TEXT,
					model_version_id VARCHAR(255),
					mode VARCHAR(255) NOT NULL,
					name VARCHAR(255) NOT NULL,
					summary TEXT,
					inputs JSON NOT NULL,
					introduction TEXT,
					system_instruction TEXT,
					system_instruction_tokens INTEGER NOT NULL DEFAULT 0,
					status VARCHAR(255) NOT NULL,
					invoke_from VARCHAR(255),
					from_source VARCHAR(255) NOT NULL,
					from_end_user_id UUID,
					from_account_id UUID,
					read_at TIMESTAMPTZ,
					read_account_id UUID,
					dialogue_count INTEGER NOT NULL DEFAULT 0,
					created_by UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_by UUID,
					deleted_at TIMESTAMPTZ,
					workflow_version_uuid UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create agents_messages table
			if err := tx.Exec(`
				CREATE TABLE agents_messages (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					agent_id UUID NOT NULL,
					model_provider VARCHAR(255),
					model_version_id VARCHAR(255),
					override_model_configs TEXT,
					conversation_id UUID NOT NULL,
					inputs JSON NOT NULL,
					query TEXT NOT NULL,
					message JSON NOT NULL,
					message_tokens INTEGER NOT NULL DEFAULT 0,
					message_unit_price NUMERIC(10,4) NOT NULL,
					message_price_unit NUMERIC(10,7) NOT NULL DEFAULT 0.001,
					answer TEXT NOT NULL,
					answer_tokens INTEGER NOT NULL DEFAULT 0,
					answer_unit_price NUMERIC(10,4) NOT NULL,
					answer_price_unit NUMERIC(10,7) NOT NULL DEFAULT 0.001,
					parent_message_id UUID,
					provider_response_latency FLOAT8 NOT NULL DEFAULT 0,
					total_price NUMERIC(10,7),
					currency VARCHAR(255) NOT NULL,
					status VARCHAR(255) NOT NULL DEFAULT 'normal',
					error TEXT,
					message_metadata TEXT,
					invoke_from VARCHAR(255),
					from_source VARCHAR(255) NOT NULL,
					from_end_user_id UUID,
					from_account_id UUID,
					created_by UUID,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_by UUID,
					deleted_at TIMESTAMPTZ,
					agent_based BOOLEAN NOT NULL DEFAULT false,
					workflow_run_id UUID,
					workflow_version_uuid UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create app_dataset_joins table
			if err := tx.Exec(`
				CREATE TABLE app_dataset_joins (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id UUID NOT NULL,
					dataset_id UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create app_extensions table
			if err := tx.Exec(`
				CREATE TABLE app_extensions (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id UUID NOT NULL,
					permission VARCHAR(32),
					extended_properties JSON,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create app_model_configs table
			if err := tx.Exec(`
				CREATE TABLE app_model_configs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id UUID NOT NULL,
					provider VARCHAR(255),
					model_id VARCHAR(255),
					configs JSON,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					opening_statement TEXT,
					suggested_questions TEXT,
					suggested_questions_after_answer TEXT,
					more_like_this TEXT,
					model TEXT,
					user_input_form TEXT,
					pre_prompt TEXT,
					agent_mode TEXT,
					speech_to_text TEXT,
					sensitive_word_avoidance TEXT,
					retriever_resource TEXT,
					dataset_query_variable VARCHAR(255),
					prompt_type VARCHAR(255) NOT NULL DEFAULT 'simple',
					chat_prompt_config TEXT,
					completion_prompt_config TEXT,
					dataset_configs TEXT,
					external_data_tools TEXT,
					file_upload TEXT,
					text_to_speech TEXT,
					created_by UUID,
					updated_by UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create apps table
			if err := tx.Exec(`
				CREATE TABLE apps (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					name VARCHAR(255) NOT NULL,
					mode VARCHAR(255) NOT NULL,
					icon TEXT,
					icon_background VARCHAR(255),
					app_model_config_id UUID,
					status VARCHAR(255) NOT NULL DEFAULT 'normal',
					enable_site BOOLEAN NOT NULL,
					enable_api BOOLEAN NOT NULL,
					api_rpm INTEGER NOT NULL DEFAULT 0,
					api_rph INTEGER NOT NULL DEFAULT 0,
					is_demo BOOLEAN NOT NULL DEFAULT false,
					is_public BOOLEAN NOT NULL DEFAULT false,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					is_universal BOOLEAN NOT NULL DEFAULT false,
					workflow_id UUID,
					description TEXT NOT NULL DEFAULT '',
					tracing TEXT,
					max_active_requests INTEGER,
					icon_type VARCHAR(255),
					created_by UUID,
					updated_by UUID,
					use_icon_as_answer_icon BOOLEAN NOT NULL DEFAULT false,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create installed_agents table
			if err := tx.Exec(`
				CREATE TABLE installed_agents (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					agent_id UUID NOT NULL,
					agent_owner_tenant_id UUID NOT NULL,
					position INTEGER NOT NULL DEFAULT 0,
					is_pinned BOOLEAN NOT NULL DEFAULT false,
					last_used_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create installed_apps table
			if err := tx.Exec(`
				CREATE TABLE installed_apps (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					app_id UUID NOT NULL,
					app_owner_tenant_id UUID NOT NULL,
					position INTEGER NOT NULL,
					is_pinned BOOLEAN NOT NULL DEFAULT false,
					last_used_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create live_agents_runtime_logs table
			if err := tx.Exec(`
				CREATE TABLE live_agents_runtime_logs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					agent_id UUID NOT NULL,
					workflow_id UUID NOT NULL,
					workflow_run_id UUID NOT NULL,
					created_from VARCHAR(255) NOT NULL,
					created_by_role VARCHAR(255) NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					graph JSONB,
					features JSONB,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create workflow_app_logs table
			if err := tx.Exec(`
				CREATE TABLE workflow_app_logs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					app_id UUID NOT NULL,
					workflow_id UUID NOT NULL,
					workflow_run_id UUID NOT NULL,
					created_from VARCHAR(255) NOT NULL,
					created_by_role VARCHAR(255) NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					agent_id UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create workflow_conversation_variables table
			if err := tx.Exec(`
				CREATE TABLE workflow_conversation_variables (
					id UUID NOT NULL,
					conversation_id UUID NOT NULL,
					app_id UUID NOT NULL,
					data TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create workflow_node_executions table
			if err := tx.Exec(`
				CREATE TABLE workflow_node_executions (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					app_id UUID NOT NULL,
					workflow_id UUID NOT NULL,
					triggered_from VARCHAR(255) NOT NULL,
					workflow_run_id UUID,
					index INTEGER NOT NULL,
					predecessor_node_id VARCHAR(255),
					node_id VARCHAR(255) NOT NULL,
					node_type VARCHAR(255) NOT NULL,
					title VARCHAR(255) NOT NULL,
					inputs TEXT,
					process_data TEXT,
					outputs TEXT,
					status VARCHAR(255) NOT NULL,
					error TEXT,
					elapsed_time FLOAT8 NOT NULL DEFAULT 0,
					execution_metadata TEXT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					created_by_role VARCHAR(255) NOT NULL,
					created_by UUID NOT NULL,
					finished_at TIMESTAMPTZ,
					node_execution_id VARCHAR(255),
					agent_id UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create workflow_node_runtime_logs table
			if err := tx.Exec(`
				CREATE TABLE workflow_node_runtime_logs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					agent_id UUID NOT NULL,
					workflow_id UUID NOT NULL,
					triggered_from VARCHAR(255) NOT NULL,
					workflow_run_id UUID,
					index INTEGER NOT NULL,
					predecessor_node_id VARCHAR(255),
					node_execution_id VARCHAR(255),
					node_id VARCHAR(255) NOT NULL,
					node_type VARCHAR(255) NOT NULL,
					title VARCHAR(255) NOT NULL,
					inputs TEXT,
					process_data TEXT,
					outputs TEXT,
					status VARCHAR(255) NOT NULL,
					error TEXT,
					elapsed_time FLOAT8 NOT NULL DEFAULT 0,
					execution_metadata TEXT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					created_by_role VARCHAR(255) NOT NULL,
					created_by UUID NOT NULL,
					deleted_at TIMESTAMPTZ,
					deleted_by UUID,
					finished_at TIMESTAMPTZ,
					graph JSONB,
					features JSONB,
					workflow_version_uuid UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create workflow_run_logs table
			if err := tx.Exec(`
				CREATE TABLE workflow_run_logs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					agent_id UUID NOT NULL,
					sequence_number INTEGER NOT NULL,
					workflow_id UUID NOT NULL,
					type VARCHAR(255) NOT NULL,
					triggered_from VARCHAR(255) NOT NULL,
					version VARCHAR(255) NOT NULL,
					graph TEXT,
					inputs TEXT,
					status VARCHAR(255) NOT NULL,
					outputs TEXT DEFAULT '{}',
					error TEXT,
					elapsed_time FLOAT8 NOT NULL DEFAULT 0,
					total_tokens BIGINT NOT NULL DEFAULT 0,
					total_steps INTEGER DEFAULT 0,
					created_by_role VARCHAR(255) NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					finished_at TIMESTAMPTZ,
					deleted_at TIMESTAMPTZ,
					deleted_by UUID,
					exceptions_count INTEGER DEFAULT 0,
					features JSONB,
					workflow_version_uuid UUID,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create workflow_runs table
			if err := tx.Exec(`
				CREATE TABLE workflow_runs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					app_id UUID NOT NULL,
					sequence_number INTEGER NOT NULL,
					workflow_id UUID NOT NULL,
					type VARCHAR(255) NOT NULL,
					triggered_from VARCHAR(255) NOT NULL,
					version VARCHAR(255) NOT NULL,
					graph TEXT,
					inputs TEXT,
					status VARCHAR(255) NOT NULL,
					outputs TEXT,
					error TEXT,
					elapsed_time FLOAT8 NOT NULL DEFAULT 0,
					total_tokens BIGINT NOT NULL DEFAULT 0,
					total_steps INTEGER DEFAULT 0,
					created_by_role VARCHAR(255) NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					finished_at TIMESTAMPTZ,
					exceptions_count INTEGER DEFAULT 0,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create workflows table
			if err := tx.Exec(`
				CREATE TABLE workflows (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					app_id UUID NOT NULL,
					type VARCHAR(255) NOT NULL,
					version VARCHAR(255) NOT NULL,
					graph TEXT NOT NULL,
					features TEXT NOT NULL,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_by UUID,
					updated_at TIMESTAMPTZ NOT NULL,
					environment_variables TEXT NOT NULL DEFAULT '{}',
					conversation_variables TEXT NOT NULL DEFAULT '{}',
					marked_name VARCHAR NOT NULL DEFAULT '',
					marked_comment VARCHAR NOT NULL DEFAULT '',
					deleted_at TIMESTAMPTZ,
					deleted_by UUID,
					agent_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
					version_uuid UUID,
					internal BOOLEAN DEFAULT false,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes
			indexes := []string{
				`CREATE INDEX idx_agent_api_key_usage_logs_agent_id ON agent_api_key_usage_logs(agent_id)`,
				`CREATE INDEX idx_agent_api_key_usage_logs_api_key_created ON agent_api_key_usage_logs(api_key_id, created_at)`,
				`CREATE INDEX idx_agent_api_key_usage_logs_api_key_id ON agent_api_key_usage_logs(api_key_id)`,
				`CREATE INDEX idx_agent_api_key_usage_logs_created_at ON agent_api_key_usage_logs(created_at)`,
				`CREATE INDEX idx_agent_api_keys_agent_id ON agent_api_keys(agent_id)`,
				`CREATE INDEX idx_agent_api_keys_agent_tenant ON agent_api_keys(agent_id, tenant_id)`,
				`CREATE INDEX idx_agent_api_keys_expires_at ON agent_api_keys(expires_at)`,
				`CREATE INDEX idx_agent_api_keys_key_hash ON agent_api_keys(key_hash)`,
				`CREATE INDEX idx_agent_api_keys_status ON agent_api_keys(status)`,
				`CREATE INDEX idx_agent_api_keys_tenant_id ON agent_api_keys(tenant_id)`,
				`CREATE INDEX agents_tenant_id_idx ON agents(tenant_id)`,
				`CREATE INDEX agents_agents_id_idx ON agents_configs(agents_id)`,
				`CREATE INDEX conversation_from_user_idx ON agents_conversations(agent_id, from_source, from_end_user_id)`,
				`CREATE INDEX agents_conversations_agent_id_idx ON agents_conversations(agent_id)`,
				`CREATE INDEX agents_messages_conversation_id_idx ON agents_messages(conversation_id)`,
				`CREATE INDEX agents_message_agents_id_idx ON agents_messages(agent_id, created_at)`,
				`CREATE INDEX agents_message_conversation_id_idx ON agents_messages(conversation_id, workflow_run_id)`,
				`CREATE INDEX agents_message_created_at_idx ON agents_messages(created_at)`,
				`CREATE INDEX agents_message_end_user_idx ON agents_messages(agent_id, from_source, from_end_user_id)`,
				`CREATE INDEX agents_message_workflow_run_id_idx ON agents_messages(conversation_id, workflow_run_id)`,
				`CREATE INDEX app_dataset_joins_app_id_idx ON app_dataset_joins(app_id)`,
				`CREATE INDEX app_dataset_joins_dataset_id_idx ON app_dataset_joins(dataset_id)`,
				`CREATE INDEX apps_tenant_id_idx ON apps(tenant_id)`,
				`CREATE INDEX installed_agents_tenant_id_idx ON installed_agents(tenant_id)`,
				`CREATE INDEX installed_apps_tenant_id_idx ON installed_apps(tenant_id)`,
				`CREATE INDEX live_agents_runtime_logs_agents_idx ON live_agents_runtime_logs(tenant_id, agent_id)`,
				`CREATE INDEX workflow_node_executions_workflow_run_id_idx ON workflow_node_executions(workflow_run_id)`,
				`CREATE INDEX workflow_node_runtime_logs_workflow_run_id_idx ON workflow_node_runtime_logs(workflow_run_id)`,
				`CREATE INDEX workflow_run_logs_agent_id_idx ON workflow_run_logs(agent_id)`,
				`CREATE INDEX workflow_runs_app_id_idx ON workflow_runs(app_id)`,
				`CREATE INDEX workflows_app_id_idx ON workflows(app_id)`,
				`CREATE INDEX workflows_tenant_id_idx ON workflows(tenant_id)`,
			}

			for _, indexSQL := range indexes {
				if err := tx.Exec(indexSQL).Error; err != nil {
					return err
				}
			}

			// Add constraints
			constraints := []string{
				`ALTER TABLE agent_extensions ADD CONSTRAINT agent_extensions_agent_id_key UNIQUE (agent_id)`,
				`ALTER TABLE agent_api_keys ADD CONSTRAINT agent_api_keys_status_check CHECK (status IN ('active', 'inactive', 'revoked'))`,
			}

			for _, constraintSQL := range constraints {
				if err := tx.Exec(constraintSQL).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(
				"agent_api_key_usage_logs",
				"agent_api_keys",
				"agent_extensions",
				"agents",
				"agents_configs",
				"agents_conversations",
				"agents_messages",
				"app_dataset_joins",
				"app_extensions",
				"app_model_configs",
				"apps",
				"installed_agents",
				"installed_apps",
				"live_agents_runtime_logs",
				"workflow_app_logs",
				"workflow_conversation_variables",
				"workflow_node_executions",
				"workflow_node_runtime_logs",
				"workflow_run_logs",
				"workflow_runs",
				"workflows",
			)
		},
	}
}
