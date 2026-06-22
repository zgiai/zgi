package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/llm/channel/dto"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	adapterprovider "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"gorm.io/gorm"
)

const ollamaChannelProvider = "ollama"

func (s *channelService) DiscoverOllamaModels(ctx context.Context, req *dto.DiscoverOllamaModelsRequest) (*dto.DiscoverOllamaModelsResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	if isOllamaExactBaseURL(req.APIBaseURL) {
		return nil, fmt.Errorf("ollama api_base_url ending with # does not support auto-discover models; enter model names manually")
	}
	if _, err := channelprovider.ValidateConnectionFields(ollamaChannelProvider, req.APIBaseURL); err != nil {
		return nil, err
	}
	lister := listOllamaModels
	if s != nil && s.ollamaModelLister != nil {
		lister = s.ollamaModelLister
	}
	models, err := lister(ctx, req.APIBaseURL, req.APIKey)
	if err != nil {
		return nil, err
	}

	views := make([]dto.OllamaModelView, 0, len(models))
	for _, model := range models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = strings.TrimSpace(model.ID)
		}
		if name == "" {
			continue
		}
		views = append(views, dto.OllamaModelView{
			Name:         name,
			DisplayName:  name,
			UseCase:      ollamaUseCaseFromAdapterModel(model),
			Capabilities: append([]string(nil), model.Capabilities...),
		})
	}

	return &dto.DiscoverOllamaModelsResponse{
		Models: views,
		Total:  len(views),
	}, nil
}

func (s *channelService) ensureOllamaCustomModels(ctx context.Context, organizationID uuid.UUID, channelProvider, apiBaseURL, apiKey string, models []string) error {
	if channelProvider != ollamaChannelProvider || len(models) == 0 {
		return nil
	}
	if s == nil || s.customProviderRepo == nil || s.customModelRepo == nil {
		return nil
	}

	selectedModels, selectedUseCases, err := s.resolveSelectedOllamaModels(ctx, organizationID, apiBaseURL, apiKey, models)
	if err != nil {
		return err
	}

	provider, err := s.ensureOllamaCustomProvider(ctx, organizationID, apiBaseURL)
	if err != nil {
		return err
	}

	for _, name := range selectedModels {
		useCase := selectedUseCases[name]
		if err := s.ensureOllamaCustomModel(ctx, organizationID, provider, name, useCase); err != nil {
			return err
		}
	}
	return nil
}

func (s *channelService) resolveSelectedOllamaModels(ctx context.Context, organizationID uuid.UUID, apiBaseURL, apiKey string, models []string) ([]string, map[string]string, error) {
	if isOllamaExactBaseURL(apiBaseURL) {
		return s.resolveSelectedExactOllamaModels(ctx, organizationID, models)
	}

	discoveredUseCases, err := s.discoverOllamaUseCases(ctx, apiBaseURL, apiKey)
	if err != nil {
		return nil, nil, err
	}

	selectedModels := normalizeSelectedOllamaModels(models)
	selectedUseCases := make(map[string]string, len(selectedModels))
	for _, name := range selectedModels {
		useCase := discoveredUseCases[name]
		if useCase == "" {
			useCase = inferOllamaUseCaseFromName(name)
		}
		if useCase == "unsupported" {
			return nil, nil, fmt.Errorf("ollama model %q is not supported by this adapter", name)
		}
		selectedUseCases[name] = useCase
	}

	return selectedModels, selectedUseCases, nil
}

func (s *channelService) resolveSelectedExactOllamaModels(ctx context.Context, organizationID uuid.UUID, models []string) ([]string, map[string]string, error) {
	selectedModels := normalizeSelectedOllamaModels(models)
	if len(selectedModels) == 0 {
		return selectedModels, map[string]string{}, nil
	}

	selectedUseCases, err := s.resolveExactOllamaUseCasesFromModelLibrary(ctx, organizationID, selectedModels)
	if err != nil {
		return nil, nil, err
	}
	if err := validateSingleOllamaExactUseCase(selectedUseCases); err != nil {
		return nil, nil, err
	}

	return selectedModels, selectedUseCases, nil
}

func (s *channelService) discoverOllamaUseCases(ctx context.Context, apiBaseURL, apiKey string) (map[string]string, error) {
	lister := s.ollamaModelLister
	if lister == nil {
		lister = listOllamaModels
	}
	discovered, err := lister(ctx, apiBaseURL, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to discover Ollama models: %w", err)
	}

	useCases := make(map[string]string, len(discovered))
	for _, model := range discovered {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			name = strings.TrimSpace(model.ID)
		}
		if name != "" {
			useCases[name] = ollamaUseCaseFromAdapterModel(model)
		}
	}
	return useCases, nil
}

func (s *channelService) resolveExactOllamaUseCasesFromModelLibrary(ctx context.Context, organizationID uuid.UUID, modelNames []string) (map[string]string, error) {
	useCases := make(map[string]string, len(modelNames))

	if s != nil && s.privateModels != nil && organizationID != uuid.Nil {
		privateModels, err := s.privateModels.ResolveActiveModelsForProvider(ctx, organizationID, ollamaChannelProvider, modelNames)
		if err != nil {
			return nil, fmt.Errorf("failed to load private model metadata: %w", err)
		}
		for _, record := range privateModels {
			if record == nil {
				continue
			}
			useCase, err := ollamaExactUseCaseFromCustomModel(record)
			if err != nil {
				return nil, err
			}
			useCases[record.Name] = useCase
		}
	}

	remaining := make([]string, 0, len(modelNames))
	for _, modelName := range modelNames {
		if _, ok := useCases[modelName]; ok {
			continue
		}
		remaining = append(remaining, modelName)
	}

	if len(remaining) == 0 {
		return useCases, nil
	}
	if s == nil || s.modelRepo == nil {
		return nil, newOllamaExactModelMetadataError(remaining[0])
	}

	globalModels, err := s.modelRepo.ListByNames(ctx, remaining)
	if err != nil {
		return nil, fmt.Errorf("failed to load model metadata: %w", err)
	}
	for _, record := range globalModels {
		if record == nil {
			continue
		}
		useCase, err := ollamaExactUseCaseFromGlobalModel(record)
		if err != nil {
			return nil, err
		}
		useCases[record.Model] = useCase
	}

	for _, modelName := range remaining {
		if _, ok := useCases[modelName]; !ok {
			return nil, newOllamaExactModelMetadataError(modelName)
		}
	}

	return useCases, nil
}

func validateSingleOllamaExactUseCase(useCases map[string]string) error {
	unique := make(map[string]struct{}, len(useCases))
	for _, useCase := range useCases {
		unique[useCase] = struct{}{}
	}
	if len(unique) <= 1 {
		return nil
	}

	values := make([]string, 0, len(unique))
	for useCase := range unique {
		values = append(values, useCase)
	}
	slices.Sort(values)
	return fmt.Errorf("ollama api_base_url ending with # requires models with a single use case; got %s", strings.Join(values, ", "))
}

func ollamaExactUseCaseFromGlobalModel(record *llmmodelmodel.LLMModel) (string, error) {
	if record == nil {
		return "", fmt.Errorf("model metadata is required")
	}

	switch {
	case record.IsEmbedding() || record.Embeddings:
		return string(llmmodelmodel.UseCaseEmbedding), nil
	case record.IsLLM() || record.ChatCompletions || record.Responses || record.HasUseCase(string(llmmodelmodel.UseCaseVision)) || record.HasUseCase(string(llmmodelmodel.UseCaseReasoning)) || record.HasUseCase(string(llmmodelmodel.UseCaseFuncCalling)):
		return string(llmmodelmodel.UseCaseTextChat), nil
	default:
		return "", fmt.Errorf("ollama api_base_url ending with # only supports chat or embedding models; model %q is not eligible", record.Model)
	}
}

func ollamaExactUseCaseFromCustomModel(record *llmmodelmodel.CustomModel) (string, error) {
	if record == nil {
		return "", fmt.Errorf("model metadata is required")
	}

	switch {
	case record.Embeddings || containsUseCase(record.UseCases, string(llmmodelmodel.UseCaseEmbedding)):
		return string(llmmodelmodel.UseCaseEmbedding), nil
	case record.ChatCompletions || record.Responses || containsUseCase(record.UseCases, string(llmmodelmodel.UseCaseTextChat)) || containsUseCase(record.UseCases, "chat") || containsUseCase(record.UseCases, string(llmmodelmodel.UseCaseVision)) || containsUseCase(record.UseCases, string(llmmodelmodel.UseCaseReasoning)) || containsUseCase(record.UseCases, string(llmmodelmodel.UseCaseFuncCalling)):
		return string(llmmodelmodel.UseCaseTextChat), nil
	default:
		return "", fmt.Errorf("ollama api_base_url ending with # only supports chat or embedding models; model %q is not eligible", record.Name)
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

func newOllamaExactModelMetadataError(modelName string) error {
	return fmt.Errorf("ollama api_base_url ending with # cannot auto-discover model capabilities; model %q must exist in the local model library first", strings.TrimSpace(modelName))
}

func (s *channelService) ensureOllamaCustomProvider(ctx context.Context, organizationID uuid.UUID, apiBaseURL string) (*providermodel.CustomProvider, error) {
	existing, err := s.customProviderRepo.GetByProvider(ctx, organizationID, ollamaChannelProvider)
	if err == nil && existing != nil {
		changed := false
		if !existing.IsActive {
			existing.IsActive = true
			changed = true
		}
		if strings.TrimSpace(apiBaseURL) != "" && existing.APIBaseURL != apiBaseURL {
			existing.APIBaseURL = apiBaseURL
			changed = true
		}
		if changed {
			if err := s.customProviderRepo.Update(ctx, existing); err != nil {
				return nil, fmt.Errorf("failed to update Ollama custom provider: %w", err)
			}
		}
		return existing, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to load Ollama custom provider: %w", err)
	}

	provider := &providermodel.CustomProvider{
		OrganizationID: organizationID,
		Provider:       ollamaChannelProvider,
		ProviderName:   "Ollama",
		APIBaseURL:     apiBaseURL,
		Description:    "Local Ollama provider",
		IsActive:       true,
		Metadata: map[string]interface{}{
			"source": "ollama",
		},
	}
	if err := s.customProviderRepo.Create(ctx, provider); err != nil {
		return nil, fmt.Errorf("failed to create Ollama custom provider: %w", err)
	}
	return provider, nil
}

func (s *channelService) ensureOllamaCustomModel(ctx context.Context, organizationID uuid.UUID, provider *providermodel.CustomProvider, name string, useCase string) error {
	existing, err := s.customModelRepo.GetByProviderAndName(ctx, organizationID, provider.ID, name)
	if err == nil && existing != nil {
		if existing.IsActive {
			return nil
		}
		existing.IsActive = true
		return s.customModelRepo.Update(ctx, existing)
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to load Ollama custom model %q: %w", name, err)
	}

	useCases := llmmodelmodel.EnsureUseCases([]string{useCase}, nil)
	endpoints := llmmodelmodel.DefaultEndpointsForUseCases(useCases)
	features := llmmodelmodel.DefaultFeaturesForLLM()
	tools := llmmodelmodel.DefaultTools()
	parameters := llmmodelmodel.DefaultParameters()

	model := &llmmodelmodel.CustomModel{
		OrganizationID:   organizationID,
		ProviderID:       provider.ID,
		Provider:         ollamaChannelProvider,
		Name:             name,
		DisplayName:      name,
		Status:           "active",
		UseCases:         llmmodelmodel.StringArray(useCases),
		AccessType:       "closed",
		Currency:         "USD",
		InputPrice:       decimal.Zero,
		OutputPrice:      decimal.Zero,
		Description:      "Discovered from Ollama",
		IsActive:         true,
		Endpoints:        &endpoints,
		Features:         &features,
		Tools:            &tools,
		Parameters:       &parameters,
		ConfigParameters: llmmodelmodel.ConfigParameters{},
		Metadata: map[string]interface{}{
			"source": "ollama",
		},
	}
	return s.customModelRepo.Create(ctx, model)
}

func listOllamaModels(ctx context.Context, apiBaseURL, apiKey string) ([]adapter.Model, error) {
	ollama, err := adapterprovider.NewOllamaAdapter(&adapter.AdapterConfig{
		ProviderName:        ollamaChannelProvider,
		BaseURL:             apiBaseURL,
		APIKey:              apiKey,
		GuardOutboundURL:    true,
		AllowPrivateBaseURL: channelprovider.AllowsPrivateBaseURL(ollamaChannelProvider),
	})
	if err != nil {
		return nil, err
	}
	return ollama.ListModels(ctx, apiKey)
}

func isOllamaExactBaseURL(raw string) bool {
	return strings.HasSuffix(strings.TrimSpace(raw), "#")
}

func normalizeSelectedOllamaModels(models []string) []string {
	normalized := make([]string, 0, len(models))
	for _, modelName := range models {
		name := strings.TrimSpace(modelName)
		if name == "" || name == "*" || slices.Contains(normalized, name) {
			continue
		}
		normalized = append(normalized, name)
	}
	return normalized
}

func ollamaUseCaseFromAdapterModel(model adapter.Model) string {
	name := strings.TrimSpace(model.Name)
	if name == "" {
		name = strings.TrimSpace(model.ID)
	}
	switch {
	case model.Type == "embedding" || slices.Contains(model.Capabilities, "embedding"):
		return string(llmmodelmodel.UseCaseEmbedding)
	case model.Type == "unsupported" || slices.Contains(model.Capabilities, "unsupported"):
		return "unsupported"
	default:
		return inferOllamaUseCaseFromName(name)
	}
}

func inferOllamaUseCaseFromName(modelName string) string {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.Contains(lower, "rerank"):
		return "unsupported"
	case strings.Contains(lower, "embed"),
		strings.Contains(lower, "embedding"),
		strings.Contains(lower, "nomic-embed"),
		strings.Contains(lower, "mxbai-embed"),
		strings.Contains(lower, "all-minilm"):
		return string(llmmodelmodel.UseCaseEmbedding)
	default:
		return string(llmmodelmodel.UseCaseTextChat)
	}
}
