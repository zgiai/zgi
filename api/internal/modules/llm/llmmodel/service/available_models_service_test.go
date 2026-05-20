package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	channelmodel "github.com/zgiai/ginext/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/ginext/internal/modules/llm/channel/repository"
	"github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	providermodel "github.com/zgiai/ginext/internal/modules/llm/provider/model"
)

type fakeTenantRouteRepo struct {
	routes []*channelmodel.LLMRoute
}

type fakeOfficialRouteBootstrapper struct {
	routeRepo *fakeTenantRouteRepo
	models    []string
	callCount int
	lastOrgID uuid.UUID
	returnErr error
}

var _ channelrepo.TenantRouteRepository = (*fakeTenantRouteRepo)(nil)

func (f *fakeTenantRouteRepo) Create(ctx context.Context, route *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *fakeTenantRouteRepo) BatchCreate(ctx context.Context, routes []*channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *fakeTenantRouteRepo) GetByID(ctx context.Context, organizationID, id uuid.UUID) (*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeTenantRouteRepo) List(ctx context.Context, organizationID uuid.UUID, isEnabled *bool, offset, limit int) ([]*channelmodel.LLMRoute, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *fakeTenantRouteRepo) Update(ctx context.Context, route *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *fakeTenantRouteRepo) Delete(ctx context.Context, organizationID, id uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *fakeTenantRouteRepo) GetEnabledRoutes(ctx context.Context, organizationID uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	var out []*channelmodel.LLMRoute
	for _, r := range f.routes {
		if r.OrganizationID == organizationID && r.IsEnabled {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeTenantRouteRepo) FindByModel(ctx context.Context, organizationID uuid.UUID, modelName string) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeTenantRouteRepo) CountByCredentialID(ctx context.Context, organizationID uuid.UUID, credentialID uuid.UUID) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f *fakeTenantRouteRepo) GetDistinctProviders(ctx context.Context, organizationID uuid.UUID) ([]string, error) {
	seen := make(map[string]struct{})
	var providers []string
	for _, r := range f.routes {
		if r.OrganizationID != organizationID {
			continue
		}
		if _, ok := seen[r.ChannelProvider]; ok {
			continue
		}
		seen[r.ChannelProvider] = struct{}{}
		providers = append(providers, r.ChannelProvider)
	}
	return providers, nil
}
func (f *fakeTenantRouteRepo) GetPlatformChannels(ctx context.Context) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeOfficialRouteBootstrapper) InitOfficialChannel(ctx context.Context, organizationID uuid.UUID) error {
	f.callCount++
	f.lastOrgID = organizationID
	if f.returnErr != nil {
		return f.returnErr
	}

	for _, route := range f.routeRepo.routes {
		if route.OrganizationID == organizationID && route.IsOfficial {
			return nil
		}
	}

	f.routeRepo.routes = append(f.routeRepo.routes, &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Name:            "ZGI Cloud",
		ChannelProvider: "zgi-cloud",
		Models:          append([]string(nil), f.models...),
		IsEnabled:       true,
		IsOfficial:      true,
	})
	return nil
}

func mustListModelNames(t *testing.T, svc AvailableModelsService, orgID uuid.UUID, provider, useCase string) []string {
	t.Helper()

	items, err := svc.ListAvailable(context.Background(), orgID, provider, useCase)
	require.NoError(t, err)

	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return names
}

func findByName(items []*AvailableModel, name string) []*AvailableModel {
	var out []*AvailableModel
	for _, it := range items {
		if it.Name == name {
			out = append(out, it)
		}
	}
	return out
}

func TestAvailableModels_PassthroughUnknownModelsAreNeverReturned(t *testing.T) {
	orgID := uuid.New()

	globalLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	// This name exists in route.models but is not in llm_models or llm_custom_models.
	unknownModelName := "unknown-llm-model-x"

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{
			{
				ID:              uuid.New(),
				OrganizationID:  orgID,
				Name:            "route-1",
				ChannelProvider: "openai",
				Models:          []string{globalLLM.Model, unknownModelName},
				IsEnabled:       true,
			},
		},
	}

	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: []*model.LLMModel{globalLLM}},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: nil},
		routeRepo,
	)

	names := mustListModelNames(t, svc, orgID, "", "")
	assert.Contains(t, names, globalLLM.Model)
	assert.NotContains(t, names, unknownModelName)
}

func TestAvailableModels_HidesModelsWhenProviderDisabled(t *testing.T) {
	orgID := uuid.New()
	providerID := uuid.New()

	globalLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{{
			ID:              uuid.New(),
			OrganizationID:  orgID,
			Name:            "route-1",
			ChannelProvider: "deepseek",
			Models:          []string{"gpt-4o"},
			IsEnabled:       true,
		}},
	}

	svc := NewAvailableModelsServiceWithProviderRepos(
		&fakeGlobalRepo{models: []*model.LLMModel{globalLLM}},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: nil},
		routeRepo,
		&fakeProviderRepo{
			providers: []*providermodel.LLMProvider{{
				ID:       providerID,
				Provider: "openai",
				IsActive: true,
			}},
		},
		&fakeProviderConfigRepo{
			configs: []*providermodel.ProviderConfig{{
				OrganizationID: orgID,
				ProviderID:     providerID,
				IsEnabled:      false,
			}},
		},
		&fakeCustomProviderRepo{},
	)

	items, err := svc.ListAvailable(context.Background(), orgID, "", "")
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestAvailableModels_BootstrapsOfficialRouteWhenMissing(t *testing.T) {
	orgID := uuid.New()

	globalLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	routeRepo := &fakeTenantRouteRepo{}
	bootstrapper := &fakeOfficialRouteBootstrapper{
		routeRepo: routeRepo,
		models:    []string{globalLLM.Model},
	}

	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: []*model.LLMModel{globalLLM}},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: nil},
		routeRepo,
	)
	svc.SetOfficialRouteBootstrapper(bootstrapper)

	names := mustListModelNames(t, svc, orgID, "", "")
	assert.Contains(t, names, globalLLM.Model)
	assert.Equal(t, 1, bootstrapper.callCount)
	assert.Equal(t, orgID, bootstrapper.lastOrgID)
}

func TestAvailableModels_DoesNotBootstrapWhenOfficialRouteAlreadyExists(t *testing.T) {
	orgID := uuid.New()

	globalLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{{
			ID:              uuid.New(),
			OrganizationID:  orgID,
			Name:            "ZGI Cloud",
			ChannelProvider: "zgi-cloud",
			Models:          []string{globalLLM.Model},
			IsEnabled:       true,
			IsOfficial:      true,
		}},
	}
	bootstrapper := &fakeOfficialRouteBootstrapper{
		routeRepo: routeRepo,
		models:    []string{globalLLM.Model},
	}

	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: []*model.LLMModel{globalLLM}},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: nil},
		routeRepo,
	)
	svc.SetOfficialRouteBootstrapper(bootstrapper)

	names := mustListModelNames(t, svc, orgID, "", "")
	assert.Contains(t, names, globalLLM.Model)
	assert.Zero(t, bootstrapper.callCount)
}

func TestAvailableModels_CustomProviderIsPreservedAndFilterable(t *testing.T) {
	orgID := uuid.New()

	custom := &model.CustomModel{
		ID:             uuid.New(),
		OrganizationID: orgID,
		ProviderID:     uuid.New(),
		Provider:       "my-openai",
		Name:           "Doubao-Seed-1.6",
		DisplayName:    "Doubao Seed 1.6",
		UseCases:       model.StringArray{"text-chat"},
		IsActive:       true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{
			{
				ID:              uuid.New(),
				OrganizationID:  orgID,
				Name:            "route-1",
				ChannelProvider: "openai",
				Models:          []string{custom.Name},
				IsEnabled:       true,
			},
		},
	}

	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: nil},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: []*model.CustomModel{custom}},
		routeRepo,
	)

	items, err := svc.ListAvailable(context.Background(), orgID, custom.Provider, "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, custom.Name, items[0].Name)
	assert.Equal(t, custom.Provider, items[0].Provider)
}

func TestAvailableModels_CustomModelIsNotReturnedAsPassthroughDuplicate(t *testing.T) {
	orgID := uuid.New()

	custom := &model.CustomModel{
		ID:             uuid.New(),
		OrganizationID: orgID,
		ProviderID:     uuid.New(),
		Provider:       "my-openai",
		Name:           "my-custom-chat-model",
		DisplayName:    "My Custom Chat Model",
		UseCases:       model.StringArray{"text-chat"},
		IsActive:       true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{
			{
				ID:              uuid.New(),
				OrganizationID:  orgID,
				Name:            "route-1",
				ChannelProvider: "openai",
				Models:          []string{custom.Name},
				IsEnabled:       true,
			},
		},
	}

	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: nil},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: []*model.CustomModel{custom}},
		routeRepo,
	)

	items, err := svc.ListAvailable(context.Background(), orgID, "", "")
	require.NoError(t, err)

	dupes := findByName(items, custom.Name)
	require.Len(t, dupes, 1)
	assert.False(t, dupes[0].IsCustom, "custom model must not be surfaced as passthrough")
	assert.Equal(t, custom.Provider, dupes[0].Provider)
	assert.Equal(t, []string(custom.UseCases), dupes[0].UseCases)
}

func TestAvailableModels_CustomModelsMustBeBackedByEnabledRoutes(t *testing.T) {
	orgID := uuid.New()

	customRouted := &model.CustomModel{
		ID:             uuid.New(),
		OrganizationID: orgID,
		ProviderID:     uuid.New(),
		Provider:       "my-openai",
		Name:           "custom-routed",
		DisplayName:    "Custom Routed",
		UseCases:       model.StringArray{"text-chat"},
		IsActive:       true,
	}
	customUnrouted := &model.CustomModel{
		ID:             uuid.New(),
		OrganizationID: orgID,
		ProviderID:     uuid.New(),
		Provider:       "my-openai",
		Name:           "custom-unrouted",
		DisplayName:    "Custom Unrouted",
		UseCases:       model.StringArray{"text-chat"},
		IsActive:       true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{
			{
				ID:              uuid.New(),
				OrganizationID:  orgID,
				Name:            "route-1",
				ChannelProvider: "openai",
				Models:          []string{customRouted.Name},
				IsEnabled:       true,
			},
		},
	}

	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: nil},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: []*model.CustomModel{customRouted, customUnrouted}},
		routeRepo,
	)

	names := mustListModelNames(t, svc, orgID, "", "")
	assert.Contains(t, names, customRouted.Name)
	assert.NotContains(t, names, customUnrouted.Name)
}

func TestAvailableModels_DefaultNoUseCaseReturnsAllRegisteredModels(t *testing.T) {
	orgID := uuid.New()

	globalLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}
	globalEmbedding := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "text-embedding-3-small",
		ModelName: "Text Embedding 3 Small",
		UseCases:  model.StringArray{"embedding"},
		IsActive:  true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{
			{
				ID:              uuid.New(),
				OrganizationID:  orgID,
				Name:            "route-1",
				ChannelProvider: "openai",
				Models:          []string{globalLLM.Model, globalEmbedding.Model},
				IsEnabled:       true,
			},
		},
	}

	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: []*model.LLMModel{globalLLM, globalEmbedding}},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: nil},
		routeRepo,
	)

	names := mustListModelNames(t, svc, orgID, "", "")
	assert.Contains(t, names, globalLLM.Model)
	assert.Contains(t, names, globalEmbedding.Model)
}

func TestAvailableModels_NoEnabledRoutesReturnsEmpty(t *testing.T) {
	orgID := uuid.New()

	globalLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{
			{
				ID:              uuid.New(),
				OrganizationID:  orgID,
				Name:            "route-1",
				ChannelProvider: "openai",
				Models:          []string{globalLLM.Model},
				IsEnabled:       false, // Disabled -> not available
			},
		},
	}

	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: []*model.LLMModel{globalLLM}},
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: nil},
		routeRepo,
	)

	items, err := svc.ListAvailable(context.Background(), orgID, "", "")
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestAvailableModels_LoadsOnlyRouteNamedGlobalModels(t *testing.T) {
	orgID := uuid.New()

	globalLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}
	unroutedLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4.1",
		ModelName: "GPT-4.1",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{{
			ID:              uuid.New(),
			OrganizationID:  orgID,
			Name:            "route-1",
			ChannelProvider: "openai",
			Models:          []string{globalLLM.Model},
			IsEnabled:       true,
		}},
	}
	globalRepo := &fakeGlobalRepo{models: []*model.LLMModel{globalLLM, unroutedLLM}}
	svc := NewAvailableModelsService(
		globalRepo,
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: nil},
		routeRepo,
	)

	names := mustListModelNames(t, svc, orgID, "openai", "text-chat")
	require.Equal(t, []string{globalLLM.Model}, names)
	assert.Equal(t, 1, globalRepo.listAvailableByNamesCalls)
	assert.Zero(t, globalRepo.listAvailableFilteredCalls)
	assert.Zero(t, globalRepo.listCalls)
	assert.Equal(t, []string{globalLLM.Model}, globalRepo.listAvailableNames)
	assert.Equal(t, "openai", globalRepo.listAvailableProvider)
	assert.Equal(t, "text-chat", globalRepo.listAvailableUseCase)
}

func TestAvailableModels_LoadsOnlyAvailableModelConfigs(t *testing.T) {
	orgID := uuid.New()

	globalLLM := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{{
			ID:              uuid.New(),
			OrganizationID:  orgID,
			Name:            "route-1",
			ChannelProvider: "openai",
			Models:          []string{globalLLM.Model},
			IsEnabled:       true,
		}},
	}
	configRepo := &fakeModelConfigRepo{
		configs: []*model.ModelConfig{{
			OrganizationID:    orgID,
			ModelID:           globalLLM.ID,
			IsEnabled:         true,
			CustomDisplayName: "Custom GPT-4o",
		}},
	}
	svc := NewAvailableModelsService(
		&fakeGlobalRepo{models: []*model.LLMModel{globalLLM}},
		configRepo,
		&fakeCustomModelRepo{models: nil},
		routeRepo,
	)

	items, err := svc.ListAvailable(context.Background(), orgID, "", "text-chat")
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "Custom GPT-4o", items[0].DisplayName)
	assert.Equal(t, 1, configRepo.listAvailableConfigsCalls)
	assert.Zero(t, configRepo.listCalls)
}

func TestAvailableModels_WildcardRoutePushesFiltersToGlobalList(t *testing.T) {
	orgID := uuid.New()

	chatModel := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "gpt-4o",
		ModelName: "GPT-4o",
		UseCases:  model.StringArray{"text-chat"},
		IsActive:  true,
	}
	embeddingModel := &model.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     "text-embedding-3-small",
		ModelName: "Text Embedding 3 Small",
		UseCases:  model.StringArray{"embedding"},
		IsActive:  true,
	}

	routeRepo := &fakeTenantRouteRepo{
		routes: []*channelmodel.LLMRoute{{
			ID:              uuid.New(),
			OrganizationID:  orgID,
			Name:            "route-1",
			ChannelProvider: "openai",
			Models:          []string{"*"},
			IsEnabled:       true,
		}},
	}
	globalRepo := &fakeGlobalRepo{models: []*model.LLMModel{chatModel, embeddingModel}}
	svc := NewAvailableModelsService(
		globalRepo,
		&fakeModelConfigRepo{configs: nil},
		&fakeCustomModelRepo{models: nil},
		routeRepo,
	)

	names := mustListModelNames(t, svc, orgID, "openai", "text-chat")
	require.Equal(t, []string{chatModel.Model}, names)
	assert.Zero(t, globalRepo.listAvailableByNamesCalls)
	assert.Equal(t, 1, globalRepo.listAvailableFilteredCalls)
	assert.Zero(t, globalRepo.listCalls)
	assert.Equal(t, "openai", globalRepo.listAvailableProvider)
	assert.Equal(t, "text-chat", globalRepo.listAvailableUseCase)
}
