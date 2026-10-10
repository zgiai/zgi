package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migrationAddLLMRouteModelOperationsID = "20260913100000_add_llm_route_model_operations"

func init() {
	registerMigration(&gormigrate.Migration{
		ID:      migrationAddLLMRouteModelOperationsID,
		Migrate: upAddLLMRouteModelOperations,
	})
}

func upAddLLMRouteModelOperations(db *gorm.DB) error {
	if !db.Migrator().HasTable("llm_routes") || db.Migrator().HasColumn("llm_routes", "model_operations") {
		return nil
	}
	// Keep this migration independent from the Go model tag. The repository
	// conditionally hydrates the field so pre-migration databases remain usable.
	return db.Exec(`ALTER TABLE llm_routes ADD COLUMN model_operations JSONB NOT NULL DEFAULT '{}'`).Error
}
