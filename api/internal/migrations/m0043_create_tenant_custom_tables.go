package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0043_create_tenant_custom_tables creates separate tables for tenant custom providers and models
// to resolve conflict with m0014 configuration tables
func M0043_create_tenant_custom_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000043",
		Migrate: func(tx *gorm.DB) error {
			// 1. Create llm_tenant_custom_providers
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_tenant_custom_providers (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					name VARCHAR(50) NOT NULL,
					display_name VARCHAR(100) NOT NULL,
					api_base_url VARCHAR(255),
					protocol VARCHAR(50) DEFAULT 'openai',
					logo_url VARCHAR(255),
					documentation_url VARCHAR(255),
					description TEXT,
					is_active BOOLEAN DEFAULT true,
					sort_order INT DEFAULT 0,
					metadata JSONB DEFAULT '{}',
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ,
					CONSTRAINT fk_tenant_custom_provider_tenant FOREIGN KEY (tenant_id)
						REFERENCES tenants(id) ON DELETE CASCADE
				)
			`).Error; err != nil {
				return err
			}

			// Indexes for llm_tenant_custom_providers
			providerIndexes := []string{
				`CREATE INDEX IF NOT EXISTS idx_tenant_custom_provider_tenant ON llm_tenant_custom_providers(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_custom_provider_active ON llm_tenant_custom_providers(is_active)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_custom_provider_deleted_at ON llm_tenant_custom_providers(deleted_at)`,
			}
			for _, idx := range providerIndexes {
				if err := tx.Exec(idx).Error; err != nil {
					return err
				}
			}

			// 2. Create llm_tenant_custom_models
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_tenant_custom_models (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					provider_id UUID NOT NULL,
					name VARCHAR(100) NOT NULL,
					display_name VARCHAR(200) NOT NULL,
					type VARCHAR(50) DEFAULT 'llm',
					context_window INT,
					max_output_tokens INT,
					input_price DECIMAL(10,4) DEFAULT 0,
					output_price DECIMAL(10,4) DEFAULT 0,
					supports_vision BOOLEAN DEFAULT false,
					supports_tool_call BOOLEAN DEFAULT false,
					supports_streaming BOOLEAN DEFAULT true,
					supports_reasoning BOOLEAN DEFAULT false,
					knowledge_cutoff VARCHAR(20),
					description TEXT,
					is_active BOOLEAN DEFAULT true,
					sort_order INT DEFAULT 0,
					metadata JSONB DEFAULT '{}',
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ,
					CONSTRAINT fk_tenant_custom_model_tenant FOREIGN KEY (tenant_id)
						REFERENCES tenants(id) ON DELETE CASCADE
					-- provider_id can reference llm_providers OR llm_tenant_custom_providers
					-- Since it's polymorphic in logic (global vs custom), we don't add strict FK here
					-- or we reference strict if it ONLY belongs to custom providers.
					-- Based on TenantCustomModel struct, it has ProviderID uuid.UUID.
					-- Usually custom models belong to custom providers or global providers.
					-- If custom model belongs to global provider, provider_id would be global provider UUID.
				)
			`).Error; err != nil {
				return err
			}

			// Indexes for llm_tenant_custom_models
			modelIndexes := []string{
				`CREATE INDEX IF NOT EXISTS idx_tenant_custom_model_tenant ON llm_tenant_custom_models(tenant_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_custom_model_provider ON llm_tenant_custom_models(provider_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_custom_model_active ON llm_tenant_custom_models(is_active)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_custom_model_deleted_at ON llm_tenant_custom_models(deleted_at)`,
			}
			for _, idx := range modelIndexes {
				if err := tx.Exec(idx).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE IF EXISTS llm_tenant_custom_models`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS llm_tenant_custom_providers`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
