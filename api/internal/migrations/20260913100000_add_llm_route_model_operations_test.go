package migrations

import (
	"testing"

	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
)

func TestAddLLMRouteModelOperationsMigrationIsRegistered(t *testing.T) {
	for _, migration := range registeredMigrations() {
		if migration.ID == migrationAddLLMRouteModelOperationsID {
			if migration.Migrate == nil {
				t.Fatal("model operations migration has nil migrate function")
			}
			if (channelmodel.LLMRoute{}).TableName() != "llm_routes" {
				t.Fatal("unexpected route table name")
			}
			return
		}
	}
	t.Fatalf("migration %s is not registered", migrationAddLLMRouteModelOperationsID)
}
