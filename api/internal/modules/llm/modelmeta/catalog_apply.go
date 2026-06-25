package modelmeta

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	"gorm.io/gorm"
)

const catalogSyncStateKey = "platform_catalog"

type PublishedCatalog struct {
	Version     int64
	PublishedAt time.Time
	Providers   []PublishedProvider
	Models      []PublishedModel
}

type PublishedProvider struct {
	Provider        string
	ProviderName    string
	Description     string
	Tagline         string
	LogoURL         string
	Website         string
	APIDocsURL      string
	PricingURL      string
	CountryCode     string
	FoundedYear     int
	Status          string
	IsActive        bool
	IsSystemEnabled bool
	Metadata        map[string]interface{}
}

type PublishedModel struct {
	Provider               string
	Model                  string
	ModelName              string
	Type                   string
	Family                 string
	FamilyName             string
	FamilyDefault          bool
	Status                 string
	Tagline                string
	Description            *string
	IsFlagship             bool
	IsRecommended          bool
	IsFeatured             bool
	IsNew                  bool
	AccessType             string
	Currency               string
	ContextWindow          int
	MaxOutputTokens        int
	InputPrice             float64
	OutputPrice            float64
	CachedInputPrice       float64
	InputPriceConfigured   bool
	OutputPriceConfigured  bool
	UseCases               []string
	InputModalities        []string
	OutputModalities       []string
	KnowledgeCutoff        string
	IsActive               bool
	IsSystemEnabled        bool
	SupportedParameters    json.RawMessage
	ConfigParameters       json.RawMessage
	Endpoints              *llmmodel.ModelEndpoints
	EndpointsAuthoritative bool
	Features               *llmmodel.ModelFeatures
	Tools                  *llmmodel.ModelTools
	Parameters             *llmmodel.ModelParameters
}

func (s *Service) ApplyPublishedCatalog(ctx context.Context, catalog PublishedCatalog) error {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txService := *s
		txService.db = tx

		providerKeys := make([]string, 0, len(catalog.Providers))
		for _, provider := range catalog.Providers {
			if err := txService.upsertPublishedProvider(ctx, provider); err != nil {
				return err
			}
			providerKeys = append(providerKeys, provider.Provider)
		}

		modelKeys := make([]catalogModelKey, 0, len(catalog.Models))
		for _, model := range catalog.Models {
			if err := txService.upsertPublishedModel(ctx, model); err != nil {
				return err
			}
			modelKeys = append(modelKeys, catalogModelKey{Provider: model.Provider, Model: model.Model})
		}

		if err := txService.softDeleteMissingProviders(ctx, providerKeys); err != nil {
			return err
		}
		if err := txService.softDeleteMissingModels(ctx, modelKeys); err != nil {
			return err
		}

		return txService.upsertCatalogSyncState(ctx, catalog.Version, catalog.PublishedAt, "")
	})
	if err != nil {
		return err
	}
	if invalidator := currentModelCacheInvalidator(); invalidator != nil {
		invalidator.InvalidateModelCache(ctx)
	}
	return nil
}

func (s *Service) RecordPublishedCatalogSyncError(ctx context.Context, message string) error {
	return s.upsertCatalogSyncState(ctx, 0, time.Time{}, message)
}

func (s *Service) upsertPublishedProvider(ctx context.Context, provider PublishedProvider) error {
	existing, err := s.findExistingProvider(ctx, provider.Provider)
	if err == gorm.ErrRecordNotFound {
		return s.createPublishedProvider(ctx, provider)
	}
	if err != nil {
		return err
	}

	if existing.DeletedAt.Valid {
		if err := s.restoreProviderByID(ctx, existing.ID); err != nil {
			return err
		}
	}

	return s.updatePublishedProvider(ctx, existing.ID, provider)
}

func (s *Service) upsertPublishedModel(ctx context.Context, model PublishedModel) error {
	existing, err := s.findExistingPublishedModel(ctx, model.Provider, model.Model)
	if err == gorm.ErrRecordNotFound {
		return s.createPublishedModel(ctx, model)
	}
	if err != nil {
		return err
	}

	if existing.DeletedAt.Valid {
		if err := s.restorePublishedModelByID(ctx, existing.ID); err != nil {
			return err
		}
	}

	return s.updatePublishedModel(ctx, existing.ID, model)
}

type existingProviderForSync struct {
	ID        string         `gorm:"column:id"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (s *Service) findExistingProvider(ctx context.Context, provider string) (*existingProviderForSync, error) {
	var existing existingProviderForSync
	err := s.db.WithContext(ctx).
		Unscoped().
		Table("llm_providers").
		Select("id", "deleted_at").
		Where("provider = ?", provider).
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *Service) createPublishedProvider(ctx context.Context, provider PublishedProvider) error {
	now := time.Now().UTC()
	values := map[string]interface{}{
		"id":            newUUIDString(),
		"provider":      provider.Provider,
		"provider_name": provider.ProviderName,
		"logo_url":      provider.LogoURL,
		"website":       provider.Website,
		"pricing_url":   provider.PricingURL,
		"country_code":  provider.CountryCode,
		"founded_year":  provider.FoundedYear,
		"tagline":       provider.Tagline,
		"description":   provider.Description,
		"metadata":      serializeJSONMap(provider.Metadata),
		"created_at":    now,
		"updated_at":    now,
	}

	if hasColumn(s.db, "llm_providers", "documentation_url") {
		values["documentation_url"] = provider.APIDocsURL
	} else if hasColumn(s.db, "llm_providers", "api_docs_url") {
		values["api_docs_url"] = provider.APIDocsURL
	}
	if hasColumn(s.db, "llm_providers", "status") {
		values["status"] = provider.Status
	}

	return s.db.WithContext(ctx).Table("llm_providers").Create(values).Error
}

func (s *Service) updatePublishedProvider(ctx context.Context, id string, provider PublishedProvider) error {
	updates := map[string]interface{}{
		"provider_name": provider.ProviderName,
		"logo_url":      provider.LogoURL,
		"website":       provider.Website,
		"pricing_url":   provider.PricingURL,
		"country_code":  provider.CountryCode,
		"founded_year":  provider.FoundedYear,
		"tagline":       provider.Tagline,
		"description":   provider.Description,
		"metadata":      serializeJSONMap(provider.Metadata),
		"updated_at":    time.Now(),
	}

	if hasColumn(s.db, "llm_providers", "documentation_url") {
		updates["documentation_url"] = provider.APIDocsURL
	} else if hasColumn(s.db, "llm_providers", "api_docs_url") {
		updates["api_docs_url"] = provider.APIDocsURL
	}
	if hasColumn(s.db, "llm_providers", "status") {
		updates["status"] = provider.Status
	}

	return s.db.WithContext(ctx).Table("llm_providers").Where("id = ?", id).Updates(updates).Error
}

func (s *Service) restoreProviderByID(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).
		Unscoped().
		Table("llm_providers").
		Where("id = ?", id).
		Update("deleted_at", nil).Error
}

func (s *Service) softDeleteMissingProviders(ctx context.Context, activeProviders []string) error {
	query := s.db.WithContext(ctx).Table("llm_providers").Where("deleted_at IS NULL")
	if len(activeProviders) > 0 {
		query = query.Where("provider NOT IN ?", activeProviders)
	}
	return query.Updates(map[string]interface{}{
		"deleted_at": time.Now().UTC(),
		"updated_at": time.Now().UTC(),
	}).Error
}

type catalogModelKey struct {
	Provider string
	Model    string
}

type existingPublishedModelForSync struct {
	ID        string         `gorm:"column:id"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (s *Service) findExistingPublishedModel(ctx context.Context, provider, model string) (*existingPublishedModelForSync, error) {
	var existing existingPublishedModelForSync
	err := s.db.WithContext(ctx).
		Unscoped().
		Table("llm_models").
		Select("id", "deleted_at").
		Where("provider = ? AND name = ?", provider, model).
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

func (s *Service) createPublishedModel(ctx context.Context, model PublishedModel) error {
	values := buildPublishedModelColumns(s.db, model)
	values["id"] = newUUIDString()
	values["created_at"] = time.Now().UTC()
	values["updated_at"] = values["created_at"]
	return s.db.WithContext(ctx).Table("llm_models").Create(values).Error
}

func (s *Service) updatePublishedModel(ctx context.Context, id string, model PublishedModel) error {
	values := buildPublishedModelColumns(s.db, model)
	values["updated_at"] = time.Now().UTC()
	return s.db.WithContext(ctx).Table("llm_models").Where("id = ?", id).Updates(values).Error
}

func (s *Service) restorePublishedModelByID(ctx context.Context, id string) error {
	return s.db.WithContext(ctx).
		Unscoped().
		Table("llm_models").
		Where("id = ?", id).
		Update("deleted_at", nil).Error
}

func (s *Service) softDeleteMissingModels(ctx context.Context, activeModels []catalogModelKey) error {
	query := s.db.WithContext(ctx).Where("deleted_at IS NULL")
	if len(activeModels) > 0 {
		clauses := make([]string, 0, len(activeModels))
		args := make([]interface{}, 0, len(activeModels)*2)
		for _, key := range activeModels {
			clauses = append(clauses, "(provider = ? AND name = ?)")
			args = append(args, key.Provider, key.Model)
		}
		query = query.Not("("+joinWithOr(clauses)+")", args...)
	}
	return query.Delete(&llmmodel.LLMModel{}).Error
}

func (s *Service) upsertCatalogSyncState(ctx context.Context, version int64, appliedAt time.Time, lastError string) error {
	now := time.Now().UTC()
	updates := map[string]interface{}{
		"updated_at": now,
		"last_error": lastError,
	}
	if version > 0 {
		updates["last_applied_version"] = version
		updates["last_applied_at"] = appliedAt
	}

	var count int64
	if err := s.db.WithContext(ctx).
		Table("llm_catalog_sync_states").
		Where("sync_key = ?", catalogSyncStateKey).
		Count(&count).Error; err != nil {
		return err
	}

	if count == 0 {
		values := map[string]interface{}{
			"sync_key":   catalogSyncStateKey,
			"updated_at": now,
			"last_error": lastError,
			"created_at": now,
		}
		if version > 0 {
			values["last_applied_version"] = version
			values["last_applied_at"] = appliedAt
		}
		return s.db.WithContext(ctx).Table("llm_catalog_sync_states").Create(values).Error
	}

	return s.db.WithContext(ctx).Table("llm_catalog_sync_states").
		Where("sync_key = ?", catalogSyncStateKey).
		Updates(updates).Error
}

func hasColumn(db *gorm.DB, table, column string) bool {
	return db.Migrator().HasColumn(table, column)
}

func joinWithOr(items []string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for i := 1; i < len(items); i++ {
		result += " OR " + items[i]
	}
	return result
}

func serializeJSONMap(value map[string]interface{}) string {
	if len(value) == 0 {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func serializeParameterDefinitions(raw json.RawMessage) string {
	normalized, err := llmmodel.NormalizeParameterDefinitionsJSON(raw)
	if err != nil || len(normalized) == 0 {
		return "[]"
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func serializeStringSlice(value []string) string {
	if len(value) == 0 {
		return "[]"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func normalizePublishedPrice(value float64) decimal.Decimal {
	return normalizeRemotePrice(value)
}

func buildPublishedModelColumns(db *gorm.DB, model PublishedModel) map[string]interface{} {
	useCases := llmmodel.EnsureUseCases(model.UseCases, model.Endpoints)
	values := map[string]interface{}{
		"provider":           model.Provider,
		"name":               model.Model,
		"display_name":       model.ModelName,
		"family":             model.Family,
		"family_name":        model.FamilyName,
		"status":             model.Status,
		"tagline":            model.Tagline,
		"use_cases":          llmmodel.StringArray(useCases),
		"is_flagship":        model.IsFlagship,
		"is_recommended":     model.IsRecommended,
		"is_featured":        model.IsFeatured,
		"is_new":             model.IsNew,
		"access_type":        model.AccessType,
		"currency":           model.Currency,
		"context_window":     model.ContextWindow,
		"max_output_tokens":  model.MaxOutputTokens,
		"knowledge_cutoff":   model.KnowledgeCutoff,
		"input_price":        normalizePublishedPrice(model.InputPrice),
		"output_price":       normalizePublishedPrice(model.OutputPrice),
		"cached_input_price": normalizePublishedPrice(model.CachedInputPrice),
	}

	if hasColumn(db, "llm_models", "input_price_configured") {
		values["input_price_configured"] = model.InputPriceConfigured
	}
	if hasColumn(db, "llm_models", "output_price_configured") {
		values["output_price_configured"] = model.OutputPriceConfigured
	}
	if hasColumn(db, "llm_models", "family_default") {
		values["family_default"] = model.FamilyDefault
	}
	if model.Description != nil && hasColumn(db, "llm_models", "description") {
		values["description"] = *model.Description
	}
	if hasColumn(db, "llm_models", "input_modalities") {
		values["input_modalities"] = types.JSONArray(model.InputModalities)
	}
	if hasColumn(db, "llm_models", "output_modalities") {
		values["output_modalities"] = types.JSONArray(model.OutputModalities)
	}
	if hasColumn(db, "llm_models", "supported_parameters") {
		values["supported_parameters"] = serializeParameterDefinitions(model.SupportedParameters)
	}
	if len(model.ConfigParameters) > 0 && hasColumn(db, "llm_models", "config_parameters") {
		values["config_parameters"] = serializeConfigParameters(model.ConfigParameters)
	}

	endpointColumns := endpointColumnsForPublishedModel(useCases, model.Endpoints, model.EndpointsAuthoritative)
	for key, value := range endpointColumns {
		if hasColumn(db, "llm_models", key) {
			values[key] = value
		}
	}
	for key, value := range featureColumnsForPublishedModel(model.Features, model.Tools) {
		if hasColumn(db, "llm_models", key) {
			values[key] = value
		}
	}
	for key, value := range parameterColumnsForPublishedModel(model.Parameters) {
		if hasColumn(db, "llm_models", key) {
			values[key] = value
		}
	}

	return values
}

func endpointColumnsForPublishedModel(useCases []string, endpoints *llmmodel.ModelEndpoints, authoritative bool) map[string]bool {
	resolved := llmmodel.DefaultEndpointsForUseCases(useCases)
	if endpoints != nil {
		if authoritative {
			resolved = *endpoints
		} else {
			resolved.ChatCompletions = resolved.ChatCompletions || endpoints.ChatCompletions
			resolved.Responses = resolved.Responses || endpoints.Responses
			resolved.Realtime = resolved.Realtime || endpoints.Realtime
			resolved.Assistants = resolved.Assistants || endpoints.Assistants
			resolved.Batch = resolved.Batch || endpoints.Batch
			resolved.Embeddings = resolved.Embeddings || endpoints.Embeddings
			resolved.FineTuning = resolved.FineTuning || endpoints.FineTuning
			resolved.ImageGeneration = resolved.ImageGeneration || endpoints.ImageGeneration
			resolved.Vision = resolved.Vision || endpoints.Vision
			resolved.SpeechGeneration = resolved.SpeechGeneration || endpoints.SpeechGeneration
			resolved.Transcription = resolved.Transcription || endpoints.Transcription
			resolved.Translation = resolved.Translation || endpoints.Translation
			resolved.Moderation = resolved.Moderation || endpoints.Moderation
			resolved.Videos = resolved.Videos || endpoints.Videos
			resolved.ImageEdit = resolved.ImageEdit || endpoints.ImageEdit
		}
	}

	return map[string]bool{
		"chat_completions":  resolved.ChatCompletions,
		"responses":         resolved.Responses,
		"realtime":          resolved.Realtime,
		"assistants":        resolved.Assistants,
		"batch":             resolved.Batch,
		"embeddings":        resolved.Embeddings,
		"fine_tuning":       resolved.FineTuning,
		"image_generation":  resolved.ImageGeneration,
		"vision":            resolved.Vision,
		"speech_generation": resolved.SpeechGeneration,
		"transcription":     resolved.Transcription,
		"translation":       resolved.Translation,
		"moderation":        resolved.Moderation,
		"videos":            resolved.Videos,
		"image_edit":        resolved.ImageEdit,
	}
}

func featureColumnsForPublishedModel(features *llmmodel.ModelFeatures, tools *llmmodel.ModelTools) map[string]bool {
	values := map[string]bool{}
	if features != nil {
		values["streaming"] = features.Streaming
		values["function_calling"] = features.FunctionCalling
		values["structured_output"] = features.StructuredOutput
		values["json_mode"] = features.JsonMode
		values["distillation"] = features.Distillation
		values["reasoning"] = features.Reasoning
		values["system_prompt"] = features.SystemPrompt
		values["logprobs"] = features.Logprobs
		values["web_search"] = features.WebSearch
		values["file_search"] = features.FileSearch
		values["code_interpreter"] = features.CodeInterpreter
		values["computer_use"] = features.ComputerUse
		values["mcp"] = features.Mcp
		values["reasoning_effort"] = features.ReasoningEffort
		values["attachment"] = features.Attachment
	}
	if tools != nil {
		values["web_search"] = values["web_search"] || tools.WebSearch
		values["file_search"] = values["file_search"] || tools.FileSearch
		values["image_generation"] = values["image_generation"] || tools.ImageGeneration
		values["code_interpreter"] = values["code_interpreter"] || tools.CodeInterpreter
		values["computer_use"] = values["computer_use"] || tools.ComputerUse
		values["mcp"] = values["mcp"] || tools.Mcp
		values["parallel_tool_calls"] = tools.ParallelToolCalls
	}
	return values
}

func parameterColumnsForPublishedModel(parameters *llmmodel.ModelParameters) map[string]interface{} {
	if parameters == nil {
		return map[string]interface{}{}
	}

	return map[string]interface{}{
		"temperature":        parameters.SupportsTemperature,
		"top_p":              parameters.SupportsTopP,
		"presence_penalty":   parameters.SupportsPresencePenalty,
		"frequency_penalty":  parameters.SupportsFrequencyPenalty,
		"logit_bias":         parameters.SupportsLogitBias,
		"seed":               parameters.SupportsSeed,
		"stop":               parameters.SupportsStop,
		"max_stop_sequences": parameters.MaxStopSequences,
	}
}

func newUUIDString() string {
	return uuid.NewString()
}
