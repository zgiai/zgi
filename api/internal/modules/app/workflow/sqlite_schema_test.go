package workflow

import (
	"github.com/zgiai/zgi/api/internal/testutil/testdb"
	"gorm.io/gorm"
)

func autoMigrateWorkflowTest(db *gorm.DB, models ...any) error {
	if err := testdb.AutoMigrate(db, models...); err != nil {
		return err
	}
	// This partial constraint is owned by the runtime-v2 migration, not a model
	// tag. Projection tests must use the same conflict target as production.
	if db.Migrator().HasTable(&WorkflowNodeRuntimeLog{}) {
		return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_node_runtime_logs_run_execution_unique
			ON workflow_node_runtime_logs (workflow_run_id, node_execution_id)
			WHERE workflow_run_id IS NOT NULL AND node_execution_id IS NOT NULL AND deleted_at IS NULL`).Error
	}
	return nil
}
