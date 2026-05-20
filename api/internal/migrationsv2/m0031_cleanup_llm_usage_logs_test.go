package migrationsv2

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestM0031CleanupLLMUsageLogs_DropsNonEmptyLegacyTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cleanup_llm_usage_logs.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_usage_logs (
			id TEXT PRIMARY KEY,
			model_name TEXT NOT NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_usage_logs (id, model_name)
		VALUES ('usage-1', 'legacy-model')
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_tenant_usage_logs (
			id TEXT PRIMARY KEY
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE VIEW llm_organization_usage_logs AS
		SELECT id, model_name FROM llm_usage_logs
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE data_retention_policies (
			id TEXT PRIMARY KEY,
			data_type TEXT NOT NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO data_retention_policies (id, data_type)
		VALUES ('policy-1', 'usage_logs')
	`).Error)

	require.NoError(t, M0031_cleanup_llm_usage_logs().Migrate(db))

	for _, name := range legacyLLMUsageRelations {
		exists, err := sqliteRelationExists(db, name)
		require.NoError(t, err)
		require.False(t, exists, "%s should be dropped", name)
	}

	var policyCount int64
	require.NoError(t, db.Raw(`
		SELECT COUNT(*)
		FROM data_retention_policies
		WHERE data_type = 'usage_logs'
	`).Scan(&policyCount).Error)
	require.Zero(t, policyCount)
}

func sqliteRelationExists(db *gorm.DB, name string) (bool, error) {
	var count int64
	err := db.Raw(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE name = ?
		  AND type IN ('table', 'view')
	`, name).Scan(&count).Error
	return count > 0, err
}
