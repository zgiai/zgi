package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	appconfig "github.com/zgiai/zgi/api/config"
	channeldto "github.com/zgiai/zgi/api/internal/modules/llm/channel/dto"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/channelprovider"
	credentialdto "github.com/zgiai/zgi/api/internal/modules/llm/credential/dto"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	credentialsvc "github.com/zgiai/zgi/api/internal/modules/llm/credential/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelrepo "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/repository"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	providerrepo "github.com/zgiai/zgi/api/internal/modules/llm/provider/repository"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

var (
	_ channelrepo.TenantRouteRepository     = (*fakeTenantRouteRepo)(nil)
	_ credentialsvc.TenantCredentialService = (*fakeTenantCredentialService)(nil)
	_ llmmodelrepo.ModelRepository          = (*fakeModelRepo)(nil)
	_ llmmodelrepo.ModelConfigRepository    = (*fakeModelConfigRepo)(nil)
	_ providerrepo.CustomProviderRepository = (*fakeCustomProviderRepo)(nil)
	_ llmmodelrepo.CustomModelRepository    = (*fakeCustomModelRepo)(nil)
	_ ChannelValidator                      = (*fakeChannelValidator)(nil)
	_ llmmodelsvc.AvailableModelsService    = (*fakeAvailableModelsService)(nil)
)

func setLLMAllowPrivateBaseURL(t *testing.T, allow bool) {
	t.Helper()
	previous := appconfig.GlobalConfig
	next := &appconfig.Config{}
	if previous != nil {
		copied := *previous
		next = &copied
	}
	next.LLM.AllowPrivateBaseURL = allow
	appconfig.GlobalConfig = next
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})
}

type fakeTenantRouteRepo struct {
	created   *channelmodel.LLMRoute
	updated   *channelmodel.LLMRoute
	routeByID *channelmodel.LLMRoute
	getErr    error
}

func (f *fakeTenantRouteRepo) Create(_ context.Context, route *channelmodel.LLMRoute) error {
	if route.ID == uuid.Nil {
		route.ID = uuid.New()
	}
	f.created = route
	return nil
}

func (f *fakeTenantRouteRepo) BatchCreate(context.Context, []*channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}

func (f *fakeTenantRouteRepo) GetByID(_ context.Context, _, _ uuid.UUID) (*channelmodel.LLMRoute, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.routeByID == nil {
		return nil, errors.New("not implemented")
	}
	return f.routeByID, nil
}

func (f *fakeTenantRouteRepo) List(context.Context, uuid.UUID, *bool, int, int) ([]*channelmodel.LLMRoute, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeTenantRouteRepo) Update(_ context.Context, route *channelmodel.LLMRoute) error {
	f.updated = routeClone(route)
	return nil
}

func (f *fakeTenantRouteRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeTenantRouteRepo) GetEnabledRoutes(context.Context, uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTenantRouteRepo) FindByModel(context.Context, uuid.UUID, string) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTenantRouteRepo) CountByCredentialID(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *fakeTenantRouteRepo) GetDistinctProviders(context.Context, uuid.UUID) ([]string, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTenantRouteRepo) GetPlatformChannels(context.Context) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

func routeClone(route *channelmodel.LLMRoute) *channelmodel.LLMRoute {
	if route == nil {
		return nil
	}
	cloned := *route
	if route.Models != nil {
		cloned.Models = append([]string(nil), route.Models...)
	}
	if route.Tags != nil {
		cloned.Tags = append([]string(nil), route.Tags...)
	}
	if route.ModelMaps != nil {
		cloned.ModelMaps = mapsClone(route.ModelMaps)
	}
	if route.ParamOverride != nil {
		cloned.ParamOverride = mapsClone(route.ParamOverride)
	}
	if route.HeaderOverride != nil {
		cloned.HeaderOverride = mapsClone(route.HeaderOverride)
	}
	if route.ValidationReport != nil {
		cloned.ValidationReport = mapsClone(route.ValidationReport)
	}
	return &cloned
}

func mapsClone(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

type fakeTenantCredentialService struct {
	createdReq *credentialdto.CreateTenantCredentialRequest
	updatedReq *credentialdto.UpdateTenantCredentialRequest
	cred       *credentialmodel.TenantCredential
}

func (f *fakeTenantCredentialService) Create(context.Context, uuid.UUID, *credentialdto.CreateTenantCredentialRequest) (*credentialmodel.TenantCredential, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeTenantCredentialService) GetOrCreateByAPIKey(_ context.Context, _ uuid.UUID, req *credentialdto.CreateTenantCredentialRequest) (*credentialmodel.TenantCredential, bool, error) {
	f.createdReq = req
	if f.cred == nil {
		f.cred = &credentialmodel.TenantCredential{
			ID:              uuid.New(),
			ChannelProvider: req.ChannelProvider,
			APIBaseURL:      req.APIBaseURL,
		}
	}
	return f.cred, true, nil
}

func (f *fakeTenantCredentialService) GetByID(context.Context, uuid.UUID, uuid.UUID) (*credentialmodel.TenantCredential, error) {
	if f.cred == nil {
		return nil, errors.New("not found")
	}
	return f.cred, nil
}

func (f *fakeTenantCredentialService) List(context.Context, uuid.UUID, *credentialdto.ListCredentialRequest) ([]*credentialmodel.TenantCredential, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeTenantCredentialService) Update(_ context.Context, _ uuid.UUID, _ uuid.UUID, req *credentialdto.UpdateTenantCredentialRequest) (*credentialmodel.TenantCredential, error) {
	f.updatedReq = req
	if f.cred == nil {
		f.cred = &credentialmodel.TenantCredential{ID: uuid.New()}
	}
	if req.ChannelProvider != nil {
		f.cred.ChannelProvider = *req.ChannelProvider
	}
	if req.APIBaseURL != nil {
		f.cred.APIBaseURL = *req.APIBaseURL
	}
	return f.cred, nil
}

func (f *fakeTenantCredentialService) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeTenantCredentialService) GetDecryptedAPIKey(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	if f.cred == nil {
		return "", errors.New("not found")
	}
	return "test-api-key", nil
}

func (f *fakeTenantCredentialService) TestCredential(context.Context, uuid.UUID, uuid.UUID, string, string) (*credentialdto.TestCredentialResult, error) {
	return nil, errors.New("not implemented")
}

type fakeChannelValidator struct {
	lastOrganizationID uuid.UUID
	lastProvider       string
	lastAPIKey         string
	lastTestProvider   string
	lastTestAPIKey     string
	lastTestBaseURL    string
	lastTestModel      string
	lastTestMethod     string
	createCalls        int
	validateCalls      int
	report             map[string]interface{}
	err                error
	testResult         *channelprovider.TestResult
	testErr            error
}

func (f *fakeChannelValidator) ValidateModels(_ context.Context, organizationID uuid.UUID, channelProvider, apiKey, _ string, models []string) (*channelprovider.ValidationResult, error) {
	f.validateCalls++
	f.lastOrganizationID = organizationID
	f.lastProvider = channelProvider
	f.lastAPIKey = apiKey
	if f.err != nil {
		if f.report != nil {
			return &channelprovider.ValidationResult{
				Report:           f.report,
				NormalizedModels: append([]string(nil), models...),
			}, f.err
		}
		return nil, f.err
	}
	if f.report != nil {
		return &channelprovider.ValidationResult{
			Report:           f.report,
			NormalizedModels: append([]string(nil), models...),
		}, nil
	}
	return &channelprovider.ValidationResult{
		Report: map[string]interface{}{
			"provider": channelProvider,
			"models":   models,
		},
		NormalizedModels: append([]string(nil), models...),
	}, nil
}

func (f *fakeChannelValidator) ValidateModelsForCreation(_ context.Context, organizationID uuid.UUID, channelProvider, apiKey, _ string, models []string) (*channelprovider.ValidationResult, error) {
	f.createCalls++
	f.lastOrganizationID = organizationID
	f.lastProvider = channelProvider
	f.lastAPIKey = apiKey
	if f.err != nil {
		if f.report != nil {
			return &channelprovider.ValidationResult{
				Report:           f.report,
				NormalizedModels: append([]string(nil), models...),
			}, f.err
		}
		return nil, f.err
	}
	if f.report != nil {
		return &channelprovider.ValidationResult{
			Report:           f.report,
			NormalizedModels: append([]string(nil), models...),
		}, nil
	}
	return &channelprovider.ValidationResult{
		Report: map[string]interface{}{
			"provider": channelProvider,
			"models":   models,
		},
		NormalizedModels: append([]string(nil), models...),
	}, nil
}

func (f *fakeChannelValidator) TestModel(_ context.Context, organizationID uuid.UUID, channelProvider, apiKey string, apiBaseURL, modelName, testMethod string) (*channelprovider.TestResult, error) {
	f.lastOrganizationID = organizationID
	f.lastTestProvider = channelProvider
	f.lastTestAPIKey = apiKey
	f.lastTestBaseURL = apiBaseURL
	f.lastTestModel = modelName
	f.lastTestMethod = testMethod
	if f.testErr != nil {
		return nil, f.testErr
	}
	if f.testResult != nil {
		return f.testResult, nil
	}
	return &channelprovider.TestResult{
		Success:        true,
		Message:        "ok",
		ResponseTimeMs: 12,
		Model:          modelName,
		UseCase:        "chat",
	}, nil
}

type fakeModelRepo struct {
	models []*llmmodelmodel.LLMModel
}

func (f *fakeModelRepo) Create(context.Context, *llmmodelmodel.LLMModel) error {
	return errors.New("not implemented")
}
func (f *fakeModelRepo) GetByID(context.Context, uuid.UUID) (*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeModelRepo) GetByName(_ context.Context, name string) (*llmmodelmodel.LLMModel, error) {
	for _, model := range f.models {
		if model != nil && model.Model == name {
			return model, nil
		}
	}
	return nil, nil
}
func (f *fakeModelRepo) ListByNames(_ context.Context, names []string) ([]*llmmodelmodel.LLMModel, error) {
	result := make([]*llmmodelmodel.LLMModel, 0, len(names))
	for _, name := range names {
		for _, model := range f.models {
			if model != nil && model.Model == name {
				result = append(result, model)
				break
			}
		}
	}
	return result, nil
}
func (f *fakeModelRepo) ListAvailableByNames(_ context.Context, names []string, provider string, useCase string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeModelRepo) ListAvailableFiltered(_ context.Context, provider string, useCase string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeModelRepo) GetByProviderAndName(context.Context, string, string) (*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeModelRepo) List(context.Context, *uuid.UUID, string, string, *bool, int, int) ([]*llmmodelmodel.LLMModel, int64, error) {
	return append([]*llmmodelmodel.LLMModel(nil), f.models...), int64(len(f.models)), nil
}
func (f *fakeModelRepo) Update(context.Context, *llmmodelmodel.LLMModel) error {
	return errors.New("not implemented")
}
func (f *fakeModelRepo) Delete(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *fakeModelRepo) ListByProvider(context.Context, string) ([]*llmmodelmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}

type fakeModelConfigRepo struct {
	configs map[uuid.UUID]*llmmodelmodel.ModelConfig
	upserts []*llmmodelmodel.ModelConfig
}

func (f *fakeModelConfigRepo) Create(context.Context, *llmmodelmodel.ModelConfig) error {
	return errors.New("not implemented")
}
func (f *fakeModelConfigRepo) GetByID(context.Context, uuid.UUID, uuid.UUID) (*llmmodelmodel.ModelConfig, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeModelConfigRepo) GetByModelID(_ context.Context, organizationID, modelID uuid.UUID) (*llmmodelmodel.ModelConfig, error) {
	if f.configs == nil {
		return nil, gorm.ErrRecordNotFound
	}
	cfg, ok := f.configs[modelID]
	if !ok || cfg == nil || cfg.OrganizationID != organizationID {
		return nil, gorm.ErrRecordNotFound
	}
	return cfg, nil
}
func (f *fakeModelConfigRepo) List(context.Context, uuid.UUID, *bool, int, int) ([]*llmmodelmodel.ModelConfig, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *fakeModelConfigRepo) ListAvailableConfigs(context.Context, uuid.UUID) ([]*llmmodelmodel.ModelConfig, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeModelConfigRepo) Update(context.Context, *llmmodelmodel.ModelConfig) error {
	return errors.New("not implemented")
}
func (f *fakeModelConfigRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *fakeModelConfigRepo) Upsert(_ context.Context, config *llmmodelmodel.ModelConfig) error {
	cloned := *config
	f.upserts = append(f.upserts, &cloned)
	if f.configs == nil {
		f.configs = make(map[uuid.UUID]*llmmodelmodel.ModelConfig)
	}
	f.configs[config.ModelID] = &cloned
	return nil
}
func (f *fakeModelConfigRepo) BatchCreate(context.Context, []*llmmodelmodel.ModelConfig) error {
	return errors.New("not implemented")
}

type fakeCustomProviderRepo struct {
	created  []*providermodel.CustomProvider
	updated  []*providermodel.CustomProvider
	bySlug   map[string]*providermodel.CustomProvider
	provider *providermodel.CustomProvider
}

func (f *fakeCustomProviderRepo) Create(_ context.Context, provider *providermodel.CustomProvider) error {
	if provider.ID == uuid.Nil {
		provider.ID = uuid.New()
	}
	f.created = append(f.created, provider)
	if f.bySlug == nil {
		f.bySlug = make(map[string]*providermodel.CustomProvider)
	}
	f.bySlug[provider.Provider] = provider
	f.provider = provider
	return nil
}

func (f *fakeCustomProviderRepo) GetByID(context.Context, uuid.UUID, uuid.UUID) (*providermodel.CustomProvider, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeCustomProviderRepo) GetByProvider(_ context.Context, _ uuid.UUID, provider string) (*providermodel.CustomProvider, error) {
	if f.bySlug != nil {
		if found, ok := f.bySlug[provider]; ok {
			return found, nil
		}
	}
	if f.provider != nil && f.provider.Provider == provider {
		return f.provider, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeCustomProviderRepo) List(context.Context, uuid.UUID, *bool, int, int) ([]*providermodel.CustomProvider, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeCustomProviderRepo) Update(_ context.Context, provider *providermodel.CustomProvider) error {
	f.updated = append(f.updated, provider)
	if f.bySlug == nil {
		f.bySlug = make(map[string]*providermodel.CustomProvider)
	}
	f.bySlug[provider.Provider] = provider
	return nil
}

func (f *fakeCustomProviderRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeCustomProviderRepo) ExistsByProvider(context.Context, uuid.UUID, string) (bool, error) {
	return false, errors.New("not implemented")
}

type fakeCustomModelRepo struct {
	created []*llmmodelmodel.CustomModel
	updated []*llmmodelmodel.CustomModel
	byName  map[string]*llmmodelmodel.CustomModel
}

func (f *fakeCustomModelRepo) Create(_ context.Context, m *llmmodelmodel.CustomModel) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	f.created = append(f.created, m)
	if f.byName == nil {
		f.byName = make(map[string]*llmmodelmodel.CustomModel)
	}
	f.byName[m.Name] = m
	return nil
}

func (f *fakeCustomModelRepo) GetByID(context.Context, uuid.UUID, uuid.UUID) (*llmmodelmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeCustomModelRepo) GetByProviderAndName(_ context.Context, _ uuid.UUID, _ uuid.UUID, name string) (*llmmodelmodel.CustomModel, error) {
	if f.byName != nil {
		if found, ok := f.byName[name]; ok {
			return found, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeCustomModelRepo) GetByProviderAndModel(_ context.Context, _ uuid.UUID, _ string, name string) (*llmmodelmodel.CustomModel, error) {
	if f.byName != nil {
		if found, ok := f.byName[name]; ok {
			return found, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeCustomModelRepo) ListByNames(_ context.Context, _ uuid.UUID, names []string, _ *bool) ([]*llmmodelmodel.CustomModel, error) {
	result := make([]*llmmodelmodel.CustomModel, 0, len(names))
	for _, name := range names {
		if f.byName != nil {
			if found, ok := f.byName[name]; ok {
				result = append(result, found)
			}
		}
	}
	return result, nil
}

func (f *fakeCustomModelRepo) List(context.Context, uuid.UUID, *uuid.UUID, string, string, *bool, int, int) ([]*llmmodelmodel.CustomModel, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeCustomModelRepo) Update(_ context.Context, m *llmmodelmodel.CustomModel) error {
	f.updated = append(f.updated, m)
	if f.byName == nil {
		f.byName = make(map[string]*llmmodelmodel.CustomModel)
	}
	f.byName[m.Name] = m
	return nil
}

func (f *fakeCustomModelRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}

func (f *fakeCustomModelRepo) ListByProvider(context.Context, uuid.UUID, uuid.UUID) ([]*llmmodelmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}

type fakeAvailableModelsService struct {
	invalidated []uuid.UUID
}

func (f *fakeAvailableModelsService) ListAvailable(context.Context, uuid.UUID, string, string) ([]*llmmodelsvc.AvailableModel, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAvailableModelsService) RefreshCache(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *fakeAvailableModelsService) InvalidateTenantCache(organizationID uuid.UUID) {
	f.invalidated = append(f.invalidated, organizationID)
}
func (f *fakeAvailableModelsService) InvalidateGlobalCache() {}
func (f *fakeAvailableModelsService) SetOfficialRouteBootstrapper(interfaces.OfficialRouteBootstrapper) {
}

func TestCreateRoute_UsesExplicitChannelProvider(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	orgID := uuid.New()
	view, err := svc.CreateRoute(context.Background(), orgID, &channeldto.CreateRouteRequest{
		Name:            "Cohere Proxy",
		ChannelProvider: "cohere",
		APIKey:          "test-key",
		APIBaseURL:      "https://api.cohere.com",
		Models:          []string{"command-r"},
	})

	require.NoError(t, err)
	require.Equal(t, "cohere", validator.lastProvider)
	require.Equal(t, 1, validator.createCalls)
	require.Equal(t, 0, validator.validateCalls)
	require.NotNil(t, credSvc.createdReq)
	require.Equal(t, "cohere", credSvc.createdReq.ChannelProvider)
	require.NotNil(t, repo.created)
	require.Equal(t, "cohere", repo.created.ChannelProvider)
	require.Equal(t, "cohere", view.ChannelProvider)
}

func TestCreateRoute_RejectsValidationFailureWithoutPersistence(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{
		report: map[string]interface{}{"provider": "openai"},
		err:    errors.New("model gpt-4o validation failed: unauthorized"),
	}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Broken OpenAI",
		ChannelProvider: "openai",
		APIKey:          "bad-key",
		Models:          []string{"gpt-4o"},
	})

	require.Error(t, err)
	require.Nil(t, repo.created)
	require.Nil(t, credSvc.createdReq)
}

func TestCreateRoute_RejectsUnsupportedNativeProtocol(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Mistral Route",
		ChannelProvider: "mistral",
		APIKey:          "test-key",
		Models:          []string{"mistral-large"},
		NativeProtocols: channelmodel.NativeProtocolConfig{
			OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "native_protocols.openai_responses")
	require.Equal(t, 0, validator.createCalls)
	require.Nil(t, repo.created)
	require.Nil(t, credSvc.createdReq)
}

func TestCreateRoute_ReturnsValidationWarningsInView(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{
		report: map[string]interface{}{
			"provider":         "openai",
			"warning_messages": []string{"sampled validation warning"},
		},
	}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	view, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Sampled OpenAI",
		ChannelProvider: "openai",
		APIKey:          "test-key",
		Models:          []string{"gpt-4o", "gpt-4.1"},
	})

	require.NoError(t, err)
	require.Equal(t, []string{"sampled validation warning"}, view.Warnings)
}

func TestCreateRoute_AutoEnablesOnlyModelsWithoutExistingConfig(t *testing.T) {
	modelID := uuid.New()
	existingModelID := uuid.New()
	orgID := uuid.New()

	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	configRepo := &fakeModelConfigRepo{
		configs: map[uuid.UUID]*llmmodelmodel.ModelConfig{
			existingModelID: {
				OrganizationID: orgID,
				ModelID:        existingModelID,
				IsEnabled:      false,
			},
		},
	}
	availableSvc := &fakeAvailableModelsService{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo: &fakeModelRepo{
			models: []*llmmodelmodel.LLMModel{
				{ID: modelID, Model: "gpt-4o"},
				{ID: existingModelID, Model: "deepseek-chat"},
			},
		},
		modelConfigRepo: configRepo,
		availableModels: availableSvc,
	}

	_, err := svc.CreateRoute(context.Background(), orgID, &channeldto.CreateRouteRequest{
		Name:            "OpenAI Route",
		ChannelProvider: "openai",
		APIKey:          "test-key",
		Models:          []string{"gpt-4o", "deepseek-chat", "gpt-4o", "*"},
	})

	require.NoError(t, err)
	require.Len(t, configRepo.upserts, 1)
	require.Equal(t, modelID, configRepo.upserts[0].ModelID)
	require.True(t, configRepo.upserts[0].IsEnabled)
	require.Equal(t, llmmodelmodel.AccessScopeAll, configRepo.upserts[0].AccessScope)
	require.Equal(t, []uuid.UUID{orgID}, availableSvc.invalidated)
}

func TestUpdateRoute_UsesCreationValidationLogic(t *testing.T) {
	credID := uuid.New()
	repo := &fakeTenantRouteRepo{
		routeByID: &channelmodel.LLMRoute{
			ID:              uuid.New(),
			OrganizationID:  uuid.New(),
			Type:            "PRIVATE",
			CredentialID:    &credID,
			Name:            "Existing Route",
			ChannelProvider: "qwen",
			Models:          []string{"qwen2.5-14b-instruct"},
			IsEnabled:       true,
		},
	}
	credSvc := &fakeTenantCredentialService{
		cred: &credentialmodel.TenantCredential{ID: credID},
	}
	validator := &fakeChannelValidator{
		report: map[string]interface{}{
			"provider":         "qwen",
			"warning_messages": []string{"representative models failed validation: qwen-image-2.0 [image-gen] (unauthorized)"},
			"failed_models": []map[string]interface{}{
				{"model": "qwen-image-2.0", "use_case": "image-gen", "message": "unauthorized"},
			},
		},
	}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	view, err := svc.UpdateRoute(context.Background(), repo.routeByID.OrganizationID, repo.routeByID.ID, &channeldto.UpdateRouteRequest{
		Models: []string{"qwen2.5-14b-instruct", "qwen-image-2.0"},
	})

	require.NoError(t, err)
	require.Equal(t, 1, validator.createCalls)
	require.Equal(t, 0, validator.validateCalls)
	require.NotNil(t, repo.updated)
	require.Equal(t, []string{"qwen2.5-14b-instruct", "qwen-image-2.0"}, repo.updated.Models)
	require.Equal(t, []string{"representative models failed validation: qwen-image-2.0 [image-gen] (unauthorized)"}, view.Warnings)
	require.Contains(t, view.ValidationReport, "failed_models")
}

func TestUpdateRoute_RejectsUnsupportedNativeProtocol(t *testing.T) {
	credID := uuid.New()
	orgID := uuid.New()
	routeID := uuid.New()
	repo := &fakeTenantRouteRepo{
		routeByID: &channelmodel.LLMRoute{
			ID:              routeID,
			OrganizationID:  orgID,
			Type:            "PRIVATE",
			CredentialID:    &credID,
			Name:            "Existing Route",
			ChannelProvider: "mistral",
			Models:          []string{"mistral-large"},
			IsEnabled:       true,
		},
	}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: &fakeTenantCredentialService{cred: &credentialmodel.TenantCredential{ID: credID}},
		validator:         &fakeChannelValidator{},
		modelRepo:         &fakeModelRepo{},
	}

	_, err := svc.UpdateRoute(context.Background(), orgID, routeID, &channeldto.UpdateRouteRequest{
		NativeProtocols: &channelmodel.NativeProtocolConfig{
			AnthropicMessages: channelmodel.NativeProtocolEndpoint{Enabled: true},
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "native_protocols.anthropic_messages")
	require.Nil(t, repo.updated)
}

func TestUpdateRoute_AutoEnablesNewModelsWithoutOverwritingExistingConfig(t *testing.T) {
	credID := uuid.New()
	orgID := uuid.New()
	modelID := uuid.New()
	existingModelID := uuid.New()

	repo := &fakeTenantRouteRepo{
		routeByID: &channelmodel.LLMRoute{
			ID:              uuid.New(),
			OrganizationID:  orgID,
			Type:            "PRIVATE",
			CredentialID:    &credID,
			Name:            "Existing Route",
			ChannelProvider: "openai",
			Models:          []string{"gpt-4o"},
			IsEnabled:       true,
		},
	}
	credSvc := &fakeTenantCredentialService{
		cred: &credentialmodel.TenantCredential{ID: credID},
	}
	configRepo := &fakeModelConfigRepo{
		configs: map[uuid.UUID]*llmmodelmodel.ModelConfig{
			existingModelID: {
				OrganizationID: orgID,
				ModelID:        existingModelID,
				IsEnabled:      false,
			},
		},
	}
	availableSvc := &fakeAvailableModelsService{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         &fakeChannelValidator{},
		modelRepo: &fakeModelRepo{
			models: []*llmmodelmodel.LLMModel{
				{ID: modelID, Model: "gpt-4.1"},
				{ID: existingModelID, Model: "deepseek-chat"},
			},
		},
		modelConfigRepo: configRepo,
		availableModels: availableSvc,
	}

	_, err := svc.UpdateRoute(context.Background(), orgID, repo.routeByID.ID, &channeldto.UpdateRouteRequest{
		Models: []string{"gpt-4.1", "deepseek-chat"},
	})

	require.NoError(t, err)
	require.Len(t, configRepo.upserts, 1)
	require.Equal(t, modelID, configRepo.upserts[0].ModelID)
	require.True(t, configRepo.upserts[0].IsEnabled)
	require.Equal(t, []uuid.UUID{orgID}, availableSvc.invalidated)
}

func TestCreateRoute_OpenAICompatibleRequiresBaseURL(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Compatible Proxy",
		ChannelProvider: "openai-compatible",
		APIKey:          "test-key",
		Models:          []string{"gpt-4o"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "api_base_url")
	require.Nil(t, repo.created)
	require.Nil(t, credSvc.createdReq)
}

func TestCreateRoute_OpenAICompatibleRequiresAPIKey(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Compatible Proxy",
		ChannelProvider: "openai-compatible",
		APIBaseURL:      "https://proxy.example.com/v1",
		Models:          []string{"gpt-4o"},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "api_key")
	require.Equal(t, 0, validator.createCalls)
	require.Nil(t, repo.created)
	require.Nil(t, credSvc.createdReq)
}

func TestCreateRoute_RejectsPrivateBaseURLBeforeValidation(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Internal Proxy",
		ChannelProvider: "openai-compatible",
		APIKey:          "test-key",
		APIBaseURL:      "http://127.0.0.1:8080/v1",
		Models:          []string{"gpt-4o"},
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "api_base_url")
	require.Equal(t, 0, validator.createCalls)
	require.Nil(t, repo.created)
	require.Nil(t, credSvc.createdReq)
}

func TestCreateRoute_OllamaRejectsPrivateBaseURLByDefault(t *testing.T) {
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Local Ollama",
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434",
		Models:          []string{"qwen3.5:4b"},
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "api_base_url")
	require.Equal(t, 0, validator.createCalls)
	require.Nil(t, repo.created)
	require.Nil(t, credSvc.createdReq)
}

func TestCreateRoute_OllamaAllowsEmptyAPIKey(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	view, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Local Ollama",
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434",
		Models:          []string{"qwen3.5:4b"},
	})

	require.NoError(t, err)
	require.Equal(t, "ollama", validator.lastProvider)
	require.Equal(t, "", validator.lastAPIKey)
	require.NotNil(t, credSvc.createdReq)
	require.Equal(t, "ollama", credSvc.createdReq.ChannelProvider)
	require.Equal(t, "", credSvc.createdReq.APIKey)
	require.NotNil(t, repo.created)
	require.Equal(t, "PRIVATE", string(repo.created.Type))
	require.False(t, repo.created.IsOfficial)
	require.Equal(t, "ollama", view.ChannelProvider)
}

func TestCreateRoute_OllamaUpsertsCustomProviderAndModels(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	customProviderRepo := &fakeCustomProviderRepo{}
	customModelRepo := &fakeCustomModelRepo{}
	svc := &channelService{
		tenantRouteRepo:    repo,
		tenantCredService:  credSvc,
		validator:          validator,
		modelRepo:          &fakeModelRepo{},
		customProviderRepo: customProviderRepo,
		customModelRepo:    customModelRepo,
		ollamaModelLister: func(context.Context, string, string) ([]adapter.Model, error) {
			return []adapter.Model{
				{Name: "qwen3.5:4b", Type: "chat", Capabilities: []string{"chat"}},
				{Name: "nomic-embed-text:latest", Type: "embedding", Capabilities: []string{"embedding"}},
				{Name: "dengcao/Qwen3-Reranker-8B:Q4_K_M", Type: "unsupported"},
			}, nil
		},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Local Ollama",
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434",
		Models:          []string{"qwen3.5:4b", "nomic-embed-text:latest"},
	})

	require.NoError(t, err)
	require.Len(t, customProviderRepo.created, 1)
	require.Equal(t, "ollama", customProviderRepo.created[0].Provider)
	require.Len(t, customModelRepo.created, 2)
	require.Equal(t, []string{"text-chat"}, []string(customModelRepo.created[0].UseCases))
	require.Equal(t, []string{"embedding"}, []string(customModelRepo.created[1].UseCases))
}

func TestCreateRoute_OllamaExactBaseURLUpsertsChatModelWithoutDiscovery(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	customProviderRepo := &fakeCustomProviderRepo{}
	customModelRepo := &fakeCustomModelRepo{}
	svc := &channelService{
		tenantRouteRepo:    repo,
		tenantCredService:  credSvc,
		validator:          validator,
		modelRepo:          &fakeModelRepo{models: []*llmmodelmodel.LLMModel{{Model: "qwen3.5:4b", ChatCompletions: true}}},
		customProviderRepo: customProviderRepo,
		customModelRepo:    customModelRepo,
		ollamaModelLister: func(context.Context, string, string) ([]adapter.Model, error) {
			t.Fatal("ollama exact mode should not auto-discover models")
			return nil, nil
		},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Exact Ollama Chat",
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434/custom-endpoint#",
		Models:          []string{"qwen3.5:4b"},
	})

	require.NoError(t, err)
	require.Len(t, customProviderRepo.created, 1)
	require.Equal(t, "http://localhost:11434/custom-endpoint#", customProviderRepo.created[0].APIBaseURL)
	require.Len(t, customModelRepo.created, 1)
	require.Equal(t, []string{"text-chat"}, []string(customModelRepo.created[0].UseCases))
}

func TestCreateRoute_OllamaExactBaseURLUpsertsEmbeddingModelWithoutDiscovery(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	repo := &fakeTenantRouteRepo{}
	credSvc := &fakeTenantCredentialService{}
	validator := &fakeChannelValidator{}
	customProviderRepo := &fakeCustomProviderRepo{}
	customModelRepo := &fakeCustomModelRepo{}
	svc := &channelService{
		tenantRouteRepo:    repo,
		tenantCredService:  credSvc,
		validator:          validator,
		modelRepo:          &fakeModelRepo{models: []*llmmodelmodel.LLMModel{{Model: "nomic-embed-text:latest", Embeddings: true}}},
		customProviderRepo: customProviderRepo,
		customModelRepo:    customModelRepo,
		ollamaModelLister: func(context.Context, string, string) ([]adapter.Model, error) {
			t.Fatal("ollama exact mode should not auto-discover models")
			return nil, nil
		},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Exact Ollama Embed",
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434/embedding-endpoint #",
		Models:          []string{"nomic-embed-text:latest"},
	})

	require.NoError(t, err)
	require.Len(t, customModelRepo.created, 1)
	require.Equal(t, []string{"embedding"}, []string(customModelRepo.created[0].UseCases))
}

func TestCreateRoute_OllamaExactBaseURLRejectsMixedUseCases(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	svc := &channelService{
		tenantRouteRepo:   &fakeTenantRouteRepo{},
		tenantCredService: &fakeTenantCredentialService{},
		validator:         &fakeChannelValidator{},
		modelRepo: &fakeModelRepo{
			models: []*llmmodelmodel.LLMModel{
				{Model: "qwen3.5:4b", ChatCompletions: true},
				{Model: "nomic-embed-text:latest", Embeddings: true},
			},
		},
		customProviderRepo: &fakeCustomProviderRepo{},
		customModelRepo:    &fakeCustomModelRepo{},
		ollamaModelLister: func(context.Context, string, string) ([]adapter.Model, error) {
			t.Fatal("ollama exact mode should not auto-discover models")
			return nil, nil
		},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Mixed Exact Ollama",
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434/custom-endpoint#",
		Models:          []string{"qwen3.5:4b", "nomic-embed-text:latest"},
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "single use case")
}

func TestCreateRoute_OllamaExactBaseURLRejectsUnknownModelWithoutDiscovery(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	svc := &channelService{
		tenantRouteRepo:    &fakeTenantRouteRepo{},
		tenantCredService:  &fakeTenantCredentialService{},
		validator:          &fakeChannelValidator{},
		modelRepo:          &fakeModelRepo{},
		customProviderRepo: &fakeCustomProviderRepo{},
		customModelRepo:    &fakeCustomModelRepo{},
		ollamaModelLister: func(context.Context, string, string) ([]adapter.Model, error) {
			t.Fatal("ollama exact mode should not auto-discover models")
			return nil, nil
		},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Unknown Exact Ollama",
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434/custom-endpoint#",
		Models:          []string{"qwen3.5:4b"},
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "local model library")
}

func TestCreateRoute_OllamaRejectsUnsupportedRerankModel(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	svc := &channelService{
		tenantRouteRepo:    &fakeTenantRouteRepo{},
		tenantCredService:  &fakeTenantCredentialService{},
		validator:          &fakeChannelValidator{},
		modelRepo:          &fakeModelRepo{},
		customProviderRepo: &fakeCustomProviderRepo{},
		customModelRepo:    &fakeCustomModelRepo{},
		ollamaModelLister: func(context.Context, string, string) ([]adapter.Model, error) {
			return []adapter.Model{
				{Name: "dengcao/Qwen3-Reranker-8B:Q4_K_M", Type: "unsupported"},
			}, nil
		},
	}

	_, err := svc.CreateRoute(context.Background(), uuid.New(), &channeldto.CreateRouteRequest{
		Name:            "Local Ollama",
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434",
		Models:          []string{"dengcao/Qwen3-Reranker-8B:Q4_K_M"},
	})

	require.ErrorContains(t, err, "is not supported by this adapter")
}

func TestDiscoverOllamaModels_MapsUseCases(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	svc := &channelService{
		ollamaModelLister: func(context.Context, string, string) ([]adapter.Model, error) {
			return []adapter.Model{
				{Name: "qwen3.5:4b", Type: "chat", Capabilities: []string{"chat"}},
				{Name: "nomic-embed-text:latest", Type: "embedding", Capabilities: []string{"embedding"}},
				{Name: "dengcao/Qwen3-Reranker-8B:Q4_K_M", Type: "unsupported"},
			}, nil
		},
	}

	result, err := svc.DiscoverOllamaModels(context.Background(), &channeldto.DiscoverOllamaModelsRequest{
		APIBaseURL: "http://localhost:11434",
	})

	require.NoError(t, err)
	require.Equal(t, 3, result.Total)
	require.Equal(t, "text-chat", result.Models[0].UseCase)
	require.Equal(t, "embedding", result.Models[1].UseCase)
	require.Equal(t, "unsupported", result.Models[2].UseCase)
}

func TestDiscoverDraftChannelModels_ReturnsUnsupportedWhenListingIsUnsupported(t *testing.T) {
	svc := &channelService{}

	result, err := svc.DiscoverDraftChannelModels(context.Background(), &channeldto.DiscoverDraftChannelModelsRequest{
		ChannelProvider: "openai-compatible",
		APIKey:          "sk-test",
		APIBaseURL:      "https://proxy.example.com/v1/chat/completions#",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.ListingSupported)
	require.Equal(t, 0, result.Total)
	require.Empty(t, result.Models)
}

func TestDiscoverOllamaModels_RejectsExactBaseURL(t *testing.T) {
	svc := &channelService{
		ollamaModelLister: func(context.Context, string, string) ([]adapter.Model, error) {
			t.Fatal("ollama exact mode should not auto-discover models")
			return nil, nil
		},
	}

	_, err := svc.DiscoverOllamaModels(context.Background(), &channeldto.DiscoverOllamaModelsRequest{
		APIBaseURL: "http://localhost:11434/custom-endpoint#",
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "does not support auto-discover")
}

func TestDraftTestChannelModel_UsesValidatorAndReturnsNormalizedResult(t *testing.T) {
	validator := &fakeChannelValidator{
		testResult: &channelprovider.TestResult{
			Success:        true,
			Message:        "ok",
			ResponseTimeMs: 34,
			Model:          "gpt-4o",
			UseCase:        "chat",
		},
	}
	svc := &channelService{
		tenantRouteRepo:   &fakeTenantRouteRepo{},
		tenantCredService: &fakeTenantCredentialService{},
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	result, err := svc.TestDraftChannelModel(context.Background(), uuid.New(), &channeldto.DraftTestChannelModelRequest{
		ChannelProvider: "openai-compatible",
		APIKey:          "test-key",
		APIBaseURL:      "https://proxy.example.com/v1",
		Model:           "gpt-4o",
	})

	require.NoError(t, err)
	require.Equal(t, "openai-compatible", validator.lastTestProvider)
	require.Equal(t, "https://proxy.example.com/v1", validator.lastTestBaseURL)
	require.Equal(t, "gpt-4o", validator.lastTestModel)
	require.Equal(t, true, result.Success)
	require.Equal(t, "chat", result.UseCase)
	require.Equal(t, "chat", result.TestMethod)
	require.Equal(t, int64(34), result.ResponseTimeMs)
}

func TestDraftTestChannelModel_OllamaAllowsEmptyAPIKey(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   &fakeTenantRouteRepo{},
		tenantCredService: &fakeTenantCredentialService{},
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	result, err := svc.TestDraftChannelModel(context.Background(), uuid.New(), &channeldto.DraftTestChannelModelRequest{
		ChannelProvider: "ollama",
		APIBaseURL:      "http://localhost:11434",
		Model:           "qwen3.5:4b",
	})

	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "ollama", validator.lastTestProvider)
	require.Equal(t, "", validator.lastTestAPIKey)
}

func TestDraftTestChannelModel_RejectsPrivateBaseURLBeforeValidation(t *testing.T) {
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   &fakeTenantRouteRepo{},
		tenantCredService: &fakeTenantCredentialService{},
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	result, err := svc.TestDraftChannelModel(context.Background(), uuid.New(), &channeldto.DraftTestChannelModelRequest{
		ChannelProvider: "openai-compatible",
		APIKey:          "test-key",
		APIBaseURL:      "http://127.0.0.1:8080/v1",
		Model:           "gpt-4o",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.Success)
	require.Contains(t, result.Message, "api_base_url")
	require.Empty(t, validator.lastTestProvider)
}

func TestUpdateRoute_RejectsPrivateBaseURLBeforeMutation(t *testing.T) {
	credID := uuid.New()
	orgID := uuid.New()
	routeID := uuid.New()
	privateBaseURL := "http://127.0.0.1:8080/v1"
	repo := &fakeTenantRouteRepo{
		routeByID: &channelmodel.LLMRoute{
			ID:              routeID,
			OrganizationID:  orgID,
			Type:            "PRIVATE",
			CredentialID:    &credID,
			Name:            "Existing Route",
			ChannelProvider: "openai-compatible",
			APIBaseURL:      "https://api.example.com/v1",
			Models:          []string{"gpt-4o"},
			IsEnabled:       true,
		},
	}
	credSvc := &fakeTenantCredentialService{
		cred: &credentialmodel.TenantCredential{ID: credID, ChannelProvider: "openai-compatible"},
	}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	_, err := svc.UpdateRoute(context.Background(), orgID, routeID, &channeldto.UpdateRouteRequest{
		APIBaseURL: &privateBaseURL,
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "api_base_url")
	require.Equal(t, 0, validator.createCalls)
	require.Nil(t, repo.updated)
	require.Nil(t, credSvc.updatedReq)
}

func TestUpdateRoute_OllamaCanClearAPIKey(t *testing.T) {
	setLLMAllowPrivateBaseURL(t, true)
	credID := uuid.New()
	orgID := uuid.New()
	repo := &fakeTenantRouteRepo{
		routeByID: &channelmodel.LLMRoute{
			ID:              uuid.New(),
			OrganizationID:  orgID,
			Type:            "PRIVATE",
			CredentialID:    &credID,
			Name:            "Existing Route",
			ChannelProvider: "ollama",
			APIBaseURL:      "http://localhost:11434",
			Models:          []string{"qwen3.5:4b"},
			IsEnabled:       true,
		},
	}
	credSvc := &fakeTenantCredentialService{
		cred: &credentialmodel.TenantCredential{ID: credID, ChannelProvider: "ollama"},
	}
	validator := &fakeChannelValidator{}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	emptyKey := ""
	_, err := svc.UpdateRoute(context.Background(), orgID, repo.routeByID.ID, &channeldto.UpdateRouteRequest{
		APIKey: &emptyKey,
	})

	require.NoError(t, err)
	require.Equal(t, "", validator.lastAPIKey)
	require.NotNil(t, credSvc.updatedReq)
	require.NotNil(t, credSvc.updatedReq.APIKey)
	require.Equal(t, "", *credSvc.updatedReq.APIKey)
}

func TestTestChannelModel_ReusesValidatorResultShape(t *testing.T) {
	routeID := uuid.New()
	credentialID := uuid.New()
	repo := &fakeTenantRouteRepo{
		routeByID: &channelmodel.LLMRoute{
			ID:              routeID,
			OrganizationID:  uuid.New(),
			ChannelProvider: "openai-compatible",
			APIBaseURL:      "https://proxy.example.com/v1",
			CredentialID:    &credentialID,
		},
	}
	credSvc := &fakeTenantCredentialService{
		cred: &credentialmodel.TenantCredential{ID: credentialID},
	}
	validator := &fakeChannelValidator{
		testResult: &channelprovider.TestResult{
			Success:        true,
			Message:        "ok",
			ResponseTimeMs: 56,
			Model:          "gpt-4o",
			UseCase:        "chat",
		},
	}
	svc := &channelService{
		tenantRouteRepo:   repo,
		tenantCredService: credSvc,
		validator:         validator,
		modelRepo:         &fakeModelRepo{},
	}

	result, err := svc.TestChannelModel(context.Background(), routeID, repo.routeByID.OrganizationID, "gpt-4o", "")

	require.NoError(t, err)
	require.Equal(t, true, result.Success)
	require.Equal(t, "gpt-4o", result.Model)
	require.Equal(t, "chat", result.UseCase)
	require.Equal(t, "chat", result.TestMethod)
	require.Equal(t, int64(56), result.ResponseTimeMs)
	require.Equal(t, "openai-compatible", validator.lastTestProvider)
}
