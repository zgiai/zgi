package migrationsv2

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestM0031CreateWorkflowTestTablesIncludesCaseIDIndex(t *testing.T) {
	db, mock, cleanup := openMigrationMockDB(t)
	defer cleanup()

	expectations := []string{
		`CREATE TABLE IF NOT EXISTS public\.workflow_test_settings`,
		`CREATE TABLE IF NOT EXISTS public\.workflow_test_scenarios`,
		`CREATE TABLE IF NOT EXISTS public\.workflow_test_cases[\s\S]*turns JSONB NOT NULL DEFAULT '\[\]'::jsonb`,
		`CREATE TABLE IF NOT EXISTS public\.workflow_test_batches`,
		`CREATE TABLE IF NOT EXISTS public\.workflow_test_batch_items[\s\S]*case_snapshot JSONB NOT NULL[\s\S]*outputs JSONB NOT NULL DEFAULT '\{\}'::jsonb`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_test_scenarios_agent`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_workflow_test_scenarios_agent_name`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_test_cases_agent_status`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_test_cases_scenario`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_test_batches_agent_status`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_test_batch_items_batch`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_test_batch_items_agent`,
		`CREATE INDEX IF NOT EXISTS idx_workflow_test_batch_items_case`,
	}
	for _, expectation := range expectations {
		mock.ExpectExec(expectation).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	if err := M0031_create_workflow_test_tables().Migrate(db); err != nil {
		t.Fatalf("migrate returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}
