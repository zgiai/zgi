package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const routeNativeProtocolsTable = "llm_routes"

func M0028_add_route_native_protocols() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddRouteNativeProtocolsID,
		Migrate: func(tx *gorm.DB) error {
			exists, err := hasExactColumnV2(tx, routeNativeProtocolsTable, "native_protocols")
			if err != nil {
				return fmt.Errorf("check llm_routes.native_protocols: %w", err)
			}
			if exists {
				return nil
			}

			columnDefinition := "JSONB NOT NULL DEFAULT '{}'::jsonb"
			if tx.Dialector.Name() == "sqlite" {
				columnDefinition = "TEXT NOT NULL DEFAULT '{}'"
			}

			statement := fmt.Sprintf(
				`ALTER TABLE %s ADD COLUMN native_protocols %s`,
				routeNativeProtocolsTable,
				columnDefinition,
			)
			if err := tx.Exec(statement).Error; err != nil {
				return fmt.Errorf("add llm_routes.native_protocols: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
