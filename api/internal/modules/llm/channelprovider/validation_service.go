package channelprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	capabilityCacheTTL         = 5 * time.Minute
	sampledValidationSize      = 5
	sampledValidationMinPass   = 4
	validationModeFull         = "full"
	validationModeSampled      = "sampled"
	validationModeMetadataOnly = "metadata_only"
	testMethodChat             = "chat"
	testMethodEmbedding        = "embedding"
	testMethodImageGeneration  = "image-gen"
	testMethodMetadata         = "metadata"
	testMethodRerank           = "rerank"
	openAICompatibleProvider   = "openai-compatible"
	defaultImageProbeSize      = "1024x1024"
)

type modelLookupRepository interface {
	ListByNames(ctx context.Context, names []string) ([]*llmmodelmodel.LLMModel, error)
}

type cachedModelCapability struct {
	capability modelCapability
	expiresAt  time.Time
}

type modelCapability struct {
	Model   string
	UseCase string
}

type validationFailure struct {
	Model   string
	UseCase string
	Message string
}

// ValidationResult is the normalized output returned by the channel validator.
type ValidationResult struct {
	Report           map[string]any
	Warnings         []string
	NormalizedModels []string
}

// Validator validates channels against local model metadata and upstream capabilities.
type Validator struct {
	modelRepo     modelLookupRepository
	privateModels llmmodelsvc.PrivateModelLookupService
	newAdapter    func(config *adapter.AdapterConfig) (adapter.LLMProviderAdapter, error)
	now           func() time.Time
	cacheTTL      time.Duration

	mu    sync.RWMutex
	cache map[string]cachedModelCapability
}

// NewValidator creates a new usecase-aware channel validator.
func NewValidator(modelRepo llmmodelrepo.ModelRepository, privateModels llmmodelsvc.PrivateModelLookupService) *Validator {
	return &Validator{
		modelRepo:     modelRepo,
		privateModels: privateModels,
		newAdapter:    adapter.NewAdapter,
		now:           time.Now,
		cacheTTL:      capabilityCacheTTL,
		cache:         make(map[string]cachedModelCapability),
	}
}

// ValidateModels validates channel models using model listing when available,
// and falls back to bounded sample validation when it is not.
func (v *Validator) ValidateModels(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL string, models []string) (*ValidationResult, error) {
	spec, err := Resolve(channelProvider)
	if err != nil {
		return nil, err
	}

	normalizedModels := normalizeModelSet(models)
	report := v.newValidationReport(spec.Name, apiBaseURL, normalizedModels)
	result := &ValidationResult{
		Report:           report,
		NormalizedModels: normalizedModels,
	}

	if len(normalizedModels) == 0 {
		report[keyValidationMode] = validationModeFull
		report[keySampled] = false
		report[keySampleSize] = 0
		report[keyValidatedCount] = 0
		report[keyPassedCount] = 0
		report[keyFailedModels] = []map[string]any{}
		report[keyUnvalidatedCount] = 0
		report[keyWarningMessages] = []string{}
		report[keyProbedModels] = []string{}
		report[keyModelListingVerified] = false
		return result, nil
	}

	capabilities, err := v.resolveModelCapabilities(ctx, organizationID, spec, normalizedModels)
	if err != nil {
		return result, err
	}

	adapterInstance, err := v.newAdapterForProvider(spec.AdapterKey, apiBaseURL, apiKey)
	if err != nil {
		return result, fmt.Errorf("failed to create adapter: %w", err)
	}

	upstreamModels, err := adapterInstance.ListModels(ctx, apiKey)
	if err == nil {
		return v.validateWithModelListing(ctx, adapterInstance, normalizedModels, capabilities, upstreamModels, report)
	}
	if !adapter.IsCapabilityUnsupported(err) {
		normalizedErr := normalizeValidationError(err)
		if normalizedErr == providerAPIKeyInvalidMessage {
			return result, newProviderAPIKeyInvalidError(fmt.Errorf("failed to list upstream models: %w", err))
		}
		return result, fmt.Errorf("failed to list upstream models: %s", normalizedErr)
	}

	return v.validateWithoutModelListing(ctx, spec.Name, apiBaseURL, adapterInstance, normalizedModels, capabilities, report)
}

// ValidateModelsForCreation validates local model metadata for channel creation.
// It does not call upstream providers; explicit test endpoints own provider probes.
func (v *Validator) ValidateModelsForCreation(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL string, models []string) (*ValidationResult, error) {
	spec, err := Resolve(channelProvider)
	if err != nil {
		return nil, err
	}

	normalizedModels := normalizeModelSet(models)
	report := v.newValidationReport(spec.Name, apiBaseURL, normalizedModels)
	result := &ValidationResult{
		Report:           report,
		NormalizedModels: normalizedModels,
	}

	if len(normalizedModels) == 0 {
		report[keyValidationMode] = validationModeMetadataOnly
		report[keySampled] = false
		report[keySampleSize] = 0
		report[keyValidatedCount] = 0
		report[keyPassedCount] = 0
		report[keyFailedModels] = []map[string]any{}
		report[keyUnvalidatedCount] = 0
		report[keyWarningMessages] = []string{}
		report[keyProbedModels] = []string{}
		report[keyModelListingVerified] = false
		return result, fmt.Errorf("at least one model is required")
	}

	capabilities, err := v.resolveModelCapabilities(ctx, organizationID, spec, normalizedModels)
	if err != nil {
		return result, err
	}

	return creationMetadataOnlyResult(normalizedModels, capabilities, report), nil
}

func creationMetadataOnlyResult(
	normalizedModels []string,
	capabilities []modelCapability,
	report map[string]any,
) *ValidationResult {
	items := make([]map[string]any, 0, len(capabilities))
	for _, capability := range capabilities {
		items = append(items, map[string]any{
			keyModel:          capability.Model,
			keyUseCase:        capability.UseCase,
			keySuccess:        true,
			keyMessage:        "validated local model metadata",
			keyResponseTimeMs: int64(0),
		})
	}

	report[keyItems] = items
	report[keyValidationMode] = validationModeMetadataOnly
	report[keySampled] = false
	report[keySampleSize] = 0
	report[keyValidatedCount] = len(normalizedModels)
	report[keyPassedCount] = len(normalizedModels)
	report[keyFailedModels] = []map[string]any{}
	report[keyUnvalidatedCount] = 0
	report[keyWarningMessages] = []string{}
	report[keyProbedModels] = []string{}
	report[keyModelListingVerified] = false

	return &ValidationResult{
		Report:           report,
		Warnings:         []string{},
		NormalizedModels: normalizedModels,
	}
}

func upstreamModelNameSet(upstreamModels []adapter.Model) map[string]struct{} {
	upstreamSet := make(map[string]struct{}, len(upstreamModels)*2)
	for _, model := range upstreamModels {
		if id := strings.TrimSpace(model.ID); id != "" {
			upstreamSet[id] = struct{}{}
		}
		if name := strings.TrimSpace(model.Name); name != "" {
			upstreamSet[name] = struct{}{}
		}
	}
	return upstreamSet
}

func modelExistsInUpstreamSet(upstreamSet map[string]struct{}, modelName string) bool {
	_, ok := upstreamSet[strings.TrimSpace(modelName)]
	return ok
}

// TestModel validates a single model using the model library as the source of truth.
func (v *Validator) TestModel(ctx context.Context, organizationID uuid.UUID, channelProvider, apiKey, apiBaseURL, modelName, testMethod string, stream bool) (*TestResult, error) {
	spec, err := Resolve(channelProvider)
	if err != nil {
		return nil, err
	}

	normalizedModel := strings.TrimSpace(modelName)
	if normalizedModel == "" {
		return nil, fmt.Errorf("model is required")
	}

	capabilities, err := v.resolveModelCapabilities(ctx, organizationID, spec, []string{normalizedModel})
	if err != nil {
		return nil, err
	}

	useCase := capabilities[0].UseCase
	explicitTestMethod := strings.TrimSpace(testMethod) != ""
	if explicitTestMethod {
		override, err := normalizeTestMethod(testMethod)
		if err != nil {
			return nil, err
		}
		if override != useCase {
			return nil, fmt.Errorf("test method %q conflicts with model %q use case %q", testMethod, normalizedModel, useCase)
		}
	}

	if useCase == testMethodImageGeneration && !explicitTestMethod {
		return v.skipImageModelTest(capabilities[0]), nil
	}

	adapterInstance, err := v.newAdapterForProvider(spec.AdapterKey, apiBaseURL, apiKey)
	if err != nil {
		return &TestResult{
			Success:        false,
			Status:         TestStatusFailed,
			Message:        fmt.Sprintf("failed to create adapter: %v", err),
			ResponseTimeMs: 0,
			Model:          normalizedModel,
			UseCase:        useCase,
			TestMethod:     useCase,
		}, nil
	}

	return v.probeModel(ctx, adapterInstance, capabilities[0], stream), nil
}

func (v *Validator) validateWithModelListing(
	ctx context.Context,
	adapterInstance adapter.LLMProviderAdapter,
	normalizedModels []string,
	capabilities []modelCapability,
	upstreamModels []adapter.Model,
	report map[string]any,
) (*ValidationResult, error) {
	upstreamSet := upstreamModelNameSet(upstreamModels)

	failures := make([]validationFailure, 0)
	for _, capability := range capabilities {
		if !modelExistsInUpstreamSet(upstreamSet, capability.Model) {
			failures = append(failures, validationFailure{
				Model:   capability.Model,
				UseCase: capability.UseCase,
				Message: "model is not returned by upstream model list",
			})
		}
	}

	probeTargets := pickRepresentativeModels(capabilities)
	items, probeFailures := v.probeModels(ctx, adapterInstance, probeTargets)
	failures = append(failures, probeFailures...)

	report[keyItems] = items
	report[keyValidationMode] = validationModeFull
	report[keySampled] = false
	report[keySampleSize] = len(probeTargets)
	report[keyValidatedCount] = len(normalizedModels)
	report[keyPassedCount] = len(normalizedModels) - len(failures)
	report[keyFailedModels] = failuresToReport(failures)
	report[keyUnvalidatedCount] = 0
	report[keyWarningMessages] = []string{}
	report[keyProbedModels] = capabilityModels(probeTargets)
	report[keyModelListingVerified] = true

	result := &ValidationResult{
		Report:           report,
		Warnings:         []string{},
		NormalizedModels: normalizedModels,
	}
	if len(failures) > 0 {
		return result, newValidationFailureError("channel validation failed", failures)
	}
	return result, nil
}

func (v *Validator) validateWithoutModelListing(
	ctx context.Context,
	channelProvider string,
	apiBaseURL string,
	adapterInstance adapter.LLMProviderAdapter,
	normalizedModels []string,
	capabilities []modelCapability,
	report map[string]any,
) (*ValidationResult, error) {
	probeTargets := capabilities
	validationMode := validationModeFull
	if len(capabilities) > sampledValidationSize {
		probeTargets = stableSampleCapabilities(channelProvider, apiBaseURL, capabilities, sampledValidationSize)
		validationMode = validationModeSampled
	}

	items, failures := v.probeModels(ctx, adapterInstance, probeTargets)
	passCount := len(probeTargets) - len(failures)
	unvalidatedCount := len(normalizedModels) - len(probeTargets)
	warnings := buildWarnings(validationMode, normalizedModels, probeTargets, failures, unvalidatedCount)

	report[keyItems] = items
	report[keyValidationMode] = validationMode
	report[keySampled] = validationMode == validationModeSampled
	report[keySampleSize] = len(probeTargets)
	report[keyValidatedCount] = len(probeTargets)
	report[keyPassedCount] = passCount
	report[keyFailedModels] = failuresToReport(failures)
	report[keyUnvalidatedCount] = unvalidatedCount
	report[keyWarningMessages] = warnings
	report[keyProbedModels] = capabilityModels(probeTargets)
	report[keyModelListingVerified] = false

	result := &ValidationResult{
		Report:           report,
		Warnings:         warnings,
		NormalizedModels: normalizedModels,
	}

	if validationMode == validationModeFull {
		if len(failures) > 0 {
			return result, newValidationFailureError("channel validation failed", failures)
		}
		return result, nil
	}

	if passCount < sampledValidationMinPass {
		return result, newValidationFailureError(
			fmt.Sprintf("sample validation failed: %d/%d sampled models passed; failures", passCount, len(probeTargets)),
			failures,
		)
	}

	return result, nil
}

func (v *Validator) probeModels(ctx context.Context, adapterInstance adapter.LLMProviderAdapter, capabilities []modelCapability) ([]map[string]any, []validationFailure) {
	items := make([]map[string]any, 0, len(capabilities))
	failures := make([]validationFailure, 0)

	for _, capability := range capabilities {
		result := v.probeModel(ctx, adapterInstance, capability, false)
		items = append(items, map[string]any{
			keyModel:          capability.Model,
			keyUseCase:        capability.UseCase,
			keySuccess:        result.Success,
			keyMessage:        result.Message,
			keyResponseTimeMs: result.ResponseTimeMs,
		})
		if !result.Success {
			failures = append(failures, validationFailure{
				Model:   capability.Model,
				UseCase: capability.UseCase,
				Message: result.Message,
			})
		}
	}

	return items, failures
}

func (v *Validator) probeModel(ctx context.Context, adapterInstance adapter.LLMProviderAdapter, capability modelCapability, stream bool) *TestResult {
	startTime := v.now()
	result := &TestResult{
		Model:      capability.Model,
		UseCase:    capability.UseCase,
		TestMethod: capability.UseCase,
	}

	var (
		responseContent string
		err             error
	)

	switch capability.UseCase {
	case testMethodEmbedding:
		err = runEmbeddingProbe(ctx, adapterInstance, capability.Model)
	case testMethodImageGeneration:
		err = runImageProbe(ctx, adapterInstance, capability.Model)
	case testMethodRerank:
		err = runRerankProbe(ctx, adapterInstance, capability.Model)
	default:
		responseContent, err = runChatProbe(ctx, adapterInstance, capability.Model, stream)
	}

	result.ResponseTimeMs = v.now().Sub(startTime).Milliseconds()
	if err != nil {
		result.Success = false
		result.Status = TestStatusFailed
		result.Message = normalizeValidationError(err)
		return result
	}

	result.Success = true
	result.Status = TestStatusSuccess
	result.Message = "ok"
	result.Response = responseContent
	return result
}

func (v *Validator) skipImageModelTest(capability modelCapability) *TestResult {
	return &TestResult{
		Success:        false,
		Status:         TestStatusSkipped,
		Message:        "image generation models require a real image generation test in the image workspace",
		ResponseTimeMs: 0,
		Model:          capability.Model,
		UseCase:        capability.UseCase,
		TestMethod:     testMethodImageGeneration,
	}
}

func (v *Validator) resolveModelCapabilities(ctx context.Context, organizationID uuid.UUID, spec Spec, models []string) ([]modelCapability, error) {
	if v.modelRepo == nil && v.privateModels == nil {
		return nil, fmt.Errorf("model repository is required for validation")
	}

	now := v.now()
	cacheScope := strings.TrimSpace(spec.Name)
	capabilityMap := make(map[string]modelCapability, len(models))
	missing := make([]string, 0)

	v.mu.RLock()
	for _, modelName := range models {
		cacheKey := capabilityCacheKey(organizationID, cacheScope, modelName)
		if cached, ok := v.cache[cacheKey]; ok && now.Before(cached.expiresAt) {
			capabilityMap[modelName] = cached.capability
			continue
		}
		missing = append(missing, modelName)
	}
	v.mu.RUnlock()

	if len(missing) > 0 {
		found := make(map[string]struct{}, len(missing))

		if v.privateModels != nil && organizationID != uuid.Nil {
			if isProviderScopedPrivateModelValidation(spec) {
				privateRecords, err := v.privateModels.ResolveActiveModelsForProvider(ctx, organizationID, spec.Name, missing)
				if err != nil {
					return nil, fmt.Errorf("failed to load private model metadata: %w", err)
				}
				if err := v.addPrivateCapabilities(organizationID, cacheScope, now, capabilityMap, found, privateRecords); err != nil {
					return nil, err
				}
			} else {
				privateRecords, err := v.privateModels.ListActiveModelsByNames(ctx, organizationID, missing)
				if err != nil {
					return nil, fmt.Errorf("failed to load private model metadata: %w", err)
				}
				if err := v.addProtocolPrivateCapabilities(organizationID, cacheScope, now, capabilityMap, found, privateRecords); err != nil {
					return nil, err
				}
			}
		}

		remaining := unresolvedModelNames(missing, found)

		if len(remaining) > 0 {
			if v.modelRepo == nil {
				return nil, newUnregisteredModelError(remaining[0])
			}

			records, err := v.modelRepo.ListByNames(ctx, remaining)
			if err != nil {
				return nil, fmt.Errorf("failed to load model metadata: %w", err)
			}

			v.mu.Lock()
			for _, record := range records {
				useCase, err := inferValidationUseCase(record)
				if err != nil {
					v.mu.Unlock()
					return nil, err
				}

				capability := modelCapability{
					Model:   record.Model,
					UseCase: useCase,
				}
				capabilityMap[record.Model] = capability
				v.cache[capabilityCacheKey(organizationID, cacheScope, record.Model)] = cachedModelCapability{
					capability: capability,
					expiresAt:  now.Add(v.cacheTTL),
				}
				found[record.Model] = struct{}{}
			}
			v.mu.Unlock()
		}

		for _, modelName := range missing {
			if _, ok := found[modelName]; !ok {
				return nil, newUnregisteredModelError(modelName)
			}
		}
	}

	resolved := make([]modelCapability, 0, len(models))
	for _, modelName := range models {
		capability, ok := capabilityMap[modelName]
		if !ok {
			return nil, fmt.Errorf("model %q capability could not be resolved", modelName)
		}
		resolved = append(resolved, capability)
	}

	return resolved, nil
}

func isProviderScopedPrivateModelValidation(spec Spec) bool {
	return strings.TrimSpace(spec.Name) != openAICompatibleProvider
}

func (v *Validator) addPrivateCapabilities(
	organizationID uuid.UUID,
	cacheScope string,
	now time.Time,
	capabilityMap map[string]modelCapability,
	found map[string]struct{},
	records []*llmmodelmodel.CustomModel,
) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, record := range records {
		if record == nil {
			continue
		}
		useCase, err := inferValidationUseCaseFromCustomModel(record)
		if err != nil {
			return err
		}

		capability := modelCapability{
			Model:   record.Name,
			UseCase: useCase,
		}
		capabilityMap[record.Name] = capability
		v.cache[capabilityCacheKey(organizationID, cacheScope, record.Name)] = cachedModelCapability{
			capability: capability,
			expiresAt:  now.Add(v.cacheTTL),
		}
		found[record.Name] = struct{}{}
	}

	return nil
}

func (v *Validator) addProtocolPrivateCapabilities(
	organizationID uuid.UUID,
	cacheScope string,
	now time.Time,
	capabilityMap map[string]modelCapability,
	found map[string]struct{},
	records []*llmmodelmodel.CustomModel,
) error {
	collapsed := make(map[string]modelCapability, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		useCase, err := inferValidationUseCaseFromCustomModel(record)
		if err != nil {
			return err
		}
		capability := modelCapability{
			Model:   record.Name,
			UseCase: useCase,
		}
		if existing, ok := collapsed[record.Name]; ok && existing.UseCase != capability.UseCase {
			return fmt.Errorf("private model %q has conflicting use cases across custom providers", record.Name)
		}
		collapsed[record.Name] = capability
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	for modelName, capability := range collapsed {
		capabilityMap[modelName] = capability
		v.cache[capabilityCacheKey(organizationID, cacheScope, modelName)] = cachedModelCapability{
			capability: capability,
			expiresAt:  now.Add(v.cacheTTL),
		}
		found[modelName] = struct{}{}
	}

	return nil
}

func unresolvedModelNames(modelNames []string, found map[string]struct{}) []string {
	remaining := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		if _, ok := found[modelName]; ok {
			continue
		}
		remaining = append(remaining, modelName)
	}
	return remaining
}

func capabilityCacheKey(organizationID uuid.UUID, channelProvider string, modelName string) string {
	return organizationID.String() + ":" + strings.TrimSpace(channelProvider) + ":" + strings.TrimSpace(modelName)
}

func newUnregisteredModelError(modelName string) error {
	normalizedModelName := strings.TrimSpace(modelName)
	return fmt.Errorf("model %q is not registered in the local model library; use the full model name exactly as shown in the model list", normalizedModelName)
}

func (v *Validator) newAdapterForProvider(adapterKey, baseURL, apiKey string) (adapter.LLMProviderAdapter, error) {
	return v.newAdapter(&adapter.AdapterConfig{
		ProviderName:        adapterKey,
		APIKey:              apiKey,
		BaseURL:             baseURL,
		Timeout:             defaultTimeout,
		MaxRetries:          defaultMaxRetries,
		GuardOutboundURL:    outboundURLGuardEnabled(),
		GuardOutboundDNS:    outboundDNSGuardEnabled(),
		AllowPrivateBaseURL: AllowsPrivateBaseURL(adapterKey),
	})
}

func (v *Validator) newValidationReport(channelProvider, baseURL string, models []string) map[string]any {
	return map[string]any{
		keyProvider:  channelProvider,
		keyBaseURL:   baseURL,
		keyModels:    append([]string(nil), models...),
		keyCheckedAt: v.now().Unix(),
		keyItems:     []map[string]any{},
	}
}

func WarningMessages(report map[string]any) []string {
	rawWarnings, ok := report[keyWarningMessages]
	if !ok {
		return nil
	}

	switch value := rawWarnings.(type) {
	case []string:
		return append([]string(nil), value...)
	case []any:
		warnings := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				warnings = append(warnings, text)
			}
		}
		return warnings
	default:
		return nil
	}
}

func normalizeModelSet(models []string) []string {
	normalized := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, modelName := range models {
		trimmed := strings.TrimSpace(modelName)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "*:") {
			trimmed = "*"
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func inferValidationUseCase(modelRecord *llmmodelmodel.LLMModel) (string, error) {
	switch {
	case modelRecord.IsEmbedding() || modelRecord.Embeddings:
		return testMethodEmbedding, nil
	case modelRecord.IsRerank():
		return testMethodRerank, nil
	case modelRecord.IsImageGeneration() || modelRecord.ImageGeneration:
		return testMethodImageGeneration, nil
	case modelRecord.IsLLM() || modelRecord.ChatCompletions || modelRecord.Responses || modelRecord.HasUseCase(string(llmmodelmodel.UseCaseVision)) || modelRecord.HasUseCase(string(llmmodelmodel.UseCaseReasoning)) || modelRecord.HasUseCase(string(llmmodelmodel.UseCaseFuncCalling)):
		return testMethodChat, nil
	default:
		return "", fmt.Errorf("model %q use case is unsupported for channel validation", modelRecord.Model)
	}
}

func inferValidationUseCaseFromCustomModel(modelRecord *llmmodelmodel.CustomModel) (string, error) {
	switch {
	case modelRecord.Embeddings || containsUseCase(modelRecord.UseCases, string(llmmodelmodel.UseCaseEmbedding)):
		return testMethodEmbedding, nil
	case containsUseCase(modelRecord.UseCases, string(llmmodelmodel.UseCaseRerank)):
		return testMethodRerank, nil
	case modelRecord.ImageGeneration || containsUseCase(modelRecord.UseCases, string(llmmodelmodel.UseCaseImageGen)):
		return testMethodImageGeneration, nil
	case modelRecord.ChatCompletions || modelRecord.Responses || containsUseCase(modelRecord.UseCases, string(llmmodelmodel.UseCaseTextChat)) || containsUseCase(modelRecord.UseCases, "chat") || containsUseCase(modelRecord.UseCases, string(llmmodelmodel.UseCaseVision)) || containsUseCase(modelRecord.UseCases, string(llmmodelmodel.UseCaseReasoning)) || containsUseCase(modelRecord.UseCases, string(llmmodelmodel.UseCaseFuncCalling)):
		return testMethodChat, nil
	default:
		return "", fmt.Errorf("model %q use case is unsupported for channel validation", modelRecord.Name)
	}
}

func containsUseCase(useCases []string, target string) bool {
	for _, useCase := range useCases {
		if useCase == target {
			return true
		}
	}
	return false
}

func normalizeTestMethod(testMethod string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(testMethod)) {
	case "", "chat", "chatcompletions", "chat_completions", "chat-completions", "responses", "response":
		return testMethodChat, nil
	case "embedding", "embeddings":
		return testMethodEmbedding, nil
	case "image", "images", "image-gen", "image_generation", "image-generation":
		return testMethodImageGeneration, nil
	case "rerank":
		return testMethodRerank, nil
	default:
		return "", fmt.Errorf("unsupported test method %q", testMethod)
	}
}

func pickRepresentativeModels(capabilities []modelCapability) []modelCapability {
	representatives := make([]modelCapability, 0)
	seenUseCase := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if _, ok := seenUseCase[capability.UseCase]; ok {
			continue
		}
		seenUseCase[capability.UseCase] = struct{}{}
		representatives = append(representatives, capability)
	}
	return representatives
}

func stableSampleCapabilities(channelProvider, apiBaseURL string, capabilities []modelCapability, size int) []modelCapability {
	if len(capabilities) <= size {
		return append([]modelCapability(nil), capabilities...)
	}

	modelNames := capabilityModels(capabilities)
	sortedNames := append([]string(nil), modelNames...)
	sort.Strings(sortedNames)
	seedInput := channelProvider + "\n" + apiBaseURL + "\n" + strings.Join(sortedNames, "\n")
	seed := sha256.Sum256([]byte(seedInput))

	type sampledCapability struct {
		capability modelCapability
		score      string
	}

	scored := make([]sampledCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		hash := sha256.Sum256([]byte(hex.EncodeToString(seed[:]) + "\n" + capability.Model))
		scored = append(scored, sampledCapability{
			capability: capability,
			score:      hex.EncodeToString(hash[:]),
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].capability.Model < scored[j].capability.Model
		}
		return scored[i].score < scored[j].score
	})

	sampled := make([]modelCapability, 0, size)
	for _, item := range scored[:size] {
		sampled = append(sampled, item.capability)
	}
	return sampled
}

func capabilityModels(capabilities []modelCapability) []string {
	models := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		models = append(models, capability.Model)
	}
	return models
}

func failuresToReport(failures []validationFailure) []map[string]any {
	report := make([]map[string]any, 0, len(failures))
	for _, failure := range failures {
		report = append(report, map[string]any{
			keyModel:   failure.Model,
			keyUseCase: failure.UseCase,
			keyMessage: failure.Message,
		})
	}
	return report
}

func formatFailures(failures []validationFailure) string {
	if len(failures) == 0 {
		return "unknown validation failure"
	}

	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		if failure.UseCase == "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", failure.Model, failure.Message))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s [%s] (%s)", failure.Model, failure.UseCase, failure.Message))
	}
	return strings.Join(parts, "; ")
}

func newValidationFailureError(prefix string, failures []validationFailure) error {
	if allFailuresAreProviderAPIKeyInvalid(failures) {
		return newProviderAPIKeyInvalidError(fmt.Errorf("%s: %s", prefix, formatFailures(failures)))
	}
	return fmt.Errorf("%s: %s", prefix, formatFailures(failures))
}

func allFailuresAreProviderAPIKeyInvalid(failures []validationFailure) bool {
	if len(failures) == 0 {
		return false
	}
	for _, failure := range failures {
		if strings.TrimSpace(failure.Message) != providerAPIKeyInvalidMessage {
			return false
		}
	}
	return true
}

func buildWarnings(validationMode string, normalizedModels []string, probeTargets []modelCapability, failures []validationFailure, unvalidatedCount int) []string {
	if validationMode != validationModeSampled {
		return []string{}
	}

	warnings := []string{
		fmt.Sprintf(
			"provider does not support upstream model listing; validated a stable sample of %d out of %d models",
			len(probeTargets),
			len(normalizedModels),
		),
	}
	if len(failures) > 0 {
		warnings = append(warnings, fmt.Sprintf("sampled models failed validation: %s", formatFailures(failures)))
	}
	if unvalidatedCount > 0 {
		warnings = append(warnings, fmt.Sprintf("%d models were not validated during channel creation", unvalidatedCount))
	}
	return warnings
}

func runChatProbe(ctx context.Context, chatAdapter adapter.ChatCapable, modelName string, stream bool) (string, error) {
	maxTokens := defaultMaxTokens
	request := &adapter.ChatRequest{
		Model: modelName,
		Messages: []adapter.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
		MaxTokens: &maxTokens,
		Stream:    stream,
	}

	if stream {
		return runChatStreamProbe(ctx, chatAdapter, request)
	}

	response, err := chatAdapter.ChatCompletion(ctx, request)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", nil
	}
	if content, ok := response.Choices[0].Message.Content.(string); ok {
		return content, nil
	}
	return "", nil
}

func runChatStreamProbe(ctx context.Context, chatAdapter adapter.ChatCapable, request *adapter.ChatRequest) (string, error) {
	stream, err := chatAdapter.ChatCompletionStream(ctx, request)
	if err != nil {
		return "", err
	}

	var firstContent string
	for chunk := range stream {
		if chunk.Error != nil {
			return "", chunk.Error
		}
		for _, choice := range chunk.Choices {
			if content, ok := choice.Delta.Content.(string); ok && content != "" && firstContent == "" {
				firstContent = content
			}
			if choice.Delta.ReasoningContent != "" && firstContent == "" {
				firstContent = choice.Delta.ReasoningContent
			}
		}
	}
	return firstContent, nil
}

func runEmbeddingProbe(ctx context.Context, embeddingAdapter adapter.EmbeddingCapable, modelName string) error {
	_, err := embeddingAdapter.CreateEmbeddings(ctx, &adapter.EmbeddingsRequest{
		Model: modelName,
		Input: "hello",
	})
	return err
}

func runImageProbe(ctx context.Context, imageAdapter adapter.ImageCapable, modelName string) error {
	count := 1
	_, err := imageAdapter.CreateImage(ctx, &adapter.ImageRequest{
		Model:  modelName,
		Prompt: "hello",
		N:      &count,
		Size:   defaultImageProbeSize,
	})
	return err
}

func runRerankProbe(ctx context.Context, rerankAdapter adapter.RerankCapable, modelName string) error {
	topN := 1
	returnDocuments := false
	_, err := rerankAdapter.Rerank(ctx, &adapter.RerankRequest{
		Model:           modelName,
		Query:           "hello",
		Documents:       []string{"hello world", "goodbye world"},
		TopN:            &topN,
		ReturnDocuments: &returnDocuments,
	})
	return err
}
