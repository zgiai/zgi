package migrations

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestM0137_normalize_supported_parameters_shape_ConvertsLegacyObjectsToArrays(t *testing.T) {
	db := openSupportedParametersMigrationTestDB(t)

	require.NoError(t, db.Exec(`
		INSERT INTO llm_models (id, supported_parameters) VALUES
		('model-object', '{"temperature":{"supported":true,"default":0.7,"min":0,"max":2},"seed":{"supported":false,"default":1}}'),
		('model-array', '[{"name":"top_p","type":"number","label":"Top P","default":1}]')
	`).Error)

	require.NoError(t, M0137_normalize_supported_parameters_shape().Migrate(db))

	objectRaw := loadSupportedParametersRaw(t, db, "model-object")
	arrayRaw := loadSupportedParametersRaw(t, db, "model-array")

	assert.Contains(t, objectRaw, `"name":"temperature"`)
	assert.NotContains(t, objectRaw, `"seed"`)
	assert.Equal(t, `[{"name":"top_p","type":"number","label":"Top P","default":1}]`, arrayRaw)

	require.NoError(t, M0137_normalize_supported_parameters_shape().Migrate(db))
	assert.Equal(t, objectRaw, loadSupportedParametersRaw(t, db, "model-object"))
	assert.Equal(t, arrayRaw, loadSupportedParametersRaw(t, db, "model-array"))
}

func openSupportedParametersMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "supported_parameters_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			supported_parameters TEXT DEFAULT '[]'
		)
	`).Error)

	return db
}

func loadSupportedParametersRaw(t *testing.T, db *gorm.DB, id string) string {
	t.Helper()

	var raw string
	row := db.Raw(`SELECT supported_parameters FROM llm_models WHERE id = ?`, id).Row()
	require.NoError(t, row.Scan(&raw))
	return raw
}
