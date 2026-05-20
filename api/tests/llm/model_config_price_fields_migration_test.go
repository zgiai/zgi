package llm_test

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/migrationsv2"
	"gorm.io/gorm"
)

func TestM0022FixLLMModelConfigPriceFieldsRenamesLegacyColumns(t *testing.T) {
	db := openModelConfigPriceFieldsMigrationTestDB(t)
	closeModelConfigPriceFieldsMigrationTestDB(t, db)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_model_configs (
			id TEXT PRIMARY KEY,
			cost_input_override DECIMAL(10,4),
			cost_output_override DECIMAL(10,4)
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_model_configs (id, cost_input_override, cost_output_override)
		VALUES ('model-config-1', 1.23, 4.56)
	`).Error)

	migration := migrationsv2.M0022_fix_llm_model_config_price_fields()
	require.NoError(t, migration.Migrate(db))
	require.NoError(t, migration.Migrate(db))

	require.True(t, db.Migrator().HasColumn("llm_model_configs", "input_price_override"))
	require.True(t, db.Migrator().HasColumn("llm_model_configs", "output_price_override"))
	require.False(t, db.Migrator().HasColumn("llm_model_configs", "cost_input_override"))
	require.False(t, db.Migrator().HasColumn("llm_model_configs", "cost_output_override"))

	var got struct {
		InputPriceOverride  float64 `gorm:"column:input_price_override"`
		OutputPriceOverride float64 `gorm:"column:output_price_override"`
	}
	require.NoError(t, db.Table("llm_model_configs").Where("id = ?", "model-config-1").First(&got).Error)
	require.InDelta(t, 1.23, got.InputPriceOverride, 0.0001)
	require.InDelta(t, 4.56, got.OutputPriceOverride, 0.0001)
}

func TestM0022FixLLMModelConfigPriceFieldsBackfillsWhenBothColumnsExist(t *testing.T) {
	db := openModelConfigPriceFieldsMigrationTestDB(t)
	closeModelConfigPriceFieldsMigrationTestDB(t, db)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_model_configs (
			id TEXT PRIMARY KEY,
			cost_input_override DECIMAL(10,4),
			cost_output_override DECIMAL(10,4),
			input_price_override DECIMAL(10,4),
			output_price_override DECIMAL(10,4)
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_model_configs (
			id,
			cost_input_override,
			cost_output_override,
			input_price_override,
			output_price_override
		)
		VALUES
			('needs-backfill', 1.23, 4.56, NULL, NULL),
			('keeps-current', 7.89, 8.91, 2.34, 5.67)
	`).Error)

	migration := migrationsv2.M0022_fix_llm_model_config_price_fields()
	require.NoError(t, migration.Migrate(db))
	require.NoError(t, migration.Migrate(db))

	require.True(t, db.Migrator().HasColumn("llm_model_configs", "cost_input_override"))
	require.True(t, db.Migrator().HasColumn("llm_model_configs", "cost_output_override"))
	require.True(t, db.Migrator().HasColumn("llm_model_configs", "input_price_override"))
	require.True(t, db.Migrator().HasColumn("llm_model_configs", "output_price_override"))

	var backfilled struct {
		InputPriceOverride  float64 `gorm:"column:input_price_override"`
		OutputPriceOverride float64 `gorm:"column:output_price_override"`
	}
	require.NoError(t, db.Table("llm_model_configs").Where("id = ?", "needs-backfill").First(&backfilled).Error)
	require.InDelta(t, 1.23, backfilled.InputPriceOverride, 0.0001)
	require.InDelta(t, 4.56, backfilled.OutputPriceOverride, 0.0001)

	var preserved struct {
		InputPriceOverride  float64 `gorm:"column:input_price_override"`
		OutputPriceOverride float64 `gorm:"column:output_price_override"`
	}
	require.NoError(t, db.Table("llm_model_configs").Where("id = ?", "keeps-current").First(&preserved).Error)
	require.InDelta(t, 2.34, preserved.InputPriceOverride, 0.0001)
	require.InDelta(t, 5.67, preserved.OutputPriceOverride, 0.0001)
}

func TestM0022FixLLMModelConfigPriceFieldsAddsMissingColumns(t *testing.T) {
	db := openModelConfigPriceFieldsMigrationTestDB(t)
	closeModelConfigPriceFieldsMigrationTestDB(t, db)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_model_configs (
			id TEXT PRIMARY KEY
		)
	`).Error)

	migration := migrationsv2.M0022_fix_llm_model_config_price_fields()
	require.NoError(t, migration.Migrate(db))
	require.NoError(t, migration.Migrate(db))

	require.True(t, db.Migrator().HasColumn("llm_model_configs", "input_price_override"))
	require.True(t, db.Migrator().HasColumn("llm_model_configs", "output_price_override"))
}

func openModelConfigPriceFieldsMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "model_config_price_fields_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	return db
}

func closeModelConfigPriceFieldsMigrationTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})
}
