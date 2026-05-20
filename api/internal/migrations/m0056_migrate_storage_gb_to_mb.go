package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0056_migrate_storage_gb_to_mb migrates storage_gb to storage_mb in quota_snapshot
func M0056_migrate_storage_gb_to_mb() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251219000056",
		Migrate: func(tx *gorm.DB) error {
			// Update group_subscriptions.quota_snapshot: storage_gb -> storage_mb (GB to MB conversion)
			// Step 1: Add storage_mb with converted value
			sql := `
				UPDATE group_subscriptions
				SET quota_snapshot = quota_snapshot ||
					jsonb_build_object('storage_mb', CAST((quota_snapshot->>'storage_gb')::numeric * 1024 AS integer))
				WHERE quota_snapshot ? 'storage_gb';
			`
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}

			// Step 2: Remove storage_gb
			sql = `
				UPDATE group_subscriptions
				SET quota_snapshot = quota_snapshot - 'storage_gb'
				WHERE quota_snapshot ? 'storage_gb';
			`
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}

			// Update subscription_plans.quota_config: storage_gb -> storage_mb (if exists)
			// Step 1: Add storage_mb
			sql = `
				UPDATE subscription_plans
				SET quota_config = quota_config ||
					jsonb_build_object('storage_mb', CAST((quota_config->>'storage_gb')::numeric * 1024 AS integer))
				WHERE quota_config ? 'storage_gb';
			`
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}

			// Step 2: Remove storage_gb
			sql = `
				UPDATE subscription_plans
				SET quota_config = quota_config - 'storage_gb'
				WHERE quota_config ? 'storage_gb';
			`
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback: storage_mb -> storage_gb (MB to GB conversion)
			sql := `
				UPDATE group_subscriptions
				SET quota_snapshot = quota_snapshot - 'storage_mb' ||
					jsonb_build_object('storage_gb', CAST((quota_snapshot->>'storage_mb')::numeric / 1024 AS integer))
				WHERE quota_snapshot ? 'storage_mb';
			`
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}

			sql = `
				UPDATE subscription_plans
				SET quota_config = quota_config - 'storage_mb' ||
					jsonb_build_object('storage_gb', CAST((quota_config->>'storage_mb')::numeric / 1024 AS integer))
				WHERE quota_config ? 'storage_mb';
			`
			if err := tx.Exec(sql).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
