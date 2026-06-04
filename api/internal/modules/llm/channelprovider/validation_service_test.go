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
	return nil, errors.New("not implemented")
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

func (f *fakeModelLookupRepo) List(_ context.Context, _ *uuid.UUID, _, _ string, _ *bool, _, _ int) ([]*llmmodelmodel.LLMModel, int64, error) {
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
	embeddingCalls  int
	imageCalls      int
	rerankCalls     int
	lastImageReq    *adapter.ImageRequest

	listModelsResult []adapter.Model
	listModelsErr    error

	chatFailures      map[string]error
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

func (f *fakeValidationAdapter) ChatCompletionStream(context.Context, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, fmt.Errorf("%w: streaming", adapter.ErrCapabilityUnsupported)
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
		chatFailures:      map[string]error{"ernie-x1-turbo-32k": errors.New("creation should not probe chat")},
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
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
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
		chatFailures:      map[string]error{"qwen3.5:9b": errors.New("creation should not probe chat")},
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
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.NotEmpty(t, WarningMessages(result.Report))
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
		chatFailures:      map[string]error{"qwen3.5:9b": errors.New("creation should not probe chat")},
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
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
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

	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai", "key", "", "embed-a", "chatCompletions")
	require.Error(t, err)
	require.Nil(t, result)
	require.Contains(t, err.Error(), "conflicts")
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

	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "qwen-image-2.0", "image-gen")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.NotNil(t, fakeAdapter.lastImageReq)
	require.Equal(t, "1024x1024", fakeAdapter.lastImageReq.Size)
}

func TestValidatorTestModel_ImageDefaultUsesModelListWithoutImageProbe(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"qwen-image-2.0": {Model: "qwen-image-2.0", UseCases: llmmodelmodel.StringArray{"image-gen"}},
		},
	}

	fakeAdapter := &fakeValidationAdapter{
		listModelsResult: []adapter.Model{{ID: "qwen-image-2.0"}},
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "qwen-image-2.0", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, "qwen-image-2.0", result.Model)
	require.Equal(t, testMethodImageGeneration, result.UseCase)
	require.Equal(t, testMethodMetadata, result.TestMethod)
	require.Equal(t, "model is returned by upstream model list; real image generation was not run", result.Message)
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Zero(t, fakeAdapter.imageCalls)
}

func TestValidatorTestModel_ImageDefaultFailsOnModelListError(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"qwen-image-2.0": {Model: "qwen-image-2.0", UseCases: llmmodelmodel.StringArray{"image-gen"}},
		},
	}

	fakeAdapter := &fakeValidationAdapter{
		listModelsErr: errors.New("invalid_api_key"),
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "qwen-image-2.0", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Equal(t, testMethodImageGeneration, result.UseCase)
	require.Equal(t, testMethodMetadata, result.TestMethod)
	require.Contains(t, result.Message, "API key is invalid")
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Zero(t, fakeAdapter.imageCalls)
}

func TestValidatorTestModel_ImageDefaultFallsBackToMetadataWhenListUnsupported(t *testing.T) {
	validator := NewValidator(nil, nil)
	validator.modelRepo = &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"qwen-image-2.0": {Model: "qwen-image-2.0", UseCases: llmmodelmodel.StringArray{"image-gen"}},
		},
	}

	fakeAdapter := &fakeValidationAdapter{
		listModelsErr: adapter.ErrCapabilityUnsupported,
	}
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.TestModel(context.Background(), uuid.Nil, "openai-compatible", "key", "https://example.com/v1", "qwen-image-2.0", "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Success)
	require.Equal(t, testMethodImageGeneration, result.UseCase)
	require.Equal(t, testMethodMetadata, result.TestMethod)
	require.Equal(t, "validated local model metadata; upstream model listing is unsupported; real image generation was not run", result.Message)
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Zero(t, fakeAdapter.imageCalls)
}

func TestValidatorValidateModelsForCreation_ImageModelUsesListingAndSkipsRealProbe(t *testing.T) {
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
	fakeAdapter := &fakeValidationAdapter{
		listModelsResult: []adapter.Model{
			{ID: "gpt-image-2"},
		},
		imageFailures: map[string]error{
			"gpt-image-2": errors.New("creation should not call image generation"),
		},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
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
	require.Equal(t, validationModeListing, result.Report[keyValidationMode])
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.imageCalls)
	require.Nil(t, fakeAdapter.lastImageReq)
}

func TestValidatorValidateModelsForCreation_UsesModelListingAndSkipsRepresentativeProbes(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-a":  {Model: "chat-a", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"chat-b":  {Model: "chat-b", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"embed-a": {Model: "embed-a", UseCases: llmmodelmodel.StringArray{"embedding"}},
		},
	}
	fakeAdapter := &fakeValidationAdapter{
		listModelsResult:  []adapter.Model{{ID: "chat-a"}, {ID: "chat-b"}, {ID: "embed-a"}},
		chatFailures:      map[string]error{"chat-a": errors.New("chat probe failed")},
		embeddingFailures: map[string]error{"embed-a": errors.New("embedding probe failed")},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
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
	require.Equal(t, validationModeListing, result.Report[keyValidationMode])
	require.Equal(t, false, result.Report[keySampled])
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.Equal(t, 0, fakeAdapter.embeddingCalls)
	require.Equal(t, 3, result.Report[keyValidatedCount])
	require.Equal(t, 3, result.Report[keyPassedCount])
	require.Equal(t, 0, result.Report[keyUnvalidatedCount])
	require.Empty(t, result.Report[keyFailedModels])
	require.Empty(t, WarningMessages(result.Report))
}

func TestValidatorValidateModelsForCreation_ReportsModelsMissingFromUpstreamList(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-a":  {Model: "chat-a", UseCases: llmmodelmodel.StringArray{"text-chat"}},
			"embed-a": {Model: "embed-a", UseCases: llmmodelmodel.StringArray{"embedding"}},
		},
	}
	fakeAdapter := &fakeValidationAdapter{
		listModelsResult:  []adapter.Model{{ID: "chat-a"}},
		chatFailures:      map[string]error{"chat-a": errors.New("creation should not probe chat")},
		embeddingFailures: map[string]error{"embed-a": errors.New("creation should not probe embedding")},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
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
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
	require.Equal(t, 0, fakeAdapter.embeddingCalls)
	require.Equal(t, 2, result.Report[keyValidatedCount])
	require.Equal(t, 1, result.Report[keyPassedCount])
	require.Contains(t, fmt.Sprint(result.Report[keyFailedModels]), "embed-a")
	require.Contains(t, fmt.Sprint(WarningMessages(result.Report)), "embed-a")
}

func TestValidatorValidateModelsForCreation_ReportsModelListingErrors(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"chat-a": {Model: "chat-a", UseCases: llmmodelmodel.StringArray{"text-chat"}},
		},
	}
	fakeAdapter := &fakeValidationAdapter{
		listModelsErr: errors.New(`request failed: Get "://bad/models": unsupported protocol scheme ""`),
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
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
	require.NotEmpty(t, WarningMessages(result.Report))
	require.Contains(t, fmt.Sprint(WarningMessages(result.Report)), "failed to list upstream models")
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
}

func TestValidatorValidateModelsForCreation_ReturnsFriendlyProviderAPIKeyError(t *testing.T) {
	modelRepo := &fakeModelLookupRepo{
		models: map[string]*llmmodelmodel.LLMModel{
			"deepseek-v4-pro": {Model: "deepseek-v4-pro", UseCases: llmmodelmodel.StringArray{"text-chat"}},
		},
	}
	fakeAdapter := &fakeValidationAdapter{
		listModelsErr: adapter.NewAdapterError(
			"invalid_api_key",
			"Authentication Fails, Your api key: ****1d35 is invalid",
			401,
			adapter.ErrAuthFailed,
		),
		chatFailures: map[string]error{
			"deepseek-v4-pro": adapter.NewAdapterError(
				"invalid_api_key",
				"Authentication Fails, Your api key: ****1d35 is invalid",
				401,
				adapter.ErrAuthFailed,
			),
		},
		embeddingFailures: map[string]error{},
		imageFailures:     map[string]error{},
		rerankFailures:    map[string]error{},
	}

	validator := NewValidator(nil, nil)
	validator.modelRepo = modelRepo
	validator.newAdapter = func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return fakeAdapter, nil
	}

	result, err := validator.ValidateModelsForCreation(
		context.Background(),
		uuid.Nil,
		"deepseek",
		"key",
		"",
		[]string{"deepseek-v4-pro"},
	)
	require.Error(t, err)
	require.NotNil(t, result)
	require.ErrorIs(t, err, ErrProviderAPIKeyInvalid)
	require.Equal(t, providerAPIKeyInvalidMessage, err.Error())
	require.NotContains(t, err.Error(), "Authentication Fails")
	require.NotContains(t, err.Error(), "1d35")
	require.Equal(t, 1, fakeAdapter.listModelsCalls)
	require.Equal(t, 0, fakeAdapter.chatCalls)
}
