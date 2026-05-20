package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0012_add_seed_executions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddSeedExecutionsID,
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS seed_executions (
					name varchar(100) NOT NULL,
					version varchar(50) NOT NULL,
					executed_at timestamptz NOT NULL DEFAULT now(),
					executed_by varchar(50) NOT NULL DEFAULT 'manual',
					status varchar(20) NOT NULL DEFAULT 'success',
					PRIMARY KEY (name, version)
				)
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS seed_executions`).Error
		},
	}
}
