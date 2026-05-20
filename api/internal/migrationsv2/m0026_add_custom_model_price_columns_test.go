package migrationsv2

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestM0026_add_custom_model_price_columns_AddsAndBackfillsLegacyPrices(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "custom_model_prices.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			cost_input NUMERIC DEFAULT 0,
			cost_output NUMERIC DEFAULT 0
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_custom_models (id, provider, cost_input, cost_output)
		VALUES ('model-1', 'ollama', 3, 5)
	`).Error)

	require.NoError(t, M0026_add_custom_model_price_columns().Migrate(db))

	hasInput, err := hasExactColumnV2(db, "llm_custom_models", "input_price")
	require.NoError(t, err)
	require.True(t, hasInput)

	hasOutput, err := hasExactColumnV2(db, "llm_custom_models", "output_price")
	require.NoError(t, err)
	require.True(t, hasOutput)

	var prices struct {
		InputPrice  float64 `gorm:"column:input_price"`
		OutputPrice float64 `gorm:"column:output_price"`
	}
	require.NoError(t, db.Raw(`
		SELECT input_price, output_price
		FROM llm_custom_models
		WHERE id = 'model-1'
	`).Scan(&prices).Error)
	require.Equal(t, 3.0, prices.InputPrice)
	require.Equal(t, 5.0, prices.OutputPrice)
}

func TestM0026_add_custom_model_price_columns_IsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "custom_model_prices_idempotent.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			input_price NUMERIC DEFAULT 0,
			output_price NUMERIC DEFAULT 0
		)
	`).Error)

	require.NoError(t, M0026_add_custom_model_price_columns().Migrate(db))
	require.NoError(t, M0026_add_custom_model_price_columns().Migrate(db))
}
