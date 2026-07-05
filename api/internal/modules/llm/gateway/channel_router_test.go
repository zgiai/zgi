package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zgiai/zgi/api/config"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	channelrepo "github.com/zgiai/zgi/api/internal/modules/llm/channel/repository"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	_ channelrepo.TenantRouteRepository     = (*fakeCandidateRouteRepo)(nil)
	_ llmmodelsvc.PrivateModelLookupService = (*fakePrivateModelLookup)(nil)
)

type stubCryptoService struct{}

func (stubCryptoService) Encrypt(plaintext string) (string, error) {
	return plaintext, nil
}

func (stubCryptoService) Decrypt(ciphertext string) (string, error) {
	return "test-api-key", nil
}

type fakeCandidateRouteRepo struct {
	routes []*channelmodel.LLMRoute
}

func (f *fakeCandidateRouteRepo) Create(context.Context, *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) BatchCreate(context.Context, []*channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) GetByID(context.Context, uuid.UUID, uuid.UUID) (*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) List(context.Context, uuid.UUID, *bool, int, int) ([]*channelmodel.LLMRoute, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) Update(context.Context, *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) GetEnabledRoutes(context.Context, uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	return f.routes, nil
}
func (f *fakeCandidateRouteRepo) FindByModel(context.Context, uuid.UUID, string) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) CountByCredentialID(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) GetDistinctProviders(context.Context, uuid.UUID) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeCandidateRouteRepo) GetPlatformChannels(context.Context) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

type fakePrivateModelLookup struct {
	model *llmmodel.CustomModel
}

func (f *fakePrivateModelLookup) ListActiveModelsByNames(context.Context, uuid.UUID, []string) ([]*llmmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePrivateModelLookup) ResolveActiveModels(context.Context, uuid.UUID, []string) ([]*llmmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePrivateModelLookup) ResolveActiveModelsForProvider(context.Context, uuid.UUID, string, []string) ([]*llmmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}
func (f *fakePrivateModelLookup) ResolveActiveModel(context.Context, uuid.UUID, string) (*llmmodel.CustomModel, error) {
	return f.model, nil
}
func (f *fakePrivateModelLookup) ResolveActiveModelForProvider(context.Context, uuid.UUID, string, string) (*llmmodel.CustomModel, error) {
	return f.model, nil
}
func (f *fakePrivateModelLookup) LoadActiveModelNameIndexes(context.Context, uuid.UUID) ([]string, map[string]string, error) {
	return nil, nil, errors.New("not implemented")
}

func openGatewayModelLookupDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("open gorm db: %v", err)
	}
	return db, mock
}

func newGatewayTestRedis(t *testing.T) *redis.Client {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
}

func TestBuildChannelSelection_UsesModelListAsSourceOfTruth(t *testing.T) {
	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "deepseek",
		APIBaseURL:      "https://api.agicto.cn/v1",
		Models:          []string{"gpt-4o", "deepseek-v3"},
		IsEnabled:       true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-4o",
		ModelName: "gpt-4o",
		Provider:  "openai",
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, "gpt-4o", false, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection == nil {
		t.Fatal("buildChannelSelection returned nil selection")
	}
	if selection.ModelName != "gpt-4o" {
		t.Fatalf("selection.ModelName = %q, want %q", selection.ModelName, "gpt-4o")
	}
	if selection.ChannelProvider != "deepseek" {
		t.Fatalf("selection.ChannelProvider = %q, want %q", selection.ChannelProvider, "deepseek")
	}
	if selection.ModelSource != channelModelSourceGlobal {
		t.Fatalf("selection.ModelSource = %q, want %q", selection.ModelSource, channelModelSourceGlobal)
	}
}

func TestBuildChannelSelection_UsesRouteChannelProviderForPrivateRoutes(t *testing.T) {
	router := &ChannelRouter{
		cryptoService: stubCryptoService{},
	}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "agicto",
		APIBaseURL:      "https://api.agicto.cn/v1",
		Models:          []string{"qwen2.5-14b-instruct"},
		IsEnabled:       true,
		TenantCredential: &credentialmodel.TenantCredential{
			ChannelProvider:  "openai",
			APIKeyCiphertext: "ciphertext",
			APIBaseURL:       "https://api.agicto.cn/v1",
			IsActive:         true,
		},
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "qwen2.5-14b-instruct",
		ModelName: "Qwen2.5-14B-Instruct",
		Provider:  "qwen",
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection == nil {
		t.Fatal("buildChannelSelection returned nil selection")
	}
	if selection.ChannelProvider != "agicto" {
		t.Fatalf("selection.ChannelProvider = %q, want %q", selection.ChannelProvider, "agicto")
	}
	if selection.APIKey != "test-api-key" {
		t.Fatalf("selection.APIKey = %q, want decrypted API key", selection.APIKey)
	}
	if selection.ModelSource != channelModelSourceGlobal {
		t.Fatalf("selection.ModelSource = %q, want %q", selection.ModelSource, channelModelSourceGlobal)
	}
}

func TestBuildChannelSelection_PassthroughModelSource(t *testing.T) {
	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "openai-compatible",
		APIBaseURL:      "https://example.com/v1",
		Models:          []string{"custom-model"},
		IsEnabled:       true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, nil, nil, "custom-model", true, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection.ModelSource != channelModelSourcePassthrough {
		t.Fatalf("selection.ModelSource = %q, want %q", selection.ModelSource, channelModelSourcePassthrough)
	}
}

func TestChannelRouterGetModelRejectsDeprecatedModel(t *testing.T) {
	db, mock := openGatewayModelLookupDB(t)
	modelName := "deprecated-model"
	mock.ExpectQuery(`(?s)FROM "llm_models" JOIN llm_providers .*llm_models\.status`).
		WithArgs(modelName, true, llmmodel.ModelStatusActive, true, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	router := &ChannelRouter{db: db}

	model, err := router.getModel(context.Background(), modelName)
	if err == nil {
		t.Fatalf("getModel returned model status %q, want error", model.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestResolveSelectionModelUsesProviderHintForGlobalModel(t *testing.T) {
	db, mock := openGatewayModelLookupDB(t)
	modelID := uuid.New()
	providerHint := "anthropic"
	modelName := "same-name"
	mock.ExpectQuery(`(?s)FROM "llm_models" JOIN llm_providers .*llm_models\.provider.*llm_models\.name`).
		WithArgs(providerHint, modelName, true, llmmodel.ModelStatusActive, true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "provider", "name", "display_name", "status", "is_active"}).
			AddRow(modelID, providerHint, modelName, "Same Name", llmmodel.ModelStatusActive, true))
	router := &ChannelRouter{db: db}

	model, privateModel, err := router.resolveSelectionModel(context.Background(), uuid.New(), providerHint, modelName)
	if err != nil {
		t.Fatalf("resolveSelectionModel returned error: %v", err)
	}
	if privateModel != nil {
		t.Fatalf("privateModel = %+v, want nil", privateModel)
	}
	if model.Provider != providerHint {
		t.Fatalf("model.Provider = %q, want %q", model.Provider, providerHint)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestConfigCacheGetModelByNameRejectsCachedDeprecatedModel(t *testing.T) {
	ctx := context.Background()
	db, mock := openGatewayModelLookupDB(t)
	redisClient := newGatewayTestRedis(t)
	cache := NewConfigCache(redisClient, db, &ConfigCacheConfig{
		ModelTTL:    time.Minute,
		ProviderTTL: time.Minute,
	})
	modelName := "cached-deprecated-model"
	data, err := marshalCachedModel(&llmmodel.LLMModel{
		ID:        uuid.New(),
		Provider:  "openai",
		Model:     modelName,
		ModelName: "Cached Deprecated Model",
		Status:    "deprecated",
		IsActive:  true,
	})
	if err != nil {
		t.Fatalf("marshal cached model: %v", err)
	}
	if err := redisClient.Set(ctx, cache.prefix+"model:name:"+modelName, data, time.Minute).Err(); err != nil {
		t.Fatalf("seed cached model: %v", err)
	}
	providerData, err := json.Marshal(&providermodel.LLMProvider{
		ID:           uuid.New(),
		Provider:     "openai",
		ProviderName: "OpenAI",
		IsActive:     true,
	})
	if err != nil {
		t.Fatalf("marshal cached provider: %v", err)
	}
	if err := redisClient.Set(ctx, cache.prefix+"provider:name:openai", providerData, time.Minute).Err(); err != nil {
		t.Fatalf("seed cached provider: %v", err)
	}
	mock.ExpectQuery(`(?s)FROM "llm_models" JOIN llm_providers .*llm_models\.status`).
		WithArgs(modelName, true, llmmodel.ModelStatusActive, true, 1).
		WillReturnError(gorm.ErrRecordNotFound)

	model, err := cache.GetModelByName(ctx, modelName)
	if err == nil {
		t.Fatalf("GetModelByName returned model status %q, want error", model.Status)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSelectChannelsForProviderRejectsKnownInactiveGlobalModel(t *testing.T) {
	db, mock := openGatewayModelLookupDB(t)
	modelName := "qwen-coder"
	providerHint := "qwen"
	mock.ExpectQuery(`(?s)FROM "llm_models" JOIN llm_providers .*llm_models\.provider.*llm_models\.name`).
		WithArgs(providerHint, modelName, true, llmmodel.ModelStatusActive, true, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "llm_models".*provider.*name`).
		WithArgs(providerHint, modelName).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	router := &ChannelRouter{db: db}

	selections, err := router.SelectChannelsForProvider(context.Background(), uuid.New(), providerHint, modelName, 1)
	if err == nil {
		t.Fatal("SelectChannelsForProvider returned nil error, want model not found")
	}
	if selections != nil {
		t.Fatalf("selections = %+v, want nil", selections)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSelectChannelsForProviderKeepsPassthroughForUnknownModel(t *testing.T) {
	db, mock := openGatewayModelLookupDB(t)
	orgID := uuid.New()
	modelName := "tenant-custom-model"
	mock.ExpectQuery(`(?s)FROM "llm_models" JOIN llm_providers .*llm_models\.status`).
		WithArgs(modelName, true, llmmodel.ModelStatusActive, true, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "llm_models".*name`).
		WithArgs(modelName).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	router := &ChannelRouter{
		db: db,
		organizationIDRouteRepo: &fakeCandidateRouteRepo{
			routes: []*channelmodel.LLMRoute{
				{
					ID:              uuid.New(),
					OrganizationID:  orgID,
					Type:            shared.RouteTypePrivate,
					ChannelProvider: "openai-compatible",
					Models:          []string{modelName},
				},
			},
		},
		strategyFactory: NewStrategyFactory(),
	}

	selections, err := router.SelectChannelsForProvider(context.Background(), orgID, "", modelName, 1)
	if err != nil {
		t.Fatalf("SelectChannelsForProvider returned error: %v", err)
	}
	if len(selections) != 1 {
		t.Fatalf("len(selections) = %d, want 1", len(selections))
	}
	if selections[0].ModelSource != channelModelSourcePassthrough {
		t.Fatalf("ModelSource = %q, want %q", selections[0].ModelSource, channelModelSourcePassthrough)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCandidateRoutesForModel_FiltersNativeProtocolLikeRealSelection(t *testing.T) {
	orgID := uuid.New()
	modelName := "custom-responses-model"
	router := &ChannelRouter{
		organizationIDRouteRepo: &fakeCandidateRouteRepo{
			routes: []*channelmodel.LLMRoute{
				{
					ID:              uuid.New(),
					OrganizationID:  orgID,
					Type:            shared.RouteTypePrivate,
					ChannelProvider: "openai-compatible",
					Models:          []string{modelName},
					NativeProtocols: channelmodel.NativeProtocolConfig{
						OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
					},
				},
				{
					ID:              uuid.New(),
					OrganizationID:  orgID,
					Type:            shared.RouteTypePrivate,
					ChannelProvider: "deepseek",
					Models:          []string{modelName},
				},
			},
		},
		strategyFactory: NewStrategyFactory(),
		privateModels: &fakePrivateModelLookup{
			model: &llmmodel.CustomModel{
				ID:        uuid.New(),
				Provider:  "openai",
				Name:      modelName,
				Responses: true,
				IsActive:  true,
			},
		},
	}
	ctx := context.WithValue(context.Background(), shared.ContextKeyModelCategory, modelCategoryResponses)

	routes, err := router.CandidateRoutesForModel(ctx, orgID, modelName, 3)
	if err != nil {
		t.Fatalf("CandidateRoutesForModel returned error: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("len(routes) = %d, want 1", len(routes))
	}
	if routes[0].ChannelProvider != "openai-compatible" {
		t.Fatalf("route provider = %q, want openai-compatible", routes[0].ChannelProvider)
	}
}

func TestCandidateRoutesForModelRejectsKnownInactiveGlobalModel(t *testing.T) {
	db, mock := openGatewayModelLookupDB(t)
	modelName := "qwen-coder"
	mock.ExpectQuery(`(?s)FROM "llm_models" JOIN llm_providers .*llm_models\.status`).
		WithArgs(modelName, true, llmmodel.ModelStatusActive, true, 1).
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectQuery(`(?s)SELECT count\(\*\) FROM "llm_models".*name`).
		WithArgs(modelName).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	router := &ChannelRouter{db: db}

	routes, err := router.CandidateRoutesForModel(context.Background(), uuid.New(), modelName, 1)
	if err == nil {
		t.Fatal("CandidateRoutesForModel returned nil error, want model not found")
	}
	if routes != nil {
		t.Fatalf("routes = %+v, want nil", routes)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestBuildChannelSelection_OfficialRouteUsesRuntimeConsoleAPIURL(t *testing.T) {
	setGatewayConsoleAPIURL(t, "https://console-api.zgi.im")

	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		APIBaseURL:      "http://zgi-console-api.zeabur.internal:2625/v1/internal",
		Models:          []string{"gpt-5"},
		IsEnabled:       true,
		IsOfficial:      true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-5",
		ModelName: "gpt-5",
		Provider:  "openai",
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection.APIBaseURL != "https://console-api.zgi.im/v1/internal" {
		t.Fatalf("selection.APIBaseURL = %q, want runtime console URL", selection.APIBaseURL)
	}
	if selection.BillingLane != UsageBillingLanePlatform || !selection.UseSystemProvider {
		t.Fatalf("selection lane = %s use_system_provider=%t, want platform/true", selection.BillingLane, selection.UseSystemProvider)
	}
	if route.APIBaseURL != "http://zgi-console-api.zeabur.internal:2625/v1/internal" {
		t.Fatalf("route.APIBaseURL mutated to %q", route.APIBaseURL)
	}
}

func TestBuildChannelSelection_OfficialRouteRequiresRuntimeConsoleAPIURL(t *testing.T) {
	setGatewayConsoleAPIURL(t, "")

	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		APIBaseURL:      "http://zgi-console-api.zeabur.internal:2625/v1/internal",
		Models:          []string{"gpt-5"},
		IsEnabled:       true,
		IsOfficial:      true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-5",
		ModelName: "gpt-5",
		Provider:  "openai",
		IsActive:  true,
	}

	_, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err == nil || !strings.Contains(err.Error(), "console api url") {
		t.Fatalf("buildChannelSelection error = %v, want console api url error", err)
	}
}

func TestBuildChannelSelection_OfficialRouteRejectsInvalidRuntimeConsoleAPIURL(t *testing.T) {
	setGatewayConsoleAPIURL(t, "console-api.zgi.im")

	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		APIBaseURL:      "http://zgi-console-api.zeabur.internal:2625/v1/internal",
		Models:          []string{"gpt-5"},
		IsEnabled:       true,
		IsOfficial:      true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-5",
		ModelName: "gpt-5",
		Provider:  "openai",
		IsActive:  true,
	}

	_, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err == nil || !strings.Contains(err.Error(), "invalid console api url") {
		t.Fatalf("buildChannelSelection error = %v, want invalid console api url error", err)
	}
}

func TestBuildChannelSelection_PrivateRouteKeepsRouteAPIBaseURL(t *testing.T) {
	setGatewayConsoleAPIURL(t, "https://console-api.zgi.im")

	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "openai-compatible",
		APIBaseURL:      "https://proxy.example.com/v1",
		Models:          []string{"gpt-4o"},
		IsEnabled:       true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "gpt-4o",
		ModelName: "gpt-4o",
		Provider:  "openai",
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, "chat")
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection.APIBaseURL != "https://proxy.example.com/v1" {
		t.Fatalf("selection.APIBaseURL = %q, want private route URL", selection.APIBaseURL)
	}
	if selection.BillingLane != UsageBillingLanePrivate || selection.UseSystemProvider {
		t.Fatalf("selection lane = %s use_system_provider=%t, want private/false", selection.BillingLane, selection.UseSystemProvider)
	}
}

func TestFilterRoutesForNativeProtocol_ResponsesSkipsChatOnlyRoutes(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
			NativeProtocols: channelmodel.NativeProtocolConfig{
				OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
			},
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "deepseek",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypeZGICloud,
			ChannelProvider: "zgi-cloud",
			IsOfficial:      true,
		},
	}
	model := &llmmodel.LLMModel{
		Model:     "gpt-4.1-mini",
		Provider:  "openai",
		Responses: true,
	}

	filtered := filterRoutesForNativeProtocol(routes, model, modelCategoryResponses)
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %d, want 2", len(filtered))
	}
	if filtered[0].ChannelProvider != "openai-compatible" || filtered[1].ChannelProvider != "zgi-cloud" {
		t.Fatalf("filtered providers = [%q, %q], want [openai-compatible, zgi-cloud]", filtered[0].ChannelProvider, filtered[1].ChannelProvider)
	}
}

func TestFilterRoutesForNativeProtocol_OpenAICompatibleRequiresExplicitResponsesConfig(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
			NativeProtocols: channelmodel.NativeProtocolConfig{
				OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
			},
		},
	}
	model := &llmmodel.LLMModel{
		Model:     "custom-model",
		Provider:  "openai",
		Responses: true,
	}

	filtered := filterRoutesForNativeProtocol(routes, model, modelCategoryResponses)
	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0].ID != routes[1].ID {
		t.Fatalf("filtered route = %s, want explicitly configured route %s", filtered[0].ID, routes[1].ID)
	}
}

func TestFilterRoutesForNativeProtocol_ResponsesRequiresModelCapabilityForKnownModels(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
			NativeProtocols: channelmodel.NativeProtocolConfig{
				OpenAIResponses: channelmodel.NativeProtocolEndpoint{Enabled: true},
			},
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypeZGICloud,
			ChannelProvider: "zgi-cloud",
			IsOfficial:      true,
		},
	}
	model := &llmmodel.LLMModel{
		Model:     "chat-only-model",
		Provider:  "openai",
		Responses: false,
	}

	filtered := filterRoutesForNativeProtocol(routes, model, modelCategoryResponses)
	if len(filtered) != 0 {
		t.Fatalf("len(filtered) = %d, want 0", len(filtered))
	}
}

func TestFilterRoutesForNativeProtocol_AnthropicSkipsOpenAIOnlyRoutes(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "openai-compatible",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "agicto",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "claude",
		},
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypeZGICloud,
			ChannelProvider: "zgi-cloud",
			IsOfficial:      true,
		},
	}
	model := &llmmodel.LLMModel{
		Model:    "claude-sonnet-4-0",
		Provider: "anthropic",
	}

	filtered := filterRoutesForNativeProtocol(routes, model, modelCategoryAnthropicMessages)
	if len(filtered) != 3 {
		t.Fatalf("len(filtered) = %d, want 3", len(filtered))
	}
	if filtered[0].ChannelProvider != "agicto" || filtered[1].ChannelProvider != "claude" || filtered[2].ChannelProvider != "zgi-cloud" {
		t.Fatalf("filtered providers = [%q, %q, %q], want [agicto, claude, zgi-cloud]", filtered[0].ChannelProvider, filtered[1].ChannelProvider, filtered[2].ChannelProvider)
	}
}

func TestBuildChannelSelection_NativeProtocolBaseURLOverridesPrivateRouteBaseURL(t *testing.T) {
	router := &ChannelRouter{}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "qwen",
		APIBaseURL:      "https://dashscope.aliyuncs.com/compatible-mode/v1",
		NativeProtocols: channelmodel.NativeProtocolConfig{
			OpenAIResponses: channelmodel.NativeProtocolEndpoint{
				Enabled: true,
				BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/",
			},
			AnthropicMessages: channelmodel.NativeProtocolEndpoint{
				Enabled: true,
				BaseURL: "https://dashscope.aliyuncs.com/api/v2/apps/anthropic",
			},
		},
		Models:    []string{"qwen3-coder"},
		IsEnabled: true,
	}
	model := &llmmodel.LLMModel{
		ID:        uuid.New(),
		Model:     "qwen3-coder",
		ModelName: "qwen3-coder",
		Provider:  "qwen",
		Responses: true,
		IsActive:  true,
	}

	selection, err := router.buildChannelSelection(context.Background(), route, model, nil, model.Model, false, modelCategoryResponses)
	if err != nil {
		t.Fatalf("buildChannelSelection returned error: %v", err)
	}
	if selection.APIBaseURL != "https://dashscope.aliyuncs.com/compatible-mode/v1" {
		t.Fatalf("selection.APIBaseURL = %q, want responses base URL", selection.APIBaseURL)
	}
}

func setGatewayConsoleAPIURL(t *testing.T, apiURL string) {
	t.Helper()
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{
		Console: config.ConsoleConfig{APIURL: apiURL},
	}
	t.Cleanup(func() {
		config.GlobalConfig = oldConfig
	})
}
