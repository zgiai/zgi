package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0014_llm_system creates all LLM system tables in a single migration
func M0014_llm_system() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251124000000",
		Migrate: func(tx *gorm.DB) error {
			// Execute all table creations
			// 1. Providers and models
			if err := createProvidersAndModelsTables(tx); err != nil {
				return err
			}
			// 2. Tenant providers and models configuration
			if err := createTenantConfigTables(tx); err != nil {
				return err
			}
			// 3. Tenant Channels
			if err := createTenantChannelsTables(tx); err != nil {
				return err
			}
			// 4. Wallet system
			if err := createWalletTables(tx); err != nil {
				return err
			}
			// 5. API Keys
			if err := createAPIKeysTables(tx); err != nil {
				return err
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop tables in reverse order (respecting foreign key dependencies)
			tables := []string{
				"llm_tenant_api_keys",
				"llm_tenant_channels",
				"llm_tenant_models",
				"llm_tenant_providers",
				"llm_models",
				"llm_providers",
				"llm_redeem_usages",
				"llm_redeem_codes",
				"llm_tenant_transactions",
				"llm_tenant_balances",
			}
			for _, table := range tables {
				if err := tx.Exec("DROP TABLE IF EXISTS " + table + " CASCADE").Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}

// createWalletTables creates wallet system tables (from m0013 and m0018)
func createWalletTables(tx *gorm.DB) error {
	// Create llm_tenant_balances table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_balances (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			balance NUMERIC(20,8) NOT NULL DEFAULT 0,
			currency VARCHAR(10) NOT NULL DEFAULT 'USD',
			frozen_balance NUMERIC(20,8) NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMPTZ,
			CONSTRAINT chk_balance_non_negative CHECK (balance >= 0),
			CONSTRAINT chk_frozen_balance_non_negative CHECK (frozen_balance >= 0),
			CONSTRAINT chk_frozen_not_exceed_balance CHECK (frozen_balance <= balance)
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_balances
	balanceIndexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_balances_unique ON llm_tenant_balances(tenant_id, currency) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_balances_tenant_id ON llm_tenant_balances(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_balances_updated_at ON llm_tenant_balances(updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_balances_deleted_at ON llm_tenant_balances(deleted_at)`,
	}
	for _, idx := range balanceIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	// Create llm_tenant_transactions table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_transactions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			transaction_type VARCHAR(50) NOT NULL,
			amount NUMERIC(20,8) NOT NULL,
			balance_before NUMERIC(20,8) NOT NULL,
			balance_after NUMERIC(20,8) NOT NULL,
			currency VARCHAR(10) NOT NULL DEFAULT 'USD',
			reference_type VARCHAR(50),
			reference_id UUID,
			description TEXT,
			metadata JSONB DEFAULT '{}',
			created_by UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT chk_transaction_type CHECK (
				transaction_type IN ('recharge', 'consume', 'redeem', 'refund', 'adjustment', 'freeze', 'unfreeze')
			)
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_transactions
	transactionIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_tenant_transactions_tenant_id ON llm_tenant_transactions(tenant_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_transactions_type ON llm_tenant_transactions(transaction_type, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_transactions_created_at ON llm_tenant_transactions(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_transactions_reference ON llm_tenant_transactions(reference_type, reference_id)`,
	}
	for _, idx := range transactionIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	// Create llm_redeem_codes table (with UUID id from m0018 fix)
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_redeem_codes (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			code VARCHAR(100) NOT NULL,
			amount NUMERIC(20,8) NOT NULL,
			currency VARCHAR(10) NOT NULL DEFAULT 'TOKEN',
			max_uses INT NOT NULL DEFAULT 1,
			used_count INT NOT NULL DEFAULT 0,
			expires_at TIMESTAMPTZ,
			is_active BOOLEAN NOT NULL DEFAULT true,
			description TEXT,
			source VARCHAR(50),
			created_by UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMPTZ,
			CONSTRAINT chk_redeem_codes_used_count CHECK (used_count <= max_uses),
			CONSTRAINT chk_redeem_codes_max_uses CHECK (max_uses > 0),
			CONSTRAINT chk_redeem_codes_amount CHECK (amount > 0)
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_redeem_codes
	redeemCodeIndexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_redeem_codes_code ON llm_redeem_codes(code)`,
		`CREATE INDEX IF NOT EXISTS idx_redeem_codes_expires_at ON llm_redeem_codes(expires_at) WHERE is_active = true AND deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_redeem_codes_created_by ON llm_redeem_codes(created_by)`,
		`CREATE INDEX IF NOT EXISTS idx_redeem_codes_deleted_at ON llm_redeem_codes(deleted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_redeem_codes_is_active ON llm_redeem_codes(is_active, created_at DESC)`,
	}
	for _, idx := range redeemCodeIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	// Create llm_redeem_usages table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_redeem_usages (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			redeem_code_id UUID NOT NULL,
			tenant_id UUID NOT NULL,
			amount NUMERIC(20,8) NOT NULL,
			currency VARCHAR(10) NOT NULL DEFAULT 'USD',
			ip_address VARCHAR(45),
			user_agent TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT fk_redeem_usages_code
				FOREIGN KEY (redeem_code_id)
				REFERENCES llm_redeem_codes(id)
				ON DELETE RESTRICT
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_redeem_usages
	redeemUsageIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_redeem_usages_redeem_code_id ON llm_redeem_usages(redeem_code_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_redeem_usages_created_at ON llm_redeem_usages(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_redeem_usages_tenant_id ON llm_redeem_usages(tenant_id, created_at DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_redeem_usages_unique_per_tenant ON llm_redeem_usages(redeem_code_id, tenant_id)`,
	}
	for _, idx := range redeemUsageIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	return nil
}

// createAPIKeysTables creates API keys table (from m0014)
func createAPIKeysTables(tx *gorm.DB) error {
	// Create llm_tenant_api_keys table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_api_keys (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL,
			key VARCHAR(48) NOT NULL,
			name VARCHAR(255) NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			accessed_at TIMESTAMPTZ,
			expires_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ,
			used_quota BIGINT NOT NULL DEFAULT 0,
			remain_quota BIGINT NOT NULL DEFAULT 0,
			quota_limit BIGINT,
			model_limits_enabled BOOLEAN NOT NULL DEFAULT false,
			model_limits JSONB,
			allow_ips TEXT NOT NULL DEFAULT '',
			CONSTRAINT chk_llm_tenant_api_keys_status CHECK (status IN ('active', 'inactive', 'revoked')),
			CONSTRAINT chk_llm_tenant_api_keys_quota CHECK (used_quota >= 0 AND remain_quota >= 0),
			CONSTRAINT chk_llm_tenant_api_keys_quota_limit CHECK (quota_limit IS NULL OR quota_limit > 0)
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_api_keys
	apiKeyIndexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_tenant_api_keys_key ON llm_tenant_api_keys(key) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_llm_tenant_api_keys_tenant_id ON llm_tenant_api_keys(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_tenant_api_keys_status ON llm_tenant_api_keys(status)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_tenant_api_keys_expires_at ON llm_tenant_api_keys(expires_at) WHERE expires_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_llm_tenant_api_keys_deleted_at ON llm_tenant_api_keys(deleted_at)`,
	}
	for _, idx := range apiKeyIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	return nil
}

// createProvidersAndModelsTables creates providers and models tables (from m0015)
func createProvidersAndModelsTables(tx *gorm.DB) error {
	// Create llm_providers table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_providers (
			id uuid NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY,
			name VARCHAR(50) NOT NULL UNIQUE,
			display_name VARCHAR(100) NOT NULL,
			api_base_url VARCHAR(255),
			env_keys JSONB,
			npm_package VARCHAR(100),
			documentation_url VARCHAR(255),
			logo_url VARCHAR(255),
			api_key TEXT,
			balance DECIMAL(15, 4) DEFAULT 0.0000,
			currency VARCHAR(10) DEFAULT 'USD',
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMPTZ
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_providers
	providerIndexes := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_name ON llm_providers(name)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_display_name ON llm_providers(display_name)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_active ON llm_providers(is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_currency ON llm_providers(currency)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_deleted_at ON llm_providers(deleted_at)`,
	}
	for _, idx := range providerIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	// Create llm_models table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_models (
			id uuid NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY,
			provider VARCHAR(100) NOT NULL,
			name VARCHAR(100) NOT NULL UNIQUE,
			display_name VARCHAR(200) NOT NULL,
			supports_attachment BOOLEAN DEFAULT false,
			supports_reasoning BOOLEAN DEFAULT false,
			supports_tool_call BOOLEAN DEFAULT false,
			supports_structured_output BOOLEAN DEFAULT false,
			supports_temperature BOOLEAN DEFAULT true,
			knowledge_cutoff VARCHAR(20),
			release_date DATE,
			last_updated DATE,
			input_modalities JSONB,
			output_modalities JSONB,
			open_weights BOOLEAN DEFAULT false,
			input_price DECIMAL(10, 4),
			output_price DECIMAL(10, 4),
			cost_cache_read DECIMAL(10, 4),
			cost_cache_write DECIMAL(10, 4),
			cost_context_over_200k JSONB,
			context_window INTEGER,
			max_output_tokens INTEGER,
			is_active BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMPTZ,
			CONSTRAINT fk_model_provider FOREIGN KEY (provider) 
				REFERENCES llm_providers(name) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_models
	modelIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_model_provider ON llm_models(provider)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_model_name ON llm_models(name)`,
		`CREATE INDEX IF NOT EXISTS idx_model_display_name ON llm_models(display_name)`,
		`CREATE INDEX IF NOT EXISTS idx_model_active ON llm_models(is_active)`,
		`CREATE INDEX IF NOT EXISTS idx_model_reasoning ON llm_models(supports_reasoning)`,
		`CREATE INDEX IF NOT EXISTS idx_model_tool_call ON llm_models(supports_tool_call)`,
		`CREATE INDEX IF NOT EXISTS idx_model_release_date ON llm_models(release_date)`,
		`CREATE INDEX IF NOT EXISTS idx_model_deleted_at ON llm_models(deleted_at)`,
	}
	for _, idx := range modelIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	return nil
}

// createTenantConfigTables creates tenant provider and model configuration tables (from m0016)
func createTenantConfigTables(tx *gorm.DB) error {
	// Create llm_tenant_providers table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_providers (
			id uuid NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY,
			tenant_id uuid NOT NULL,
			provider VARCHAR(100) NOT NULL,
			is_enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMPTZ,
			CONSTRAINT fk_tenant_provider_tenant FOREIGN KEY (tenant_id) 
				REFERENCES tenants(id) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE,
			CONSTRAINT fk_tenant_provider_provider FOREIGN KEY (provider) 
				REFERENCES llm_providers(name) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE,
			CONSTRAINT uq_tenant_provider UNIQUE (tenant_id, provider)
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_providers
	tenantProviderIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_tenant_provider_tenant ON llm_tenant_providers(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_provider_provider ON llm_tenant_providers(provider)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_provider_enabled ON llm_tenant_providers(is_enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_provider_deleted_at ON llm_tenant_providers(deleted_at)`,
	}
	for _, idx := range tenantProviderIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	// Create llm_tenant_models table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_models (
			id uuid NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY,
			tenant_id uuid NOT NULL,
			provider VARCHAR(100) NOT NULL,
			model VARCHAR(100) NOT NULL,
			is_enabled BOOLEAN DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMPTZ,
			CONSTRAINT fk_tenant_model_tenant FOREIGN KEY (tenant_id) 
				REFERENCES tenants(id) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE,
			CONSTRAINT fk_tenant_model_provider FOREIGN KEY (provider) 
				REFERENCES llm_providers(name) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE,
			CONSTRAINT fk_tenant_model_model FOREIGN KEY (model) 
				REFERENCES llm_models(name) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE,
			CONSTRAINT uq_tenant_model UNIQUE (tenant_id, model)
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_models
	tenantModelIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_tenant ON llm_tenant_models(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_provider ON llm_tenant_models(provider)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_model ON llm_tenant_models(model)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_enabled ON llm_tenant_models(is_enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_deleted_at ON llm_tenant_models(deleted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_model_tenant_provider ON llm_tenant_models(tenant_id, provider)`,
	}
	for _, idx := range tenantModelIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	return nil
}

// createTenantChannelsTables creates tenant channels table (from m0017)
func createTenantChannelsTables(tx *gorm.DB) error {
	// Create llm_tenant_channels table
	if err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS llm_tenant_channels (
			id uuid NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY,
			tenant_id uuid,
			name VARCHAR(255),
			provider VARCHAR(100),
			models JSONB NOT NULL DEFAULT '[]'::jsonb,
			api_key TEXT,
			api_base_url VARCHAR(500) NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
			weight INTEGER NOT NULL DEFAULT 1,
			model_maps JSONB NOT NULL DEFAULT '{}'::jsonb,
			param_override JSONB NOT NULL DEFAULT '{}'::jsonb,
			header_override JSONB NOT NULL DEFAULT '{}'::jsonb,
			status_code_maps JSONB NOT NULL DEFAULT '{}'::jsonb,
			tags JSONB NOT NULL DEFAULT '[]'::jsonb,
			auto_ban BOOLEAN NOT NULL DEFAULT false,
			is_enabled BOOLEAN NOT NULL DEFAULT true,
			balance DECIMAL(15, 4) NOT NULL DEFAULT 0.0000,
			currency VARCHAR(10) NOT NULL DEFAULT 'USD',
			created_at TIMESTAMPTZ,
			updated_at TIMESTAMPTZ,
			deleted_at TIMESTAMPTZ,
			CONSTRAINT fk_tenant_channel_tenant FOREIGN KEY (tenant_id) 
				REFERENCES tenants(id) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE
		)
	`).Error; err != nil {
		return err
	}

	// Create indexes for llm_tenant_channels
	channelIndexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_tenant ON llm_tenant_channels(tenant_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_provider ON llm_tenant_channels(provider)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_priority ON llm_tenant_channels(priority DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_weight ON llm_tenant_channels(weight)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_auto_ban ON llm_tenant_channels(auto_ban)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_is_enabled ON llm_tenant_channels(is_enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_deleted_at ON llm_tenant_channels(deleted_at)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_tags ON llm_tenant_channels USING GIN(tags)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_models ON llm_tenant_channels USING GIN(models)`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_channel_tenant_provider ON llm_tenant_channels(tenant_id, provider)`,
	}
	for _, idx := range channelIndexes {
		if err := tx.Exec(idx).Error; err != nil {
			return err
		}
	}

	return nil
}
