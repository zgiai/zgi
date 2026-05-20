package migrationsv2

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"gorm.io/gorm"
)

func TestM0027_fix_custom_model_runtime_schema_AddsFieldsRequiredByCustomModelSelect(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "custom_model_runtime.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL,
			input_price NUMERIC DEFAULT 0,
			output_price NUMERIC DEFAULT 0,
			is_active BOOLEAN DEFAULT true,
			sort_order INTEGER DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error)

	require.NoError(t, M0027_fix_custom_model_runtime_schema().Migrate(db))

	for _, column := range []string{
		"context_window",
		"max_output_tokens",
		"max_input_tokens",
		"supported_parameters",
		"config_parameters",
		"default_parameters",
	} {
		exists, err := hasExactColumnV2(db, "llm_custom_models", column)
		require.NoError(t, err)
		require.True(t, exists, "expected column %s to exist", column)
	}

	var models []llmmodel.CustomModel
	require.NoError(t, db.Find(&models).Error)
}

func TestM0027_fix_custom_model_runtime_schema_BackfillsLegacyRuntimeFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "custom_model_runtime_backfill.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			context_limit INTEGER,
			output_limit INTEGER
		)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_custom_models (id, context_limit, output_limit)
		VALUES ('model-1', 8192, 2048)
	`).Error)

	require.NoError(t, M0027_fix_custom_model_runtime_schema().Migrate(db))

	var got struct {
		ContextWindow   int `gorm:"column:context_window"`
		MaxOutputTokens int `gorm:"column:max_output_tokens"`
	}
	require.NoError(t, db.Raw(`
		SELECT context_window, max_output_tokens
		FROM llm_custom_models
		WHERE id = 'model-1'
	`).Scan(&got).Error)
	require.Equal(t, 8192, got.ContextWindow)
	require.Equal(t, 2048, got.MaxOutputTokens)
}

func TestM0027_fix_custom_model_runtime_schema_IsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "custom_model_runtime_idempotent.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			context_window INTEGER DEFAULT 0,
			max_output_tokens INTEGER DEFAULT 0,
			max_input_tokens INTEGER DEFAULT 0,
			supported_parameters TEXT DEFAULT '[]',
			config_parameters TEXT DEFAULT '[]',
			default_parameters TEXT DEFAULT '{}'
		)
	`).Error)

	require.NoError(t, M0027_fix_custom_model_runtime_schema().Migrate(db))
	require.NoError(t, M0027_fix_custom_model_runtime_schema().Migrate(db))
}
