package workflow_test

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	workflowpkg "github.com/zgiai/zgi/api/internal/modules/app/workflow"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These integration tests require a dedicated PostgreSQL database, never the
// application's configured database. Each test owns and removes its schema.
func initTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	require.NotEmpty(t, dsn, "set TEST_POSTGRES_DSN to an isolated PostgreSQL test database")
	config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true}
	admin, err := gorm.Open(postgres.Open(dsn), config)
	require.NoError(t, err)
	adminSQL, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adminSQL.Close()) })
	schema := "workflow_integration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, admin.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() { require.NoError(t, admin.Exec("DROP SCHEMA "+schema+" CASCADE").Error) })
	if strings.Contains(dsn, "://") {
		parsed, err := url.Parse(dsn)
		require.NoError(t, err)
		query := parsed.Query()
		query.Set("search_path", schema+",public")
		parsed.RawQuery = query.Encode()
		dsn = parsed.String()
	} else {
		dsn += " search_path=" + schema + ",public"
	}
	db, err := gorm.Open(postgres.Open(dsn), config)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&agents.Agent{}, &workflowpkg.Workflow{},
		&workflowpkg.WorkflowRunLog{}, &workflowpkg.WorkflowNodeRuntimeLog{},
		&workflowpause.RunPause{}, &workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{}, &workflowpause.RuntimeOutbox{}))
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_workflow_node_runtime_logs_run_execution_unique
		ON workflow_node_runtime_logs (workflow_run_id, node_execution_id)
		WHERE workflow_run_id IS NOT NULL AND node_execution_id IS NOT NULL AND deleted_at IS NULL`).Error)
	// The authorization store uses private persistence records. An empty table
	// exercises the existing legacy-Agent fallback rather than bypassing access.
	require.NoError(t, db.Exec(`CREATE TABLE published_runtime_surfaces (
		id uuid PRIMARY KEY DEFAULT uuid_generate_v4(),
		resource_type varchar(32) NOT NULL, resource_id uuid NOT NULL,
		organization_id uuid NOT NULL, workspace_id uuid,
		surface varchar(32) NOT NULL, enabled boolean NOT NULL DEFAULT false,
		compatibility_source varchar(64) NOT NULL,
		created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP, deleted_at timestamptz
	)`).Error)
	previous := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous) })
	return db
}
