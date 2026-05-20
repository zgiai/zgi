package migrations

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestM0139_add_llm_usage_bills_CreatesTableAndIndexes(t *testing.T) {
	db := openUsageBillsMigrationTestDB(t)

	require.NoError(t, M0139_add_llm_usage_bills().Migrate(db))

	require.True(t, db.Migrator().HasTable("llm_usage_bills"))
	for _, column := range []string{
		"attempt_id",
		"organization_id",
		"app_id",
		"app_type",
		"official_points",
		"private_points",
		"total_points",
		"request_created_at",
		"settled_at",
	} {
		require.Truef(t, db.Migrator().HasColumn("llm_usage_bills", column), "expected column %s", column)
	}

	for _, index := range []string{
		"idx_usage_bills_org_created",
		"idx_usage_bills_org_model_created",
		"idx_usage_bills_org_app_type_created",
		"idx_usage_bills_org_app_created",
		"idx_usage_bills_org_source_created",
		"idx_usage_bills_request_id",
	} {
		require.Truef(t, db.Migrator().HasIndex("llm_usage_bills", index), "expected index %s", index)
	}
}

func openUsageBillsMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "usage_bills_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	return db
}
