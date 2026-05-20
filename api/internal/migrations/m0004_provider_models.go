package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0004_provider_models creates provider models related tables
func M0004_provider_models() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100300",
		Migrate: func(tx *gorm.DB) error {
			// Create load_balancing_model_configs table
			if err := tx.Exec(`
				CREATE TABLE load_balancing_model_configs (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					provider_name VARCHAR(255) NOT NULL,
					model_name VARCHAR(255) NOT NULL,
					model_type VARCHAR(40) NOT NULL,
					name VARCHAR(255) NOT NULL,
					encrypted_config TEXT,
					enabled BOOLEAN NOT NULL DEFAULT true,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create plugin_installations table
			if err := tx.Exec(`
				CREATE TABLE plugin_installations (
					id VARCHAR(255) NOT NULL,
					tenant_id VARCHAR(255) NOT NULL,
					plugin_unique_identifier VARCHAR(255) NOT NULL,
					version VARCHAR(100),
					source VARCHAR(50) NOT NULL,
					is_active BOOLEAN NOT NULL DEFAULT true,
					installed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
					created_at TIMESTAMPTZ DEFAULT now(),
					updated_at TIMESTAMPTZ DEFAULT now(),
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create provider_model_settings table
			if err := tx.Exec(`
				CREATE TABLE provider_model_settings (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					provider_name VARCHAR(255) NOT NULL,
					model_name VARCHAR(255) NOT NULL,
					model_type VARCHAR(40) NOT NULL,
					enabled BOOLEAN NOT NULL DEFAULT true,
					load_balancing_enabled BOOLEAN NOT NULL DEFAULT false,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create provider_models table
			if err := tx.Exec(`
				CREATE TABLE provider_models (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					provider_name VARCHAR(255) NOT NULL,
					model_name VARCHAR(255) NOT NULL,
					model_type VARCHAR(40) NOT NULL,
					encrypted_config TEXT,
					is_valid BOOLEAN NOT NULL DEFAULT false,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create provider_settings table
			if err := tx.Exec(`
				CREATE TABLE provider_settings (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					provider_name VARCHAR(255) NOT NULL,
					enabled BOOLEAN NOT NULL DEFAULT true,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create providers table
			if err := tx.Exec(`
				CREATE TABLE providers (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					provider_name VARCHAR(255) NOT NULL,
					provider_type VARCHAR(40) NOT NULL DEFAULT 'custom',
					encrypted_config TEXT,
					is_valid BOOLEAN NOT NULL DEFAULT false,
					last_used TIMESTAMP,
					quota_type VARCHAR(40) DEFAULT '',
					quota_limit BIGINT,
					quota_used BIGINT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create tenant_default_models table
			if err := tx.Exec(`
				CREATE TABLE tenant_default_models (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					provider_name VARCHAR(255) NOT NULL,
					model_name VARCHAR(255) NOT NULL,
					model_type VARCHAR(40) NOT NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create tenant_preferred_model_providers table
			if err := tx.Exec(`
				CREATE TABLE tenant_preferred_model_providers (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					provider_name VARCHAR(255) NOT NULL,
					preferred_provider_type VARCHAR(40) NOT NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes and constraints
			statements := []string{
				// load_balancing_model_configs indexes
				`CREATE INDEX load_balancing_model_config_tenant_provider_model_idx ON load_balancing_model_configs(tenant_id, provider_name, model_name)`,

				// plugin_installations indexes
				`CREATE INDEX idx_plugin_installations_tenant_id ON plugin_installations(tenant_id)`,
				`CREATE INDEX idx_plugin_installations_tenant_plugin ON plugin_installations(tenant_id, plugin_unique_identifier)`,

				// provider_model_settings indexes
				`CREATE INDEX provider_model_setting_tenant_provider_model_idx ON provider_model_settings(tenant_id, provider_name, model_name)`,

				// provider_models indexes and constraints
				`CREATE INDEX provider_model_tenant_id_provider_idx ON provider_models(tenant_id, provider_name)`,
				`ALTER TABLE provider_models ADD CONSTRAINT unique_provider_model_name UNIQUE (tenant_id, provider_name, model_name, model_type)`,

				// provider_settings indexes
				`CREATE INDEX provider_setting_tenant_provider_idx ON provider_settings(tenant_id, provider_name)`,

				// providers indexes and constraints
				`CREATE INDEX provider_tenant_id_provider_idx ON providers(tenant_id, provider_name)`,
				`ALTER TABLE providers ADD CONSTRAINT unique_provider_name_type_quota UNIQUE (tenant_id, provider_name, provider_type, quota_type)`,

				// tenant_default_models indexes
				`CREATE INDEX tenant_default_model_tenant_id_provider_type_idx ON tenant_default_models(tenant_id, provider_name, model_type)`,

				// tenant_preferred_model_providers indexes
				`CREATE INDEX tenant_preferred_model_provider_tenant_provider_idx ON tenant_preferred_model_providers(tenant_id, provider_name)`,
			}

			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			tables := []string{
				"tenant_preferred_model_providers",
				"tenant_default_models",
				"providers",
				"provider_settings",
				"provider_models",
				"provider_model_settings",
				"plugin_installations",
				"load_balancing_model_configs",
			}
			for _, table := range tables {
				if err := tx.Migrator().DropTable(table); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
