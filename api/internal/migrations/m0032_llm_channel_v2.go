package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0032_llm_channel_v2 implements the LLM Channel v2 architecture
// This migration separates system domain (global definitions) from tenant domain (configurations + custom)
func M0032_llm_channel_v2() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251212000032",
		Migrate: func(tx *gorm.DB) error {
			// Phase 1: Create credential tables (split system and tenant)
			if err := createCredentialTablesV2(tx); err != nil {
				return err
			}

			// Phase 2: Create tenant configuration tables
			if err := createTenantConfigTablesV2(tx); err != nil {
				return err
			}

			// Phase 3: Create tenant custom provider/model tables
			if err := createTenantCustomTables(tx); err != nil {
				return err
			}

			// Phase 4: Update tenant routes table
			if err := updateTenantRoutesTable(tx); err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback in reverse order
			sqls := []string{
				// Remove added columns from llm_tenant_routes
				`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS provider_type`,
				`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS global_provider_id`,
				`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS custom_provider_id`,
				`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS credential_id`,

				// Drop tenant custom tables
				`DROP TABLE IF EXISTS llm_tenant_models CASCADE`,
				`DROP TABLE IF EXISTS llm_tenant_providers CASCADE`,

				// Drop tenant config tables
				`DROP TABLE IF EXISTS llm_tenant_model_configs CASCADE`,
				`DROP TABLE IF EXISTS llm_tenant_provider_configs CASCADE`,

				// Drop credential tables
				`DROP TABLE IF EXISTS llm_tenant_credentials CASCADE`,
				`DROP TABLE IF EXISTS llm_system_credentials CASCADE`,
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

// createCredentialTablesV2 creates system and tenant credential tables
func createCredentialTablesV2(tx *gorm.DB) error {
	// Create llm_system_credentials table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_system_credentials (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			name VARCHAR(100) NOT NULL,
			provider VARCHAR(50) NOT NULL,
			protocol VARCHAR(50),
			api_key_ciphertext TEXT NOT NULL,
			api_key_hash VARCHAR(64),
			api_base_url VARCHAR(500),
			is_active BOOLEAN DEFAULT true,
			last_used_at TIMESTAMP,
			expires_at TIMESTAMP,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_system_credentials
	sysCredIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_sys_cred_provider ON llm_system_credentials(provider)`,
		`CREATE INDEX IF NOT EXISTS idx_sys_cred_active ON llm_system_credentials(is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_sys_cred_hash ON llm_system_credentials(api_key_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_sys_cred_deleted_at ON llm_system_credentials(deleted_at)`,
	}
	for _, sql := range sysCredIndexes {
		if err := tx.Exec(sql).Error; err != nil {
			return err
		}
	}

	// Create llm_tenant_credentials table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_credentials (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id UUID NOT NULL,
			name VARCHAR(100) NOT NULL,
			provider VARCHAR(50) NOT NULL,
			protocol VARCHAR(50),
			api_key_ciphertext TEXT NOT NULL,
			api_key_hash VARCHAR(64),
			api_base_url VARCHAR(500),
			is_active BOOLEAN DEFAULT true,
			last_used_at TIMESTAMP,
			expires_at TIMESTAMP,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_credentials
	tenantCredIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_tenant_cred_tenant ON llm_tenant_credentials(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_cred_provider ON llm_tenant_credentials(provider)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_cred_active ON llm_tenant_credentials(is_active)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_cred_hash ON llm_tenant_credentials(tenant_id, api_key_hash) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_cred_deleted_at ON llm_tenant_credentials(deleted_at)`,
	}
	for _, sql := range tenantCredIndexes {
		if err := tx.Exec(sql).Error; err != nil {
			return err
		}
	}

	return nil
}

// createTenantConfigTablesV2 creates tenant configuration tables for global providers/models
func createTenantConfigTablesV2(tx *gorm.DB) error {
	// Create llm_tenant_provider_configs table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_provider_configs (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id UUID NOT NULL,
			provider_id UUID NOT NULL,
			is_enabled BOOLEAN DEFAULT true,
			custom_display_name VARCHAR(100),
			custom_api_base_url VARCHAR(255),
			custom_logo_url VARCHAR(255),
			sort_order INT DEFAULT 0,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP,
			CONSTRAINT fk_tenant_provider_config_provider FOREIGN KEY (provider_id)
				REFERENCES llm_providers(id) ON DELETE CASCADE
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_provider_configs
	providerConfigIndexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_provider_config_unique ON llm_tenant_provider_configs(tenant_id, provider_id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_provider_config_tenant ON llm_tenant_provider_configs(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_provider_config_enabled ON llm_tenant_provider_configs(is_enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_provider_config_deleted_at ON llm_tenant_provider_configs(deleted_at)`,
	}
	for _, sql := range providerConfigIndexes {
		if err := tx.Exec(sql).Error; err != nil {
			return err
		}
	}

	// Create llm_tenant_model_configs table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_model_configs (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			tenant_id UUID NOT NULL,
			model_id UUID NOT NULL,
			is_enabled BOOLEAN DEFAULT true,
			custom_display_name VARCHAR(200),
			input_price_override DECIMAL(10,4),
			output_price_override DECIMAL(10,4),
			access_scope VARCHAR(20) DEFAULT 'all',
			visible_groups JSONB DEFAULT '[]',
			visible_users JSONB DEFAULT '[]',
			sort_order INT DEFAULT 0,
			metadata JSONB DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP,
			CONSTRAINT fk_tenant_model_config_model FOREIGN KEY (model_id)
				REFERENCES llm_models(id) ON DELETE CASCADE,
			CONSTRAINT chk_access_scope CHECK (access_scope IN ('all', 'group', 'user'))
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_model_configs
	modelConfigIndexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_model_config_unique ON llm_tenant_model_configs(tenant_id, model_id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_config_tenant ON llm_tenant_model_configs(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_config_enabled ON llm_tenant_model_configs(is_enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_config_deleted_at ON llm_tenant_model_configs(deleted_at)`,
	}
	for _, sql := range modelConfigIndexes {
		if err := tx.Exec(sql).Error; err != nil {
			return err
		}
	}

	return nil
}

// createTenantCustomTables creates tenant custom provider/model tables
// NOTE: llm_tenant_providers and llm_tenant_models tables are already created by m0014_llm_system
// with a different schema (configuration tables for enabling global providers/models).
// This function is kept for compatibility but skips table creation.
func createTenantCustomTables(tx *gorm.DB) error {
	// Tables llm_tenant_providers and llm_tenant_models already exist from m0014
	// They serve as configuration tables (tenant enables which global providers/models)
	// Skip creation to avoid conflicts
	return nil
}

// updateTenantRoutesTable updates the existing tenant routes table
func updateTenantRoutesTable(tx *gorm.DB) error {
	sqls := []string{
		// Add credential_id column (reference to llm_tenant_credentials)
		`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS credential_id UUID`,

		// Add provider type columns
		`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS provider_type VARCHAR(20)`,
		`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS global_provider_id UUID`,
		`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS custom_provider_id UUID`,

		// Add tags column if not exists
		`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]'`,

		// Add description column
		`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS description TEXT`,

		// Create indexes
		`CREATE INDEX IF NOT EXISTS idx_route_credential ON llm_tenant_routes(credential_id)`,
		`CREATE INDEX IF NOT EXISTS idx_route_provider_type ON llm_tenant_routes(provider_type)`,
		`CREATE INDEX IF NOT EXISTS idx_route_global_provider ON llm_tenant_routes(global_provider_id)`,
		`CREATE INDEX IF NOT EXISTS idx_route_custom_provider ON llm_tenant_routes(custom_provider_id)`,

		// Add foreign key constraints
		`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS fk_route_credential`,
		`ALTER TABLE llm_tenant_routes ADD CONSTRAINT fk_route_credential
			FOREIGN KEY (credential_id) REFERENCES llm_tenant_credentials(id) ON DELETE SET NULL`,

		`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS fk_route_global_provider`,
		`ALTER TABLE llm_tenant_routes ADD CONSTRAINT fk_route_global_provider
			FOREIGN KEY (global_provider_id) REFERENCES llm_providers(id) ON DELETE SET NULL`,

		`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS fk_route_custom_provider`,
		`ALTER TABLE llm_tenant_routes ADD CONSTRAINT fk_route_custom_provider
			FOREIGN KEY (custom_provider_id) REFERENCES llm_tenant_providers(id) ON DELETE SET NULL`,

		// Add check constraint for provider_type
		`ALTER TABLE llm_tenant_routes DROP CONSTRAINT IF EXISTS chk_route_provider_type`,
		`ALTER TABLE llm_tenant_routes ADD CONSTRAINT chk_route_provider_type
			CHECK (provider_type IS NULL OR provider_type IN ('global', 'custom'))`,
	}

	for _, sql := range sqls {
		if err := tx.Exec(sql).Error; err != nil {
			return err
		}
	}

	return nil
}
