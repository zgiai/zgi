package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0113_add_dataset_segmentation_method() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260213000200",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE datasets 
				ADD COLUMN IF NOT EXISTS segmentation_method VARCHAR(50) DEFAULT 'parent_child';
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE datasets 
				ADD COLUMN IF NOT EXISTS process_rule JSONB;
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_datasets_segmentation_method ON datasets(segmentation_method);
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				UPDATE datasets d
				SET segmentation_method = COALESCE(
					(SELECT (r.rules::jsonb)->>'parent_mode' 
					 FROM dataset_process_rules r 
					 WHERE r.dataset_id = d.id 
					 ORDER BY r.created_at DESC 
					 LIMIT 1),
					'parent_child'
				)
				WHERE d.segmentation_method IS NULL OR d.segmentation_method = 'parent_child';
			`).Error; err != nil {
				return err
			}

			// For the process_rule update, we need to handle potential type mismatch if rules is text
			// but target process_rule is jsonb.
			if err := tx.Exec(`
				UPDATE datasets d
				SET process_rule = (
					SELECT r.rules::jsonb 
					FROM dataset_process_rules r 
					WHERE r.dataset_id = d.id 
					ORDER BY r.created_at DESC 
					LIMIT 1
				)
				WHERE d.process_rule IS NULL;
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_datasets_segmentation_method`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE datasets DROP COLUMN IF EXISTS segmentation_method`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE datasets DROP COLUMN IF EXISTS process_rule`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
