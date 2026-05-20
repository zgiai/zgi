package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/ginext/internal/modules/llm/channel/repository"
	credentialmodel "github.com/zgiai/ginext/internal/modules/llm/credential/model"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/ginext/internal/modules/llm/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var _ channelrepo.TenantRouteRepository = (*fakeGatewayRouteRepo)(nil)

type fakeGatewayRouteRepo struct {
	routes []*channelmodel.LLMRoute
}

func (f *fakeGatewayRouteRepo) Create(context.Context, *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) BatchCreate(context.Context, []*channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) GetByID(context.Context, uuid.UUID, uuid.UUID) (*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) List(context.Context, uuid.UUID, *bool, int, int) ([]*channelmodel.LLMRoute, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) Update(context.Context, *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) GetEnabledRoutes(context.Context, uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	return append([]*channelmodel.LLMRoute(nil), f.routes...), nil
}

func (f *fakeGatewayRouteRepo) FindByModel(context.Context, uuid.UUID, string) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) CountByCredentialID(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) GetDistinctProviders(context.Context, uuid.UUID) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeGatewayRouteRepo) GetPlatformChannels(context.Context) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

type fakeGatewayPrivateModelLookup struct {
	models map[string]*llmmodel.CustomModel
}

func (f *fakeGatewayPrivateModelLookup) ListActiveModelsByNames(_ context.Context, _ uuid.UUID, modelNames []string) ([]*llmmodel.CustomModel, error) {
	result := make([]*llmmodel.CustomModel, 0, len(modelNames))
	for _, name := range modelNames {
		if modelRecord, ok := f.models[name]; ok {
			result = append(result, modelRecord)
		}
	}
	return result, nil
}

func (f *fakeGatewayPrivateModelLookup) ResolveActiveModels(_ context.Context, _ uuid.UUID, modelNames []string) ([]*llmmodel.CustomModel, error) {
	result := make([]*llmmodel.CustomModel, 0, len(modelNames))
	for _, name := range modelNames {
		if modelRecord, ok := f.models[name]; ok {
			result = append(result, modelRecord)
		}
	}
	return result, nil
}

func (f *fakeGatewayPrivateModelLookup) ResolveActiveModelsForProvider(_ context.Context, _ uuid.UUID, provider string, modelNames []string) ([]*llmmodel.CustomModel, error) {
	records, err := f.ResolveActiveModels(context.Background(), uuid.Nil, modelNames)
	if err != nil {
		return nil, err
	}
	result := make([]*llmmodel.CustomModel, 0, len(records))
	for _, record := range records {
		if record != nil && record.Provider == provider {
			result = append(result, record)
		}
	}
	return result, nil
}

func (f *fakeGatewayPrivateModelLookup) ResolveActiveModel(_ context.Context, _ uuid.UUID, modelName string) (*llmmodel.CustomModel, error) {
	if modelRecord, ok := f.models[modelName]; ok {
		return modelRecord, nil
	}
	return nil, nil
}

func (f *fakeGatewayPrivateModelLookup) ResolveActiveModelForProvider(_ context.Context, _ uuid.UUID, provider string, modelName string) (*llmmodel.CustomModel, error) {
	modelRecord, err := f.ResolveActiveModel(context.Background(), uuid.Nil, modelName)
	if err != nil || modelRecord == nil || modelRecord.Provider != provider {
		return nil, err
	}
	return modelRecord, nil
}

func (f *fakeGatewayPrivateModelLookup) LoadActiveModelNameIndexes(_ context.Context, _ uuid.UUID) ([]string, map[string]string, error) {
	return nil, nil, errors.New("not implemented")
}

func TestSelectChannel_CustomWorkspaceModelSkipsOfficialRoutes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	orgID := uuid.New()
	privateRouteID := uuid.New()
	privateRoute := &channelmodel.LLMRoute{
		ID:              privateRouteID,
		OrganizationID:  orgID,
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "openai-compatible",
		APIBaseURL:      "https://proxy.example.com/v1",
		Models:          []string{"ernie-x1-turbo-32k"},
		IsEnabled:       true,
		Priority:        100,
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  "openai-compatible",
			APIBaseURL:       "https://proxy.example.com/v1",
			APIKeyCiphertext: "ciphertext",
			IsActive:         true,
		},
	}
	officialRoute := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  orgID,
		Type:            shared.RouteTypeZGICloud,
		IsOfficial:      true,
		ChannelProvider: "zgi-cloud",
		Models:          []string{"ernie-x1-turbo-32k"},
		IsEnabled:       true,
		Priority:        200,
	}

	router := NewChannelRouter(db, stubCryptoService{}, nil)
	router.organizationIDRouteRepo = &fakeGatewayRouteRepo{
		routes: []*channelmodel.LLMRoute{officialRoute, privateRoute},
	}
	router.privateModels = &fakeGatewayPrivateModelLookup{
		models: map[string]*llmmodel.CustomModel{
			"ernie-x1-turbo-32k": {
				Name:            "ernie-x1-turbo-32k",
				DisplayName:     "ernie-x1-turbo-32k",
				UseCases:        llmmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
		},
	}

	selection, err := router.SelectChannel(context.Background(), orgID, "ernie-x1-turbo-32k")
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, privateRouteID, selection.RouteID)
	require.False(t, selection.IsOfficial)
	require.Equal(t, "openai-compatible", selection.ChannelProvider)
}

func TestSelectChannelsForProvider_CustomModelProviderDoesNotFilterProtocolRoute(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	createCustomProviderTable(t, db)

	orgID := uuid.New()
	customProviderID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO llm_custom_providers (id, organization_id, provider, provider_name, api_base_url, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		customProviderID.String(), orgID.String(), "custom-1", "Custom 1", "https://example.com/v1", true,
	).Error)

	privateRouteID := uuid.New()
	privateRoute := &channelmodel.LLMRoute{
		ID:              privateRouteID,
		OrganizationID:  orgID,
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "openai-compatible",
		APIBaseURL:      "https://example.com/v1",
		Models:          []string{"qwen3.5:9b"},
		IsEnabled:       true,
		Priority:        100,
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  "openai-compatible",
			APIBaseURL:       "https://example.com/v1",
			APIKeyCiphertext: "ciphertext",
			IsActive:         true,
		},
	}

	router := NewChannelRouter(db, stubCryptoService{}, nil)
	router.organizationIDRouteRepo = &fakeGatewayRouteRepo{
		routes: []*channelmodel.LLMRoute{privateRoute},
	}
	router.privateModels = &fakeGatewayPrivateModelLookup{
		models: map[string]*llmmodel.CustomModel{
			"qwen3.5:9b": {
				Name:            "qwen3.5:9b",
				DisplayName:     "qwen3.5:9b",
				ProviderID:      customProviderID,
				Provider:        "custom-1",
				UseCases:        llmmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
		},
	}

	selections, err := router.SelectChannelsForProvider(context.Background(), orgID, "custom-1", "qwen3.5:9b", 1)
	require.NoError(t, err)
	require.Len(t, selections, 1)
	require.Equal(t, privateRouteID, selections[0].RouteID)
	require.Equal(t, "openai-compatible", selections[0].ChannelProvider)
	require.Equal(t, channelModelSourceCustom, selections[0].ModelSource)
	require.Equal(t, customProviderID, selections[0].ModelProviderID)
}

func TestConvertToProviderSelection_PrivateCustomModelUsesCustomProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	createCustomProviderTable(t, db)

	orgID := uuid.New()
	customProviderID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO llm_custom_providers (id, organization_id, provider, provider_name, api_base_url, is_active) VALUES (?, ?, ?, ?, ?, ?)`,
		customProviderID.String(),
		orgID.String(),
		"ollama",
		"Ollama",
		"http://localhost:11434",
		true,
	).Error)

	privateRouteID := uuid.New()
	privateRoute := &channelmodel.LLMRoute{
		ID:              privateRouteID,
		OrganizationID:  orgID,
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434",
		Models:          []string{"qwen3.5:4b"},
		IsEnabled:       true,
		Priority:        100,
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  "ollama",
			APIBaseURL:       "http://localhost:11434",
			APIKeyCiphertext: "ciphertext",
			IsActive:         true,
		},
	}

	router := NewChannelRouter(db, stubCryptoService{}, nil)
	router.organizationIDRouteRepo = &fakeGatewayRouteRepo{
		routes: []*channelmodel.LLMRoute{privateRoute},
	}
	router.privateModels = &fakeGatewayPrivateModelLookup{
		models: map[string]*llmmodel.CustomModel{
			"qwen3.5:4b": {
				ID:              uuid.New(),
				OrganizationID:  orgID,
				ProviderID:      customProviderID,
				Provider:        "ollama",
				Name:            "qwen3.5:4b",
				DisplayName:     "qwen3.5:4b",
				UseCases:        llmmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
		},
	}

	selection, err := router.SelectChannel(context.Background(), orgID, "qwen3.5:4b")
	require.NoError(t, err)

	providerSelection, err := selection.ConvertToProviderSelection(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, customProviderID, providerSelection.Provider.ID)
	require.Equal(t, "ollama", providerSelection.Provider.Provider)
	require.Equal(t, "qwen3.5:4b", providerSelection.Model.Model)
	require.Equal(t, PricingModelSourceCustom, providerSelection.ModelSource)
}

func TestConvertToProviderSelection_PrivateCustomModelRequiresCustomProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	createCustomProviderTable(t, db)

	orgID := uuid.New()
	missingProviderID := uuid.New()
	privateRoute := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  orgID,
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434",
		Models:          []string{"qwen3.5:4b"},
		IsEnabled:       true,
		Priority:        100,
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  "ollama",
			APIBaseURL:       "http://localhost:11434",
			APIKeyCiphertext: "ciphertext",
			IsActive:         true,
		},
	}

	router := NewChannelRouter(db, stubCryptoService{}, nil)
	router.organizationIDRouteRepo = &fakeGatewayRouteRepo{
		routes: []*channelmodel.LLMRoute{privateRoute},
	}
	router.privateModels = &fakeGatewayPrivateModelLookup{
		models: map[string]*llmmodel.CustomModel{
			"qwen3.5:4b": {
				ID:              uuid.New(),
				OrganizationID:  orgID,
				ProviderID:      missingProviderID,
				Provider:        "ollama",
				Name:            "qwen3.5:4b",
				DisplayName:     "qwen3.5:4b",
				UseCases:        llmmodel.StringArray{"text-chat"},
				ChatCompletions: true,
				IsActive:        true,
			},
		},
	}

	selection, err := router.SelectChannel(context.Background(), orgID, "qwen3.5:4b")
	require.NoError(t, err)

	_, err = selection.ConvertToProviderSelection(context.Background(), db)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to load custom provider")
}

func TestConvertToProviderSelection_GlobalModelStillUsesGlobalProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	createGlobalProviderTable(t, db)

	globalProviderID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO llm_providers (id, provider, provider_name, is_active) VALUES (?, ?, ?, ?)`,
		globalProviderID.String(),
		"openai",
		"OpenAI",
		true,
	).Error)

	selection := &ChannelSelection{
		RouteID:         uuid.New(),
		ChannelProvider: "openai-compatible",
		ModelName:       "gpt-4o",
		Model: &llmmodel.LLMModel{
			ID:        uuid.New(),
			Model:     "gpt-4o",
			ModelName: "gpt-4o",
			Provider:  "openai",
			IsActive:  true,
		},
		ModelSource: channelModelSourceGlobal,
	}

	providerSelection, err := selection.ConvertToProviderSelection(context.Background(), db)
	require.NoError(t, err)
	require.Equal(t, globalProviderID, providerSelection.Provider.ID)
	require.Equal(t, "openai", providerSelection.Provider.Provider)
	require.Equal(t, PricingModelSourceGlobal, providerSelection.ModelSource)
}

func createCustomProviderTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_custom_providers (
			id TEXT PRIMARY KEY,
			organization_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			provider_name TEXT NOT NULL,
			api_base_url TEXT,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			deleted_at DATETIME
		)
	`).Error)
}

func createGlobalProviderTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`
		CREATE TABLE llm_providers (
			id TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			provider_name TEXT NOT NULL,
			api_base_url TEXT,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			deleted_at DATETIME
		)
	`).Error)
}
