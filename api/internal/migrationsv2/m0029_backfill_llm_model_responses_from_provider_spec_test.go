package migrationsv2

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestM0029_backfill_llm_model_responses_from_provider_spec_BackfillsSupportedProviders(t *testing.T) {
	db := openModelResponsesMigrationTestDB(t, "model_responses.db")
	seedModelResponsesRows(t, db)

	require.NoError(t, M0029_backfill_llm_model_responses_from_provider_spec().Migrate(db))

	for _, provider := range modelResponsesProviderSpecs {
		assertModelResponses(t, db, provider, true)
	}

	for _, provider := range []string{"anthropic", "deepseek", "siliconflow", "minimax", "moonshot", "zhipu"} {
		assertModelResponses(t, db, provider, false)
	}

	assertModelResponses(t, db, "deleted-openai", false)
}

func TestM0029_backfill_llm_model_responses_from_provider_spec_IsIdempotent(t *testing.T) {
	db := openModelResponsesMigrationTestDB(t, "model_responses_idempotent.db")
	seedModelResponsesRows(t, db)

	require.NoError(t, M0029_backfill_llm_model_responses_from_provider_spec().Migrate(db))
	require.NoError(t, M0029_backfill_llm_model_responses_from_provider_spec().Migrate(db))

	for _, provider := range modelResponsesProviderSpecs {
		assertModelResponses(t, db, provider, true)
	}
	assertModelResponses(t, db, "deepseek", false)
}

func openModelResponsesMigrationTestDB(t *testing.T, filename string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), filename)
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			responses BOOLEAN DEFAULT FALSE,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error)
	return db
}

func seedModelResponsesRows(t *testing.T, db *gorm.DB) {
	t.Helper()

	providers := append([]string{}, modelResponsesProviderSpecs...)
	providers = append(providers, "anthropic", "deepseek", "siliconflow", "minimax", "moonshot", "zhipu")
	for _, provider := range providers {
		require.NoError(t, db.Exec(`
			INSERT INTO llm_models (id, provider, name, responses, updated_at)
			VALUES (?, ?, ?, FALSE, CURRENT_TIMESTAMP)
		`, provider+"-model", provider, provider+"-model").Error)
	}

	require.NoError(t, db.Exec(`
		INSERT INTO llm_models (id, provider, name, responses, updated_at)
		VALUES ('already-true-model', 'openai', 'already-true-model', TRUE, CURRENT_TIMESTAMP)
	`).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_models (id, provider, name, responses, updated_at, deleted_at)
		VALUES ('deleted-openai-model', 'deleted-openai', 'deleted-openai-model', FALSE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`).Error)
}

func assertModelResponses(t *testing.T, db *gorm.DB, provider string, want bool) {
	t.Helper()

	var got bool
	require.NoError(t, db.Raw(`
		SELECT responses
		FROM llm_models
		WHERE provider = ?
		LIMIT 1
	`, provider).Scan(&got).Error)
	require.Equal(t, want, got, "provider %s", provider)
}
