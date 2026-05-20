package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0009_drop_provider_model_schema removes the retired provider_model tables after runtime cutover.
func M0009_drop_provider_model_schema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropProviderModelSchemaID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP INDEX IF EXISTS public.load_balancing_model_config_tenant_provider_model_idx;
				DROP INDEX IF EXISTS public.provider_model_setting_tenant_provider_model_idx;
				DROP INDEX IF EXISTS public.provider_model_tenant_id_provider_idx;
				DROP INDEX IF EXISTS public.provider_setting_tenant_provider_idx;
				DROP INDEX IF EXISTS public.provider_tenant_id_provider_idx;
				DROP INDEX IF EXISTS public.provider_group_id_provider_idx;
				DROP INDEX IF EXISTS public.tenant_default_model_tenant_id_provider_type_idx;
				DROP INDEX IF EXISTS public.tenant_preferred_model_provider_tenant_provider_idx;

				ALTER TABLE IF EXISTS public.load_balancing_model_configs
					DROP CONSTRAINT IF EXISTS load_balancing_model_configs_pkey;
				ALTER TABLE IF EXISTS public.provider_model_settings
					DROP CONSTRAINT IF EXISTS provider_model_settings_pkey;
				ALTER TABLE IF EXISTS public.provider_models
					DROP CONSTRAINT IF EXISTS provider_models_pkey,
					DROP CONSTRAINT IF EXISTS unique_provider_model_name;
				ALTER TABLE IF EXISTS public.provider_settings
					DROP CONSTRAINT IF EXISTS provider_settings_pkey;
				ALTER TABLE IF EXISTS public.tenant_default_models
					DROP CONSTRAINT IF EXISTS tenant_default_models_pkey;
				ALTER TABLE IF EXISTS public.tenant_preferred_model_providers
					DROP CONSTRAINT IF EXISTS tenant_preferred_model_providers_pkey;
				ALTER TABLE IF EXISTS public.providers
					DROP CONSTRAINT IF EXISTS providers_pkey,
					DROP CONSTRAINT IF EXISTS unique_provider_name_type_quota;
				ALTER TABLE IF EXISTS public.enterprise_group_providers
					DROP CONSTRAINT IF EXISTS enterprise_group_provider_pkey,
					DROP CONSTRAINT IF EXISTS unique_enterprise_group_provider_name_type_quota;

				DROP TABLE IF EXISTS public.load_balancing_model_configs;
				DROP TABLE IF EXISTS public.provider_model_settings;
				DROP TABLE IF EXISTS public.provider_models;
				DROP TABLE IF EXISTS public.provider_settings;
				DROP TABLE IF EXISTS public.tenant_default_models;
				DROP TABLE IF EXISTS public.tenant_preferred_model_providers;
				DROP TABLE IF EXISTS public.providers;
				DROP TABLE IF EXISTS public.enterprise_group_providers;
			`).Error; err != nil {
				return fmt.Errorf("drop retired provider_model schema: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
