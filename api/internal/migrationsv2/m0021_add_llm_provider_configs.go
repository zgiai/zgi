package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0021_add_llm_provider_configs repairs the fresh v2 baseline by restoring
// the organization-scoped provider config table expected by the provider module.
func M0021_add_llm_provider_configs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddLLMProviderConfigsID,
		Migrate: func(tx *gorm.DB) error {
			if tableExists(tx, "llm_tenant_provider_configs") && !tableExists(tx, "llm_provider_configs") {
				if err := tx.Exec(`
					ALTER TABLE public.llm_tenant_provider_configs
					RENAME TO llm_provider_configs
				`).Error; err != nil {
					return fmt.Errorf("rename llm_tenant_provider_configs: %w", err)
				}
			}

			if tableExists(tx, "llm_provider_configs") &&
				columnExists(tx, "llm_provider_configs", "tenant_id") &&
				!columnExists(tx, "llm_provider_configs", "organization_id") {
				if err := tx.Exec(`
					ALTER TABLE public.llm_provider_configs
					RENAME COLUMN tenant_id TO organization_id
				`).Error; err != nil {
					return fmt.Errorf("rename llm_provider_configs.tenant_id: %w", err)
				}
			}

			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS public.llm_provider_configs (
					id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
					organization_id uuid NOT NULL,
					provider_id uuid NOT NULL,
					is_enabled boolean DEFAULT true,
					custom_display_name character varying(100),
					custom_api_base_url character varying(255),
					custom_logo_url character varying(255),
					sort_order integer DEFAULT 0,
					metadata jsonb DEFAULT '{}'::jsonb,
					created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
					updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
					deleted_at timestamp without time zone,
					CONSTRAINT llm_provider_configs_pkey PRIMARY KEY (id),
					CONSTRAINT fk_tenant_provider_config_provider FOREIGN KEY (provider_id)
						REFERENCES public.llm_providers(id) ON DELETE CASCADE
				)
			`).Error; err != nil {
				return fmt.Errorf("create llm_provider_configs: %w", err)
			}

			if err := tx.Exec(`
				ALTER TABLE public.llm_provider_configs
					ADD COLUMN IF NOT EXISTS organization_id uuid,
					ADD COLUMN IF NOT EXISTS provider_id uuid,
					ADD COLUMN IF NOT EXISTS is_enabled boolean DEFAULT true,
					ADD COLUMN IF NOT EXISTS custom_display_name character varying(100),
					ADD COLUMN IF NOT EXISTS custom_api_base_url character varying(255),
					ADD COLUMN IF NOT EXISTS custom_logo_url character varying(255),
					ADD COLUMN IF NOT EXISTS sort_order integer DEFAULT 0,
					ADD COLUMN IF NOT EXISTS metadata jsonb DEFAULT '{}'::jsonb,
					ADD COLUMN IF NOT EXISTS created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
					ADD COLUMN IF NOT EXISTS updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
					ADD COLUMN IF NOT EXISTS deleted_at timestamp without time zone
			`).Error; err != nil {
				return fmt.Errorf("align llm_provider_configs columns: %w", err)
			}

			indexStatements := []string{
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_provider_config_unique ON public.llm_provider_configs USING btree (organization_id, provider_id) WHERE (deleted_at IS NULL)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_provider_config_tenant ON public.llm_provider_configs USING btree (organization_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_provider_config_enabled ON public.llm_provider_configs USING btree (is_enabled)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_provider_config_deleted_at ON public.llm_provider_configs USING btree (deleted_at)`,
			}
			for _, statement := range indexStatements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("ensure llm_provider_configs index %q: %w", statementPreview(statement), err)
				}
			}

			hasProviderFK, err := constraintExists(tx, "fk_tenant_provider_config_provider")
			if err != nil {
				return fmt.Errorf("check llm_provider_configs provider fk: %w", err)
			}
			if !hasProviderFK {
				if err := tx.Exec(`
					ALTER TABLE public.llm_provider_configs
					ADD CONSTRAINT fk_tenant_provider_config_provider
					FOREIGN KEY (provider_id) REFERENCES public.llm_providers(id) ON DELETE CASCADE
				`).Error; err != nil {
					return fmt.Errorf("add llm_provider_configs provider fk: %w", err)
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func columnExists(tx *gorm.DB, tableName, columnName string) bool {
	var count int64
	if err := tx.Raw(`
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = ?
		  AND column_name = ?
	`, tableName, columnName).Scan(&count).Error; err != nil {
		return false
	}

	return count > 0
}
