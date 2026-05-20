package service

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/dto"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	providermodel "github.com/zgiai/ginext/internal/modules/llm/provider/model"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"gorm.io/gorm"
)

type fakeGlobalRepo struct {
	models                     []*model.LLMModel
	listCalls                  int
	listAvailableByNamesCalls  int
	listAvailableFilteredCalls int
	listAvailableNames         []string
	listAvailableProvider      string
	listAvailableUseCase       string
}

func (f *fakeGlobalRepo) Create(ctx context.Context, m *model.LLMModel) error {
	return nil
}

func (f *fakeGlobalRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.LLMModel, error) {
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeGlobalRepo) GetByName(ctx context.Context, name string) (*model.LLMModel, error) {
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeGlobalRepo) ListByNames(ctx context.Context, names []string) ([]*model.LLMModel, error) {
	if len(names) == 0 {
		return []*model.LLMModel{}, nil
	}

	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	models := make([]*model.LLMModel, 0, len(names))
	for _, item := range f.models {
		if _, ok := nameSet[item.Model]; ok {
			models = append(models, item)
		}
	}
	return models, nil
}

func (f *fakeGlobalRepo) ListAvailableByNames(ctx context.Context, names []string, provider string, useCase string) ([]*model.LLMModel, error) {
	f.listAvailableByNamesCalls++
	f.listAvailableNames = append([]string(nil), names...)
	f.listAvailableProvider = provider
	f.listAvailableUseCase = useCase
	return f.filterAvailable(names, provider, useCase), nil
}

func (f *fakeGlobalRepo) ListAvailableFiltered(ctx context.Context, provider string, useCase string) ([]*model.LLMModel, error) {
	f.listAvailableFilteredCalls++
	f.listAvailableProvider = provider
	f.listAvailableUseCase = useCase
	return f.filterAvailable(nil, provider, useCase), nil
}

func (f *fakeGlobalRepo) GetByProviderAndName(ctx context.Context, provider string, name string) (*model.LLMModel, error) {
	for _, item := range f.models {
		if item.Provider == provider && item.Model == name {
			return item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeGlobalRepo) List(ctx context.Context, providerID *uuid.UUID, provider string, useCase string, isActive *bool, offset, limit int) ([]*model.LLMModel, int64, error) {
	f.listCalls++
	out := make([]*model.LLMModel, 0, len(f.models))
	for _, item := range f.models {
		if provider != "" && item.Provider != provider {
			continue
		}
		if useCase != "" && !containsUseCase(item.UseCases, useCase) {
			continue
		}
		if isActive != nil && item.IsActive != *isActive {
			continue
		}
		out = append(out, item)
	}
	return out, int64(len(out)), nil
}

func (f *fakeGlobalRepo) filterAvailable(names []string, provider string, useCase string) []*model.LLMModel {
	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	out := make([]*model.LLMModel, 0, len(f.models))
	for _, item := range f.models {
		if len(nameSet) > 0 {
			if _, ok := nameSet[item.Model]; !ok {
				continue
			}
		}
		if provider != "" && item.Provider != provider {
			continue
		}
		if useCase != "" && !containsUseCase(item.UseCases, useCase) {
			continue
		}
		if !item.IsActive {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (f *fakeGlobalRepo) Update(ctx context.Context, m *model.LLMModel) error {
	return nil
}

func (f *fakeGlobalRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (f *fakeGlobalRepo) ListByProvider(ctx context.Context, providerID string) ([]*model.LLMModel, error) {
	return nil, nil
}

type fakeModelConfigRepo struct {
	configs                   []*model.ModelConfig
	listCalls                 int
	listAvailableConfigsCalls int
}

func (f *fakeModelConfigRepo) Create(ctx context.Context, config *model.ModelConfig) error {
	return nil
}

func (f *fakeModelConfigRepo) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.ModelConfig, error) {
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeModelConfigRepo) GetByModelID(ctx context.Context, organizationID, modelID uuid.UUID) (*model.ModelConfig, error) {
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeModelConfigRepo) List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*model.ModelConfig, int64, error) {
	f.listCalls++
	return f.configs, int64(len(f.configs)), nil
}

func (f *fakeModelConfigRepo) ListAvailableConfigs(ctx context.Context, organizationID uuid.UUID) ([]*model.ModelConfig, error) {
	f.listAvailableConfigsCalls++
	return f.configs, nil
}

func (f *fakeModelConfigRepo) Update(ctx context.Context, config *model.ModelConfig) error {
	return nil
}

func (f *fakeModelConfigRepo) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return nil
}

func (f *fakeModelConfigRepo) Upsert(ctx context.Context, config *model.ModelConfig) error {
	return nil
}

func (f *fakeModelConfigRepo) BatchCreate(ctx context.Context, configs []*model.ModelConfig) error {
	return nil
}

type fakeAvailableModelsService struct {
	invalidated []uuid.UUID
}

func (f *fakeAvailableModelsService) ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*AvailableModel, error) {
	return nil, nil
}

func (f *fakeAvailableModelsService) RefreshCache(ctx context.Context, organizationID uuid.UUID) error {
	return nil
}

func (f *fakeAvailableModelsService) InvalidateTenantCache(organizationID uuid.UUID) {
	f.invalidated = append(f.invalidated, organizationID)
}

func (f *fakeAvailableModelsService) InvalidateGlobalCache() {}

func (f *fakeAvailableModelsService) SetOfficialRouteBootstrapper(bootstrapper interfaces.OfficialRouteBootstrapper) {
}

type fakeCustomModelRepo struct {
	models       []*model.CustomModel
	listErr      error
	listCalls    int
	lastProvider string
}

func (f *fakeCustomModelRepo) Create(ctx context.Context, m *model.CustomModel) error {
	f.models = append(f.models, m)
	return nil
}

func (f *fakeCustomModelRepo) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*model.CustomModel, error) {
	for _, item := range f.models {
		if item.OrganizationID == organizationID && item.ID == id {
			return item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeCustomModelRepo) GetByProviderAndName(ctx context.Context, organizationID, providerID uuid.UUID, name string) (*model.CustomModel, error) {
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeCustomModelRepo) GetByProviderAndModel(ctx context.Context, organizationID uuid.UUID, provider string, name string) (*model.CustomModel, error) {
	for _, item := range f.models {
		if item.OrganizationID == organizationID && item.Provider == provider && item.Name == name {
			return item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeCustomModelRepo) List(ctx context.Context, organizationID uuid.UUID, providerID *uuid.UUID, provider string, useCase string, isActive *bool, offset, limit int) ([]*model.CustomModel, int64, error) {
	f.listCalls++
	f.lastProvider = provider
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	out := make([]*model.CustomModel, 0, len(f.models))
	for _, item := range f.models {
		if item.OrganizationID != organizationID {
			continue
		}
		if provider != "" && item.Provider != provider {
			continue
		}
		if isActive != nil && item.IsActive != *isActive {
			continue
		}
		out = append(out, item)
	}
	return out, int64(len(out)), nil
}

func (f *fakeCustomModelRepo) ListByNames(ctx context.Context, organizationID uuid.UUID, names []string, isActive *bool) ([]*model.CustomModel, error) {
	if len(names) == 0 {
		return []*model.CustomModel{}, nil
	}

	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	models := make([]*model.CustomModel, 0, len(names))
	for _, item := range f.models {
		if item.OrganizationID != organizationID {
			continue
		}
		if _, ok := nameSet[item.Name]; !ok {
			continue
		}
		if isActive != nil && item.IsActive != *isActive {
			continue
		}
		models = append(models, item)
	}
	return models, nil
}

func (f *fakeCustomModelRepo) Update(ctx context.Context, m *model.CustomModel) error {
	for idx, item := range f.models {
		if item.OrganizationID == m.OrganizationID && item.ID == m.ID {
			f.models[idx] = m
			return nil
		}
	}
	return nil
}

func (f *fakeCustomModelRepo) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return nil
}

func (f *fakeCustomModelRepo) ListByProvider(ctx context.Context, organizationID, providerID uuid.UUID) ([]*model.CustomModel, error) {
	return nil, nil
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_routes (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			models TEXT,
			model_maps TEXT,
			is_enabled BOOLEAN DEFAULT true,
			is_official BOOLEAN DEFAULT false,
			deleted_at DATETIME
		)
	`).Error)

	return db
}

func TestListTenantModels_CustomModelProviderMapped(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()

	customModel := &model.CustomModel{
		ID:             uuid.New(),
		OrganizationID: organizationID,
		ProviderID:     uuid.New(),
		Provider:       "test-provider",
		Name:           "ernie-x1-turbo-32k",
		DisplayName:    "文心x1",
		UseCases:       model.StringArray{"text-chat"},
		IsActive:       true,
	}

	svc := &modelService{
		globalRepo: &fakeGlobalRepo{},
		configRepo: &fakeModelConfigRepo{},
		customRepo: &fakeCustomModelRepo{
			models: []*model.CustomModel{customModel},
		},
		db: db,
	}

	models, err := svc.ListTenantModels(context.Background(), organizationID, "", "")
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "test-provider", models[0].Provider)
	assert.Equal(t, "ernie-x1-turbo-32k", models[0].Model)
}

func TestListTenantModels_GlobalProviderDoesNotQueryCustomModels(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	providerID := uuid.New()
	globalRepo := &fakeGlobalRepo{
		models: []*model.LLMModel{{
			ID:        uuid.New(),
			Provider:  "openai",
			Model:     "gpt-5",
			ModelName: "GPT-5",
			UseCases:  model.StringArray{"text-chat"},
			IsActive:  true,
		}},
	}
	customRepo := &fakeCustomModelRepo{
		listErr: errors.New(`ERROR: column llm_custom_models.context_window does not exist`),
	}

	svc := NewModelServiceWithProviderRepos(
		db,
		globalRepo,
		&fakeModelConfigRepo{},
		customRepo,
		nil,
		&fakeCustomProviderRepo{},
		&fakeProviderRepo{
			providers: []*providermodel.LLMProvider{{
				ID:       providerID,
				Provider: "openai",
				IsActive: true,
			}},
		},
		&fakeProviderConfigRepo{},
	)

	models, err := svc.ListTenantModels(context.Background(), organizationID, "", "openai")
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "openai", models[0].Provider)
	assert.Equal(t, "gpt-5", models[0].Model)
	assert.Equal(t, 1, globalRepo.listCalls)
	assert.Equal(t, 0, customRepo.listCalls)
}

func TestListTenantModels_CustomProviderDoesNotQueryGlobalModels(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	providerID := uuid.New()
	globalRepo := &fakeGlobalRepo{
		models: []*model.LLMModel{{
			ID:        uuid.New(),
			Provider:  "openai",
			Model:     "gpt-5",
			ModelName: "GPT-5",
			IsActive:  true,
		}},
	}
	customRepo := &fakeCustomModelRepo{
		models: []*model.CustomModel{{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			ProviderID:     providerID,
			Provider:       "custom-1",
			Name:           "qwen3.5:9b",
			DisplayName:    "Qwen 3.5 9B",
			IsActive:       true,
		}},
	}

	svc := NewModelServiceWithProviderRepos(
		db,
		globalRepo,
		&fakeModelConfigRepo{},
		customRepo,
		nil,
		&fakeCustomProviderRepo{
			providers: []*providermodel.CustomProvider{{
				ID:             providerID,
				OrganizationID: organizationID,
				Provider:       "custom-1",
				IsActive:       true,
			}},
		},
		&fakeProviderRepo{},
		&fakeProviderConfigRepo{},
	)

	models, err := svc.ListTenantModels(context.Background(), organizationID, "", "custom-1")
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "custom-1", models[0].Provider)
	assert.Equal(t, "qwen3.5:9b", models[0].Model)
	assert.Equal(t, 0, globalRepo.listCalls)
	assert.Equal(t, 1, customRepo.listCalls)
	assert.Equal(t, "custom-1", customRepo.lastProvider)
}

func TestListTenantModels_SameProviderNameQueriesGlobalAndCustom(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	globalProviderID := uuid.New()
	customProviderID := uuid.New()

	globalRepo := &fakeGlobalRepo{
		models: []*model.LLMModel{{
			ID:        uuid.New(),
			Provider:  "openai",
			Model:     "gpt-5",
			ModelName: "GPT-5",
			IsActive:  true,
		}},
	}
	customRepo := &fakeCustomModelRepo{
		models: []*model.CustomModel{{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			ProviderID:     customProviderID,
			Provider:       "openai",
			Name:           "gpt-5-private",
			DisplayName:    "GPT-5 Private",
			IsActive:       true,
		}},
	}

	svc := NewModelServiceWithProviderRepos(
		db,
		globalRepo,
		&fakeModelConfigRepo{},
		customRepo,
		nil,
		&fakeCustomProviderRepo{
			providers: []*providermodel.CustomProvider{{
				ID:             customProviderID,
				OrganizationID: organizationID,
				Provider:       "openai",
				IsActive:       true,
			}},
		},
		&fakeProviderRepo{
			providers: []*providermodel.LLMProvider{{
				ID:       globalProviderID,
				Provider: "openai",
				IsActive: true,
			}},
		},
		&fakeProviderConfigRepo{},
	)

	models, err := svc.ListTenantModels(context.Background(), organizationID, "", "openai")
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, 1, globalRepo.listCalls)
	assert.Equal(t, 1, customRepo.listCalls)
	assert.Equal(t, "openai", customRepo.lastProvider)
}

func TestListTenantModels_HidesModelsWhenProviderDisabled(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	providerID := uuid.New()

	globalModel := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	svc := NewModelServiceWithProviderRepos(
		db,
		&fakeGlobalRepo{models: []*model.LLMModel{globalModel}},
		&fakeModelConfigRepo{},
		&fakeCustomModelRepo{},
		nil,
		&fakeCustomProviderRepo{},
		&fakeProviderRepo{
			providers: []*providermodel.LLMProvider{{
				ID:       providerID,
				Provider: "openai",
				IsActive: true,
			}},
		},
		&fakeProviderConfigRepo{
			configs: []*providermodel.ProviderConfig{{
				OrganizationID: organizationID,
				ProviderID:     providerID,
				IsEnabled:      false,
			}},
		},
	)

	models, err := svc.ListTenantModels(context.Background(), organizationID, "", "")
	require.NoError(t, err)
	require.Empty(t, models)
}

func TestListTenantModels_GlobalModelsRequireExplicitTenantEnable(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	modelID := uuid.New()

	globalModel := &model.LLMModel{
		ID:        modelID,
		Provider:  "baidu",
		Model:     "ernie-4.0",
		ModelName: "ERNIE 4.0",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	require.NoError(t, db.Exec(`
		INSERT INTO llm_routes (id, organization_id, models, model_maps, is_enabled, is_official, deleted_at)
		VALUES (?, ?, ?, '{}', true, false, NULL)
	`, uuid.NewString(), organizationID.String(), `["ernie-4.0"]`).Error)

	svc := &modelService{
		globalRepo: &fakeGlobalRepo{models: []*model.LLMModel{globalModel}},
		configRepo: &fakeModelConfigRepo{},
		customRepo: &fakeCustomModelRepo{},
		db:         db,
	}

	models, err := svc.ListTenantModels(context.Background(), organizationID, "", "")
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.False(t, models[0].IsEnabled)
	assert.False(t, models[0].Callable)

	svc.configRepo = &fakeModelConfigRepo{
		configs: []*model.ModelConfig{{
			OrganizationID: organizationID,
			ModelID:        modelID,
			IsEnabled:      true,
		}},
	}

	models, err = svc.ListTenantModels(context.Background(), organizationID, "", "")
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.True(t, models[0].IsEnabled)
	assert.True(t, models[0].Callable)
}

func TestBatchToggleModels_InvalidatesAvailableModelsCache(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	modelIDs := []uuid.UUID{uuid.New(), uuid.New()}
	availableSvc := &fakeAvailableModelsService{}

	svc := &modelService{
		configRepo:      &fakeModelConfigRepo{},
		availableModels: availableSvc,
		db:              db,
	}

	err := svc.BatchToggleModels(context.Background(), organizationID, modelIDs, false)
	require.NoError(t, err)
	require.Len(t, availableSvc.invalidated, 1)
	assert.Equal(t, organizationID, availableSvc.invalidated[0])
}

func TestCreateCustomRejectsInvalidInputPrice(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	providerID := uuid.New()

	svc := &modelService{
		customRepo: &fakeCustomModelRepo{},
		customProviderRepo: &fakeCustomProviderRepo{
			providers: []*providermodel.CustomProvider{{
				ID:             providerID,
				OrganizationID: organizationID,
				Provider:       "ollama",
				IsActive:       true,
			}},
		},
		db: db,
	}

	_, err := svc.CreateCustom(context.Background(), organizationID, &dto.CreateCustomModelRequest{
		Provider:    "ollama",
		Name:        "qwen3.5:4b",
		DisplayName: "Qwen 3.5 4B",
		UseCases:    []string{"text-chat"},
		InputPrice:  "not-a-number",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "input_price")
}

func TestUpdateCustomRejectsInvalidOutputPrice(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	modelID := uuid.New()
	badPrice := "bad-price"

	svc := &modelService{
		customRepo: &fakeCustomModelRepo{
			models: []*model.CustomModel{{
				ID:             modelID,
				OrganizationID: organizationID,
				Provider:       "ollama",
				Name:           "qwen3.5:4b",
				DisplayName:    "Qwen 3.5 4B",
				IsActive:       true,
			}},
		},
		db: db,
	}

	_, err := svc.UpdateCustom(context.Background(), organizationID, modelID, &dto.UpdateCustomModelRequest{
		OutputPrice: &badPrice,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "output_price")
}

func TestGetModelParametersPrefersCustomModel(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()

	svc := &modelService{
		globalRepo: &fakeGlobalRepo{
			models: []*model.LLMModel{{
				Provider: "openai",
				Model:    "gpt-4.1",
				ConfigParameters: model.ConfigParameters{
					{Name: "temperature", TemplateKey: "temperature", Type: "float"},
				},
			}},
		},
		configRepo: &fakeModelConfigRepo{},
		customRepo: &fakeCustomModelRepo{
			models: []*model.CustomModel{{
				OrganizationID: organizationID,
				Provider:       "openai",
				Name:           "gpt-4.1",
				ConfigParameters: model.ConfigParameters{
					{Name: "top_p", TemplateKey: "top_p", Type: "float"},
				},
			}},
		},
		db: db,
	}

	params, err := svc.GetModelParameters(context.Background(), organizationID, "openai", "gpt-4.1")
	require.NoError(t, err)
	require.Len(t, params, 1)
	assert.Equal(t, "top_p", params[0].Name)
}

func TestGetModelParametersFallsBackToGlobalModel(t *testing.T) {
	db := setupTestDB(t)
	organizationID := uuid.New()
	globalModel := &model.LLMModel{
		Provider: "openai",
		Model:    "gpt-4.1-mini",
		ConfigParameters: model.ConfigParameters{
			{Name: "temperature", TemplateKey: "temperature", Type: "float"},
		},
	}

	svc := &modelService{
		globalRepo: &fakeGlobalRepo{models: []*model.LLMModel{globalModel}},
		configRepo: &fakeModelConfigRepo{},
		customRepo: &fakeCustomModelRepo{},
		db:         db,
	}

	params, err := svc.GetModelParameters(context.Background(), organizationID, "openai", "gpt-4.1-mini")
	require.NoError(t, err)
	require.Len(t, params, 1)
	assert.Equal(t, "temperature", params[0].Name)
}

func TestGetModelParametersReturnsNotFound(t *testing.T) {
	db := setupTestDB(t)
	svc := &modelService{
		globalRepo: &fakeGlobalRepo{},
		configRepo: &fakeModelConfigRepo{},
		customRepo: &fakeCustomModelRepo{},
		db:         db,
	}

	_, err := svc.GetModelParameters(context.Background(), uuid.New(), "openai", "missing")
	require.ErrorIs(t, err, ErrModelNotFound)
}
