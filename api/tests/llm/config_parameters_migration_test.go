package llm_test

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/migrations"
	"gorm.io/gorm"
)

func TestM0141AddLLMModelConfigParameters(t *testing.T) {
	db := openConfigParametersMigrationTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer func() {
		require.NoError(t, sqlDB.Close())
	}()

	require.NoError(t, migrations.M0141_add_llm_model_config_parameters().Migrate(db))
	require.True(t, db.Migrator().HasColumn("llm_models", "config_parameters"))
	require.True(t, db.Migrator().HasColumn("llm_custom_models", "config_parameters"))
}

func openConfigParametersMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "config_parameters_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			name TEXT NOT NULL
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			name TEXT NOT NULL
		)
	`).Error)

	return db
}
