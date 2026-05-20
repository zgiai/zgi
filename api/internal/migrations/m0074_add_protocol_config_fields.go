package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0074_AddProtocolConfigFields adds protocol configuration and health monitoring fields
func M0074_AddProtocolConfigFields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260108000074",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add protocol_config to llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS protocol_config JSONB DEFAULT '[]'::jsonb
			`).Error; err != nil {
				return err
			}

			// 2. Add protocol and health fields to llm_system_channels
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels 
				ADD COLUMN IF NOT EXISTS protocol VARCHAR(50),
				ADD COLUMN IF NOT EXISTS fallback_protocol VARCHAR(50),
				ADD COLUMN IF NOT EXISTS protocol_source VARCHAR(20) DEFAULT 'default',
				ADD COLUMN IF NOT EXISTS success_count INT DEFAULT 0,
				ADD COLUMN IF NOT EXISTS failure_count INT DEFAULT 0,
				ADD COLUMN IF NOT EXISTS avg_latency_ms INT DEFAULT 0,
				ADD COLUMN IF NOT EXISTS last_health_check_at TIMESTAMP
			`).Error; err != nil {
				return err
			}

			// 3. Add protocol and health fields to llm_tenant_routes
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ADD COLUMN IF NOT EXISTS protocol VARCHAR(50),
				ADD COLUMN IF NOT EXISTS fallback_protocol VARCHAR(50),
				ADD COLUMN IF NOT EXISTS protocol_source VARCHAR(20) DEFAULT 'default',
				ADD COLUMN IF NOT EXISTS success_count INT DEFAULT 0,
				ADD COLUMN IF NOT EXISTS failure_count INT DEFAULT 0,
				ADD COLUMN IF NOT EXISTS avg_latency_ms INT DEFAULT 0,
				ADD COLUMN IF NOT EXISTS last_health_check_at TIMESTAMP
			`).Error; err != nil {
				return err
			}

			// 4. Add protocol field to llm_providers for provider-protocol mapping
			if err := tx.Exec(`
				ALTER TABLE llm_providers 
				ADD COLUMN IF NOT EXISTS protocol VARCHAR(50),
				ADD COLUMN IF NOT EXISTS fallback_protocol VARCHAR(50)
			`).Error; err != nil {
				return err
			}

			// 5. Seed provider-protocol mappings with comprehensive data
			// This ensures all existing providers have protocol configuration
			if err := tx.Exec(`
			-- First, set protocol = name for all providers (safe default)
			UPDATE llm_providers SET protocol = name WHERE protocol IS NULL;
			
			-- Then, set fallback_protocol for non-OpenAI providers
			UPDATE llm_providers SET fallback_protocol = 'openai' 
			WHERE name != 'openai' AND fallback_protocol IS NULL;
			
			-- Special cases: providers that use different protocol names
			-- (Add more as needed when new providers are added)
			UPDATE llm_providers SET protocol = 'openai' WHERE name IN ('azure', 'openrouter');
			UPDATE llm_providers SET protocol = 'anthropic' WHERE name = 'claude';
			
			-- Providers with no fallback (they are the standard)
			UPDATE llm_providers SET fallback_protocol = NULL WHERE name IN ('openai');
		`).Error; err != nil {
				return err
			}

			// 6. Create indexes for performance
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_system_channels_protocol 
				ON llm_system_channels(protocol) WHERE protocol IS NOT NULL;
				
				CREATE INDEX IF NOT EXISTS idx_tenant_routes_protocol 
				ON llm_tenant_routes(protocol) WHERE protocol IS NOT NULL;
				
				CREATE INDEX IF NOT EXISTS idx_system_channels_health 
				ON llm_system_channels(success_count, failure_count) 
				WHERE success_count > 0 OR failure_count > 0;
				
				CREATE INDEX IF NOT EXISTS idx_tenant_routes_health 
				ON llm_tenant_routes(success_count, failure_count) 
				WHERE success_count > 0 OR failure_count > 0
			`).Error; err != nil {
				return err
			}

			// 5. Add comment for documentation
			if err := tx.Exec(`
				COMMENT ON COLUMN llm_models.protocol_config IS 'Protocol configuration array in JSONB format';
				COMMENT ON COLUMN llm_system_channels.protocol IS 'Current active protocol for this channel';
				COMMENT ON COLUMN llm_system_channels.fallback_protocol IS 'Fallback protocol if primary fails';
				COMMENT ON COLUMN llm_system_channels.protocol_source IS 'Source of protocol: default or user';
				COMMENT ON COLUMN llm_system_channels.success_count IS 'Number of successful requests (rolling window)';
				COMMENT ON COLUMN llm_system_channels.failure_count IS 'Number of failed requests (rolling window)';
				COMMENT ON COLUMN llm_system_channels.avg_latency_ms IS 'Average latency in milliseconds';
				COMMENT ON COLUMN llm_system_channels.last_health_check_at IS 'Timestamp of last health check'
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop indexes
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_system_channels_protocol;
				DROP INDEX IF EXISTS idx_tenant_routes_protocol;
				DROP INDEX IF EXISTS idx_system_channels_health;
				DROP INDEX IF EXISTS idx_tenant_routes_health
			`).Error; err != nil {
				return err
			}

			// Drop columns from llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_models 
				DROP COLUMN IF EXISTS protocol_config
			`).Error; err != nil {
				return err
			}

			// Drop columns from llm_system_channels
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels 
				DROP COLUMN IF EXISTS protocol,
				DROP COLUMN IF EXISTS fallback_protocol,
				DROP COLUMN IF EXISTS protocol_source,
				DROP COLUMN IF EXISTS success_count,
				DROP COLUMN IF EXISTS failure_count,
				DROP COLUMN IF EXISTS avg_latency_ms,
				DROP COLUMN IF EXISTS last_health_check_at
			`).Error; err != nil {
				return err
			}

			// Drop columns from llm_tenant_routes
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				DROP COLUMN IF EXISTS protocol,
				DROP COLUMN IF EXISTS fallback_protocol,
				DROP COLUMN IF EXISTS protocol_source,
				DROP COLUMN IF EXISTS success_count,
				DROP COLUMN IF EXISTS failure_count,
				DROP COLUMN IF EXISTS avg_latency_ms,
				DROP COLUMN IF EXISTS last_health_check_at
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
