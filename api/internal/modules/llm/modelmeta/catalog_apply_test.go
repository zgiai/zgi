package modelmeta

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
)

func TestApplyPublishedCatalogNormalizesSupportedParameters(t *testing.T) {
	db := openModelMetaTestDB(t)
	svc := NewService(db)
	precision := 2

	catalog := PublishedCatalog{
		Version:     7,
		PublishedAt: time.Now().UTC(),
		Providers: []PublishedProvider{
			{
				Provider:     "doubao",
				ProviderName: "Doubao",
				IsActive:     true,
			},
		},
		Models: []PublishedModel{
			{
				Provider:         "doubao",
				Model:            "doubao-pro",
				ModelName:        "Doubao Pro",
				Type:             "llm",
				Status:           "active",
				InputModalities:  []string{"text"},
				OutputModalities: []string{"text"},
				IsActive:         true,
				SupportedParameters: json.RawMessage(`{
					"temperature": {"supported": true, "default": 0.7, "min": 0, "max": 2},
					"stream": {"supported": true, "default": false},
					"seed": {"supported": false, "default": 1}
				}`),
				ConfigParameters: json.RawMessage(`[
					{"name":"temperature","template_key":"temperature","type":"float","required":false,"default":1,"min":0,"max":2,"precision":2}
				]`),
			},
		},
	}

	require.NoError(t, svc.ApplyPublishedCatalog(context.Background(), catalog))

	var raw []byte
	row := db.Raw(`SELECT supported_parameters FROM llm_models WHERE provider = ? AND name = ?`, "doubao", "doubao-pro").Row()
	require.NoError(t, row.Scan(&raw))

	var params llmmodel.ParameterDefinitions
	require.NoError(t, params.Scan(raw))
	require.Len(t, params, 2)

	byName := make(map[string]llmmodel.ParameterDefinition, len(params))
	for _, param := range params {
		byName[param.Name] = param
	}

	assert.Contains(t, string(raw), `"name":"temperature"`)
	assert.Contains(t, string(raw), `"name":"stream"`)
	assert.NotContains(t, string(raw), `"seed"`)
	assert.Equal(t, "number", byName["temperature"].Type)
	assert.Equal(t, "switch", byName["stream"].Type)

	var configRaw []byte
	row = db.Raw(`SELECT config_parameters FROM llm_models WHERE provider = ? AND name = ?`, "doubao", "doubao-pro").Row()
	require.NoError(t, row.Scan(&configRaw))

	var configParams llmmodel.ConfigParameters
	require.NoError(t, configParams.Scan(configRaw))
	require.Len(t, configParams, 1)
	assert.Equal(t, "temperature", configParams[0].TemplateKey)
	require.NotNil(t, configParams[0].Precision)
	assert.Equal(t, precision, *configParams[0].Precision)
}

func TestSerializeConfigParametersDefaultsToEmptyArray(t *testing.T) {
	assert.Equal(t, "[]", serializeConfigParameters(nil))
	assert.Equal(t, "[]", serializeConfigParameters(json.RawMessage(`{"invalid":true}`)))
}

func TestApplyPublishedCatalogPreservesLocalStatusFields(t *testing.T) {
	db := openModelMetaTestDB(t)
	require.NoError(t, db.Exec(`ALTER TABLE llm_providers ADD COLUMN is_system_enabled BOOLEAN DEFAULT true`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE llm_models ADD COLUMN is_system_enabled BOOLEAN DEFAULT true`).Error)

	svc := NewService(db)
	now := time.Now().UTC()

	require.NoError(t, db.Exec(`
		INSERT INTO llm_providers (
			id, provider, provider_name, is_active, is_system_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "provider-1", "doubao", "Old Doubao", false, false, now, now).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_models (
			id, provider, name, display_name, status, type, is_active, is_system_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "model-1", "doubao", "doubao-pro", "Old Doubao Pro", "active", "llm", false, false, now, now).Error)

	err := svc.ApplyPublishedCatalog(context.Background(), PublishedCatalog{
		Version:     8,
		PublishedAt: now.Add(time.Minute),
		Providers: []PublishedProvider{{
			Provider:        "doubao",
			ProviderName:    "Doubao",
			Description:     "new desc",
			IsActive:        true,
			IsSystemEnabled: true,
		}},
		Models: []PublishedModel{{
			Provider:            "doubao",
			Model:               "doubao-pro",
			ModelName:           "Doubao Pro",
			Type:                "llm",
			Status:              "active",
			ContextWindow:       128000,
			MaxOutputTokens:     8192,
			IsActive:            true,
			IsSystemEnabled:     true,
			SupportedParameters: json.RawMessage(`[]`),
		}},
	})
	require.NoError(t, err)

	var providerRow struct {
		ProviderName    string `gorm:"column:provider_name"`
		IsActive        bool   `gorm:"column:is_active"`
		IsSystemEnabled bool   `gorm:"column:is_system_enabled"`
	}
	require.NoError(t, db.Table("llm_providers").
		Select("provider_name, is_active, is_system_enabled").
		Where("provider = ?", "doubao").
		First(&providerRow).Error)
	assert.Equal(t, "Doubao", providerRow.ProviderName)
	assert.False(t, providerRow.IsActive)
	assert.False(t, providerRow.IsSystemEnabled)

	var modelRow struct {
		DisplayName     string `gorm:"column:display_name"`
		IsActive        bool   `gorm:"column:is_active"`
		IsSystemEnabled bool   `gorm:"column:is_system_enabled"`
	}
	require.NoError(t, db.Table("llm_models").
		Select("display_name, is_active, is_system_enabled").
		Where("provider = ? AND name = ?", "doubao", "doubao-pro").
		First(&modelRow).Error)
	assert.Equal(t, "Doubao Pro", modelRow.DisplayName)
	assert.False(t, modelRow.IsActive)
	assert.False(t, modelRow.IsSystemEnabled)
}
