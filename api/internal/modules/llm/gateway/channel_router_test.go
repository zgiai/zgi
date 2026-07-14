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
	credentialrepo "github.com/zgiai/zgi/api/internal/modules/llm/credential/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/credential/upstreamstate"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
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

type identityCryptoService struct{}

func (identityCryptoService) Encrypt(plaintext string) (string, error)  { return plaintext, nil }
func (identityCryptoService) Decrypt(ciphertext string) (string, error) { return ciphertext, nil }

type fakeCandidateRouteRepo struct {
	routes       []*channelmodel.LLMRoute
	enabledCalls int
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
	f.enabledCalls++
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

func openGatewayUpstreamGuardDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE llm_credential_upstream_states (
		credential_id text PRIMARY KEY,
		organization_id text NOT NULL,
		generation integer NOT NULL DEFAULT 1,
		balance_capability text NOT NULL DEFAULT 'unknown',
		balance_snapshot text,
		balance_observed_at datetime,
		warning_thresholds text NOT NULL DEFAULT '[]',
		availability text NOT NULL DEFAULT 'unknown',
		observation_source text,
		availability_observed_at datetime,
		last_check_at datetime,
		last_check_status text NOT NULL DEFAULT 'unknown',
		last_check_error_kind text,
		next_check_at datetime,
		check_lease_until datetime,
		consecutive_failures integer NOT NULL DEFAULT 0,
		block_reason text,
		cooldown_until datetime,
		guard_strikes integer NOT NULL DEFAULT 0,
		half_open_lease_until datetime,
		manual_retry_requested_at datetime,
		provider_error_code text,
		provider_error_status integer NOT NULL DEFAULT 0,
		created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error; err != nil {
		t.Fatalf("create upstream state table: %v", err)
	}
	return db
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

func TestCandidateRoutesForModelsLoadsEnabledRoutesOnce(t *testing.T) {
	orgID := uuid.New()
	routeRepo := &fakeCandidateRouteRepo{
		routes: []*channelmodel.LLMRoute{
			{
				ID:              uuid.New(),
				OrganizationID:  orgID,
				Type:            shared.RouteTypePrivate,
				ChannelProvider: "openai-compatible",
				Models:          []string{"model-a", "model-b"},
			},
		},
	}
	router := &ChannelRouter{
		organizationIDRouteRepo: routeRepo,
		strategyFactory:         NewStrategyFactory(),
		privateModels: &fakePrivateModelLookup{
			model: &llmmodel.CustomModel{
				ID:       uuid.New(),
				Provider: "openai",
				Name:     "tenant-model",
				IsActive: true,
				UseCases: []string{"text-chat"},
			},
		},
	}

	routesByModel, err := router.CandidateRoutesForModels(context.Background(), orgID, []string{"model-a", "model-b"}, 1)
	if err != nil {
		t.Fatalf("CandidateRoutesForModels returned error: %v", err)
	}
	if routeRepo.enabledCalls != 1 {
		t.Fatalf("GetEnabledRoutes calls = %d, want 1", routeRepo.enabledCalls)
	}
	if len(routesByModel["model-a"]) != 1 {
		t.Fatalf("model-a routes = %d, want 1", len(routesByModel["model-a"]))
	}
	if len(routesByModel["model-b"]) != 1 {
		t.Fatalf("model-b routes = %d, want 1", len(routesByModel["model-b"]))
	}
}

func TestPrepareCandidateRoutes_GuardLetsFourthHealthyRouteFillAttemptWindow(t *testing.T) {
	db := openGatewayUpstreamGuardDB(t)
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{LLM: config.LLMConfig{
		UpstreamGuardMode:       "enforce",
		UpstreamGuardPercentage: 100,
	}}
	t.Cleanup(func() { config.GlobalConfig = oldConfig })

	organizationID := uuid.New()
	modelName := "deepseek-chat"
	cooldownUntil := time.Now().Add(time.Hour)
	routes := make([]*channelmodel.LLMRoute, 0, 4)
	for index := range 4 {
		credentialID := uuid.New()
		routes = append(routes, &channelmodel.LLMRoute{
			ID:              uuid.New(),
			OrganizationID:  organizationID,
			Type:            shared.RouteTypePrivate,
			CredentialID:    &credentialID,
			ChannelProvider: "deepseek",
			Models:          []string{modelName},
			Priority:        400 - index*100,
			IsEnabled:       true,
		})
		state := &upstreamstate.State{
			CredentialID:      credentialID,
			OrganizationID:    organizationID,
			Generation:        1,
			BalanceCapability: upstreamstate.BalanceCapabilitySupported,
			Availability:      upstreamstate.AvailabilityAvailable,
			LastCheckStatus:   upstreamstate.CheckStatusSuccess,
			WarningThresholds: []upstreamstate.WarningThreshold{},
		}
		if index < 3 {
			state.Availability = upstreamstate.AvailabilityExhausted
			state.BlockReason = upstreamstate.GuardReasonBalanceExhausted
			state.CooldownUntil = &cooldownUntil
			state.GuardStrikes = 1
		}
		if err := db.Create(state).Error; err != nil {
			t.Fatalf("create upstream state %d: %v", index, err)
		}
	}

	router := &ChannelRouter{
		strategyFactory: NewStrategyFactory(),
		upstreamState:   upstreamstate.NewService(db, stubCryptoService{}),
	}
	eligible, err := router.prepareCandidateRoutes(
		context.Background(),
		organizationID,
		routes,
		modelName,
		"deepseek",
		false,
		&llmmodel.LLMModel{Provider: "deepseek", Model: modelName},
		true,
		true,
	)
	if err != nil {
		t.Fatalf("prepareCandidateRoutes() error = %v", err)
	}
	selected := router.selectRoutesByPriorityAndWeight(eligible, 3)
	if len(selected) != 1 || selected[0].ID != routes[3].ID {
		t.Fatalf("selected routes = %#v, want fourth healthy route", selected)
	}

	if err := db.Model(&upstreamstate.State{}).
		Where("credential_id = ?", *routes[3].CredentialID).
		Updates(map[string]any{
			"availability":   upstreamstate.AvailabilityExhausted,
			"block_reason":   upstreamstate.GuardReasonBalanceExhausted,
			"cooldown_until": cooldownUntil,
			"guard_strikes":  1,
		}).Error; err != nil {
		t.Fatalf("guard fourth credential: %v", err)
	}
	_, err = router.prepareCandidateRoutes(
		context.Background(),
		organizationID,
		routes,
		modelName,
		"deepseek",
		false,
		&llmmodel.LLMModel{Provider: "deepseek", Model: modelName},
		true,
		true,
	)
	if !errors.Is(err, llmerrors.DomainErrPrivateChannelUpstreamUnavailable) {
		t.Fatalf("prepareCandidateRoutes(all guarded) error = %v, want private upstream unavailable", err)
	}
}

func TestUpstreamGenerationReloadsMatchingCredential(t *testing.T) {
	db := openGatewayUpstreamGuardDB(t)
	if err := db.Exec(`CREATE TABLE llm_credentials (
		id text PRIMARY KEY,
		organization_id text NOT NULL,
		name text NOT NULL,
		provider text NOT NULL,
		api_key_ciphertext text NOT NULL,
		api_key_hash text,
		api_base_url text,
		is_active numeric NOT NULL DEFAULT 1,
		last_used_at datetime,
		expires_at datetime,
		metadata text,
		created_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
		deleted_at datetime
	)`).Error; err != nil {
		t.Fatalf("create credential table: %v", err)
	}
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{LLM: config.LLMConfig{UpstreamGuardMode: "off"}}
	t.Cleanup(func() { config.GlobalConfig = oldConfig })

	organizationID := uuid.New()
	credentialID := uuid.New()
	credential := &credentialmodel.TenantCredential{
		ID:               credentialID,
		OrganizationID:   organizationID,
		Name:             "current",
		ChannelProvider:  "openrouter",
		APIKeyCiphertext: "current-key",
		APIBaseURL:       "https://openrouter.ai/api/v1",
		IsActive:         true,
	}
	if err := db.Create(credential).Error; err != nil {
		t.Fatalf("create current credential: %v", err)
	}
	if err := db.Create(&upstreamstate.State{
		CredentialID:      credentialID,
		OrganizationID:    organizationID,
		Generation:        2,
		BalanceCapability: upstreamstate.BalanceCapabilityUnknown,
		Availability:      upstreamstate.AvailabilityUnknown,
		LastCheckStatus:   upstreamstate.CheckStatusUnknown,
		WarningThresholds: []upstreamstate.WarningThreshold{},
	}).Error; err != nil {
		t.Fatalf("create upstream state: %v", err)
	}

	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypePrivate,
		CredentialID:    &credentialID,
		ChannelProvider: "deepseek",
		APIBaseURL:      "https://api.deepseek.com/v1",
		Models:          []string{"shared-model"},
		IsEnabled:       true,
		TenantCredential: &credentialmodel.TenantCredential{
			ID:               credentialID,
			OrganizationID:   organizationID,
			ChannelProvider:  "deepseek",
			APIKeyCiphertext: "stale-key",
			APIBaseURL:       "https://api.deepseek.com/v1",
		},
	}
	router := &ChannelRouter{
		organizationIDCredRepo: credentialrepo.NewTenantCredentialRepository(db),
		cryptoService:          identityCryptoService{},
		strategyFactory:        NewStrategyFactory(),
		upstreamState:          upstreamstate.NewService(db, identityCryptoService{}),
	}

	filtered, guarded := router.filterRoutesByUpstreamGuard(context.Background(), organizationID, []*channelmodel.LLMRoute{route}, true, true)
	if len(filtered) != 1 || guarded != 0 {
		t.Fatalf("filter result = %d/%d, want one eligible route", len(filtered), guarded)
	}
	selection, err := router.buildChannelSelection(
		context.Background(),
		filtered[0],
		&llmmodel.LLMModel{Model: "shared-model", Provider: "openrouter"},
		nil,
		"shared-model",
		false,
		"chat",
	)
	if err != nil {
		t.Fatalf("buildChannelSelection() error = %v", err)
	}
	if selection.CredentialGeneration != 2 || selection.APIKey != "current-key" {
		t.Fatalf("generation/key = %d/%q, want 2/current-key", selection.CredentialGeneration, selection.APIKey)
	}
	if selection.ChannelProvider != "openrouter" || selection.APIBaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("provider/base_url = %q/%q, want current credential values", selection.ChannelProvider, selection.APIBaseURL)
	}
}

func TestUpstreamGuardOffKeepsRecoveryEvidenceWithoutBlocking(t *testing.T) {
	db := openGatewayUpstreamGuardDB(t)
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{LLM: config.LLMConfig{UpstreamGuardMode: "off"}}
	t.Cleanup(func() { config.GlobalConfig = oldConfig })

	organizationID := uuid.New()
	credentialID := uuid.New()
	cooldownUntil := time.Now().Add(time.Hour)
	if err := db.Create(&upstreamstate.State{
		CredentialID:      credentialID,
		OrganizationID:    organizationID,
		Generation:        1,
		BalanceCapability: upstreamstate.BalanceCapabilitySupported,
		Availability:      upstreamstate.AvailabilityExhausted,
		LastCheckStatus:   upstreamstate.CheckStatusSuccess,
		WarningThresholds: []upstreamstate.WarningThreshold{},
		BlockReason:       upstreamstate.GuardReasonBalanceExhausted,
		CooldownUntil:     &cooldownUntil,
		GuardStrikes:      1,
	}).Error; err != nil {
		t.Fatalf("create upstream state: %v", err)
	}
	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypePrivate,
		CredentialID:    &credentialID,
		ChannelProvider: "deepseek",
	}
	router := &ChannelRouter{upstreamState: upstreamstate.NewService(db, stubCryptoService{})}

	eligible, guarded := router.filterRoutesByUpstreamGuard(context.Background(), organizationID, []*channelmodel.LLMRoute{route}, true, true)
	if len(eligible) != 1 || guarded != 0 {
		t.Fatalf("filter result = %d/%d, want route allowed while guard is off", len(eligible), guarded)
	}
	if !eligible[0].UpstreamWouldGuard || eligible[0].UpstreamHalfOpen {
		t.Fatalf("route evidence = would_guard:%t half_open:%t, want recovery evidence only", eligible[0].UpstreamWouldGuard, eligible[0].UpstreamHalfOpen)
	}
}

func TestUpstreamGuardAutomaticHalfOpenRequiresHealthyBackup(t *testing.T) {
	db := openGatewayUpstreamGuardDB(t)
	oldConfig := config.GlobalConfig
	config.GlobalConfig = &config.Config{LLM: config.LLMConfig{
		UpstreamGuardMode:       "enforce",
		UpstreamGuardPercentage: 100,
	}}
	t.Cleanup(func() { config.GlobalConfig = oldConfig })

	organizationID := uuid.New()
	blockedCredentialID := uuid.New()
	healthyCredentialID := uuid.New()
	cooldownEnded := time.Now().Add(-time.Minute)
	states := []*upstreamstate.State{
		{
			CredentialID: blockedCredentialID, OrganizationID: organizationID, Generation: 1,
			BalanceCapability: upstreamstate.BalanceCapabilityUnsupported,
			Availability:      upstreamstate.AvailabilityExhausted, LastCheckStatus: upstreamstate.CheckStatusUnsupported,
			WarningThresholds: []upstreamstate.WarningThreshold{}, BlockReason: upstreamstate.GuardReasonBillingUnavailable,
			CooldownUntil: &cooldownEnded, GuardStrikes: 1,
		},
		{
			CredentialID: healthyCredentialID, OrganizationID: organizationID, Generation: 1,
			BalanceCapability: upstreamstate.BalanceCapabilityUnsupported,
			Availability:      upstreamstate.AvailabilityUnknown, LastCheckStatus: upstreamstate.CheckStatusUnsupported,
			WarningThresholds: []upstreamstate.WarningThreshold{},
		},
	}
	for _, state := range states {
		if err := db.Create(state).Error; err != nil {
			t.Fatalf("create state: %v", err)
		}
	}
	blockedRoute := &channelmodel.LLMRoute{ID: uuid.New(), OrganizationID: organizationID, Type: shared.RouteTypePrivate, CredentialID: &blockedCredentialID, ChannelProvider: "qwen"}
	healthyRoute := &channelmodel.LLMRoute{ID: uuid.New(), OrganizationID: organizationID, Type: shared.RouteTypePrivate, CredentialID: &healthyCredentialID, ChannelProvider: "qwen"}
	router := &ChannelRouter{upstreamState: upstreamstate.NewService(db, stubCryptoService{})}

	withoutBackup, guarded := router.filterRoutesByUpstreamGuard(context.Background(), organizationID, []*channelmodel.LLMRoute{blockedRoute}, true, true)
	if len(withoutBackup) != 0 || guarded != 1 {
		t.Fatalf("without backup = %d/%d, want blocked", len(withoutBackup), guarded)
	}
	invalidPrivateRoute := &channelmodel.LLMRoute{ID: uuid.New(), OrganizationID: organizationID, Type: shared.RouteTypePrivate, ChannelProvider: "qwen"}
	withInvalidBackup, guarded := router.filterRoutesByUpstreamGuard(context.Background(), organizationID, []*channelmodel.LLMRoute{blockedRoute, invalidPrivateRoute}, true, true)
	if len(withInvalidBackup) != 1 || guarded != 1 || withInvalidBackup[0].ID != invalidPrivateRoute.ID {
		t.Fatalf("invalid backup = %#v/%d, want guarded credential and unchanged invalid route", withInvalidBackup, guarded)
	}

	withoutFallback, guarded := router.filterRoutesByUpstreamGuard(context.Background(), organizationID, []*channelmodel.LLMRoute{blockedRoute, healthyRoute}, true, false)
	if len(withoutFallback) != 1 || guarded != 1 || withoutFallback[0].ID != healthyRoute.ID {
		t.Fatalf("without fallback support = %#v/%d, want only healthy route", withoutFallback, guarded)
	}
	if blockedRoute.UpstreamHalfOpen {
		t.Fatal("blocked route received half-open lease without request fallback support")
	}

	withBackup, guarded := router.filterRoutesByUpstreamGuard(context.Background(), organizationID, []*channelmodel.LLMRoute{blockedRoute, healthyRoute}, true, true)
	if len(withBackup) != 2 || guarded != 0 {
		t.Fatalf("with backup = %d/%d, want probe candidate plus healthy", len(withBackup), guarded)
	}
	if !blockedRoute.UpstreamProbe || blockedRoute.UpstreamHalfOpen || !blockedRoute.UpstreamWouldGuard {
		t.Fatalf("blocked route evidence = probe:%t half_open:%t would_guard:%t", blockedRoute.UpstreamProbe, blockedRoute.UpstreamHalfOpen, blockedRoute.UpstreamWouldGuard)
	}
	stored, err := upstreamstate.NewRepository(db).Get(context.Background(), organizationID, blockedCredentialID)
	if err != nil {
		t.Fatalf("load blocked state: %v", err)
	}
	if stored.HalfOpenLeaseUntil != nil {
		t.Fatalf("route filtering acquired half-open lease until %v", stored.HalfOpenLeaseUntil)
	}
}

func TestSelectRoutesByPriorityAndWeightPutsProbeBeforeFallback(t *testing.T) {
	router := &ChannelRouter{}
	probe := &channelmodel.LLMRoute{ID: uuid.New(), Priority: 1, Weight: 1, UpstreamProbe: true}
	fallback := &channelmodel.LLMRoute{ID: uuid.New(), Priority: 100, Weight: 1}

	selected := router.selectRoutesByPriorityAndWeight([]*channelmodel.LLMRoute{fallback, probe}, 2)
	if len(selected) != 2 || selected[0].ID != probe.ID || selected[1].ID != fallback.ID {
		t.Fatalf("selected routes = %#v, want probe then fallback", selected)
	}
}

func TestFilterRoutesForModelScene_AgentUsesCatalogTagForOfficialAndPrivateRoutes(t *testing.T) {
	official := &channelmodel.LLMRoute{
		ID:         uuid.New(),
		Type:       shared.RouteTypeZGICloud,
		IsOfficial: true,
		Models:     []string{"gpt-agent"},
	}
	private := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "openai-compatible",
		Models:          []string{"gpt-agent"},
	}
	modelRecord := &llmmodel.LLMModel{
		Model:    "gpt-agent",
		UseCases: llmmodel.StringArray{"text-chat", "function-calling", "agent"},
	}

	got := filterRoutesForModelScene([]*channelmodel.LLMRoute{official, private}, "gpt-agent", modelRecord, string(llmmodel.UseCaseAgent))
	if len(got) != 2 {
		t.Fatalf("agent routes = %#v, want official and private", got)
	}
}

func TestFilterRoutesForModelScene_AgentRejectsUntaggedOfficialModel(t *testing.T) {
	official := &channelmodel.LLMRoute{
		ID:         uuid.New(),
		Type:       shared.RouteTypeZGICloud,
		IsOfficial: true,
		Models:     []string{"gpt-workflow"},
	}
	modelRecord := &llmmodel.LLMModel{
		Model:    "gpt-workflow",
		UseCases: llmmodel.StringArray{"text-chat"},
	}

	got := filterRoutesForModelScene([]*channelmodel.LLMRoute{official}, "gpt-workflow", modelRecord, string(llmmodel.UseCaseAgent))
	if len(got) != 0 {
		t.Fatalf("agent routes = %#v, want none", got)
	}
}

func TestFilterRoutesForModelScene_AgentRejectsUnsupportedAdapter(t *testing.T) {
	google := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "google",
		Models:          []string{"gemini-agent"},
	}
	modelRecord := &llmmodel.LLMModel{
		Model:    "gemini-agent",
		UseCases: llmmodel.StringArray{"text-chat", "function-calling", "agent"},
	}

	got := filterRoutesForModelScene([]*channelmodel.LLMRoute{google}, "gemini-agent", modelRecord, string(llmmodel.UseCaseAgent))
	if len(got) != 0 {
		t.Fatalf("agent routes = %#v, want none for unsupported adapter", got)
	}
}

func TestFinalizeUpstreamProbeSelectionsRequiresBuiltFallback(t *testing.T) {
	probeCredentialID := uuid.New()
	probe := &ProviderSelection{
		RouteID:                     uuid.New(),
		CredentialID:                probeCredentialID,
		UpstreamProbe:               true,
		UpstreamProbeRequiresBackup: true,
	}
	if got := finalizeUpstreamProbeSelections([]*ProviderSelection{probe}); len(got) != 0 {
		t.Fatalf("probe without built fallback = %#v, want no selections", got)
	}

	fallback := &ProviderSelection{RouteID: uuid.New(), CredentialID: uuid.New()}
	got := finalizeUpstreamProbeSelections([]*ProviderSelection{fallback, probe})
	if len(got) != 2 || got[0] != probe || got[1] != fallback || !probe.UpstreamProbeHasBackup {
		t.Fatalf("probe with built fallback = %#v, want probe first with backup evidence", got)
	}

	manual := &ProviderSelection{RouteID: uuid.New(), CredentialID: uuid.New(), UpstreamProbe: true}
	got = finalizeUpstreamProbeSelections([]*ProviderSelection{manual})
	if len(got) != 1 || got[0] != manual || manual.UpstreamProbeHasBackup {
		t.Fatalf("manual probe without fallback = %#v, want one manual probe", got)
	}
}

func TestActivateUpstreamProbeAcquiresLeaseAtAttempt(t *testing.T) {
	db := openGatewayUpstreamGuardDB(t)
	organizationID := uuid.New()
	credentialID := uuid.New()
	cooldownEnded := time.Now().Add(-time.Minute)
	if err := db.Create(&upstreamstate.State{
		CredentialID: credentialID, OrganizationID: organizationID, Generation: 3,
		BalanceCapability: upstreamstate.BalanceCapabilityUnsupported,
		Availability:      upstreamstate.AvailabilityExhausted,
		LastCheckStatus:   upstreamstate.CheckStatusUnsupported,
		WarningThresholds: []upstreamstate.WarningThreshold{},
		BlockReason:       upstreamstate.GuardReasonBillingUnavailable,
		CooldownUntil:     &cooldownEnded,
		GuardStrikes:      1,
	}).Error; err != nil {
		t.Fatalf("create upstream state: %v", err)
	}

	service := &llmGatewayServiceImpl{upstreamState: upstreamstate.NewService(db, stubCryptoService{})}
	selection := &ProviderSelection{
		OrganizationID:         organizationID,
		CredentialID:           credentialID,
		CredentialGeneration:   3,
		ChannelProvider:        "qwen",
		UpstreamProbe:          true,
		UpstreamProbeHasBackup: true,
	}
	billingCtx := &BillingContext{}
	if err := service.activateUpstreamProbe(context.Background(), selection, billingCtx); err != nil {
		t.Fatalf("activateUpstreamProbe() error = %v", err)
	}
	if !selection.UpstreamHalfOpen || !billingCtx.UpstreamHalfOpen {
		t.Fatalf("half-open evidence = selection:%t billing:%t, want true", selection.UpstreamHalfOpen, billingCtx.UpstreamHalfOpen)
	}

	stored, err := upstreamstate.NewRepository(db).Get(context.Background(), organizationID, credentialID)
	if err != nil {
		t.Fatalf("load upstream state: %v", err)
	}
	if stored.HalfOpenLeaseUntil == nil || time.Until(*stored.HalfOpenLeaseUntil) < 10*time.Minute {
		t.Fatalf("half-open lease until = %v, want request-length lease", stored.HalfOpenLeaseUntil)
	}
}

func TestAllCandidateUpstreamBalancesLowRequiresEveryCandidate(t *testing.T) {
	db := openGatewayUpstreamGuardDB(t)
	organizationID := uuid.New()
	observedAt := time.Now()
	routes := make([]*channelmodel.LLMRoute, 0, 2)
	for index, remaining := range []string{"2", "3"} {
		credentialID := uuid.New()
		routes = append(routes, &channelmodel.LLMRoute{
			ID:             uuid.New(),
			OrganizationID: organizationID,
			Type:           shared.RouteTypePrivate,
			CredentialID:   &credentialID,
		})
		state := &upstreamstate.State{
			CredentialID:      credentialID,
			OrganizationID:    organizationID,
			Generation:        1,
			BalanceCapability: upstreamstate.BalanceCapabilitySupported,
			BalanceSnapshot: &upstreamstate.BalanceSnapshot{
				Scope: "account_balance",
				Items: []upstreamstate.BalanceAmount{{Currency: "USD", Remaining: remaining}},
			},
			BalanceObservedAt: &observedAt,
			WarningThresholds: []upstreamstate.WarningThreshold{{Currency: "USD", Amount: "5"}},
			Availability:      upstreamstate.AvailabilityAvailable,
			LastCheckStatus:   upstreamstate.CheckStatusSuccess,
		}
		if err := db.Create(state).Error; err != nil {
			t.Fatalf("create state %d: %v", index, err)
		}
	}
	svc := &llmGatewayServiceImpl{upstreamState: upstreamstate.NewService(db, stubCryptoService{})}

	allLow, err := svc.allCandidateUpstreamBalancesLow(context.Background(), organizationID, routes)
	if err != nil {
		t.Fatalf("allCandidateUpstreamBalancesLow() error = %v", err)
	}
	if !allLow {
		t.Fatal("allCandidateUpstreamBalancesLow() = false, want true")
	}

	if err := db.Model(&upstreamstate.State{}).
		Where("credential_id = ?", *routes[1].CredentialID).
		Update("balance_snapshot", `{"scope":"account_balance","items":[{"currency":"USD","remaining":"8"}]}`).Error; err != nil {
		t.Fatalf("raise second balance: %v", err)
	}
	allLow, err = svc.allCandidateUpstreamBalancesLow(context.Background(), organizationID, routes)
	if err != nil {
		t.Fatalf("allCandidateUpstreamBalancesLow() error = %v", err)
	}
	if allLow {
		t.Fatal("allCandidateUpstreamBalancesLow() = true with one healthy candidate")
	}

	official := &channelmodel.LLMRoute{ID: uuid.New(), OrganizationID: organizationID, Type: shared.RouteTypeZGICloud, IsOfficial: true}
	allLow, err = svc.allCandidateUpstreamBalancesLow(context.Background(), organizationID, append(routes, official))
	if err != nil {
		t.Fatalf("allCandidateUpstreamBalancesLow(mixed) error = %v", err)
	}
	if allLow {
		t.Fatal("allCandidateUpstreamBalancesLow() = true with official candidate")
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

func TestFilterRoutesForNativeProtocolOrError_ReportsUnsupportedProtocol(t *testing.T) {
	routes := []*channelmodel.LLMRoute{
		{
			ID:              uuid.New(),
			Type:            shared.RouteTypePrivate,
			ChannelProvider: "qwen",
		},
	}
	model := &llmmodel.LLMModel{
		Model:     "chat-only-model",
		Provider:  "qwen",
		Responses: false,
	}

	filtered, err := filterRoutesForNativeProtocolOrError(routes, model, modelCategoryResponses)
	if !adapter.IsCapabilityUnsupported(err) {
		t.Fatalf("error = %v, want ErrCapabilityUnsupported", err)
	}
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
