package modelmeta

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFeatureColumnsForPublishedModelIncludesAttachment(t *testing.T) {
	values := featureColumnsForPublishedModel(&llmmodel.ModelFeatures{
		Attachment: true,
	}, nil)

	require.True(t, values["attachment"])
}

func TestApplyPublishedCatalogMarksMissingModelsDeprecated(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	insertCatalogApplyProvider(t, db, "openai", false)
	insertCatalogApplyModel(t, db, "openai", "gpt-5", "active", false)
	insertCatalogApplyModel(t, db, "openai", "gpt-old", "active", false)

	svc := NewService(db)
	err := svc.ApplyPublishedCatalog(context.Background(), PublishedCatalog{
		Version:     1,
		PublishedAt: time.Now(),
		Providers: []PublishedProvider{{
			Provider:        "openai",
			ProviderName:    "OpenAI",
			Status:          "active",
			IsActive:        true,
			IsSystemEnabled: true,
		}},
		Models: []PublishedModel{{
			Provider:        "openai",
			Model:           "gpt-5",
			ModelName:       "GPT 5",
			Status:          "active",
			IsActive:        true,
			IsSystemEnabled: true,
		}},
	})
	require.NoError(t, err)

	var oldModel struct {
		Status              string
		DeletedAt           *time.Time
		ReplacementProvider string
		ReplacementModel    string
		DeprecationReason   string
	}
	require.NoError(t, db.Table("llm_models").
		Select("status", "deleted_at", "replacement_provider", "replacement_model", "deprecation_reason").
		Where("provider = ? AND name = ?", "openai", "gpt-old").
		First(&oldModel).Error)
	require.Equal(t, "deprecated", oldModel.Status)
	require.Nil(t, oldModel.DeletedAt)
	require.Empty(t, oldModel.ReplacementProvider)
	require.Empty(t, oldModel.ReplacementModel)
	require.Empty(t, oldModel.DeprecationReason)
}

func TestApplyPublishedCatalogDoesNotRestoreSoftDeletedRecords(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	insertCatalogApplyProvider(t, db, "openai", true)
	insertCatalogApplyModel(t, db, "openai", "gpt-5", "active", true)

	svc := NewService(db)
	err := svc.ApplyPublishedCatalog(context.Background(), PublishedCatalog{
		Version:     1,
		PublishedAt: time.Now(),
		Providers: []PublishedProvider{{
			Provider:        "openai",
			ProviderName:    "OpenAI",
			Status:          "active",
			IsActive:        true,
			IsSystemEnabled: true,
		}},
		Models: []PublishedModel{{
			Provider:        "openai",
			Model:           "gpt-5",
			ModelName:       "GPT 5",
			Status:          "active",
			IsActive:        true,
			IsSystemEnabled: true,
		}},
	})
	require.NoError(t, err)

	var providerDeletedAt sql.NullTime
	require.NoError(t, db.Table("llm_providers").
		Select("deleted_at").
		Where("provider = ?", "openai").
		Scan(&providerDeletedAt).Error)
	require.True(t, providerDeletedAt.Valid)

	var modelDeletedAt sql.NullTime
	require.NoError(t, db.Table("llm_models").
		Select("deleted_at").
		Where("provider = ? AND name = ?", "openai", "gpt-5").
		Scan(&modelDeletedAt).Error)
	require.True(t, modelDeletedAt.Valid)
}

func TestApplyPublishedCatalogStoresOptionalDeprecatedLifecycleFields(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	insertCatalogApplyProvider(t, db, "deepseek", false)

	svc := NewService(db)
	err := svc.ApplyPublishedCatalog(context.Background(), PublishedCatalog{
		Version:     1,
		PublishedAt: time.Now(),
		Providers: []PublishedProvider{{
			Provider:        "deepseek",
			ProviderName:    "DeepSeek",
			Status:          "active",
			IsActive:        true,
			IsSystemEnabled: true,
		}},
		Models: []PublishedModel{
			{
				Provider:            "deepseek",
				Model:               "deepseek-chat",
				ModelName:           "DeepSeek Chat",
				Status:              "deprecated",
				IsActive:            true,
				IsSystemEnabled:     true,
				ReplacementProvider: "deepseek",
				ReplacementModel:    "deepseek-v4-flash",
				DeprecationReason:   "Compatibility model is deprecated.",
			},
			{
				Provider:        "deepseek",
				Model:           "deepseek-old",
				ModelName:       "DeepSeek Old",
				Status:          "deprecated",
				IsActive:        true,
				IsSystemEnabled: true,
			},
		},
	})
	require.NoError(t, err)

	var withReason struct {
		Status              string
		ReplacementProvider string
		ReplacementModel    string
		DeprecationReason   string
	}
	require.NoError(t, db.Table("llm_models").
		Select("status", "replacement_provider", "replacement_model", "deprecation_reason").
		Where("provider = ? AND name = ?", "deepseek", "deepseek-chat").
		First(&withReason).Error)
	require.Equal(t, "deprecated", withReason.Status)
	require.Equal(t, "deepseek", withReason.ReplacementProvider)
	require.Equal(t, "deepseek-v4-flash", withReason.ReplacementModel)
	require.Equal(t, "Compatibility model is deprecated.", withReason.DeprecationReason)

	var withoutReason struct {
		Status              string
		ReplacementProvider string
		ReplacementModel    string
		DeprecationReason   string
	}
	require.NoError(t, db.Table("llm_models").
		Select("status", "replacement_provider", "replacement_model", "deprecation_reason").
		Where("provider = ? AND name = ?", "deepseek", "deepseek-old").
		First(&withoutReason).Error)
	require.Equal(t, "deprecated", withoutReason.Status)
	require.Empty(t, withoutReason.ReplacementProvider)
	require.Empty(t, withoutReason.ReplacementModel)
	require.Empty(t, withoutReason.DeprecationReason)
}

func TestApplyPublishedCatalogDoesNotSoftDeleteMissingProviders(t *testing.T) {
	db := newCatalogApplyTestDB(t)
	insertCatalogApplyProvider(t, db, "anthropic", false)

	svc := NewService(db)
	err := svc.ApplyPublishedCatalog(context.Background(), PublishedCatalog{
		Version:     1,
		PublishedAt: time.Now(),
		Providers: []PublishedProvider{{
			Provider:        "openai",
			ProviderName:    "OpenAI",
			Status:          "active",
			IsActive:        true,
			IsSystemEnabled: true,
		}},
	})
	require.NoError(t, err)

	var deletedAt sql.NullTime
	require.NoError(t, db.Table("llm_providers").
		Select("deleted_at").
		Where("provider = ?", "anthropic").
		Scan(&deletedAt).Error)
	require.False(t, deletedAt.Valid)
}

func newCatalogApplyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_providers (
			id TEXT PRIMARY KEY,
			provider TEXT,
			provider_name TEXT,
			logo_url TEXT,
			website TEXT,
			documentation_url TEXT,
			pricing_url TEXT,
			country_code TEXT,
			founded_year INTEGER DEFAULT 0,
			tagline TEXT,
			description TEXT,
			metadata TEXT,
			status TEXT,
			deleted_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT,
			name TEXT,
			display_name TEXT,
			family TEXT,
			family_name TEXT,
			status TEXT,
			replacement_provider TEXT,
			replacement_model TEXT,
			deprecation_reason TEXT,
			tagline TEXT,
			use_cases TEXT,
			is_flagship BOOLEAN DEFAULT false,
			is_recommended BOOLEAN DEFAULT false,
			is_featured BOOLEAN DEFAULT false,
			is_new BOOLEAN DEFAULT false,
			access_type TEXT,
			currency TEXT,
			context_window INTEGER DEFAULT 0,
			max_output_tokens INTEGER DEFAULT 0,
			knowledge_cutoff TEXT,
			input_price NUMERIC,
			output_price NUMERIC,
			cached_input_price NUMERIC,
			is_active BOOLEAN DEFAULT true,
			is_system_enabled BOOLEAN DEFAULT true,
			deleted_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_catalog_sync_states (
			sync_key TEXT PRIMARY KEY,
			last_applied_version INTEGER DEFAULT 0,
			last_applied_at DATETIME,
			last_error TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error)
	return db
}

func insertCatalogApplyProvider(t *testing.T, db *gorm.DB, provider string, deleted bool) {
	t.Helper()
	var deletedAt interface{}
	if deleted {
		deletedAt = time.Now().UTC()
	}
	require.NoError(t, db.Table("llm_providers").Create(map[string]interface{}{
		"id":            provider + "-id",
		"provider":      provider,
		"provider_name": provider,
		"metadata":      "{}",
		"status":        "active",
		"created_at":    time.Now().UTC(),
		"updated_at":    time.Now().UTC(),
		"deleted_at":    deletedAt,
	}).Error)
}

func insertCatalogApplyModel(t *testing.T, db *gorm.DB, provider, name, status string, deleted bool) {
	t.Helper()
	var deletedAt interface{}
	if deleted {
		deletedAt = time.Now().UTC()
	}
	require.NoError(t, db.Table("llm_models").Create(map[string]interface{}{
		"id":                provider + "-" + name + "-id",
		"provider":          provider,
		"name":              name,
		"display_name":      name,
		"status":            status,
		"use_cases":         "{}",
		"is_active":         true,
		"is_system_enabled": true,
		"created_at":        time.Now().UTC(),
		"updated_at":        time.Now().UTC(),
		"deleted_at":        deletedAt,
	}).Error)
}
