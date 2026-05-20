package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0031_add_channel_architecture creates the new channel architecture tables
// This implements the reference-based routing + credential center design pattern
func M0031_add_channel_architecture() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251207000031",
		Migrate: func(tx *gorm.DB) error {
			// 1. Create credentials table (centralized credential storage)
			if err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS llm_credentials (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				tenant_id UUID,
				name VARCHAR(100),
				provider VARCHAR(50) NOT NULL,
				api_key_ciphertext TEXT NOT NULL,
				api_key_hash VARCHAR(64),
				api_base_url VARCHAR(500),
				is_active BOOLEAN DEFAULT true,
				last_used_at TIMESTAMPTZ,
				expires_at TIMESTAMPTZ,
				metadata JSONB DEFAULT '{}',
				created_at TIMESTAMPTZ DEFAULT NOW(),
				updated_at TIMESTAMPTZ DEFAULT NOW(),
				deleted_at TIMESTAMPTZ
			);

			CREATE INDEX IF NOT EXISTS idx_credential_tenant ON llm_credentials(tenant_id);
			CREATE INDEX IF NOT EXISTS idx_credential_provider ON llm_credentials(provider);
			CREATE INDEX IF NOT EXISTS idx_credential_active ON llm_credentials(is_active);
			CREATE INDEX IF NOT EXISTS idx_credential_hash ON llm_credentials(api_key_hash);
			CREATE INDEX IF NOT EXISTS idx_credential_deleted ON llm_credentials(deleted_at);

			COMMENT ON TABLE llm_credentials IS 'Centralized credential storage for LLM API keys';
			COMMENT ON COLUMN llm_credentials.tenant_id IS 'NULL = system credential, otherwise user credential';
			COMMENT ON COLUMN llm_credentials.api_key_ciphertext IS 'AES-256-GCM encrypted API key';
		`).Error; err != nil {
				return err
			}

			// 2. Create system channels table (admin-managed official channels)
			if err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS llm_system_channels (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				credential_id UUID NOT NULL REFERENCES llm_credentials(id) ON DELETE RESTRICT,
				name VARCHAR(100) NOT NULL,
				provider VARCHAR(50) NOT NULL,
				protocol VARCHAR(50),
				models JSONB DEFAULT '[]',
				api_base_url VARCHAR(500),
				default_priority INT DEFAULT 10,
				default_weight INT DEFAULT 50,
				model_maps JSONB DEFAULT '{}',
				param_override JSONB DEFAULT '{}',
				header_override JSONB DEFAULT '{}',
				tags JSONB DEFAULT '[]',
				description TEXT,
				is_active BOOLEAN DEFAULT true,
				created_at TIMESTAMPTZ DEFAULT NOW(),
				updated_at TIMESTAMPTZ DEFAULT NOW(),
				deleted_at TIMESTAMPTZ
			);

			CREATE INDEX IF NOT EXISTS idx_sys_channel_credential ON llm_system_channels(credential_id);
			CREATE INDEX IF NOT EXISTS idx_sys_channel_provider ON llm_system_channels(provider);
			CREATE INDEX IF NOT EXISTS idx_sys_channel_protocol ON llm_system_channels(protocol);
			CREATE INDEX IF NOT EXISTS idx_sys_channel_active ON llm_system_channels(is_active);
			CREATE INDEX IF NOT EXISTS idx_sys_channel_deleted ON llm_system_channels(deleted_at);

			COMMENT ON TABLE llm_system_channels IS 'System-level official channels managed by administrators';
		`).Error; err != nil {
				return err
			}

			// 3. Create tenant routes table (core routing table for load balancing)
			if err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS llm_tenant_routes (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				tenant_id UUID NOT NULL,
				type VARCHAR(20) NOT NULL,
				system_channel_id UUID REFERENCES llm_system_channels(id) ON DELETE CASCADE,
				user_credential_id UUID REFERENCES llm_credentials(id) ON DELETE CASCADE,
				name VARCHAR(255),
				provider VARCHAR(100),
				protocol VARCHAR(50),
				models JSONB DEFAULT '[]',
				api_base_url VARCHAR(500),
				model_maps JSONB DEFAULT '{}',
				param_override JSONB DEFAULT '{}',
				header_override JSONB DEFAULT '{}',
				priority INT NOT NULL DEFAULT 0,
				weight INT NOT NULL DEFAULT 1,
				is_enabled BOOLEAN NOT NULL DEFAULT true,
				auto_ban BOOLEAN DEFAULT false,
				balance DECIMAL(15,4) DEFAULT 0,
				currency VARCHAR(10) DEFAULT 'USD',
				created_at TIMESTAMPTZ DEFAULT NOW(),
				updated_at TIMESTAMPTZ DEFAULT NOW(),
				deleted_at TIMESTAMPTZ,

				CONSTRAINT chk_route_type CHECK (type IN ('ZGI_CLOUD', 'PRIVATE')),
				CONSTRAINT chk_system_ref CHECK (
					(type = 'ZGI_CLOUD' AND system_channel_id IS NOT NULL) OR
					(type = 'PRIVATE' AND user_credential_id IS NOT NULL)
				)
			);

			-- Primary index for high-frequency queries
			CREATE INDEX IF NOT EXISTS idx_routes_query ON llm_tenant_routes(tenant_id, is_enabled, priority DESC, weight DESC);
			CREATE INDEX IF NOT EXISTS idx_route_tenant ON llm_tenant_routes(tenant_id);
			CREATE INDEX IF NOT EXISTS idx_route_type ON llm_tenant_routes(type);
			CREATE INDEX IF NOT EXISTS idx_route_sys_channel ON llm_tenant_routes(system_channel_id);
			CREATE INDEX IF NOT EXISTS idx_route_usr_cred ON llm_tenant_routes(user_credential_id);
			CREATE INDEX IF NOT EXISTS idx_route_enabled ON llm_tenant_routes(is_enabled);
			CREATE INDEX IF NOT EXISTS idx_route_deleted ON llm_tenant_routes(deleted_at);

			COMMENT ON TABLE llm_tenant_routes IS 'Tenant routing configuration for load balancing';
			COMMENT ON COLUMN llm_tenant_routes.type IS 'ZGI_CLOUD = ZGI official cloud service channel, PRIVATE = user private channel';
		`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE IF EXISTS llm_tenant_routes CASCADE`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS llm_system_channels CASCADE`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS llm_credentials CASCADE`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
