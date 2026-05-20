package modelmeta

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSyncProvidersIncludesGLM(t *testing.T) {
	if !slices.Contains(syncProviders, "glm") {
		t.Fatalf("syncProviders = %#v, want to contain %q", syncProviders, "glm")
	}
}

func TestModelMetaRequiresExplicitAPIURL(t *testing.T) {
	svc := NewService(nil)
	require.False(t, svc.HasConfiguredAPIBaseURL())

	_, err := svc.fetchProviders()
	require.ErrorIs(t, err, errModelMetaAPIURLNotConfigured)

	assert.Equal(t, "", normalizeModelMetaAPIBase(""))
	assert.Equal(t, "https://api.modelmeta.dev/v1", normalizeModelMetaAPIBase("https://api.modelmeta.dev"))
	assert.Equal(t, "https://api.modelmeta.dev/v1", normalizeModelMetaAPIBase("https://api.modelmeta.dev/v1"))
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func openModelMetaTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:modelmeta_%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil && strings.Contains(err.Error(), "requires cgo") {
		t.Skip("sqlite driver requires cgo in this environment")
	}
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_providers (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			provider_name TEXT NOT NULL,
			logo_url TEXT,
			website TEXT,
			documentation_url TEXT,
			pricing_url TEXT,
			country_code TEXT,
			founded_year INTEGER DEFAULT 0,
			tagline TEXT,
			description TEXT,
			metadata TEXT DEFAULT '{}',
			api_base_url TEXT,
			provider_type TEXT DEFAULT 'vendor',
			is_active BOOLEAN DEFAULT true,
			sort_order INTEGER DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE UNIQUE INDEX idx_provider_name ON llm_providers(provider)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_models (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			display_name TEXT NOT NULL,
			type TEXT DEFAULT 'llm',
			family TEXT,
			family_name TEXT,
			parent_id TEXT,
			family_default BOOLEAN DEFAULT false,
			status TEXT,
			tagline TEXT,
			description TEXT,
			access_type TEXT,
			currency TEXT,
			use_cases TEXT,
			is_flagship BOOLEAN DEFAULT false,
			is_recommended BOOLEAN DEFAULT false,
			is_featured BOOLEAN DEFAULT false,
			is_new BOOLEAN DEFAULT false,
			reasoning BOOLEAN DEFAULT false,
			function_calling BOOLEAN DEFAULT false,
			structured_output BOOLEAN DEFAULT false,
			temperature BOOLEAN DEFAULT true,
			top_p BOOLEAN,
			presence_penalty BOOLEAN,
			frequency_penalty BOOLEAN,
			logit_bias BOOLEAN,
			seed BOOLEAN DEFAULT false,
			stop BOOLEAN DEFAULT true,
			max_stop_sequences INTEGER DEFAULT 4,
			vision BOOLEAN DEFAULT false,
			audio BOOLEAN DEFAULT false,
			json_mode BOOLEAN DEFAULT false,
			streaming BOOLEAN DEFAULT true,
			chat_completions BOOLEAN DEFAULT true,
			embeddings BOOLEAN DEFAULT false,
			image_generation BOOLEAN DEFAULT false,
			speech_generation BOOLEAN DEFAULT false,
			transcription BOOLEAN DEFAULT false,
			translation BOOLEAN DEFAULT false,
			moderation BOOLEAN DEFAULT false,
			videos BOOLEAN DEFAULT false,
			image_edit BOOLEAN DEFAULT false,
			realtime BOOLEAN DEFAULT false,
			batch BOOLEAN DEFAULT false,
			fine_tuning BOOLEAN DEFAULT false,
			assistants BOOLEAN DEFAULT false,
			responses BOOLEAN DEFAULT false,
			distillation BOOLEAN DEFAULT false,
			system_prompt BOOLEAN DEFAULT true,
			logprobs BOOLEAN DEFAULT false,
			web_search BOOLEAN DEFAULT false,
			file_search BOOLEAN DEFAULT false,
			code_interpreter BOOLEAN DEFAULT false,
			computer_use BOOLEAN DEFAULT false,
			mcp BOOLEAN DEFAULT false,
			parallel_tool_calls BOOLEAN DEFAULT false,
			reasoning_effort BOOLEAN DEFAULT false,
			input_modalities TEXT DEFAULT '[]',
			output_modalities TEXT DEFAULT '[]',
			knowledge_cutoff TEXT,
			open_weights BOOLEAN DEFAULT false,
			context_window INTEGER,
			max_output_tokens INTEGER,
			max_input_tokens INTEGER,
			supported_parameters BLOB DEFAULT '[]',
			config_parameters BLOB DEFAULT '[]',
			default_parameters TEXT DEFAULT '{}',
			is_moderated BOOLEAN DEFAULT false,
			is_finetuned BOOLEAN DEFAULT false,
			cost_rate TEXT DEFAULT '{"input":1, "output":1}',
			input_price TEXT,
			output_price TEXT,
			cached_input_price TEXT,
			cost_cache_read TEXT DEFAULT '0',
			cost_cache_write TEXT DEFAULT '0',
			cost_context_over_200k TEXT DEFAULT '{}',
			image_prices TEXT DEFAULT '[]',
			is_active BOOLEAN DEFAULT true,
			is_configured BOOLEAN DEFAULT false,
			sort_order INTEGER DEFAULT 0,
			model_tier TEXT DEFAULT '',
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE UNIQUE INDEX idx_model_provider_name ON llm_models(provider, name)
	`).Error)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_catalog_sync_states (
			sync_key TEXT PRIMARY KEY,
			last_applied_version INTEGER NOT NULL DEFAULT 0,
			last_applied_at DATETIME,
			last_error TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error)

	return db
}

func newTestService(t *testing.T, db *gorm.DB, provider string, remoteModels []ModelMetaData) *Service {
	t.Helper()

	payload, err := json.Marshal(ModelMetaResponse{
		Data:       remoteModels,
		Object:     "list",
		Page:       1,
		PageSize:   100,
		Total:      len(remoteModels),
		TotalPages: 1,
	})
	require.NoError(t, err)

	svc := NewService(db)
	svc.apiBaseURL = "https://api.modelmeta.dev/v1"
	svc.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			expectedPath := fmt.Sprintf("/v1/providers/%s/models", provider)
			if req.URL.Path != expectedPath {
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("not found")),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(payload)),
			}, nil
		}),
	}

	return svc
}

func mustDecimal(t *testing.T, value string) decimal.Decimal {
	t.Helper()

	result, err := decimal.NewFromString(value)
	require.NoError(t, err)
	return result
}

func diffFieldMap(fields []DiffField) map[string]DiffField {
	result := make(map[string]DiffField, len(fields))
	for _, field := range fields {
		result[field.Field] = field
	}
	return result
}

func minimalRemoteModel(provider, model string) ModelMetaData {
	return ModelMetaData{
		Provider:    provider,
		Model:       model,
		ModelName:   model,
		Status:      "active",
		UseCases:    []string{"text-chat"},
		Currency:    "USD",
		InputPrice:  0,
		OutputPrice: 0,
	}
}

func TestHasChangesRecognizesExpandedMetadata(t *testing.T) {
	svc := NewService(nil)
	local := &llmmodel.LLMModel{
		ModelName:        "GPT-4o",
		ContextWindow:    128000,
		MaxOutputTokens:  4096,
		Status:           "active",
		Tagline:          "Fast and reliable",
		Family:           "gpt-4o",
		FamilyName:       "GPT-4o",
		IsFlagship:       true,
		IsRecommended:    false,
		IsFeatured:       true,
		IsNew:            false,
		AccessType:       "closed",
		Currency:         "USD",
		UseCases:         llmmodel.StringArray{"chat", "analysis"},
		InputPrice:       mustDecimal(t, "10"),
		OutputPrice:      mustDecimal(t, "30"),
		CachedInputPrice: mustDecimal(t, "2.5"),
	}
	remote := &ModelMetaData{
		ModelName:        "GPT-4o",
		ContextWindow:    128000,
		MaxOutputTokens:  4096,
		Status:           "active",
		Tagline:          "Fast and reliable",
		Family:           "gpt-4o",
		FamilyName:       "GPT-4o",
		IsFlagship:       true,
		IsRecommended:    true,
		IsFeatured:       true,
		IsNew:            false,
		AccessType:       "closed",
		Currency:         "EUR",
		UseCases:         []string{"analysis", "reasoning", "chat"},
		InputPrice:       5,
		OutputPrice:      15,
		CachedInputPrice: 1.25,
		Endpoints: map[string]interface{}{
			"image_generation": true,
		},
	}

	require.True(t, svc.hasChanges(local, remote))

	fields := diffFieldMap(svc.computeDiffFields(local, remote))
	require.Contains(t, fields, "input_price")
	require.Contains(t, fields, "output_price")
	require.Contains(t, fields, "cached_input_price")
	require.Contains(t, fields, "currency")
	require.Contains(t, fields, "use_cases")
	require.Contains(t, fields, "is_recommended")

	assert.Equal(t, 10.0, fields["input_price"].OldValue)
	assert.Equal(t, 5.0, fields["input_price"].NewValue)
	assert.Equal(t, 30.0, fields["output_price"].OldValue)
	assert.Equal(t, 15.0, fields["output_price"].NewValue)
	assert.Equal(t, 2.5, fields["cached_input_price"].OldValue)
	assert.Equal(t, 1.25, fields["cached_input_price"].NewValue)
	assert.Equal(t, "USD", fields["currency"].OldValue)
	assert.Equal(t, "EUR", fields["currency"].NewValue)
	assert.Equal(t, false, fields["is_recommended"].OldValue)
	assert.Equal(t, true, fields["is_recommended"].NewValue)
	assert.Equal(t, []string{"analysis", "text-chat"}, fields["use_cases"].OldValue)
	assert.Equal(t, []string{"analysis", "reasoning", "text-chat"}, fields["use_cases"].NewValue)
}

func TestHasChangesIgnoresPricePrecisionAndUseCaseOrder(t *testing.T) {
	svc := NewService(nil)
	local := &llmmodel.LLMModel{
		ModelName:        "text-embedding-3-small",
		ContextWindow:    8192,
		MaxOutputTokens:  0,
		Status:           "active",
		Tagline:          "Embedding model",
		Family:           "text-embedding",
		FamilyName:       "Text Embedding",
		IsRecommended:    true,
		AccessType:       "closed",
		Currency:         "USD",
		UseCases:         llmmodel.StringArray{"embedding", "analysis"},
		InputPrice:       mustDecimal(t, "1.2346"),
		OutputPrice:      mustDecimal(t, "2.3457"),
		CachedInputPrice: mustDecimal(t, "0.1234"),
	}
	remote := &ModelMetaData{
		ModelName:        "text-embedding-3-small",
		ContextWindow:    8192,
		MaxOutputTokens:  0,
		Status:           "active",
		Tagline:          "Embedding model",
		Family:           "text-embedding",
		FamilyName:       "Text Embedding",
		IsRecommended:    true,
		AccessType:       "closed",
		Currency:         "USD",
		UseCases:         []string{"analysis", "embedding", "embedding"},
		InputPrice:       1.23459,
		OutputPrice:      2.34569,
		CachedInputPrice: 0.12341,
		Endpoints: map[string]interface{}{
			"embeddings": true,
		},
	}

	assert.False(t, svc.hasChanges(local, remote))
	assert.Empty(t, svc.computeDiffFields(local, remote))
}

func TestHasChangesIgnoresMatchingUseCases(t *testing.T) {
	svc := NewService(nil)
	local := &llmmodel.LLMModel{
		ModelName: "Rerank 3.5",
		Status:    "active",
		UseCases:  llmmodel.StringArray{"rerank"},
	}
	remote := &ModelMetaData{
		ModelName: "Rerank 3.5",
		Status:    "active",
		UseCases:  []string{"rerank"},
	}

	assert.False(t, svc.hasChanges(local, remote))
	assert.Empty(t, svc.computeDiffFields(local, remote))
}

func TestSyncProviderModelsCreatesModelWithExpandedMetadata(t *testing.T) {
	db := openModelMetaTestDB(t)
	remoteModel := ModelMetaData{
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		ModelName:        "GPT-4o Mini",
		Family:           "gpt-4o",
		FamilyName:       "GPT-4o",
		Status:           "active",
		Tagline:          "Affordable multimodal model",
		AccessType:       "closed",
		Currency:         "USD",
		InputPrice:       0.15,
		OutputPrice:      0.60,
		CachedInputPrice: 0.05,
		IsRecommended:    true,
		UseCases:         []string{"chat", "analysis"},
		Endpoints: map[string]interface{}{
			"image_generation": true,
		},
	}
	svc := newTestService(t, db, "openai", []ModelMetaData{remoteModel})

	result, err := svc.SyncProviderModels(context.Background(), "openai", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.NewModels)

	var stored struct {
		InputPrice       decimal.Decimal      `gorm:"column:input_price"`
		OutputPrice      decimal.Decimal      `gorm:"column:output_price"`
		CachedInputPrice decimal.Decimal      `gorm:"column:cached_input_price"`
		Currency         string               `gorm:"column:currency"`
		IsRecommended    bool                 `gorm:"column:is_recommended"`
		UseCases         llmmodel.StringArray `gorm:"column:use_cases"`
		ChatCompletions  bool                 `gorm:"column:chat_completions"`
		ImageGeneration  bool                 `gorm:"column:image_generation"`
		ConfigParameters []byte               `gorm:"column:config_parameters"`
	}
	require.NoError(t, db.Table("llm_models").
		Select("input_price, output_price, cached_input_price, currency, is_recommended, use_cases, chat_completions, image_generation, config_parameters").
		Where("provider = ? AND name = ?", "openai", "gpt-4o-mini").
		First(&stored).Error)
	assert.True(t, stored.InputPrice.Equal(mustDecimal(t, "0.15")))
	assert.True(t, stored.OutputPrice.Equal(mustDecimal(t, "0.6")))
	assert.True(t, stored.CachedInputPrice.Equal(mustDecimal(t, "0.05")))
	assert.Equal(t, "USD", stored.Currency)
	assert.True(t, stored.IsRecommended)
	assert.True(t, stored.ChatCompletions)
	assert.True(t, stored.ImageGeneration)
	assert.Equal(t, llmmodel.StringArray{"analysis", "text-chat"}, stored.UseCases)
	assert.JSONEq(t, "[]", string(stored.ConfigParameters))
}

func TestSyncProviderModelsTransformsLatestModelMetaShape(t *testing.T) {
	db := openModelMetaTestDB(t)
	description := "Latest payload model"
	remoteModel := ModelMetaData{
		Provider:         "openai",
		Model:            "gpt-4o",
		ModelName:        "GPT 4o",
		Description:      &description,
		Family:           "gpt-4",
		FamilyName:       "GPT-4",
		FamilyDefault:    true,
		Status:           "active",
		Tagline:          "Fast, intelligent, flexible GPT model",
		AccessType:       "closed",
		ContextWindow:    128000,
		MaxOutputTokens:  16384,
		Currency:         "USD",
		InputPrice:       4.25,
		OutputPrice:      17,
		CachedInputPrice: 2.125,
		IsFlagship:       true,
		IsRecommended:    true,
		IsFeatured:       true,
		IsNew:            true,
		UseCases:         []string{"vision", "text-chat", "function-calling"},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
		Endpoints: map[string]interface{}{
			"chat_completions":  false,
			"responses":         true,
			"realtime":          false,
			"assistants":        false,
			"batch":             true,
			"embeddings":        false,
			"fine_tuning":       false,
			"image_generation":  false,
			"vision":            true,
			"speech_generation": false,
			"transcription":     false,
			"translation":       false,
			"moderation":        false,
			"videos":            true,
			"image_edit":        true,
		},
		Features: map[string]interface{}{
			"streaming":         true,
			"function_calling":  true,
			"structured_output": true,
			"json_mode":         true,
			"system_prompt":     true,
		},
		Parameters: map[string]interface{}{
			"temperature":        map[string]interface{}{"supported": true, "min": float64(0), "max": float64(2), "default": float64(1)},
			"top_p":              map[string]interface{}{"supported": false},
			"supports_seed":      true,
			"supports_stop":      false,
			"max_stop_sequences": float64(8),
		},
		ConfigParameters: json.RawMessage(`[
			{
				"name": "temperature",
				"use_template": "temperature",
				"label": {"en_US": "Temperature"},
				"help": {"en_US": "Controls randomness."},
				"type": "float",
				"required": false,
				"default": 1,
				"min": 0,
				"max": 2,
				"precision": 2
			},
			{
				"name": "response_format",
				"label": {"en_US": "Response Format"},
				"help": {"en_US": "Specifies the format that the model must output."},
				"type": "string",
				"required": false,
				"default": "text",
				"options": ["text", "json_object"]
			}
		]`),
	}
	svc := newTestService(t, db, "openai", []ModelMetaData{remoteModel})

	result, err := svc.SyncProviderModels(context.Background(), "openai", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.NewModels)

	var stored struct {
		FamilyDefault       bool   `gorm:"column:family_default"`
		Description         string `gorm:"column:description"`
		ChatCompletions     bool   `gorm:"column:chat_completions"`
		Responses           bool   `gorm:"column:responses"`
		Batch               bool   `gorm:"column:batch"`
		Vision              bool   `gorm:"column:vision"`
		Videos              bool   `gorm:"column:videos"`
		ImageEdit           bool   `gorm:"column:image_edit"`
		Streaming           bool   `gorm:"column:streaming"`
		FunctionCalling     bool   `gorm:"column:function_calling"`
		StructuredOutput    bool   `gorm:"column:structured_output"`
		JsonMode            bool   `gorm:"column:json_mode"`
		Seed                bool   `gorm:"column:seed"`
		Stop                bool   `gorm:"column:stop"`
		MaxStopSequences    int    `gorm:"column:max_stop_sequences"`
		SupportedParameters []byte `gorm:"column:supported_parameters"`
		ConfigParameters    []byte `gorm:"column:config_parameters"`
	}
	require.NoError(t, db.Table("llm_models").
		Select("family_default, description, chat_completions, responses, batch, vision, videos, image_edit, streaming, function_calling, structured_output, json_mode, seed, stop, max_stop_sequences, supported_parameters, config_parameters").
		Where("provider = ? AND name = ?", "openai", "gpt-4o").
		First(&stored).Error)

	assert.True(t, stored.FamilyDefault)
	assert.Equal(t, description, stored.Description)
	assert.False(t, stored.ChatCompletions)
	assert.True(t, stored.Responses)
	assert.True(t, stored.Batch)
	assert.True(t, stored.Vision)
	assert.True(t, stored.Videos)
	assert.True(t, stored.ImageEdit)
	assert.True(t, stored.Streaming)
	assert.True(t, stored.FunctionCalling)
	assert.True(t, stored.StructuredOutput)
	assert.True(t, stored.JsonMode)
	assert.True(t, stored.Seed)
	assert.False(t, stored.Stop)
	assert.Equal(t, 8, stored.MaxStopSequences)

	var supportedParams llmmodel.ParameterDefinitions
	require.NoError(t, supportedParams.Scan(stored.SupportedParameters))
	supportedByName := make(map[string]llmmodel.ParameterDefinition, len(supportedParams))
	for _, param := range supportedParams {
		supportedByName[param.Name] = param
	}
	require.Contains(t, supportedByName, "temperature")
	assert.NotContains(t, supportedByName, "top_p")

	var configParams llmmodel.ConfigParameters
	require.NoError(t, configParams.Scan(stored.ConfigParameters))
	require.Len(t, configParams, 2)
	assert.Equal(t, "temperature", configParams[0].Name)
	assert.Equal(t, "temperature", configParams[0].TemplateKey)
	assert.JSONEq(t, `{"en_US":"Temperature"}`, string(configParams[0].Label))
	assert.JSONEq(t, `{"en_US":"Controls randomness."}`, string(configParams[0].Help))
	assert.Equal(t, "response_format", configParams[1].Name)
	assert.Equal(t, "response_format", configParams[1].TemplateKey)
	assert.Equal(t, []string{"text", "json_object"}, configParams[1].Options)
}

func TestSyncProviderModelsUpdatesExpandedMetadataAndClearsDiff(t *testing.T) {
	db := openModelMetaTestDB(t)
	existing := &llmmodel.LLMModel{
		ID:               uuid.New(),
		Provider:         "openai",
		Model:            "text-embedding-3-small",
		ModelName:        "Text Embedding 3 Small",
		Family:           "text-embedding",
		FamilyName:       "Text Embedding",
		Status:           "active",
		Tagline:          "Legacy description",
		AccessType:       "closed",
		Currency:         "USD",
		UseCases:         llmmodel.StringArray{"analysis"},
		InputPrice:       mustDecimal(t, "1.0000"),
		OutputPrice:      mustDecimal(t, "2.0000"),
		CachedInputPrice: mustDecimal(t, "0.5000"),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	require.NoError(t, db.Create(existing).Error)

	remoteModel := ModelMetaData{
		Provider:         "openai",
		Model:            "text-embedding-3-small",
		ModelName:        "Text Embedding 3 Small",
		Family:           "text-embedding",
		FamilyName:       "Text Embedding",
		Status:           "active",
		Tagline:          "Legacy description",
		AccessType:       "closed",
		Currency:         "EUR",
		InputPrice:       0,
		OutputPrice:      0.5,
		CachedInputPrice: 0,
		IsRecommended:    true,
		UseCases:         []string{"embedding", "analysis", "embedding"},
		Endpoints: map[string]interface{}{
			"embeddings": true,
		},
	}
	svc := newTestService(t, db, "openai", []ModelMetaData{remoteModel})

	result, err := svc.SyncProviderModels(context.Background(), "openai", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedModels)

	var stored struct {
		InputPrice       decimal.Decimal      `gorm:"column:input_price"`
		OutputPrice      decimal.Decimal      `gorm:"column:output_price"`
		CachedInputPrice decimal.Decimal      `gorm:"column:cached_input_price"`
		Currency         string               `gorm:"column:currency"`
		IsRecommended    bool                 `gorm:"column:is_recommended"`
		UseCases         llmmodel.StringArray `gorm:"column:use_cases"`
		Embeddings       bool                 `gorm:"column:embeddings"`
		ConfigParameters []byte               `gorm:"column:config_parameters"`
	}
	require.NoError(t, db.Table("llm_models").
		Select("input_price, output_price, cached_input_price, currency, is_recommended, use_cases, embeddings, config_parameters").
		Where("provider = ? AND name = ?", "openai", "text-embedding-3-small").
		First(&stored).Error)
	assert.True(t, stored.InputPrice.Equal(mustDecimal(t, "0")))
	assert.True(t, stored.OutputPrice.Equal(mustDecimal(t, "0.5")))
	assert.True(t, stored.CachedInputPrice.Equal(mustDecimal(t, "0")))
	assert.Equal(t, "EUR", stored.Currency)
	assert.True(t, stored.IsRecommended)
	assert.Equal(t, llmmodel.StringArray{"analysis", "embedding"}, stored.UseCases)
	assert.True(t, stored.Embeddings)
	assert.JSONEq(t, "[]", string(stored.ConfigParameters))

	diff, err := svc.ComputeDiff(context.Background(), "openai")
	require.NoError(t, err)
	assert.Len(t, diff.Changes.Updated, 0)
	assert.Equal(t, 0, diff.Summary.UpdatedModels)
	assert.Equal(t, 1, diff.Summary.UnchangedModels)
}

func TestSyncProviderModelsRestoresSoftDeletedModel(t *testing.T) {
	db := openModelMetaTestDB(t)
	deletedAt := time.Now()
	existing := &llmmodel.LLMModel{
		ID:               uuid.New(),
		Provider:         "cohere",
		Model:            "rerank-v3.5",
		ModelName:        "Old Rerank 3.5",
		Status:           "deprecated",
		UseCases:         llmmodel.StringArray{"rerank"},
		InputPrice:       mustDecimal(t, "1.0"),
		OutputPrice:      mustDecimal(t, "2.0"),
		CachedInputPrice: mustDecimal(t, "0"),
		DeletedAt:        gorm.DeletedAt{Time: deletedAt, Valid: true},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	require.NoError(t, db.Unscoped().Create(existing).Error)

	remoteModel := ModelMetaData{
		Provider:    "cohere",
		Model:       "rerank-v3.5",
		ModelName:   "Rerank 3.5",
		Status:      "active",
		UseCases:    []string{"rerank"},
		InputPrice:  0.8,
		OutputPrice: 1.6,
	}
	svc := newTestService(t, db, "cohere", []ModelMetaData{remoteModel})

	result, err := svc.SyncProviderModels(context.Background(), "cohere", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, result.UpdatedModels)
	assert.Empty(t, result.Errors)

	var stored struct {
		ModelName string               `gorm:"column:display_name"`
		Status    string               `gorm:"column:status"`
		UseCases  llmmodel.StringArray `gorm:"column:use_cases"`
		DeletedAt gorm.DeletedAt       `gorm:"column:deleted_at"`
	}
	require.NoError(t, db.Table("llm_models").
		Select("display_name, status, use_cases, deleted_at").
		Where("provider = ? AND name = ?", "cohere", "rerank-v3.5").
		First(&stored).Error)
	assert.Equal(t, "Rerank 3.5", stored.ModelName)
	assert.Equal(t, "active", stored.Status)
	assert.Equal(t, llmmodel.StringArray{"rerank"}, stored.UseCases)
	assert.False(t, stored.DeletedAt.Valid)
}

func TestSyncProviderModelsMarksPartialFailures(t *testing.T) {
	db := openModelMetaTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER block_bad_model_insert
		BEFORE INSERT ON llm_models
		WHEN NEW.name = 'bad-model'
		BEGIN
			SELECT RAISE(ABORT, 'bad model blocked');
		END
	`).Error)

	svc := newTestService(t, db, "openai", []ModelMetaData{
		minimalRemoteModel("openai", "good-model"),
		minimalRemoteModel("openai", "bad-model"),
	})

	result, err := svc.SyncProviderModels(context.Background(), "openai", nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, SyncResultStatusPartial, result.Status)
	assert.Equal(t, 2, result.TotalModels)
	assert.Equal(t, 1, result.SuccessModels)
	assert.Equal(t, 1, result.FailedModels)
	assert.Equal(t, 1, result.NewModels)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "bad-model")
}

func TestSyncProviderModelsMarksAllFailures(t *testing.T) {
	db := openModelMetaTestDB(t)
	require.NoError(t, db.Exec(`
		CREATE TRIGGER block_all_model_inserts
		BEFORE INSERT ON llm_models
		BEGIN
			SELECT RAISE(ABORT, 'all models blocked');
		END
	`).Error)

	svc := newTestService(t, db, "openai", []ModelMetaData{
		minimalRemoteModel("openai", "bad-model"),
	})

	result, err := svc.SyncProviderModels(context.Background(), "openai", nil)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, SyncResultStatusFailed, result.Status)
	assert.Equal(t, 1, result.TotalModels)
	assert.Equal(t, 0, result.SuccessModels)
	assert.Equal(t, 1, result.FailedModels)
	assert.Equal(t, 0, result.NewModels)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "bad-model")
}

func TestSyncProviderSerializesProviderMetadata(t *testing.T) {
	db := openModelMetaTestDB(t)
	svc := NewService(db)
	result := &ProviderSyncResult{Errors: []string{}}

	provider := &ModelMetaProvider{
		Provider:    "qwen",
		Name:        "Alibaba Cloud",
		Website:     "https://www.aliyun.com/product/bailian",
		APIDocsURL:  "https://help.aliyun.com",
		PricingURL:  "https://help.aliyun.com/pricing",
		CountryCode: "CN",
		FoundedYear: 2009,
		Tagline:     "Tongyi Qianwen",
		Description: "Alibaba Cloud Qwen models",
		Metadata: map[string]interface{}{
			"i18n": map[string]interface{}{
				"zh": map[string]interface{}{
					"tagline": "Qwen models",
				},
			},
		},
	}

	require.NoError(t, svc.syncProvider(context.Background(), provider, result))
	assert.Equal(t, 1, result.CreatedProviders)

	var metadata string
	require.NoError(t, db.Table("llm_providers").
		Select("metadata").
		Where("provider = ?", "qwen").
		Scan(&metadata).Error)
	assert.JSONEq(t, `{"i18n":{"zh":{"tagline":"Qwen models"}}}`, metadata)

	provider.Metadata = map[string]interface{}{
		"social": map[string]interface{}{
			"github": "https://github.com/QwenLM",
		},
	}
	require.NoError(t, svc.syncProvider(context.Background(), provider, result))
	assert.Equal(t, 1, result.UpdatedProviders)

	require.NoError(t, db.Table("llm_providers").
		Select("metadata").
		Where("provider = ?", "qwen").
		Scan(&metadata).Error)
	assert.JSONEq(t, `{"social":{"github":"https://github.com/QwenLM"}}`, metadata)
}

func TestGetSyncStatusReportsProviderErrors(t *testing.T) {
	db := openModelMetaTestDB(t)
	svc := NewService(db)
	svc.apiBaseURL = "https://api.modelmeta.dev/v1"
	svc.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/v1/providers":
				payload, err := json.Marshal(ModelMetaProviderResponse{
					Data: []ModelMetaProvider{
						{Provider: "openai", Name: "OpenAI"},
						{Provider: "broken", Name: "Broken Provider"},
					},
				})
				require.NoError(t, err)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(payload)),
				}, nil
			case "/v1/providers/openai/models":
				payload, err := json.Marshal(ModelMetaResponse{
					Data:       []ModelMetaData{minimalRemoteModel("openai", "gpt-4o")},
					Page:       1,
					PageSize:   100,
					Total:      1,
					TotalPages: 1,
				})
				require.NoError(t, err)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(bytes.NewReader(payload)),
				}, nil
			case "/v1/providers/broken/models":
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("unavailable")),
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("not found")),
				}, nil
			}
		}),
	}

	status, err := svc.GetSyncStatus(context.Background())
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.True(t, status.Degraded)
	assert.True(t, status.HasUpdates)
	assert.Equal(t, 1, status.Models.New)
	require.Len(t, status.ProviderErrors, 1)
	assert.Equal(t, "broken", status.ProviderErrors[0].Provider)
	assert.Contains(t, status.ProviderErrors[0].Error, "status 503")
}

func TestApplyPublishedCatalogRestoresSoftDeletedRowsAndRetiresMissingData(t *testing.T) {
	db := openModelMetaTestDB(t)
	require.NoError(t, db.Exec(`ALTER TABLE llm_providers ADD COLUMN is_system_enabled BOOLEAN DEFAULT true`).Error)
	require.NoError(t, db.Exec(`ALTER TABLE llm_models ADD COLUMN is_system_enabled BOOLEAN DEFAULT true`).Error)
	svc := NewService(db)
	now := time.Now()

	require.NoError(t, db.Exec(`
		INSERT INTO llm_providers (id, provider, provider_name, is_active, is_system_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, uuid.NewString(), "openai", "Old OpenAI", false, false, now, now).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_providers (id, provider, provider_name, is_active, is_system_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, uuid.NewString(), "legacy", "Legacy", true, true, now, now).Error)

	deletedModelID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO llm_models (
			id, provider, name, display_name, status, use_cases, is_active, is_system_enabled, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, deletedModelID.String(), "openai", "gpt-4.1", "Old GPT-4.1", "deprecated", `["text-chat"]`, false, false, now, now, now).Error)
	require.NoError(t, db.Exec(`
		INSERT INTO llm_models (
			id, provider, name, display_name, status, use_cases, is_active, is_system_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, uuid.NewString(), "legacy", "legacy-model", "Legacy Model", "active", `["text-chat"]`, true, true, now, now).Error)

	err := svc.ApplyPublishedCatalog(context.Background(), PublishedCatalog{
		Version:     12,
		PublishedAt: now.Add(time.Minute),
		Providers: []PublishedProvider{
			{
				Provider:     "openai",
				ProviderName: "OpenAI",
				IsActive:     true,
			},
		},
		Models: []PublishedModel{
			{
				Provider:        "openai",
				Model:           "gpt-4.1",
				ModelName:       "GPT-4.1",
				Type:            "llm",
				Status:          "active",
				IsActive:        true,
				ContextWindow:   128000,
				MaxOutputTokens: 16384,
			},
		},
	})
	require.NoError(t, err)

	var openaiProvider struct {
		ProviderName    string `gorm:"column:provider_name"`
		IsActive        bool   `gorm:"column:is_active"`
		IsSystemEnabled bool   `gorm:"column:is_system_enabled"`
	}
	require.NoError(t, db.Table("llm_providers").
		Select("provider_name, is_active, is_system_enabled").
		Where("provider = ?", "openai").
		First(&openaiProvider).Error)
	assert.Equal(t, "OpenAI", openaiProvider.ProviderName)
	assert.False(t, openaiProvider.IsActive)
	assert.False(t, openaiProvider.IsSystemEnabled)

	var legacyProvider struct {
		DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
	}
	require.NoError(t, db.Unscoped().Table("llm_providers").
		Select("deleted_at").
		Where("provider = ?", "legacy").
		First(&legacyProvider).Error)
	assert.True(t, legacyProvider.DeletedAt.Valid)

	var restoredModel struct {
		ID              string         `gorm:"column:id"`
		DisplayName     string         `gorm:"column:display_name"`
		IsActive        bool           `gorm:"column:is_active"`
		IsSystemEnabled bool           `gorm:"column:is_system_enabled"`
		DeletedAt       gorm.DeletedAt `gorm:"column:deleted_at"`
	}
	require.NoError(t, db.Table("llm_models").
		Select("id, display_name, is_active, is_system_enabled, deleted_at").
		Where("provider = ? AND name = ?", "openai", "gpt-4.1").
		First(&restoredModel).Error)
	assert.Equal(t, deletedModelID.String(), restoredModel.ID)
	assert.Equal(t, "GPT-4.1", restoredModel.DisplayName)
	assert.False(t, restoredModel.IsActive)
	assert.False(t, restoredModel.IsSystemEnabled)
	assert.False(t, restoredModel.DeletedAt.Valid)

	var restoredCount int64
	require.NoError(t, db.Unscoped().Table("llm_models").
		Where("provider = ? AND name = ?", "openai", "gpt-4.1").
		Count(&restoredCount).Error)
	assert.Equal(t, int64(1), restoredCount)

	var retiredModel struct {
		DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
	}
	require.NoError(t, db.Unscoped().Table("llm_models").
		Select("deleted_at").
		Where("provider = ? AND name = ?", "legacy", "legacy-model").
		First(&retiredModel).Error)
	assert.True(t, retiredModel.DeletedAt.Valid)

	var syncState struct {
		LastAppliedVersion int64  `gorm:"column:last_applied_version"`
		LastError          string `gorm:"column:last_error"`
	}
	require.NoError(t, db.Table("llm_catalog_sync_states").
		Select("last_applied_version, last_error").
		Where("sync_key = ?", "platform_catalog").
		First(&syncState).Error)
	assert.Equal(t, int64(12), syncState.LastAppliedVersion)
	assert.Equal(t, "", syncState.LastError)
}
