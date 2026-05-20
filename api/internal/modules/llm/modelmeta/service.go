package modelmeta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	appconfig "github.com/zgiai/zgi/api/config"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/observability"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	modelMetaAPIVersion = "v1"
	priceScale          = 4
)

var errModelMetaAPIURLNotConfigured = errors.New("MODELMETA_API_URL is not configured")

const (
	SyncResultStatusSuccess = "success"
	SyncResultStatusPartial = "partial"
	SyncResultStatusFailed  = "failed"
)

var syncProviders = []string{"openai", "anthropic", "cohere", "google", "qwen", "deepseek", "glm", "moonshot"}

// Service handles model metadata synchronization from modelmeta.dev
type Service struct {
	db         *gorm.DB
	httpClient *http.Client
	apiBaseURL string
}

// NewService creates a new model metadata service
func NewService(db *gorm.DB) *Service {
	return &Service{
		db:         db,
		apiBaseURL: resolveModelMetaAPIBase(),
		httpClient: observability.HTTPClient(&http.Client{
			Timeout: 30 * time.Second,
		}),
	}
}

func resolveModelMetaAPIBase() string {
	cfg := appconfig.GlobalConfig
	if cfg == nil || strings.TrimSpace(cfg.ModelMeta.APIURL) == "" {
		return ""
	}
	return normalizeModelMetaAPIBase(cfg.ModelMeta.APIURL)
}

func normalizeModelMetaAPIBase(rawURL string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if baseURL == "" {
		return ""
	}
	if strings.HasSuffix(baseURL, "/"+modelMetaAPIVersion) {
		return baseURL
	}
	return baseURL + "/" + modelMetaAPIVersion
}

func (s *Service) HasConfiguredAPIBaseURL() bool {
	return s != nil && strings.TrimSpace(s.apiBaseURL) != ""
}

// ModelMetaResponse represents the response from modelmeta.dev API
type ModelMetaResponse struct {
	Data       []ModelMetaData `json:"data"`
	Object     string          `json:"object"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	Total      int             `json:"total"`
	TotalPages int             `json:"total_pages"`
}

// ModelMetaProvider represents a provider from modelmeta.dev API
type ModelMetaProvider struct {
	ID          string                 `json:"id"`
	Object      string                 `json:"object"`
	Provider    string                 `json:"provider"`
	Name        string                 `json:"provider_name"`
	LogoURL     string                 `json:"logo_url"`
	Website     string                 `json:"website"`
	APIDocsURL  string                 `json:"api_docs_url"`
	PricingURL  string                 `json:"pricing_url"`
	CountryCode string                 `json:"country_code"`
	FoundedYear int                    `json:"founded_year"`
	Tagline     string                 `json:"tagline"`
	Description string                 `json:"description"`
	ModelCount  int                    `json:"model_count"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   int64                  `json:"created_at"`
	UpdatedAt   int64                  `json:"updated_at"`
}

// ModelMetaProviderResponse represents the provider list response from modelmeta.dev
type ModelMetaProviderResponse struct {
	Data       []ModelMetaProvider `json:"data"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	TotalPages int                 `json:"total_pages"`
}

// ModelMetaData represents a model from modelmeta.dev
type ModelMetaData struct {
	ID               string                 `json:"id"`
	Object           string                 `json:"object"`
	Provider         string                 `json:"provider"`
	IconSlug         string                 `json:"icon_slug"`
	Model            string                 `json:"model"`
	ModelName        string                 `json:"model_name"`
	Description      *string                `json:"description"`
	Tagline          string                 `json:"tagline"`
	Family           string                 `json:"family"`
	FamilyName       string                 `json:"family_name"`
	FamilyDefault    bool                   `json:"family_default"`
	Status           string                 `json:"status"`
	AccessType       string                 `json:"access_type"`
	ContextWindow    int                    `json:"context_window"`
	MaxOutputTokens  int                    `json:"max_output_tokens"`
	Currency         string                 `json:"currency"`
	InputPrice       float64                `json:"input_price"`
	OutputPrice      float64                `json:"output_price"`
	CachedInputPrice float64                `json:"cached_input_price"`
	IsFlagship       bool                   `json:"is_flagship"`
	IsRecommended    bool                   `json:"is_recommended"`
	IsFeatured       bool                   `json:"is_featured"`
	IsNew            bool                   `json:"is_new"`
	Endpoints        map[string]interface{} `json:"endpoints"`
	Features         map[string]interface{} `json:"features"`
	Tools            map[string]interface{} `json:"tools"`
	UseCases         []string               `json:"use_cases"`
	InputModalities  []string               `json:"input_modalities"`
	OutputModalities []string               `json:"output_modalities"`
	Parameters       map[string]interface{} `json:"parameters"`
	Evaluation       map[string]interface{} `json:"evaluation"`
	ConfigParameters json.RawMessage        `json:"config_parameters"`
	KnowledgeCutoff  string                 `json:"knowledge_cutoff"`
	ReleaseDate      string                 `json:"release_date"`
	LastUpdated      int64                  `json:"last_updated"`
	CreatedAt        int64                  `json:"created_at"`
	UpdatedAt        int64                  `json:"updated_at"`
}

// SyncResult represents the result of a sync operation
type SyncResult struct {
	Provider      string   `json:"provider"`
	Status        string   `json:"status"`
	TotalModels   int      `json:"total_models"`
	SuccessModels int      `json:"success_models"`
	FailedModels  int      `json:"failed_models"`
	NewModels     int      `json:"new_models"`
	UpdatedModels int      `json:"updated_models"`
	SkippedModels int      `json:"skipped_models"`
	Errors        []string `json:"errors,omitempty"`
	DurationMs    int64    `json:"duration_ms"`
}

// DiffResult represents the comparison result between local and remote models
// Note: deprecated/deleted models are NOT included here — use ComputeDeprecated instead.
type DiffResult struct {
	Provider  string      `json:"provider"`
	CheckedAt time.Time   `json:"checked_at"`
	Summary   DiffSummary `json:"summary"`
	Changes   DiffChanges `json:"changes"`
}

type DiffSummary struct {
	TotalRemote     int `json:"total_remote"`
	TotalLocal      int `json:"total_local"`
	NewModels       int `json:"new_models"`
	UpdatedModels   int `json:"updated_models"`
	UnchangedModels int `json:"unchanged_models"`
}

type DiffChanges struct {
	New     []ModelChange `json:"new"`
	Updated []ModelChange `json:"updated"`
}

// DeprecatedResult represents local models not found in upstream (potentially deprecated)
type DeprecatedResult struct {
	Provider  string        `json:"provider"`
	CheckedAt time.Time     `json:"checked_at"`
	Total     int           `json:"total"`
	Models    []ModelChange `json:"models"`
}

type ModelChange struct {
	Model             string         `json:"model"`
	ModelName         string         `json:"model_name"`
	ChangeType        string         `json:"change_type"` // new, updated, deleted
	RemoteData        *ModelMetaData `json:"remote_data,omitempty"`
	LocalData         interface{}    `json:"local_data,omitempty"`
	DiffFields        []DiffField    `json:"diff_fields,omitempty"`
	RecommendedAction string         `json:"recommended_action"` // sync, skip, manual_review
}

type DiffField struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}

// SyncProviderModels syncs models for a specific provider from modelmeta.dev
// If modelNames is provided and not empty, only sync those specific models.
// If modelNames is nil or empty, sync all models (backward compatible).
func (s *Service) SyncProviderModels(ctx context.Context, provider string, modelNames []string) (*SyncResult, error) {
	startTime := time.Now()
	result := &SyncResult{
		Provider: provider,
		Status:   SyncResultStatusSuccess,
		Errors:   []string{},
	}

	// Fetch all models from modelmeta.dev
	allModels, err := s.fetchProviderModels(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch models: %w", err)
	}

	// Filter models if specific models are requested
	var modelsToSync []ModelMetaData
	if len(modelNames) > 0 {
		// Create a map for quick lookup
		modelNameSet := make(map[string]bool)
		for _, name := range modelNames {
			modelNameSet[name] = true
		}

		// Filter to only requested models (already deduplicated by fetchProviderModels)
		foundSet := make(map[string]bool)
		for _, model := range allModels {
			if modelNameSet[model.Model] && !foundSet[model.Model] {
				modelsToSync = append(modelsToSync, model)
				foundSet[model.Model] = true
			}
		}

		// Check if all requested models were found
		if len(foundSet) != len(modelNameSet) {
			missing := make([]string, 0)
			for name := range modelNameSet {
				if !foundSet[name] {
					missing = append(missing, name)
				}
			}
			return nil, fmt.Errorf("models not found in remote data: %v", missing)
		}
	} else {
		// Sync all models
		modelsToSync = allModels
	}

	result.TotalModels = len(modelsToSync)

	// Sync each model to database
	for _, modelData := range modelsToSync {
		if err := s.syncModel(ctx, &modelData, result); err != nil {
			result.FailedModels++
			result.Errors = append(result.Errors, fmt.Sprintf("Model %s: %v", modelData.Model, err))
			continue
		}
		result.SuccessModels++
	}

	result.Status = resolveSyncResultStatus(result.SuccessModels, result.FailedModels)
	result.DurationMs = time.Since(startTime).Milliseconds()
	return result, nil
}

func resolveSyncResultStatus(successModels, failedModels int) string {
	switch {
	case failedModels == 0:
		return SyncResultStatusSuccess
	case successModels == 0:
		return SyncResultStatusFailed
	default:
		return SyncResultStatusPartial
	}
}

// SyncAllProviders syncs models for all providers
func (s *Service) SyncAllProviders(ctx context.Context) (map[string]*SyncResult, error) {
	results := make(map[string]*SyncResult)

	for _, provider := range syncProviders {
		result, err := s.SyncProviderModels(ctx, provider, nil) // nil = sync all models
		if err != nil {
			results[provider] = &SyncResult{
				Provider: provider,
				Status:   SyncResultStatusFailed,
				Errors:   []string{err.Error()},
			}
			continue
		}
		results[provider] = result
	}

	return results, nil
}

// ComputeDiff compares local models with remote models and returns new/updated differences.
// Deprecated/deleted models are excluded — use ComputeDeprecated for those.
func (s *Service) ComputeDiff(ctx context.Context, provider string) (*DiffResult, error) {
	result := &DiffResult{
		Provider:  provider,
		CheckedAt: time.Now(),
		Changes: DiffChanges{
			New:     []ModelChange{},
			Updated: []ModelChange{},
		},
	}

	// Fetch remote models
	remoteModels, err := s.fetchProviderModels(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote models: %w", err)
	}

	result.Summary.TotalRemote = len(remoteModels)

	// Fetch local models
	var localModels []llmmodel.LLMModel
	if err := s.db.WithContext(ctx).
		Select(
			"id",
			"provider",
			"name",
			"display_name",
			"family",
			"family_name",
			"family_default",
			"status",
			"tagline",
			"description",
			"is_flagship",
			"is_recommended",
			"is_featured",
			"is_new",
			"access_type",
			"currency",
			"use_cases",
			"context_window",
			"max_output_tokens",
			"input_price",
			"output_price",
			"cached_input_price",
			"knowledge_cutoff",
			"input_modalities",
			"output_modalities",
		).
		Where("provider = ? AND deleted_at IS NULL", provider).
		Find(&localModels).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch local models: %w", err)
	}

	result.Summary.TotalLocal = len(localModels)

	// Create maps for comparison
	remoteMap := make(map[string]*ModelMetaData)
	for i := range remoteModels {
		remoteMap[remoteModels[i].Model] = &remoteModels[i]
	}

	localMap := make(map[string]*llmmodel.LLMModel)
	for i := range localModels {
		localMap[localModels[i].Model] = &localModels[i]
	}

	// Find new and updated models (no deleted/deprecated)
	for modelName, remoteMeta := range remoteMap {
		if localModel, exists := localMap[modelName]; exists {
			if s.hasChanges(localModel, remoteMeta) {
				diffFields := s.computeDiffFields(localModel, remoteMeta)
				result.Changes.Updated = append(result.Changes.Updated, ModelChange{
					Model:             modelName,
					ModelName:         remoteMeta.ModelName,
					ChangeType:        "updated",
					RemoteData:        remoteMeta,
					LocalData:         localModel,
					DiffFields:        diffFields,
					RecommendedAction: "sync",
				})
				result.Summary.UpdatedModels++
			} else {
				result.Summary.UnchangedModels++
			}
		} else {
			result.Changes.New = append(result.Changes.New, ModelChange{
				Model:             modelName,
				ModelName:         remoteMeta.ModelName,
				ChangeType:        "new",
				RemoteData:        remoteMeta,
				RecommendedAction: "sync",
			})
			result.Summary.NewModels++
		}
	}

	return result, nil
}

// ComputeDeprecated returns local models that no longer exist in upstream (potentially deprecated).
// This is a separate operation from diff to keep concerns independent.
func (s *Service) ComputeDeprecated(ctx context.Context, provider string) (*DeprecatedResult, error) {
	result := &DeprecatedResult{
		Provider:  provider,
		CheckedAt: time.Now(),
		Models:    []ModelChange{},
	}

	// Fetch remote models
	remoteModels, err := s.fetchProviderModels(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote models: %w", err)
	}

	remoteSet := make(map[string]bool, len(remoteModels))
	for _, m := range remoteModels {
		remoteSet[m.Model] = true
	}

	// Fetch local models
	var localModels []llmmodel.LLMModel
	if err := s.db.WithContext(ctx).
		Where("provider = ? AND deleted_at IS NULL", provider).
		Find(&localModels).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch local models: %w", err)
	}

	// Find local models not in upstream
	for i := range localModels {
		if !remoteSet[localModels[i].Model] {
			result.Models = append(result.Models, ModelChange{
				Model:             localModels[i].Model,
				ModelName:         localModels[i].ModelName,
				ChangeType:        "deprecated",
				LocalData:         &localModels[i],
				RecommendedAction: "review",
			})
		}
	}

	result.Total = len(result.Models)
	return result, nil
}

// GetModelInfo retrieves model information from modelmeta.dev
func (s *Service) GetModelInfo(provider, modelName string) (*ModelMetaData, error) {
	models, err := s.fetchProviderModels(provider)
	if err != nil {
		return nil, err
	}

	for _, m := range models {
		if m.Model == modelName {
			return &m, nil
		}
	}

	return nil, fmt.Errorf("model %s not found for provider %s", modelName, provider)
}

// fetchProviderModels fetches ALL models for a provider from modelmeta.dev
// It paginates through all pages (page_size=100) to avoid the default 20-model limit.
func (s *Service) fetchProviderModels(provider string) ([]ModelMetaData, error) {
	if !s.HasConfiguredAPIBaseURL() {
		return nil, errModelMetaAPIURLNotConfigured
	}

	var allModels []ModelMetaData
	page := 1
	const pageSize = 100

	for {
		url := fmt.Sprintf("%s/providers/%s/models?page=%d&page_size=%d", s.apiBaseURL, provider, page, pageSize)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("HTTP request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		var pageResp ModelMetaResponse
		if err := json.Unmarshal(body, &pageResp); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		allModels = append(allModels, pageResp.Data...)

		// Stop if we've fetched all pages
		if page >= pageResp.TotalPages || len(pageResp.Data) == 0 {
			break
		}
		page++
	}

	// Deduplicate by model name: last occurrence wins (most up-to-date data).
	// modelmeta.dev may return multiple entries for the same model identifier
	// (e.g. gpt-3.5-turbo appears twice with different context_window values).
	seen := make(map[string]int, len(allModels))
	var deduped []ModelMetaData
	for _, m := range allModels {
		if idx, exists := seen[m.Model]; exists {
			deduped[idx] = m // overwrite with later (more up-to-date) entry
		} else {
			seen[m.Model] = len(deduped)
			deduped = append(deduped, m)
		}
	}

	return deduped, nil
}

// syncModel syncs a single model to the database
func (s *Service) syncModel(ctx context.Context, meta *ModelMetaData, result *SyncResult) error {
	existing, err := s.findExistingModel(ctx, meta.Provider, meta.Model)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := s.createModel(ctx, meta, result); err != nil {
			if isUniqueConstraintError(err) {
				return s.recoverConflictedModel(ctx, meta, result)
			}
			return err
		}
		return nil
	}
	if err != nil {
		return err
	}

	return s.syncExistingModel(ctx, existing, meta, result)
}

func (s *Service) findExistingModel(ctx context.Context, provider, model string) (*llmmodel.LLMModel, error) {
	var existing llmmodel.LLMModel
	err := s.db.WithContext(ctx).
		Unscoped().
		Select("id", "deleted_at").
		Where("provider = ? AND name = ?", provider, model).
		First(&existing).Error
	if err != nil {
		return nil, err
	}

	return &existing, nil
}

func (s *Service) syncExistingModel(ctx context.Context, existing *llmmodel.LLMModel, meta *ModelMetaData, result *SyncResult) error {
	if existing.DeletedAt.Valid {
		if err := s.restoreModelByID(ctx, existing.ID); err != nil {
			return err
		}
		existing.DeletedAt = gorm.DeletedAt{}
	}

	return s.updateModel(ctx, existing, meta, result)
}

func (s *Service) recoverConflictedModel(ctx context.Context, meta *ModelMetaData, result *SyncResult) error {
	existing, err := s.findExistingModel(ctx, meta.Provider, meta.Model)
	if err != nil {
		return err
	}

	return s.syncExistingModel(ctx, existing, meta, result)
}

func (s *Service) restoreModelByID(ctx context.Context, id uuid.UUID) error {
	return s.db.WithContext(ctx).
		Unscoped().
		Model(&llmmodel.LLMModel{}).
		Where("id = ?", id).
		Update("deleted_at", nil).Error
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key value") ||
		strings.Contains(msg, "violates unique constraint") ||
		strings.Contains(msg, "unique constraint failed")
}

// createModel creates a new model in the database
func (s *Service) createModel(ctx context.Context, meta *ModelMetaData, result *SyncResult) error {
	values := buildPublishedModelColumns(s.db, publishedModelFromMeta(meta))
	values["id"] = uuid.NewString()
	values["created_at"] = time.Now().UTC()
	values["updated_at"] = values["created_at"]

	if err := s.db.WithContext(ctx).Table("llm_models").Create(values).Error; err != nil {
		return err
	}

	result.NewModels++
	return nil
}

// updateModel updates an existing model in the database
func (s *Service) updateModel(ctx context.Context, existing *llmmodel.LLMModel, meta *ModelMetaData, result *SyncResult) error {
	updates := buildPublishedModelColumns(s.db, publishedModelFromMeta(meta))
	updates["updated_at"] = time.Now().UTC()

	tx := s.db.WithContext(ctx).Model(existing).Updates(updates)
	if tx.Error != nil {
		return tx.Error
	}
	if tx.RowsAffected > 0 {
		result.UpdatedModels++
	} else {
		result.SkippedModels++
	}
	return nil
}

func normalizeRemotePrice(value float64) decimal.Decimal {
	return decimal.NewFromFloat(value).Round(priceScale)
}

func normalizeLocalPrice(value decimal.Decimal) decimal.Decimal {
	return value.Round(priceScale)
}

func normalizedPriceValue(value decimal.Decimal) float64 {
	normalized, _ := normalizeLocalPrice(value).Float64()
	return normalized
}

func normalizedRemotePriceValue(value float64) float64 {
	normalized, _ := normalizeRemotePrice(value).Float64()
	return normalized
}

func pricesDiffer(local decimal.Decimal, remote float64) bool {
	return !normalizeLocalPrice(local).Equal(normalizeRemotePrice(remote))
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func useCasesDiffer(local llmmodel.StringArray, remote []string) bool {
	return !equalStringSlices(llmmodel.NormalizeUseCases([]string(local)), llmmodel.NormalizeUseCases(remote))
}

// hasChanges checks if a model has changes compared to remote data
func (s *Service) hasChanges(local *llmmodel.LLMModel, remote *ModelMetaData) bool {
	if local.ModelName != remote.ModelName {
		return true
	}
	if local.ContextWindow != remote.ContextWindow {
		return true
	}
	if remote.MaxOutputTokens > 0 && local.MaxOutputTokens != remote.MaxOutputTokens {
		return true
	}
	if pricesDiffer(local.InputPrice, remote.InputPrice) {
		return true
	}
	if pricesDiffer(local.OutputPrice, remote.OutputPrice) {
		return true
	}
	if pricesDiffer(local.CachedInputPrice, remote.CachedInputPrice) {
		return true
	}
	if local.Status != remote.Status {
		return true
	}
	if local.Tagline != remote.Tagline {
		return true
	}
	if local.Family != remote.Family {
		return true
	}
	if local.FamilyName != remote.FamilyName {
		return true
	}
	if local.FamilyDefault != remote.FamilyDefault {
		return true
	}
	if remote.Description != nil && local.Description != strings.TrimSpace(*remote.Description) {
		return true
	}
	if local.IsFlagship != remote.IsFlagship {
		return true
	}
	if local.IsRecommended != remote.IsRecommended {
		return true
	}
	if local.IsFeatured != remote.IsFeatured {
		return true
	}
	if local.IsNew != remote.IsNew {
		return true
	}
	if local.AccessType != remote.AccessType {
		return true
	}
	if local.Currency != remote.Currency {
		return true
	}
	if useCasesDiffer(local.UseCases, ensureRemoteUseCases(remote)) {
		return true
	}
	if local.KnowledgeCutoff != remote.KnowledgeCutoff {
		return true
	}
	if !equalStringSlices(normalizeStringValues([]string(local.InputModalities)), normalizeStringValues(remote.InputModalities)) {
		return true
	}
	if !equalStringSlices(normalizeStringValues([]string(local.OutputModalities)), normalizeStringValues(remote.OutputModalities)) {
		return true
	}
	return false
}

// computeDiffFields computes the specific fields that have changed
func (s *Service) computeDiffFields(local *llmmodel.LLMModel, remote *ModelMetaData) []DiffField {
	var diffs []DiffField
	remoteUseCases := ensureRemoteUseCases(remote)

	if local.ModelName != remote.ModelName {
		diffs = append(diffs, DiffField{Field: "model_name", OldValue: local.ModelName, NewValue: remote.ModelName})
	}
	if local.ContextWindow != remote.ContextWindow {
		diffs = append(diffs, DiffField{Field: "context_window", OldValue: local.ContextWindow, NewValue: remote.ContextWindow})
	}
	if remote.MaxOutputTokens > 0 && local.MaxOutputTokens != remote.MaxOutputTokens {
		diffs = append(diffs, DiffField{Field: "max_output_tokens", OldValue: local.MaxOutputTokens, NewValue: remote.MaxOutputTokens})
	}
	if pricesDiffer(local.InputPrice, remote.InputPrice) {
		diffs = append(diffs, DiffField{
			Field:    "input_price",
			OldValue: normalizedPriceValue(local.InputPrice),
			NewValue: normalizedRemotePriceValue(remote.InputPrice),
		})
	}
	if pricesDiffer(local.OutputPrice, remote.OutputPrice) {
		diffs = append(diffs, DiffField{
			Field:    "output_price",
			OldValue: normalizedPriceValue(local.OutputPrice),
			NewValue: normalizedRemotePriceValue(remote.OutputPrice),
		})
	}
	if pricesDiffer(local.CachedInputPrice, remote.CachedInputPrice) {
		diffs = append(diffs, DiffField{
			Field:    "cached_input_price",
			OldValue: normalizedPriceValue(local.CachedInputPrice),
			NewValue: normalizedRemotePriceValue(remote.CachedInputPrice),
		})
	}
	if local.Status != remote.Status {
		diffs = append(diffs, DiffField{Field: "status", OldValue: local.Status, NewValue: remote.Status})
	}
	if local.Tagline != remote.Tagline {
		diffs = append(diffs, DiffField{Field: "tagline", OldValue: local.Tagline, NewValue: remote.Tagline})
	}
	if local.Family != remote.Family {
		diffs = append(diffs, DiffField{Field: "family", OldValue: local.Family, NewValue: remote.Family})
	}
	if local.FamilyName != remote.FamilyName {
		diffs = append(diffs, DiffField{Field: "family_name", OldValue: local.FamilyName, NewValue: remote.FamilyName})
	}
	if local.FamilyDefault != remote.FamilyDefault {
		diffs = append(diffs, DiffField{Field: "family_default", OldValue: local.FamilyDefault, NewValue: remote.FamilyDefault})
	}
	if remote.Description != nil {
		remoteDescription := strings.TrimSpace(*remote.Description)
		if local.Description != remoteDescription {
			diffs = append(diffs, DiffField{Field: "description", OldValue: local.Description, NewValue: remoteDescription})
		}
	}
	if local.IsFlagship != remote.IsFlagship {
		diffs = append(diffs, DiffField{Field: "is_flagship", OldValue: local.IsFlagship, NewValue: remote.IsFlagship})
	}
	if local.IsRecommended != remote.IsRecommended {
		diffs = append(diffs, DiffField{Field: "is_recommended", OldValue: local.IsRecommended, NewValue: remote.IsRecommended})
	}
	if local.IsFeatured != remote.IsFeatured {
		diffs = append(diffs, DiffField{Field: "is_featured", OldValue: local.IsFeatured, NewValue: remote.IsFeatured})
	}
	if local.IsNew != remote.IsNew {
		diffs = append(diffs, DiffField{Field: "is_new", OldValue: local.IsNew, NewValue: remote.IsNew})
	}
	if local.AccessType != remote.AccessType {
		diffs = append(diffs, DiffField{Field: "access_type", OldValue: local.AccessType, NewValue: remote.AccessType})
	}
	if local.Currency != remote.Currency {
		diffs = append(diffs, DiffField{Field: "currency", OldValue: local.Currency, NewValue: remote.Currency})
	}
	if useCasesDiffer(local.UseCases, remoteUseCases) {
		diffs = append(diffs, DiffField{
			Field:    "use_cases",
			OldValue: llmmodel.NormalizeUseCases([]string(local.UseCases)),
			NewValue: remoteUseCases,
		})
	}
	if local.KnowledgeCutoff != remote.KnowledgeCutoff {
		diffs = append(diffs, DiffField{Field: "knowledge_cutoff", OldValue: local.KnowledgeCutoff, NewValue: remote.KnowledgeCutoff})
	}
	if !equalStringSlices(normalizeStringValues([]string(local.InputModalities)), normalizeStringValues(remote.InputModalities)) {
		diffs = append(diffs, DiffField{
			Field:    "input_modalities",
			OldValue: normalizeStringValues([]string(local.InputModalities)),
			NewValue: normalizeStringValues(remote.InputModalities),
		})
	}
	if !equalStringSlices(normalizeStringValues([]string(local.OutputModalities)), normalizeStringValues(remote.OutputModalities)) {
		diffs = append(diffs, DiffField{
			Field:    "output_modalities",
			OldValue: normalizeStringValues([]string(local.OutputModalities)),
			NewValue: normalizeStringValues(remote.OutputModalities),
		})
	}

	return diffs
}

func publishedModelFromMeta(meta *ModelMetaData) PublishedModel {
	endpoints := parsePublishedModelEndpoints(meta.Endpoints)
	features := parsePublishedModelFeatures(meta.Features)
	tools := parsePublishedModelTools(meta.Tools)
	parameters := parsePublishedModelParameters(meta.Parameters)
	description := normalizeOptionalString(meta.Description)

	return PublishedModel{
		Provider:               meta.Provider,
		Model:                  meta.Model,
		ModelName:              meta.ModelName,
		Family:                 meta.Family,
		FamilyName:             meta.FamilyName,
		FamilyDefault:          meta.FamilyDefault,
		Status:                 meta.Status,
		Tagline:                meta.Tagline,
		Description:            description,
		IsFlagship:             meta.IsFlagship,
		IsRecommended:          meta.IsRecommended,
		IsFeatured:             meta.IsFeatured,
		IsNew:                  meta.IsNew,
		AccessType:             meta.AccessType,
		Currency:               meta.Currency,
		ContextWindow:          meta.ContextWindow,
		MaxOutputTokens:        meta.MaxOutputTokens,
		InputPrice:             meta.InputPrice,
		OutputPrice:            meta.OutputPrice,
		CachedInputPrice:       meta.CachedInputPrice,
		UseCases:               llmmodel.EnsureUseCases(meta.UseCases, endpoints),
		InputModalities:        normalizeStringValues(meta.InputModalities),
		OutputModalities:       normalizeStringValues(meta.OutputModalities),
		KnowledgeCutoff:        strings.TrimSpace(meta.KnowledgeCutoff),
		SupportedParameters:    marshalJSONRaw(meta.Parameters),
		ConfigParameters:       meta.ConfigParameters,
		Endpoints:              endpoints,
		EndpointsAuthoritative: isAuthoritativeEndpointPayload(meta.Endpoints),
		Features:               features,
		Tools:                  tools,
		Parameters:             parameters,
	}
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	return &normalized
}

func ensureRemoteUseCases(meta *ModelMetaData) []string {
	return publishedModelFromMeta(meta).UseCases
}

func normalizeStringValues(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func parsePublishedModelEndpoints(raw map[string]interface{}) *llmmodel.ModelEndpoints {
	if len(raw) == 0 {
		return nil
	}
	return &llmmodel.ModelEndpoints{
		ChatCompletions:  boolValue(raw, "chat_completions"),
		Responses:        boolValue(raw, "responses"),
		Realtime:         boolValue(raw, "realtime"),
		Assistants:       boolValue(raw, "assistants"),
		Batch:            boolValue(raw, "batch"),
		Embeddings:       boolValue(raw, "embeddings"),
		FineTuning:       boolValue(raw, "fine_tuning"),
		ImageGeneration:  boolValue(raw, "image_generation"),
		Vision:           boolValue(raw, "vision"),
		SpeechGeneration: boolValue(raw, "speech_generation"),
		Transcription:    boolValue(raw, "transcription"),
		Translation:      boolValue(raw, "translation"),
		Moderation:       boolValue(raw, "moderation"),
		Videos:           boolValue(raw, "videos"),
		ImageEdit:        boolValue(raw, "image_edit"),
	}
}

func isAuthoritativeEndpointPayload(raw map[string]interface{}) bool {
	_, ok := raw["chat_completions"]
	return ok
}

func parsePublishedModelFeatures(raw map[string]interface{}) *llmmodel.ModelFeatures {
	if len(raw) == 0 {
		return nil
	}
	return &llmmodel.ModelFeatures{
		Streaming:        boolValue(raw, "streaming"),
		FunctionCalling:  boolValue(raw, "function_calling"),
		StructuredOutput: boolValue(raw, "structured_output"),
		JsonMode:         boolValue(raw, "json_mode"),
		Distillation:     boolValue(raw, "distillation"),
		Reasoning:        boolValue(raw, "reasoning"),
		SystemPrompt:     boolValue(raw, "system_prompt"),
		Logprobs:         boolValue(raw, "logprobs"),
		WebSearch:        boolValue(raw, "web_search"),
		FileSearch:       boolValue(raw, "file_search"),
		CodeInterpreter:  boolValue(raw, "code_interpreter"),
		ComputerUse:      boolValue(raw, "computer_use"),
		Mcp:              boolValue(raw, "mcp"),
		ReasoningEffort:  boolValue(raw, "reasoning_effort"),
	}
}

func parsePublishedModelTools(raw map[string]interface{}) *llmmodel.ModelTools {
	if len(raw) == 0 {
		return nil
	}
	return &llmmodel.ModelTools{
		WebSearch:         boolValue(raw, "web_search"),
		FileSearch:        boolValue(raw, "file_search"),
		ImageGeneration:   boolValue(raw, "image_generation"),
		CodeInterpreter:   boolValue(raw, "code_interpreter"),
		ComputerUse:       boolValue(raw, "computer_use"),
		Mcp:               boolValue(raw, "mcp"),
		ParallelToolCalls: boolValue(raw, "parallel_tool_calls"),
	}
}

func parsePublishedModelParameters(raw map[string]interface{}) *llmmodel.ModelParameters {
	if len(raw) == 0 {
		return nil
	}
	params := llmmodel.DefaultParameters()
	params.SupportsTemperature = parameterSupported(raw, "temperature", true)
	params.SupportsTopP = parameterSupported(raw, "top_p", false)
	params.SupportsPresencePenalty = parameterSupported(raw, "presence_penalty", false)
	params.SupportsFrequencyPenalty = parameterSupported(raw, "frequency_penalty", false)
	params.SupportsLogitBias = parameterSupported(raw, "logit_bias", false)
	params.SupportsSeed = boolValueWithFallback(raw, "supports_seed", parameterSupported(raw, "seed", false))
	params.SupportsStop = boolValueWithFallback(raw, "supports_stop", parameterSupported(raw, "stop", true))
	params.MaxStopSequences = intValue(raw, "max_stop_sequences", params.MaxStopSequences)
	return &params
}

func boolValue(raw map[string]interface{}, key string) bool {
	value, ok := raw[key]
	if !ok {
		return false
	}
	boolValue, ok := value.(bool)
	return ok && boolValue
}

func boolValueWithFallback(raw map[string]interface{}, key string, fallback bool) bool {
	value, ok := raw[key]
	if !ok {
		return fallback
	}
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func intValue(raw map[string]interface{}, key string, fallback int) int {
	value, ok := raw[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return fallback
	}
}

func parameterSupported(raw map[string]interface{}, key string, fallback bool) bool {
	value, ok := raw[key]
	if !ok {
		return fallback
	}
	definition, ok := value.(map[string]interface{})
	if !ok {
		return fallback
	}
	supported, exists := definition["supported"]
	if !exists {
		return true
	}
	supportedBool, ok := supported.(bool)
	if !ok {
		return fallback
	}
	return supportedBool
}

func marshalJSONRaw(value interface{}) json.RawMessage {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}

// ============================================================================
// Provider Sync Methods
// ============================================================================

// ProviderSyncResult represents the result of provider sync operation
type ProviderSyncResult struct {
	TotalProviders   int      `json:"total_providers"`
	CreatedProviders int      `json:"created_providers"`
	UpdatedProviders int      `json:"updated_providers"`
	Errors           []string `json:"errors,omitempty"`
	DurationMs       int64    `json:"duration_ms"`
}

// SyncProviders syncs all providers from ModelMeta API
func (s *Service) SyncProviders(ctx context.Context) (*ProviderSyncResult, error) {
	startTime := time.Now()
	result := &ProviderSyncResult{
		Errors: []string{},
	}

	// Fetch providers from ModelMeta
	providers, err := s.fetchProviders()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch providers: %w", err)
	}

	result.TotalProviders = len(providers)

	// Sync each provider
	for _, provider := range providers {
		if err := s.syncProvider(ctx, &provider, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("provider %s: %v", provider.Provider, err))
		}
	}

	result.DurationMs = time.Since(startTime).Milliseconds()
	return result, nil
}

// SyncProviderWithModels syncs a provider and all its models
func (s *Service) SyncProviderWithModels(ctx context.Context, providerName string) (*SyncResult, error) {
	// 1. Sync provider first
	providerResult := &ProviderSyncResult{Errors: []string{}}

	providers, err := s.fetchProviders()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch providers: %w", err)
	}

	// Find the specific provider
	var found *ModelMetaProvider
	for i := range providers {
		if providers[i].Provider == providerName {
			found = &providers[i]
			break
		}
	}

	if found == nil {
		return nil, fmt.Errorf("provider %s not found in ModelMeta API", providerName)
	}

	// Sync provider
	if err := s.syncProvider(ctx, found, providerResult); err != nil {
		return nil, fmt.Errorf("failed to sync provider: %w", err)
	}

	// 2. Sync models (reuse existing logic)
	return s.SyncProviderModels(ctx, providerName, nil)
}

// fetchProviders fetches all providers from ModelMeta API
func (s *Service) fetchProviders() ([]ModelMetaProvider, error) {
	if !s.HasConfiguredAPIBaseURL() {
		return nil, errModelMetaAPIURLNotConfigured
	}

	url := fmt.Sprintf("%s/providers?limit=500", s.apiBaseURL)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var response ModelMetaProviderResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	return response.Data, nil
}

// syncProvider syncs a single provider to the database
func (s *Service) syncProvider(ctx context.Context, meta *ModelMetaProvider, result *ProviderSyncResult) error {
	// Check if provider exists
	var existing struct {
		ID          uuid.UUID `gorm:"column:id"`
		Name        string    `gorm:"column:provider"`
		DisplayName string    `gorm:"column:provider_name"`
	}

	err := s.db.WithContext(ctx).
		Table("llm_providers").
		Where("provider = ?", meta.Provider).
		First(&existing).Error

	if err == gorm.ErrRecordNotFound {
		// Create new provider
		return s.createProvider(ctx, meta, result)
	} else if err != nil {
		return fmt.Errorf("failed to query provider: %w", err)
	}

	// Update existing provider
	return s.updateProvider(ctx, &existing, meta, result)
}

// createProvider creates a new provider in the database
func (s *Service) createProvider(ctx context.Context, meta *ModelMetaProvider, result *ProviderSyncResult) error {
	newProvider := map[string]interface{}{
		"id":                uuid.New(),
		"provider":          meta.Provider,
		"provider_name":     meta.Name,
		"logo_url":          meta.LogoURL,
		"website":           meta.Website,
		"documentation_url": meta.APIDocsURL,
		"pricing_url":       meta.PricingURL,
		"country_code":      meta.CountryCode,
		"founded_year":      meta.FoundedYear,
		"tagline":           meta.Tagline,
		"description":       meta.Description,
		"metadata":          serializeProviderMetadata(meta.Metadata),
		"is_active":         true,
		"created_at":        time.Now(),
		"updated_at":        time.Now(),
	}

	if err := s.db.WithContext(ctx).Table("llm_providers").Create(newProvider).Error; err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	result.CreatedProviders++
	return nil
}

// updateProvider updates an existing provider in the database
func (s *Service) updateProvider(ctx context.Context, existing interface{}, meta *ModelMetaProvider, result *ProviderSyncResult) error {
	updates := map[string]interface{}{
		"provider_name":     meta.Name,
		"logo_url":          meta.LogoURL,
		"website":           meta.Website,
		"documentation_url": meta.APIDocsURL,
		"pricing_url":       meta.PricingURL,
		"country_code":      meta.CountryCode,
		"founded_year":      meta.FoundedYear,
		"tagline":           meta.Tagline,
		"description":       meta.Description,
		"metadata":          serializeProviderMetadata(meta.Metadata),
		"updated_at":        time.Now(),
	}

	if err := s.db.WithContext(ctx).
		Table("llm_providers").
		Where("provider = ?", meta.Provider).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update provider: %w", err)
	}

	result.UpdatedProviders++
	return nil
}

func serializeProviderMetadata(metadata map[string]interface{}) datatypes.JSON {
	return datatypes.JSON([]byte(serializeJSONMap(metadata)))
}
