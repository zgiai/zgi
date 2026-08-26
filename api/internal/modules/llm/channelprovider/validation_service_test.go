package channelprovider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type fakeModelLookupRepo struct {
	models map[string]*llmmodelmodel.LLMModel
}

func (f *fakeModelLookupRepo) ListByNames(_ context.Context, names []string) ([]*llmmodelmodel.LLMModel, error) {
	result := make([]*llmmodelmodel.LLMModel, 0, len(names))
	for _, name := range names {
		if modelRecord, ok := f.models[name]; ok {
			result = append(result, modelRecord)
		}
	}
	return result, nil
}

func (f *fakeModelLookupRepo) ListAvailableByNames(_ context.Context, names []string, provider string, useCase string) ([]*llmmodelmodel.LLMModel, error) {
	result := make([]*llmmodelmodel.LLMModel, 0, len(names))
	for _, name := range names {
		modelRecord, ok := f.models[name]
		if !ok || modelRecord == nil {
			continue
		}
		if provider != "" && modelRecord.Provider != "" && modelRecord.Provider != provider {
			continue
		}
		if modelRecord.Status == llmmodelmodel.ModelStatusDeprecated {
			continue
		}
		if useCase != "" && !containsString([]string(modelRecord.UseCases), useCase) {
			continue
		}
		result = append(result, modelRecord)
	}
	return result, nil
}

func (f *fakeModelLookupRepo) ListAvailableFiltered(_ context.Context, provider string, useCase string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelLookupRepo) Create(context.Context, *llmmodelmodel.LLMModel) error {
	return errors.New("not implemented")
}

func (f *fakeModelLookupRepo) GetByID(context.Context, uuid.UUID) (*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelLookupRepo) GetByName(_ context.Context, name string) (*llmmodelmodel.LLMModel, error) {
	if modelRecord, ok := f.models[name]; ok {
		return modelRecord, nil
	}
	return nil, nil
}

func (f *fakeModelLookupRepo) GetByProviderAndName(context.Context, string, string) (*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeModelLookupRepo) List(_ context.Context, _ *uuid.UUID, _, _, _ string, _ *bool, _, _ int) ([]*llmmodelmodel.LLMModel, int64, error) {
	result := make([]*llmmodelmodel.LLMModel, 0, len(f.models))
	for _, modelRecord := range f.models {
		result = append(result, modelRecord)
	}
	return result, int64(len(result)), nil
}

func (f *fakeModelLookupRepo) Update(context.Context, *llmmodelmodel.LLMModel) error {
	return errors.New("not implemented")
}

func (f *fakeModelLookupRepo) Delete(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeModelLookupRepo) ListByProvider(context.Context, string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}

type fakePrivateModelLookup struct {
	models                map[string]*llmmodelmodel.CustomModel
	records               []*llmmodelmodel.CustomModel
	listCalls             int
	genericCalls          int
	providerSpecificCalls int
}

func (f *fakePrivateModelLookup) allRecords() []*llmmodelmodel.CustomModel {
	if len(f.records) > 0 {
		return f.records
	}

	records := make([]*llmmodelmodel.CustomModel, 0, len(f.models))
	for _, modelRecord := range f.models {
		records = append(records, modelRecord)
	}
	return records
}

func (f *fakePrivateModelLookup) ListActiveModelsByNames(_ context.Context, _ uuid.UUID, modelNames []string) ([]*llmmodelmodel.CustomModel, error) {
	f.listCalls++
	result := make([]*llmmodelmodel.CustomModel, 0, len(modelNames))
	for _, modelRecord := range f.allRecords() {
		if modelRecord != nil && containsString(modelNames, modelRecord.Name) {
			result = append(result, modelRecord)
		}
	}
	return result, nil
}

func (f *fakePrivateModelLookup) ResolveActiveModels(_ context.Context, _ uuid.UUID, modelNames []string) ([]*llmmodelmodel.CustomModel, error) {
	f.genericCalls++
	result := make([]*llmmodelmodel.CustomModel, 0, len(modelNames))
	seen := make(map[string]string, len(modelNames))
	for _, modelRecord := range f.allRecords() {
		if modelRecord == nil || !containsString(modelNames, modelRecord.Name) {
			continue
		}
		if existingProvider, ok := seen[modelRecord.Name]; ok && existingProvider != modelRecord.Provider {
			return nil, fmt.Errorf("private model %q is defined by multiple custom providers (%s, %s)", modelRecord.Name, existingProvider, modelRecord.Provider)
		}
		seen[modelRecord.Name] = modelRecord.Provider
		result = append(result, modelRecord)
	}
	return result, nil
}

func (f *fakePrivateModelLookup) ResolveActiveModelsForProvider(_ context.Context, _ uuid.UUID, provider string, modelNames []string) ([]*llmmodelmodel.CustomModel, error) {
	f.providerSpecificCalls++
	result := make([]*llmmodelmodel.CustomModel, 0, len(modelNames))
	for _, modelRecord := range f.allRecords() {
		if modelRecord != nil && modelRecord.Provider == provider && containsString(modelNames, modelRecord.Name) {
			result = append(result, modelRecord)
		}
	}
	return result, nil
}

func (f *fakePrivateModelLookup) ResolveActiveModel(_ context.Context, _ uuid.UUID, modelName string) (*llmmodelmodel.CustomModel, error) {
	for _, modelRecord := range f.allRecords() {
		if modelRecord != nil && modelRecord.Name == modelName {
			return modelRecord, nil
		}
	}
	return nil, nil
}

func (f *fakePrivateModelLookup) ResolveActiveModelForProvider(_ context.Context, _ uuid.UUID, provider string, modelName string) (*llmmodelmodel.CustomModel, error) {
	for _, modelRecord := range f.allRecords() {
		if modelRecord != nil && modelRecord.Name == modelName && modelRecord.Provider == provider {
			return modelRecord, nil
		}
	}
	return nil, nil
}

func (f *fakePrivateModelLookup) LoadActiveModelNameIndexes(_ context.Context, _ uuid.UUID) ([]string, map[string]string, error) {
	records := f.allRecords()
	exactNames := make([]string, 0, len(records))
	legacyShortNames := make(map[string]string, len(records))
	for _, modelRecord := range records {
		if modelRecord == nil || modelRecord.Name == "" {
			continue
		}
		exactNames = append(exactNames, modelRecord.Name)
	}
	return exactNames, legacyShortNames, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type fakeValidationAdapter struct {
	listModelsCalls int
	chatCalls       int
	streamCalls     int
	embeddingCalls  int
	imageCalls      int
	rerankCalls     int
	lastImageReq    *adapter.ImageRequest

	listModelsResult []adapter.Model
	listModelsErr    error

	chatFailures      map[string]error
	streamFailures    map[string]error
	embeddingFailures map[string]error
	imageFailures     map[string]error
	rerankFailures    map[string]error
}

func (f *fakeValidationAdapter) ChatCompletion(_ context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	f.chatCalls++
	if err := f.chatFailures[request.Model]; err != nil {
		return nil, err
	}
	return &adapter.ChatResponse{
		Choices: []adapter.Choice{
			{Message: adapter.Message{Content: "ok"}},
		},
	}, nil
}

func (f *fakeValidationAdapter) ChatCompletionStream(_ context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	f.streamCalls++
	if err := f.streamFailures[request.Model]; err != nil {
		return nil, err
	}
	ch := make(chan adapter.StreamResponse, 1)
	ch <- adapter.StreamResponse{
		Choices: []adapter.StreamChoice{
			{Delta: adapter.Message{Content: "ok"}},
		},
	}
	close(ch)
	return ch, nil
}

func (f *fakeValidationAdapter) CreateResponse(context.Context, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("%w: responses", adapter.ErrCapabilityUnsupported)
}

func (f *fakeValidationAdapter) CreateEmbeddings(_ context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	f.embeddingCalls++
	if err := f.embeddingFailures[request.Model]; err != nil {
		return nil, err
	}
	return &adapter.EmbeddingsResponse{}, nil
}

func (f *fakeValidationAdapter) CreateImage(_ context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	f.imageCalls++
	f.lastImageReq = request
	if err := f.imageFailures[request.Model]; err != nil {
		return nil, err
	}
	return &adapter.ImageResponse{}, nil
}

func (f *fakeValidationAdapter) Rerank(_ context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	f.rerankCalls++
	if err := f.rerankFailures[request.Model]; err != nil {
		return nil, err
	}
	return &adapter.RerankResponse{}, nil
}

func (f *fakeValidationAdapter) ListModels(context.Context, string) ([]adapter.Model, error) {
	f.listModelsCalls++
	if f.listModelsErr != nil {
		return nil, f.listModelsErr
	}
	return f.listModelsResult, nil
}

func (f *fakeValidationAdapter) GetBalance(context.Context, string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: balance", adapter.ErrCapabilityUnsupported)
}

func (f *fakeValidationAdapter) ValidateConfig(*adapter.AdapterConfig) error {
	return nil
}

func (f *fakeValidationAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{Name: "fake"}
}

func TestValidatorValidateModels_UsesListModelsAndRepresentativeProbes(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-a":  {Model: "chat-a", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"chat-b":  {Model: "chat-b", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"embed-a": {Model: "embed-a", UseCases: llmmodelmodel.StringArray{"embedding"}},
		},
	}
	fakeAdapter := &fakeValidationAdapter{
		listModelsResult: []adapter.Model{
			{ID: "chat-a"},
			{ID: "chat-b"},
			{ID: "embed-a"},
		},
		chatFailures:      map[string]error{},
		embeddingFailures: map[string]error{},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModels(context.Background(), uuid.Nil, "openai", "key", "https://example.com", []string{"chat-a", "chat-b", "embed-a"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, validationModeFull, result.Report[keyValidationMode])
	require.Equal(t, false, result.Report[keySampled])
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 1, fakeAdapter.chatCalls)
	require.Equal(t, 1, fakeAdapter.embeddingCalls)
	require.Equal(t, 0, fakeAdapter.imageCalls)
	require.Equal(t, 0, fakeAdapter.rerankCalls)
	require.Empty(t, WarningMessages(result.Report))
}

func TestValidatorValidateModels_FailsFullValidationWithoutListModels(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-a":   {Model: "chat-a", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"embed-a":  {Model: "embed-a", UseCases: llmmodelmodel.StringArray{"embedding"}},
			"rerank-a": {Model: "rerank-a", UseCases: llmmodelmodel.StringArray{"rerank"}},
		},
	}
	fakeAdapter := &fakeValidationAdapter{
		listModelsErr:     fmt.Errorf("%w: list models", adapter.ErrCapabilityUnsupported),
		chatFailures:      map[string]error{},
		embeddingFailures: map[string]error{"embed-a": errors.New("bad embedding model")},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
		listModelsResult:  nil,
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModels(context.Background(), uuid.Nil, "openai", "key", "", []string{"chat-a", "embed-a", "rerank-a"})
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "embed-a")
	require.Equal(t, validationModeFull, result.Report[keyValidationMode])
	require.Equal(t, 3, result.Report[keyValidatedCount])
	require.Equal(t, 2, result.Report[keyPassedCount])
}

func TestValidatorValidateModels_AllowsSampledSuccessWhenFourOfFivePass(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-1": {Model: "chat-1", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"chat-2": {Model: "chat-2", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"chat-3": {Model: "chat-3", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"chat-4": {Model: "chat-4", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"chat-5": {Model: "chat-5", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"chat-6": {Model: "chat-6", UseCases: llmmodelmodel.StringArray{"text-chat"}},
		},
	}
	allCapabilities := []modelCapability{
		{Model: "chat-1", UseCase: testMethodChat},
		{Model: "chat-2", UseCase: testMethodChat},
		{Model: "chat-3", UseCase: testMethodChat},
		{Model: "chat-4", UseCase: testMethodChat},
		{Model: "chat-5", UseCase: testMethodChat},
		{Model: "chat-6", UseCase: testMethodChat},
	}
	sampled := stableSampleCapabilities("openai", "", allCapabilities, sampledValidationSize)
	failedModel := sampled[0].Model

	fakeAdapter := &fakeValidationAdapter{
		listModelsErr:     fmt.Errorf("%w: list models", adapter.ErrCapabilityUnsupported),
		chatFailures:      map[string]error{failedModel: errors.New("temporary failure")},
		embeddingFailures: map[string]error{},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}
	validator.now = func() time.Time { return time.Unix(100, 0) }

	result, err := validator.ValidateModels(context.Background(), uuid.Nil, "openai", "key", "", []string{"chat-1", "chat-2", "chat-3", "chat-4", "chat-5", "chat-6"})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, validationModeSampled, result.Report[keyValidationMode])
	require.Equal(t, true, result.Report[keySampled])
	require.Equal(t, sampledValidationSize, result.Report[keySampleSize])
	require.Equal(t, sampledValidationSize-1, result.Report[keyPassedCount])
	require.Equal(t, 1, result.Report[keyUnvalidatedCount])
	warnings := WarningMessages(result.Report)
	require.NotEmpty(t, warnings)
	require.Contains(t, warnings[0], "validated a stable sample")
	require.Contains(t, fmt.Sprint(result.Report[keyFailedModels]), failedModel)
}

func TestValidatorValidateModels_RejectsUnknownModelBeforeUpstreamCall(t *testing.T) {
	validator := NewValidator(nil, nil)
	fakeAdapter := &fakeValidationAdapter{}
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModels(context.Background(), uuid.Nil, "openai", "key", "", []string{"missing-model"})
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "missing-model")
	require.Equal(t, 0, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
}

func TestValidatorValidateModelsForCreation_RejectsShortNameAndAsksForFullName(t *testing.T) {
	validator := NewValidator(nil, nil)
	fakeAdapter := &fakeValidationAdapter{}
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"ByteDance-Seed/Seed-OSS-36B-Instruct": {
				Model:    "ByteDance-Seed/Seed-OSS-36B-Instruct",
				UseCases: llmmodelmodel.StringArray{"text-chat"},
			},
		},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai",
		"key",
		"",
		[]string{"Seed-OSS-36B-Instruct"},
	)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "Seed-OSS-36B-Instruct")
	require.Contains(t, err.Error(), "full model name")
	require.Equal(t, 0, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
}

func TestValidatorValidateModelsForCreation_AllowsWorkspacePrivateModels(t *testing.T) {
	orgID := uuid.New()
	validator := NewValidator(&fakeModelLookupRepo{models: map[string]*llmmodelmodel.LLMModel{}}, nil)
	validator.privateModels = &fakePrivateModelLookup{
		models: map[string]*llmmodelmodel.CustomModel{
			"ernie-x1-turbo-32k": {
				Name:            "ernie-x1-turbo-32k",
				DisplayName:     "ernie-x1-turbo-32k",
				UseCases:        llmmodelmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
		},
	}
	fakeAdapter := &fakeValidationAdapter{
		listModelsResult: []adapter.Model{
			{ID: "ernie-x1-turbo-32k"},
		},
		chatFailures:      map[string]error{},
		embeddingFailures: map[string]error{},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		orgID,
		"openai-compatible",
		"key",
		"https://example.com/v1",
		[]string{"ernie-x1-turbo-32k"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 0, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.Equal(t, []string{"ernie-x1-turbo-32k"}, result.NormalizedModels)
}

func TestValidatorValidateModelsForCreation_UsesChannelProviderForDuplicatePrivateModels(t *testing.T) {
	orgID := uuid.New()
	privateModels := &fakePrivateModelLookup{
		records: []*llmmodelmodel.CustomModel{
			{
				Name:            "qwen3.5:9b",
				Provider:        "ollama",
				UseCases:        llmmodelmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
			{
				Name:            "qwen3.5:9b",
				Provider:        "custom-1",
				UseCases:        llmmodelmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
		},
	}
	validator := NewValidator(&fakeModelLookupRepo{models: map[string]*llmmodelmodel.LLMModel{}}, privateModels)
	fakeAdapter := &fakeValidationAdapter{
		listModelsErr:     fmt.Errorf("%w: list models", adapter.ErrCapabilityUnsupported),
		chatFailures:      map[string]error{},
		embeddingFailures: map[string]error{},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		orgID,
		"ollama",
		"",
		"http://localhost:11434",
		[]string{"qwen3.5:9b"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, privateModels.providerSpecificCalls)
	require.Equal(t, 0, privateModels.genericCalls)
	require.Equal(t, 0, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.Empty(t, WarningMessages(result.Report))
	require.Equal(t, []string{"qwen3.5:9b"}, result.NormalizedModels)
}

func TestValidatorValidateModelsForCreation_OpenAICompatibleAllowsDuplicatePrivateModelName(t *testing.T) {
	orgID := uuid.New()
	privateModels := &fakePrivateModelLookup{
		records: []*llmmodelmodel.CustomModel{
			{
				Name:            "qwen3.5:9b",
				Provider:        "ollama",
				UseCases:        llmmodelmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
			{
				Name:            "qwen3.5:9b",
				Provider:        "custom-1",
				UseCases:        llmmodelmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
		},
	}
	validator := NewValidator(&fakeModelLookupRepo{models: map[string]*llmmodelmodel.LLMModel{}}, privateModels)
	fakeAdapter := &fakeValidationAdapter{
		listModelsResult: []adapter.Model{
			{Name: "qwen3.5:9b"},
		},
		chatFailures:      map[string]error{},
		embeddingFailures: map[string]error{},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		orgID,
		"openai-compatible",
		"key",
		"https://example.com/v1",
		[]string{"qwen3.5:9b"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, 1, privateModels.listCalls)
	require.Equal(t, 0, privateModels.providerSpecificCalls)
	require.Equal(t, 0, privateModels.genericCalls)
	require.Equal(t, 0, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.Equal(t, []string{"qwen3.5:9b"}, result.NormalizedModels)
}

func TestValidatorValidateModelsForCreation_OpenAICompatibleRejectsConflictingDuplicatePrivateModelUseCases(t *testing.T) {
	orgID := uuid.New()
	privateModels := &fakePrivateModelLookup{
		records: []*llmmodelmodel.CustomModel{
			{
				Name:            "qwen3.5:9b",
				Provider:        "ollama",
				UseCases:        llmmodelmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
			{
				Name:       "qwen3.5:9b",
				Provider:   "custom-1",
				UseCases:   llmmodelmodel.StringArray{"embedding"},
				Embeddings: true,
				IsActive:   true,
			},
		},
	}
	validator := NewValidator(&fakeModelLookupRepo{models: map[string]*llmmodelmodel.LLMModel{}}, privateModels)
	fakeAdapter := &fakeValidationAdapter{}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		orgID,
		"openai-compatible",
		"key",
		"https://example.com/v1",
		[]string{"qwen3.5:9b"},
	)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "conflicting use cases")
	require.Equal(t, 1, privateModels.listCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
}

func TestValidatorValidateModelsForCreation_OpenAICompatibleRejectsConflictingDuplicatePrivateModelStreamingSupport(t *testing.T) {
	orgID := uuid.New()
	privateModels := &fakePrivateModelLookup{
		records: []*llmmodelmodel.CustomModel{
			{
				Name:              "shared-chat-model",
				Provider:          "custom-1",
				UseCases:          llmmodelmodel.StringArray{"text-chat"},
				ChatCompletions:   true,
				SupportsStreaming: true,
				IsActive:          true,
			},
			{
				Name:              "shared-chat-model",
				Provider:          "custom-2",
				UseCases:          llmmodelmodel.StringArray{"text-chat"},
				ChatCompletions:   true,
				SupportsStreaming: false,
				IsActive:          true,
			},
		},
	}
	validator := NewValidator(&fakeModelLookupRepo{models: map[string]*llmmodelmodel.LLMModel{}}, privateModels)
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return &fakeValidationAdapter{}, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		orgID,
		"openai-compatible",
		"key",
		"https://example.com/v1",
		[]string{"shared-chat-model"},
	)

	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "conflicting streaming support")
}

func TestValidatorValidateModelsForCreation_ScopesGlobalModelsToChannelProvider(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"gpt-4o": {
				Model:    "gpt-4o",
				Provider: "openai",
				UseCases: llmmodelmodel.StringArray{
					"text-chat",
				},
			},
			"doubao-seedance-2-0-fast-260128": {
				Model:    "doubao-seedance-2-0-fast-260128",
				Provider: "doubao",
				UseCases: llmmodelmodel.StringArray{
					string(llmmodelmodel.UseCaseVideoGen),
				},
				Videos: true,
			},
		},
	}
	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	adapterCreated := false
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		adapterCreated = true
		return &fakeValidationAdapter{}, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"qwen",
		"key",
		"",
		[]string{"gpt-4o"},
	)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "gpt-4o")
	require.False(t, adapterCreated)

	result, err = validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai-compatible",
		"key",
		"https://example.com/v1",
		[]string{"gpt-4o"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])

	result, err = validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai",
		"key",
		"https://api.openai.com/v1",
		[]string{"doubao-seedance-2-0-fast-260128"},
	)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "doubao-seedance-2-0-fast-260128")

	result, err = validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai",
		"key",
		"https://api.agicto.cn/v1",
		[]string{"doubao-seedance-2-0-fast-260128"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.Equal(t, []string{"doubao-seedance-2-0-fast-260128"}, result.NormalizedModels)
}

func TestValidatorTestModel_RejectsConflictingTestMethod(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"embed-a": {Model: "embed-a", UseCases: llmmodelmodel.StringArray{"embedding"}},
		},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return &fakeValidationAdapter{
			chatFailures:      map[string]error{},
			embeddingFailures: map[string]error{},
			imageFailures:     map[string]error{},
			rerankFailures:    map[string]error{},
		}, nil
	}

	stream := false
	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai", "key", "", "embed-a", "chatCompletions", &stream)
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "conflicts")
}

func TestValidatorTestModel_StreamUsesChatCompletionStream(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"qwen-plus": {Model: "qwen-plus", UseCases: llmmodelmodel.StringArray{"text-chat"}},
		},
	}

	fakeAdapter := &fakeValidationAdapter{}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	stream := true
	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "qwen-plus", "", &stream)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.Equal(t, 1, fakeAdapter.streamCalls)
}

func TestValidatorTestModel_QwQAutoUsesStreamingFromMetadata(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"qwq-plus": {
				Model:             "qwq-plus",
				Provider:          "qwen",
				UseCases:          llmmodelmodel.StringArray{"text-chat"},
				SupportsStreaming: true,
			},
		},
	}
	fakeAdapter := &fakeValidationAdapter{}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.TestModel(context.Background(), uuid.Nil, "alibaba", "key", "", "qwq-plus", "", nil)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.Equal(t, 1, fakeAdapter.streamCalls)
}

func TestValidatorTestModel_AutoUsesBlockingWhenMetadataDoesNotSupportStreaming(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"blocking-chat-model": {
				Model:             "blocking-chat-model",
				UseCases:          llmmodelmodel.StringArray{"text-chat"},
				SupportsStreaming: false,
			},
		},
	}
	fakeAdapter := &fakeValidationAdapter{}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "blocking-chat-model", "", nil)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, fakeAdapter.chatCalls)
	require.Equal(t, 0, fakeAdapter.streamCalls)
}

func TestValidatorTestModel_AutoDoesNotRetryAnotherModeAfterFailure(t *testing.T) {
	providerErr := errors.New("stream request failed")
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"streaming-chat-model": {
				Model:             "streaming-chat-model",
				UseCases:          llmmodelmodel.StringArray{"text-chat"},
				SupportsStreaming: true,
			},
		},
	}
	fakeAdapter := &fakeValidationAdapter{
		streamFailures: map[string]error{"streaming-chat-model": providerErr},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "streaming-chat-model", "", nil)

	require.NoError(t, err)
	require.False(t, result.Success)
	require.ErrorIs(t, result.ProviderError, providerErr)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.Equal(t, 1, fakeAdapter.streamCalls)
}

func TestValidatorTestModel_ExplicitBlockingOverridesStreamingMetadata(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"dual-mode-chat-model": {
				Model:             "dual-mode-chat-model",
				UseCases:          llmmodelmodel.StringArray{"text-chat"},
				SupportsStreaming: true,
			},
		},
	}
	fakeAdapter := &fakeValidationAdapter{}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}
	stream := false

	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "dual-mode-chat-model", "", &stream)

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, fakeAdapter.chatCalls)
	require.Equal(t, 0, fakeAdapter.streamCalls)
}

func TestValidatorProbeModels_AutoSelectsModeForEachChatModel(t *testing.T) {
	validator := NewValidator(nil, nil)
	fakeAdapter := &fakeValidationAdapter{}

	items, failures := validator.probeModels(context.Background(), fakeAdapter, []modelCapability{
		{Model: "streaming-chat-model", UseCase: testMethodChat, SupportsStreaming: true},
		{Model: "blocking-chat-model", UseCase: testMethodChat, SupportsStreaming: false},
	})

	require.Len(t, items, 2)
	require.Empty(t, failures)
	require.Equal(t, 1, fakeAdapter.streamCalls)
	require.Equal(t, 1, fakeAdapter.chatCalls)
}

func TestValidatorTestModel_PreservesProviderErrorForStateObservation(t *testing.T) {
	providerErr := adapter.NewAdapterError("AUTHENTICATION_FAILED", "invalid key", 401, adapter.ErrAuthFailed)
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"deepseek-chat": {
				Provider: "deepseek",
				Model:    "deepseek-chat",
				UseCases: llmmodelmodel.StringArray{"text-chat"},
			},
		},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return &fakeValidationAdapter{
			chatFailures: map[string]error{"deepseek-chat": providerErr},
		}, nil
	}

	stream := false
	result, err := validator.TestModel(context.Background(), uuid.Nil, "deepseek", "deleted-key", "", "deepseek-chat", "", &stream)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Equal(t, providerAPIKeyInvalidMessage, result.Message)
	require.ErrorIs(t, result.ProviderError, adapter.ErrAuthFailed)
}

func TestValidatorTestModel_ImageProbeUsesDefaultSize(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"qwen-image-2.0": {Model: "qwen-image-2.0", UseCases: llmmodelmodel.StringArray{"image-gen"}},
		},
	}

	fakeAdapter := &fakeValidationAdapter{
		chatFailures:      map[string]error{},
		embeddingFailures: map[string]error{},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	stream := false
	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "qwen-image-2.0", "image-gen", &stream)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, TestStatusSuccess, result.Status)
	require.NotNil(t, fakeAdapter.lastImageReq)
	require.Equal(t, "1024x1024", fakeAdapter.lastImageReq.Size)
}

func TestValidatorTestModel_ImageDefaultSkipsWithoutListingOrImageProbe(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"qwen-image-2.0": {Model: "qwen-image-2.0", UseCases: llmmodelmodel.StringArray{"image-gen"}},
		},
	}

	fakeAdapter := &fakeValidationAdapter{
		listModelsErr: errors.New("default image test should not list upstream models"),
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	stream := false
	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "qwen-image-2.0", "", &stream)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Equal(t, TestStatusSkipped, result.Status)
	require.Equal(t, "qwen-image-2.0", result.Model)
	require.Equal(t, testMethodImageGeneration, result.UseCase)
	require.Equal(t, testMethodImageGeneration, result.TestMethod)
	require.Contains(t, result.Message, "image generation models require a real image generation test")
	require.Zero(t, fakeAdapter.listModelsCalls)
	require.Zero(t, fakeAdapter.imageCalls)
}

func TestValidatorValidateModelsForCreation_RejectsEmptyModelList(t *testing.T) {
	validator := NewValidator(nil, nil)

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai-compatible",
		"key",
		"https://example.com/v1",
		nil,
	)
	require.Error(t, err)
	require.NotNil(t, result)
	require.Contains(t, err.Error(), "at least one model is required")
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.Equal(t, 0, result.Report[keyValidatedCount])
}

func TestValidatorValidateModelsForCreation_ImageModelUsesLocalMetadataOnly(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"gpt-image-2": {
				Model:    "gpt-image-2",
				Provider: "openai",
				UseCases: llmmodelmodel.StringArray{
					"image-gen",
				},
			},
		},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	adapterCreated := false
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		adapterCreated = true
		return &fakeValidationAdapter{}, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai-compatible",
		"key",
		"https://example.com/v1",
		[]string{"gpt-image-2"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.False(t, adapterCreated)
	require.Equal(t, 1, result.Report[keyValidatedCount])
	require.Equal(t, 1, result.Report[keyPassedCount])
	require.Equal(t, []string{}, result.Report[keyProbedModels])
}

func TestValidatorValidateModelsForCreation_AllowsMusicAndAudioModels(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{models: map[string]*llmmodelmodel.LLMModel{
		"music-2.6": {
			Model: "music-2.6", Provider: "minimax",
			UseCases: llmmodelmodel.StringArray{string(llmmodelmodel.UseCaseMusicGen)}, MusicGeneration: true,
		},
		"music-3.0": {
			Model: "music-3.0", Provider: "minimax",
			UseCases: llmmodelmodel.StringArray{string(llmmodelmodel.UseCaseMusicGen)}, MusicGeneration: true,
		},
		"seed-tts-2.0": {
			Model: "seed-tts-2.0", Provider: "doubao",
			UseCases: llmmodelmodel.StringArray{string(llmmodelmodel.UseCaseTextToSpeech)}, SpeechGeneration: true,
		},
		"volc.seedasr.sauc.duration": {
			Model: "volc.seedasr.sauc.duration", Provider: "doubao",
			UseCases: llmmodelmodel.StringArray{string(llmmodelmodel.UseCaseSpeechToText)}, Transcription: true,
		},
	}}

	for _, test := range []struct {
		name     string
		provider string
		models   []string
	}{
		{name: "minimax music", provider: "minimax", models: []string{"music-2.6", "music-3.0"}},
		{name: "doubao audio", provider: "doubao-speech", models: []string{"seed-tts-2.0", "volc.seedasr.sauc.duration"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			validator := NewValidator(nil, nil)
			validator.modelRepo = modelRepo

			result, err := validator.ValidateModelsForCreation(
				t.Context(), uuid.Nil, test.provider, "key", "", test.models,
			)
			require.NoError(t, err)
			require.Equal(t, test.models, result.NormalizedModels)
			require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
		})
	}
}

func TestValidatorValidateModelsForCreation_EnforcesDoubaoCredentialScopes(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{models: map[string]*llmmodelmodel.LLMModel{
		"doubao-seed-2.0-lite": {
			Model: "doubao-seed-2.0-lite", Provider: "doubao",
			UseCases: llmmodelmodel.StringArray{string(llmmodelmodel.UseCaseTextChat)}, ChatCompletions: true,
		},
		"seed-tts-2.0": {
			Model: "seed-tts-2.0", Provider: "doubao",
			UseCases: llmmodelmodel.StringArray{string(llmmodelmodel.UseCaseTextToSpeech)}, SpeechGeneration: true,
		},
		"volc.seedasr.sauc.duration": {
			Model: "volc.seedasr.sauc.duration", Provider: "doubao",
			UseCases: llmmodelmodel.StringArray{string(llmmodelmodel.UseCaseSpeechToText)}, Transcription: true,
		},
	}}

	for _, test := range []struct {
		name     string
		provider string
		models   []string
		wantErr  string
	}{
		{name: "ark accepts ark models", provider: "doubao", models: []string{"doubao-seed-2.0-lite"}},
		{name: "ark rejects speech models", provider: "doubao", models: []string{"seed-tts-2.0"}, wantErr: `model "seed-tts-2.0" requires channel_provider "doubao-speech"`},
		{name: "speech accepts tts and stt", provider: "doubao-speech", models: []string{"seed-tts-2.0", "volc.seedasr.sauc.duration"}},
		{name: "speech rejects ark models", provider: "doubao-speech", models: []string{"doubao-seed-2.0-lite"}, wantErr: `model "doubao-seed-2.0-lite" requires channel_provider "doubao"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			validator := NewValidator(nil, nil)
			validator.modelRepo = modelRepo

			result, err := validator.ValidateModelsForCreation(
				t.Context(), uuid.Nil, test.provider, "key", "", test.models,
			)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.models, result.NormalizedModels)
		})
	}
}

func TestValidatorTestModel_SkipsMusicAndAudioWorkspaceModels(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider string
		useCase  llmmodelmodel.UseCase
		model    *llmmodelmodel.LLMModel
	}{
		{
			name: "music", provider: "minimax", useCase: llmmodelmodel.UseCaseMusicGen,
			model: &llmmodelmodel.LLMModel{Model: "music-3.0", Provider: "minimax", MusicGeneration: true, SupportsStreaming: true},
		},
		{
			name: "speech", provider: "doubao-speech", useCase: llmmodelmodel.UseCaseTextToSpeech,
			model: &llmmodelmodel.LLMModel{Model: "seed-tts-2.0", Provider: "doubao", SpeechGeneration: true, SupportsStreaming: true},
		},
		{
			name: "transcription", provider: "doubao-speech", useCase: llmmodelmodel.UseCaseSpeechToText,
			model: &llmmodelmodel.LLMModel{Model: "volc.seedasr.sauc.duration", Provider: "doubao", Transcription: true, SupportsStreaming: true},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.model.UseCases = llmmodelmodel.StringArray{string(test.useCase)}
			fakeAdapter := &fakeValidationAdapter{}
			validator := NewValidator(nil, nil)
			validator.modelRepo = &fakeModelLookupRepo{models: map[string]*llmmodelmodel.LLMModel{test.model.Model: test.model}}
			validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
				return fakeAdapter, nil
			}

			result, err := validator.TestModel(t.Context(), uuid.Nil, test.provider, "key", "", test.model.Model, "", nil)
			require.NoError(t, err)
			require.Equal(t, TestStatusSkipped, result.Status)
			require.Equal(t, string(test.useCase), result.UseCase)
			require.Zero(t, fakeAdapter.chatCalls)
			require.Zero(t, fakeAdapter.streamCalls)
		})
	}
}

func TestValidatorValidateModelsForCreation_UsesLocalMetadataOnlyAndIgnoresListing(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-a":  {Model: "chat-a", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"chat-b":  {Model: "chat-b", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"embed-a": {Model: "embed-a", UseCases: llmmodelmodel.StringArray{"embedding"}},
		},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	adapterCreated := false
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		adapterCreated = true
		return &fakeValidationAdapter{}, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai",
		"key",
		"",
		[]string{"chat-a", "chat-b", "embed-a"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.Equal(t, false, result.Report[keySampled])
	require.False(t, adapterCreated)
	require.Equal(t, 3, result.Report[keyValidatedCount])
	require.Equal(t, 3, result.Report[keyPassedCount])
	require.Equal(t, 0, result.Report[keyUnvalidatedCount])
	require.Empty(t, result.Report[keyFailedModels])
	require.Empty(t, WarningMessages(result.Report))
	require.Equal(t, []string{}, result.Report[keyProbedModels])
}

func TestValidatorValidateModelsForCreation_AllowsModelsMissingFromUpstreamList(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-a":  {Model: "chat-a", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"embed-a": {Model: "embed-a", UseCases: llmmodelmodel.StringArray{"embedding"}},
		},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	adapterCreated := false
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		adapterCreated = true
		return &fakeValidationAdapter{}, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai",
		"key",
		"",
		[]string{"chat-a", "embed-a"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.False(t, adapterCreated)
	require.Equal(t, 2, result.Report[keyValidatedCount])
	require.Equal(t, 2, result.Report[keyPassedCount])
	require.Empty(t, result.Report[keyFailedModels])
	require.Empty(t, WarningMessages(result.Report))
}

func TestValidatorValidateModelsForCreation_IgnoresModelListingErrors(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-a": {Model: "chat-a", UseCases: llmmodelmodel.StringArray{"text-chat"}},
		},
	}
	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	adapterCreated := false
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		adapterCreated = true
		return &fakeValidationAdapter{}, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"openai-compatible",
		"key",
		"://bad",
		[]string{"chat-a"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.Empty(t, WarningMessages(result.Report))
	require.False(t, adapterCreated)
}

func TestValidatorValidateModelsForCreation_DoesNotProbeProviderAPIKey(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"deepseek-v4-pro": {Model: "deepseek-v4-pro", UseCases: llmmodelmodel.StringArray{"text-chat"}},
		},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	adapterCreated := false
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		adapterCreated = true
		return &fakeValidationAdapter{}, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"deepseek",
		"key",
		"",
		[]string{"deepseek-v4-pro"},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, adapterCreated)
	require.Equal(t, validationModeMetadataOnly, result.Report[keyValidationMode])
	require.Equal(t, 1, result.Report[keyValidatedCount])
}
