package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0136_add_catalog_sync_state() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260326000136",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_catalog_sync_states (
					sync_key VARCHAR(50) PRIMARY KEY,
					last_applied_version BIGINT NOT NULL DEFAULT 0,
					last_applied_at TIMESTAMPTZ NULL,
					last_error TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS llm_catalog_sync_states`).Error
		},
	}
}
