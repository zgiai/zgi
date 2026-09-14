package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
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
	if !db.Migrator().HasTable(&channelmodel.LLMRoute{}) || db.Migrator().HasColumn(&channelmodel.LLMRoute{}, "model_operations") {
		return nil
	}
	return db.Migrator().AddColumn(&channelmodel.LLMRoute{}, "ModelOperations")
}
