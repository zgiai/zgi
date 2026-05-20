package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0095_align_models_with_modelmeta_v2 aligns llm_models table with ModelMeta API standard (V2 - Revised)
// This migration:
// - Adds 7 new fields (FamilyName, ParentID, FamilyDefault, CachedInputPrice, Videos, ImageEdit, Translation)
// - Removes 7 fields (Slug, FamilySlug, ReleaseDate, LastUpdated, Attachment, CostImage, CostAudio)
// - Keeps CostRate (billing core), IsFinetuned (business value)
// - Keeps Tokenizer/InstructType in Architecture structure (upstream API metadata)
func M0095_align_models_with_modelmeta_v2() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260127000095",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add new fields for ModelMeta alignment
			if err := tx.Exec(`
				-- Basic fields
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS family_name VARCHAR(200),
				ADD COLUMN IF NOT EXISTS parent_id UUID,
				ADD COLUMN IF NOT EXISTS family_default BOOLEAN DEFAULT false;

				-- Pricing field
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS cached_input_price DECIMAL(10,4);

				-- Endpoints fields
				ALTER TABLE llm_models 
				ADD COLUMN IF NOT EXISTS videos BOOLEAN DEFAULT false,
				ADD COLUMN IF NOT EXISTS image_edit BOOLEAN DEFAULT false,
				ADD COLUMN IF NOT EXISTS translation BOOLEAN DEFAULT false;

				-- Create index for parent_id
				CREATE INDEX IF NOT EXISTS idx_models_parent_id ON llm_models(parent_id);

				-- Add comments for new fields
				COMMENT ON COLUMN llm_models.family_name IS 'Model family display name (e.g., "GPT-4o")';
				COMMENT ON COLUMN llm_models.parent_id IS 'Parent model ID for version relationships';
				COMMENT ON COLUMN llm_models.family_default IS 'Whether this is the default model in its family';
				COMMENT ON COLUMN llm_models.cached_input_price IS 'Price per million cached input tokens (USD)';
				COMMENT ON COLUMN llm_models.videos IS 'Whether model supports video processing';
				COMMENT ON COLUMN llm_models.image_edit IS 'Whether model supports image editing';
				COMMENT ON COLUMN llm_models.translation IS 'Whether model supports translation';
			`).Error; err != nil {
				return err
			}

			// 2. Remove unused/redundant fields (V2 - only 7 fields)
			if err := tx.Exec(`
				-- Remove SEO fields (can use Name/Family instead)
				ALTER TABLE llm_models DROP COLUMN IF EXISTS slug;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS family_slug;

				-- Remove time fields (no business dependency)
				ALTER TABLE llm_models DROP COLUMN IF EXISTS release_date;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS last_updated;

				-- Remove redundant capability field (use vision || audio instead)
				ALTER TABLE llm_models DROP COLUMN IF EXISTS attachment;

				-- Remove unused cost fields
				ALTER TABLE llm_models DROP COLUMN IF EXISTS cost_image;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS cost_audio;
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback: Remove new fields and restore old ones
			if err := tx.Exec(`
				-- Remove new fields
				ALTER TABLE llm_models DROP COLUMN IF EXISTS family_name;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS parent_id;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS family_default;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS cached_input_price;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS videos;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS image_edit;
				ALTER TABLE llm_models DROP COLUMN IF EXISTS translation;

				-- Remove index
				DROP INDEX IF EXISTS idx_models_parent_id;

				-- Restore old fields (with default values)
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS slug VARCHAR(100);
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS family_slug VARCHAR(100);
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS release_date DATE;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS last_updated DATE;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS attachment BOOLEAN DEFAULT false;
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS cost_image DECIMAL(10,4);
				ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS cost_audio DECIMAL(10,4);
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
