package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0028_add_protocol_layer adds protocol layer to support multiple API protocols
func M0028_add_protocol_layer() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251207000028",
		Migrate: func(tx *gorm.DB) error {
			// 1. Create llm_protocols table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_protocols (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					name VARCHAR(50) NOT NULL UNIQUE,
					display_name VARCHAR(100) NOT NULL,
					version VARCHAR(20),
					description TEXT,
					adapter_type VARCHAR(50) NOT NULL,
					schema_url VARCHAR(255),
					is_active BOOLEAN DEFAULT true,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMPTZ
				)
			`).Error; err != nil {
				return err
			}

			// 2. Create indexes for llm_protocols
			if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_protocol_name ON llm_protocols(name)`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_protocol_active ON llm_protocols(is_active)`).Error; err != nil {
				return err
			}

			// 3. Create llm_provider_protocols table (many-to-many)
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_provider_protocols (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					provider_id UUID NOT NULL REFERENCES llm_providers(id) ON DELETE CASCADE,
					protocol_id UUID NOT NULL REFERENCES llm_protocols(id) ON DELETE CASCADE,
					base_url VARCHAR(255),
					is_default BOOLEAN DEFAULT false,
					priority INT DEFAULT 0,
					config JSONB DEFAULT '{}',
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(provider_id, protocol_id)
				)
			`).Error; err != nil {
				return err
			}

			// 4. Create indexes for llm_provider_protocols
			if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_provider_protocol ON llm_provider_protocols(provider_id, protocol_id)`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_provider_protocol_default ON llm_provider_protocols(provider_id, is_default)`).Error; err != nil {
				return err
			}

			// 5. Insert standard protocols
			protocols := []struct {
				Name        string
				DisplayName string
				Version     string
				AdapterType string
				Description string
			}{
				{
					Name:        "openai",
					DisplayName: "OpenAI Standard",
					Version:     "v1",
					AdapterType: "openai",
					Description: "OpenAI API standard protocol, compatible with many providers",
				},
				{
					Name:        "anthropic",
					DisplayName: "Claude API",
					Version:     "2023-06-01",
					AdapterType: "anthropic",
					Description: "Anthropic Claude API protocol",
				},
				{
					Name:        "google",
					DisplayName: "Google AI",
					Version:     "v1",
					AdapterType: "google",
					Description: "Google Gemini API protocol",
				},
			}

			for _, p := range protocols {
				if err := tx.Exec(`
					INSERT INTO llm_protocols (name, display_name, version, adapter_type, description, is_active)
					VALUES (?, ?, ?, ?, ?, true)
					ON CONFLICT (name) DO NOTHING
				`, p.Name, p.DisplayName, p.Version, p.AdapterType, p.Description).Error; err != nil {
					return err
				}
			}

			// 6. Migrate existing providers to use protocol layer
			// For providers with openai_compatible=true, link them to openai protocol
			if err := tx.Exec(`
				INSERT INTO llm_provider_protocols (provider_id, protocol_id, base_url, is_default, priority)
				SELECT
					p.id,
					(SELECT id FROM llm_protocols WHERE name = 'openai'),
					p.api_base_url,
					true,
					1
				FROM llm_providers p
				WHERE p.openai_compatible = true
				ON CONFLICT (provider_id, protocol_id) DO NOTHING
			`).Error; err != nil {
				return err
			}

			// 7. Add protocol field to llm_tenant_channels (optional, for explicit protocol selection)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_channels
				ADD COLUMN IF NOT EXISTS protocol VARCHAR(50)
			`).Error; err != nil {
				return err
			}

			// 8. Create index for protocol in channels
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_tenant_channel_protocol
				ON llm_tenant_channels(protocol)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP TABLE IF EXISTS llm_provider_protocols`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS llm_protocols`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_tenant_channels DROP COLUMN IF EXISTS protocol`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
