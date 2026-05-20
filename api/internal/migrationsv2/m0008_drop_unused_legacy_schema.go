package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var removedLegacyTables = []string{
	"app_extensions",
	"app_dataset_joins",
	"app_model_configs",
	"apps",
	"completions",
	"dataset_retriever_resources",
	"embeddings",
	"installed_apps",
	"llm_provider_protocols",
	"llm_protocols",
	"llm_system_channels",
	"llm_tenant_channels",
	"llm_tenant_credentials",
	"llm_tenant_model_settings",
	"llm_tenant_providers",
	"live_agents_runtime_logs",
	"message_agent_thoughts",
	"model_configs",
	"sites",
	"statistics",
	"tool_workflow_providers",
}

func M0008_drop_unused_legacy_schema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropUnusedLegacySchemaID,
		Migrate: func(tx *gorm.DB) error {
			if tableExists(tx, "llm_routes") {
				if err := tx.Exec(`
					ALTER TABLE IF EXISTS public.llm_routes
						ADD COLUMN IF NOT EXISTS is_official BOOLEAN NOT NULL DEFAULT false;
					CREATE INDEX IF NOT EXISTS idx_routes_is_official ON public.llm_routes USING btree (is_official);
					UPDATE public.llm_routes
					SET is_official = true
					WHERE type = 'ZGI_CLOUD' AND COALESCE(is_official, false) = false;
					ALTER TABLE IF EXISTS public.llm_routes
						DROP CONSTRAINT IF EXISTS llm_tenant_routes_system_channel_id_fkey;
					DROP INDEX IF EXISTS public.idx_route_sys_channel;
					ALTER TABLE IF EXISTS public.llm_routes
						DROP CONSTRAINT IF EXISTS chk_route_ref;
					ALTER TABLE IF EXISTS public.llm_routes
						DROP CONSTRAINT IF EXISTS chk_system_ref;
					ALTER TABLE IF EXISTS public.llm_routes
						DROP COLUMN IF EXISTS system_channel_id;
					ALTER TABLE IF EXISTS public.llm_routes
						ADD CONSTRAINT chk_system_ref CHECK (
							((type)::text = 'ZGI_CLOUD'::text AND is_official = true) OR
							((type)::text = 'PRIVATE'::text AND user_credential_id IS NOT NULL)
						);
				`).Error; err != nil {
					return fmt.Errorf("cleanup llm_routes legacy official channel references: %w", err)
				}
			}

			if err := tx.Exec(`
				DROP INDEX IF EXISTS public.conversation_app_model_config_id_idx;
				ALTER TABLE IF EXISTS public.apps
					DROP COLUMN IF EXISTS app_model_config_id;
				ALTER TABLE IF EXISTS public.conversations
					DROP COLUMN IF EXISTS app_model_config_id;
			`).Error; err != nil {
				return fmt.Errorf("drop app_model_config legacy columns: %w", err)
			}

			for _, table := range removedLegacyTables {
				if err := tx.Exec(fmt.Sprintf("DROP TABLE IF EXISTS public.%s", table)).Error; err != nil {
					return fmt.Errorf("drop removed legacy table %s: %w", table, err)
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
