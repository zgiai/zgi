package migrations

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestM0138_fix_llm_custom_models_runtime_compatibility_RepairsPostCompatibilitySchema(t *testing.T) {
	db := openLegacyCustomModelsMigrationTestDB(t)

	require.NoError(t, M0138_fix_llm_custom_models_runtime_compatibility().Migrate(db))

	for _, column := range []string{
		"provider",
		"vision",
		"function_calling",
		"reasoning",
		"responses",
		"image_generation",
		"temperature",
		"default_parameters",
	} {
		exists, err := hasExactColumn(db, "llm_custom_models", column)
		require.NoError(t, err)
		require.Truef(t, exists, "expected column %s to exist", column)
	}

	var provider string
	require.NoError(t, db.Raw(`SELECT provider FROM llm_custom_models WHERE id = ?`, "model-1").Scan(&provider).Error)
	require.Equal(t, "ollama", provider)

	type capabilityRow struct {
		Vision          bool
		FunctionCalling bool
		Reasoning       bool
	}

	var row capabilityRow
	require.NoError(t, db.Raw(`
		SELECT vision, function_calling, reasoning
		FROM llm_custom_models
		WHERE id = ?
	`, "model-1").Scan(&row).Error)
	require.True(t, row.Vision)
	require.True(t, row.FunctionCalling)
	require.True(t, row.Reasoning)
}

func TestM0138_fix_llm_custom_models_runtime_compatibility_RepairsLegacyTableNamesAndColumns(t *testing.T) {
	db := openPreCompatibilityCustomModelsMigrationTestDB(t)

	require.NoError(t, M0138_fix_llm_custom_models_runtime_compatibility().Migrate(db))

	require.True(t, db.Migrator().HasTable("llm_custom_providers"))
	require.True(t, db.Migrator().HasTable("llm_custom_models"))
	require.False(t, db.Migrator().HasTable("llm_tenant_custom_providers"))
	require.False(t, db.Migrator().HasTable("llm_tenant_custom_models"))

	for _, column := range []string{"organization_id", "provider", "provider_name"} {
		exists, err := hasExactColumn(db, "llm_custom_providers", column)
		require.NoError(t, err)
		require.Truef(t, exists, "expected provider column %s to exist", column)
	}

	for _, column := range []string{"organization_id", "provider", "vision", "function_calling", "reasoning"} {
		exists, err := hasExactColumn(db, "llm_custom_models", column)
		require.NoError(t, err)
		require.Truef(t, exists, "expected model column %s to exist", column)
	}

	var providerRow struct {
		OrganizationID string
		Provider       string
		ProviderName   string
	}
	require.NoError(t, db.Raw(`
		SELECT organization_id, provider, provider_name
		FROM llm_custom_providers
		WHERE id = ?
	`, "provider-1").Scan(&providerRow).Error)
	require.Equal(t, "org-1", providerRow.OrganizationID)
	require.Equal(t, "ollama", providerRow.Provider)
	require.Equal(t, "Ollama", providerRow.ProviderName)

	var modelRow struct {
		OrganizationID  string
		Provider        string
		Vision          bool
		FunctionCalling bool
		Reasoning       bool
	}
	require.NoError(t, db.Raw(`
		SELECT organization_id, provider, vision, function_calling, reasoning
		FROM llm_custom_models
		WHERE id = ?
	`, "model-1").Scan(&modelRow).Error)
	require.Equal(t, "org-1", modelRow.OrganizationID)
	require.Equal(t, "ollama", modelRow.Provider)
	require.True(t, modelRow.Vision)
	require.True(t, modelRow.FunctionCalling)
	require.True(t, modelRow.Reasoning)
}

func TestM0138_fix_llm_custom_models_runtime_compatibility_IsIdempotent(t *testing.T) {
	db := openLegacyCustomModelsMigrationTestDB(t)

	require.NoError(t, M0138_fix_llm_custom_models_runtime_compatibility().Migrate(db))
	require.NoError(t, M0138_fix_llm_custom_models_runtime_compatibility().Migrate(db))

	var provider string
	require.NoError(t, db.Raw(`SELECT provider FROM llm_custom_models WHERE id = ?`, "model-1").Scan(&provider).Error)
	require.Equal(t, "ollama", provider)
}

func openLegacyCustomModelsMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "custom_models_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_providers (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL
		)
	`).Error)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'llm',
			context_window INTEGER DEFAULT 0,
			max_output_tokens INTEGER DEFAULT 0,
			input_price TEXT DEFAULT '0',
			output_price TEXT DEFAULT '0',
			config_parameters TEXT DEFAULT '[]',
			supports_vision BOOLEAN DEFAULT false,
			supports_tool_call BOOLEAN DEFAULT false,
			supports_streaming BOOLEAN DEFAULT true,
			supports_reasoning BOOLEAN DEFAULT false,
			knowledge_cutoff TEXT DEFAULT '',
			description TEXT DEFAULT '',
			is_active BOOLEAN DEFAULT true,
			sort_order INTEGER DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			created_at DATETIME,
			updated_at DATETIME,
			use_cases TEXT DEFAULT '[]'
		)
	`).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO llm_custom_providers (id, provider)
		VALUES ('provider-1', 'ollama')
	`).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO llm_custom_models (
			id, organization_id, provider_id, name, display_name,
			supports_vision, supports_tool_call, supports_reasoning
		) VALUES (
			'model-1', 'org-1', 'provider-1', 'ernie-4.5-0.3b', '文心4.5',
			1, 1, 1
		)
	`).Error)

	return db
}

func openPreCompatibilityCustomModelsMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "custom_models_pre_compatibility_migration_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_tenant_custom_providers (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL
		)
	`).Error)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_tenant_custom_models (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL,
			type TEXT NOT NULL DEFAULT 'llm',
			supports_vision BOOLEAN DEFAULT false,
			supports_tool_call BOOLEAN DEFAULT false,
			supports_reasoning BOOLEAN DEFAULT false
		)
	`).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO llm_tenant_custom_providers (id, tenant_id, name, display_name)
		VALUES ('provider-1', 'org-1', 'ollama', 'Ollama')
	`).Error)

	require.NoError(t, db.Exec(`
		INSERT INTO llm_tenant_custom_models (
			id, tenant_id, provider_id, name, display_name,
			supports_vision, supports_tool_call, supports_reasoning
		) VALUES (
			'model-1', 'org-1', 'provider-1', 'ernie-4.5-0.3b', '文心4.5',
			1, 1, 1
		)
	`).Error)

	return db
}
