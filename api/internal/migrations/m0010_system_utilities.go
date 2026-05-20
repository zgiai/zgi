package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0010_system_utilities creates system utility tables
func M0010_system_utilities() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100900",
		Migrate: func(tx *gorm.DB) error {
			// Create sites table
			if err := tx.Exec(`
				CREATE TABLE sites (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					app_id UUID NOT NULL,
					title VARCHAR(255) NOT NULL,
					icon VARCHAR(255),
					icon_background VARCHAR(255),
					description TEXT,
					default_language VARCHAR(255) NOT NULL,
					copyright VARCHAR(255),
					privacy_policy VARCHAR(255),
					customize_domain VARCHAR(255),
					customize_token_strategy VARCHAR(255) NOT NULL,
					prompt_public BOOLEAN NOT NULL DEFAULT false,
					status VARCHAR(255) NOT NULL DEFAULT 'normal',
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					code VARCHAR(255),
					custom_disclaimer TEXT NOT NULL,
					show_workflow_steps BOOLEAN NOT NULL DEFAULT true,
					chat_color_theme VARCHAR(255),
					chat_color_theme_inverted BOOLEAN NOT NULL DEFAULT false,
					icon_type VARCHAR(255),
					created_by UUID,
					updated_by UUID,
					use_icon_as_answer_icon BOOLEAN NOT NULL DEFAULT false,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create tool_files table
			if err := tx.Exec(`
				CREATE TABLE tool_files (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					user_id UUID NOT NULL,
					tenant_id UUID NOT NULL,
					conversation_id UUID,
					file_key VARCHAR(255) NOT NULL,
					mimetype VARCHAR(255) NOT NULL,
					original_url VARCHAR(2048),
					name VARCHAR NOT NULL,
					size INTEGER NOT NULL,
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create tool_workflow_providers table
			if err := tx.Exec(`
				CREATE TABLE tool_workflow_providers (
					id UUID NOT NULL DEFAULT uuid_generate_v4(),
					name VARCHAR(40) NOT NULL,
					icon VARCHAR(255) NOT NULL,
					app_id UUID NOT NULL,
					user_id UUID NOT NULL,
					tenant_id UUID NOT NULL,
					description TEXT NOT NULL,
					parameter_configuration TEXT NOT NULL DEFAULT '[]',
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					privacy_policy VARCHAR(255) DEFAULT '',
					version VARCHAR(255) NOT NULL DEFAULT '',
					label VARCHAR(255) NOT NULL DEFAULT '',
					PRIMARY KEY (id)
				)
			`).Error; err != nil {
				return err
			}

			// Create zgi_setups table
			if err := tx.Exec(`
				CREATE TABLE zgi_setups (
					version VARCHAR(255) NOT NULL,
					setup_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (version)
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes
			indexes := []string{
				`CREATE INDEX site_app_id_idx ON sites(app_id)`,
				`CREATE INDEX site_code_idx ON sites(code)`,
				`CREATE INDEX tool_file_conversation_id_idx ON tool_files(conversation_id)`,
				`CREATE UNIQUE INDEX unique_workflow_tool_provider ON tool_workflow_providers(name, tenant_id)`,
				`CREATE UNIQUE INDEX unique_workflow_tool_provider_app_id ON tool_workflow_providers(tenant_id, app_id)`,
			}

			for _, indexSQL := range indexes {
				if err := tx.Exec(indexSQL).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(
				"sites",
				"tool_files",
				"tool_workflow_providers",
				"zgi_setups",
			)
		},
	}
}
