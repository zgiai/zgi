package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0067_add_model_capabilities adds ModelHub-aligned capability fields to llm_models
func M0067_add_model_capabilities() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "0067_add_model_capabilities",
		Migrate: func(tx *gorm.DB) error {
			// ============================================================
			// llm_models table - add new ModelHub-aligned fields
			// ============================================================
			if err := tx.Exec(`
				-- Basic info fields
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS slug VARCHAR(100);
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS family VARCHAR(100);
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS family_slug VARCHAR(100);
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active';
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS tagline TEXT;
				
				-- Flag fields
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS is_flagship BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS is_featured BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS is_new BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS access_type VARCHAR(20) DEFAULT 'closed';
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS currency VARCHAR(10) DEFAULT 'USD';
				
				-- Capability JSONB fields (ModelHub-aligned nested structures)
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS endpoints JSONB DEFAULT '{}';
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS features JSONB DEFAULT '{}';
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS tools JSONB DEFAULT '{}';
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS parameters JSONB DEFAULT '{}';
				
				-- Create indexes
				CREATE INDEX IF NOT EXISTS idx_llm_models_slug ON llm_models(slug);
				CREATE INDEX IF NOT EXISTS idx_llm_models_family ON llm_models(family);
				CREATE INDEX IF NOT EXISTS idx_llm_models_status ON llm_models(status);
				CREATE INDEX IF NOT EXISTS idx_llm_models_is_flagship ON llm_models(is_flagship);
			`).Error; err != nil {
				return err
			}

			// ============================================================
			// llm_tenant_custom_models table - add same fields
			// ============================================================
			if err := tx.Exec(`
				-- Basic info fields
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS slug VARCHAR(100);
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS family VARCHAR(100);
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS family_slug VARCHAR(100);
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS status VARCHAR(20) DEFAULT 'active';
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS tagline TEXT;
				
				-- Flag fields
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS is_flagship BOOLEAN DEFAULT false;
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS is_featured BOOLEAN DEFAULT false;
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS is_new BOOLEAN DEFAULT false;
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS access_type VARCHAR(20) DEFAULT 'closed';
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS currency VARCHAR(10) DEFAULT 'USD';
				
				-- Capability JSONB fields
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS endpoints JSONB DEFAULT '{}';
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS features JSONB DEFAULT '{}';
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS tools JSONB DEFAULT '{}';
				ALTER TABLE llm_tenant_custom_models ADD COLUMN IF NOT EXISTS parameters JSONB DEFAULT '{}';
			`).Error; err != nil {
				return err
			}

			// Set default slug values from name
			tx.Exec(`UPDATE llm_models SET slug = name WHERE slug IS NULL OR slug = ''`)
			tx.Exec(`UPDATE llm_tenant_custom_models SET slug = name WHERE slug IS NULL OR slug = ''`)

			// Add comments
			tx.Exec(`COMMENT ON COLUMN llm_models.slug IS 'URL-friendly model identifier'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.family IS 'Model family (e.g., GPT-4, Claude)'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.status IS 'Model status: active, deprecated'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.endpoints IS 'Supported API endpoints (JSONB)'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.features IS 'Model features/capabilities (JSONB)'`)
			tx.Exec(`COMMENT ON COLUMN llm_models.tools IS 'Supported tools (JSONB)'`)

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop columns from llm_models
			columns := []string{
				"slug", "family", "family_slug", "status", "tagline",
				"is_flagship", "is_featured", "is_new", "access_type", "currency",
				"endpoints", "features", "tools", "parameters",
			}
			for _, col := range columns {
				tx.Exec("ALTER TABLE llm_models DROP COLUMN IF EXISTS " + col)
				tx.Exec("ALTER TABLE llm_tenant_custom_models DROP COLUMN IF EXISTS " + col)
			}
			return nil
		},
	}
}
