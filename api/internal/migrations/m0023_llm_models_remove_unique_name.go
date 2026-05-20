package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0023_llm_models_remove_unique_name removes the unique constraint on llm_models name
// and updates related constraints in llm_tenant_models
func M0023_llm_models_remove_unique_name() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251128000001",
		Migrate: func(tx *gorm.DB) error {
			// 1. First drop dependent constraints in llm_tenant_models
			// Drop existing constraints that rely on unique name
			if err := tx.Exec(`ALTER TABLE llm_tenant_models DROP CONSTRAINT IF EXISTS fk_tenant_model_model`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_tenant_models DROP CONSTRAINT IF EXISTS uq_tenant_model`).Error; err != nil {
				return err
			}

			// 2. Now drop llm_models constraints
			// Drop the unique index on name
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_model_name`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_models DROP CONSTRAINT IF EXISTS llm_models_name_key`).Error; err != nil {
				return err
			}
			// Add composite unique index on provider and name
			if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_model_provider_name ON llm_models(provider, name)`).Error; err != nil {
				return err
			}

			// 3. Continue with llm_tenant_models modifications

			// Drop the new constraint if it exists (from partial migration)
			if err := tx.Exec(`ALTER TABLE llm_tenant_models DROP CONSTRAINT IF EXISTS uq_tenant_provider_model`).Error; err != nil {
				return err
			}

			// Create new composite unique constraint for tenant models
			if err := tx.Exec(`ALTER TABLE llm_tenant_models ADD CONSTRAINT uq_tenant_provider_model UNIQUE (tenant_id, provider, model)`).Error; err != nil {
				return err
			}

			// Clean up orphaned records before adding foreign key
			// Delete llm_tenant_models records where (provider, model) doesn't exist in llm_models
			if err := tx.Exec(`
				DELETE FROM llm_tenant_models 
				WHERE NOT EXISTS (
					SELECT 1 FROM llm_models 
					WHERE llm_models.provider = llm_tenant_models.provider 
					AND llm_models.name = llm_tenant_models.model
				)
			`).Error; err != nil {
				return err
			}

			// Add composite foreign key
			// Note: We assume provider and model columns in llm_tenant_models match provider and name in llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_models 
				ADD CONSTRAINT fk_tenant_model_provider_model 
				FOREIGN KEY (provider, model) 
				REFERENCES llm_models (provider, name) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Revert llm_tenant_models changes
			if err := tx.Exec(`ALTER TABLE llm_tenant_models DROP CONSTRAINT IF EXISTS fk_tenant_model_provider_model`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE llm_tenant_models DROP CONSTRAINT IF EXISTS uq_tenant_provider_model`).Error; err != nil {
				return err
			}

			// Warning: This rollback might fail if there are duplicate model names across providers
			// We attempt to restore the old state but data might prevent it

			// Restore llm_models changes
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_model_provider_name`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_model_name ON llm_models(name)`).Error; err != nil {
				return err
			}

			// Restore llm_tenant_models constraints
			if err := tx.Exec(`ALTER TABLE llm_tenant_models ADD CONSTRAINT uq_tenant_model UNIQUE (tenant_id, model)`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_models 
				ADD CONSTRAINT fk_tenant_model_model 
				FOREIGN KEY (model) 
				REFERENCES llm_models (name) 
				ON DELETE CASCADE 
				ON UPDATE CASCADE
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
