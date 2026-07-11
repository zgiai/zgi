package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmmodeldto "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/dto"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"
)

type availableModelRepoFake struct {
	models                []*llmmodel.LLMModel
	listAvailableByNames  int
	listAvailableFiltered int
}

func (f *availableModelRepoFake) Create(context.Context, *llmmodel.LLMModel) error {
	return errors.New("not implemented")
}
func (f *availableModelRepoFake) GetByID(_ context.Context, id uuid.UUID) (*llmmodel.LLMModel, error) {
	for _, m := range f.models {
		if m == nil {
			continue
		}
		if m.ID == id {
			return m, nil
		}
	}
	return nil, errors.New("not implemented")
}
func (f *availableModelRepoFake) GetByName(context.Context, string) (*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availableModelRepoFake) ListByNames(context.Context, []string) ([]*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availableModelRepoFake) ListAvailableByNames(context.Context, []string, string, string) ([]*llmmodel.LLMModel, error) {
	f.listAvailableByNames++
	return f.models, nil
}
func (f *availableModelRepoFake) ListAvailableFiltered(context.Context, string, string) ([]*llmmodel.LLMModel, error) {
	f.listAvailableFiltered++
	return f.models, nil
}
func (f *availableModelRepoFake) GetByProviderAndName(context.Context, string, string) (*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availableModelRepoFake) List(context.Context, *uuid.UUID, string, string, string, *bool, int, int) ([]*llmmodel.LLMModel, int64, error) {
	return f.models, int64(len(f.models)), nil
}
func (f *availableModelRepoFake) Update(context.Context, *llmmodel.LLMModel) error {
	return errors.New("not implemented")
}
func (f *availableModelRepoFake) Delete(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *availableModelRepoFake) ListByProvider(context.Context, string) ([]*llmmodel.LLMModel, error) {
	return nil, errors.New("not implemented")
}

type availableConfigRepoFake struct {
	configs              map[uuid.UUID]*llmmodel.ModelConfig
	upserts              []*llmmodel.ModelConfig
	err                  error
	listAvailableConfigs int
}

func (f *availableConfigRepoFake) Create(context.Context, *llmmodel.ModelConfig) error {
	return errors.New("not implemented")
}
func (f *availableConfigRepoFake) GetByID(context.Context, uuid.UUID, uuid.UUID) (*llmmodel.ModelConfig, error) {
	return nil, errors.New("not implemented")
}
func (f *availableConfigRepoFake) GetByModelID(_ context.Context, _ uuid.UUID, modelID uuid.UUID) (*llmmodel.ModelConfig, error) {
	if f.configs != nil {
		if cfg, ok := f.configs[modelID]; ok {
			return cfg, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *availableConfigRepoFake) List(context.Context, uuid.UUID, *bool, int, int) ([]*llmmodel.ModelConfig, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *availableConfigRepoFake) ListAvailableConfigs(context.Context, uuid.UUID) ([]*llmmodel.ModelConfig, error) {
	f.listAvailableConfigs++
	return nil, f.err
}
func (f *availableConfigRepoFake) Update(context.Context, *llmmodel.ModelConfig) error {
	return errors.New("not implemented")
}
func (f *availableConfigRepoFake) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *availableConfigRepoFake) Upsert(_ context.Context, config *llmmodel.ModelConfig) error {
	if f.err != nil {
		return f.err
	}
	if f.configs == nil {
		f.configs = make(map[uuid.UUID]*llmmodel.ModelConfig)
	}
	cloned := *config
	f.configs[config.ModelID] = &cloned
	f.upserts = append(f.upserts, &cloned)
	return nil
}
func (f *availableConfigRepoFake) BatchCreate(context.Context, []*llmmodel.ModelConfig) error {
	return errors.New("not implemented")
}

type availableCustomRepoFake struct {
	model     *llmmodel.CustomModel
	err       error
	deleted   bool
	listCalls int
}

func (f *availableCustomRepoFake) Create(context.Context, *llmmodel.CustomModel) error {
	return f.err
}
func (f *availableCustomRepoFake) GetByID(context.Context, uuid.UUID, uuid.UUID) (*llmmodel.CustomModel, error) {
	if f.model != nil {
		return f.model, nil
	}
	return nil, errors.New("not implemented")
}
func (f *availableCustomRepoFake) GetByProviderAndName(context.Context, uuid.UUID, uuid.UUID, string) (*llmmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availableCustomRepoFake) GetByProviderAndModel(context.Context, uuid.UUID, string, string) (*llmmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availableCustomRepoFake) ListByNames(context.Context, uuid.UUID, []string, *bool) ([]*llmmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}
func (f *availableCustomRepoFake) List(context.Context, uuid.UUID, *uuid.UUID, string, string, *bool, int, int) ([]*llmmodel.CustomModel, int64, error) {
	f.listCalls++
	return nil, 0, f.err
}
func (f *availableCustomRepoFake) Update(context.Context, *llmmodel.CustomModel) error {
	return f.err
}
func (f *availableCustomRepoFake) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	f.deleted = true
	return f.err
}
func (f *availableCustomRepoFake) ListByProvider(context.Context, uuid.UUID, uuid.UUID) ([]*llmmodel.CustomModel, error) {
	return nil, errors.New("not implemented")
}

type availableRouteRepoFake struct {
	routes       []*channelmodel.LLMRoute
	err          error
	enabledCalls int
}

func (f *availableRouteRepoFake) Create(context.Context, *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *availableRouteRepoFake) BatchCreate(context.Context, []*channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *availableRouteRepoFake) GetByID(context.Context, uuid.UUID, uuid.UUID) (*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}
func (f *availableRouteRepoFake) List(context.Context, uuid.UUID, *bool, int, int) ([]*channelmodel.LLMRoute, int64, error) {
	return nil, 0, errors.New("not implemented")
}
func (f *availableRouteRepoFake) Update(context.Context, *channelmodel.LLMRoute) error {
	return errors.New("not implemented")
}
func (f *availableRouteRepoFake) Delete(context.Context, uuid.UUID, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *availableRouteRepoFake) GetEnabledRoutes(context.Context, uuid.UUID) ([]*channelmodel.LLMRoute, error) {
	f.enabledCalls++
	return f.routes, f.err
}
func (f *availableRouteRepoFake) FindByModel(context.Context, uuid.UUID, string) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}
func (f *availableRouteRepoFake) CountByCredentialID(context.Context, uuid.UUID, uuid.UUID) (int64, error) {
	return 0, errors.New("not implemented")
}
func (f *availableRouteRepoFake) GetDistinctProviders(context.Context, uuid.UUID) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (f *availableRouteRepoFake) GetPlatformChannels(context.Context) ([]*channelmodel.LLMRoute, error) {
	return nil, errors.New("not implemented")
}

type availableOfficialBootstrapFake struct {
	calls int
}

func (f *availableOfficialBootstrapFake) InitOfficialChannel(context.Context, uuid.UUID) error {
	f.calls++
	return nil
}

type modelServiceAvailableModelsFake struct {
	invalidated []uuid.UUID
}

func (f *modelServiceAvailableModelsFake) ListAvailable(context.Context, uuid.UUID, string, string) ([]*AvailableModel, error) {
	return nil, errors.New("not implemented")
}
func (f *modelServiceAvailableModelsFake) RefreshCache(context.Context, uuid.UUID) error {
	return errors.New("not implemented")
}
func (f *modelServiceAvailableModelsFake) InvalidateTenantCache(organizationID uuid.UUID) {
	f.invalidated = append(f.invalidated, organizationID)
}
func (f *modelServiceAvailableModelsFake) InvalidateGlobalCache() {}
func (f *modelServiceAvailableModelsFake) SetOfficialRouteBootstrapper(interfaces.OfficialRouteBootstrapper) {
}

func TestAvailableModels_ReturnsRouteRepoError(t *testing.T) {
	wantErr := errors.New("route repo down")
	svc := NewAvailableModelsService(
		&availableModelRepoFake{},
		&availableConfigRepoFake{},
		&availableCustomRepoFake{},
		&availableRouteRepoFake{err: wantErr},
	)

	_, err := svc.ListAvailable(context.Background(), uuid.New(), "", "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("ListAvailable error = %v, want %v", err, wantErr)
	}
}

func TestAvailableModels_ListAvailableDoesNotBootstrapOfficialRoute(t *testing.T) {
	bootstrapper := &availableOfficialBootstrapFake{}
	svc := NewAvailableModelsService(
		&availableModelRepoFake{},
		&availableConfigRepoFake{},
		&availableCustomRepoFake{},
		&availableRouteRepoFake{},
	)
	svc.SetOfficialRouteBootstrapper(bootstrapper)

	_, err := svc.ListAvailable(context.Background(), uuid.New(), "", "")
	if err != nil {
		t.Fatalf("ListAvailable returned error: %v", err)
	}
	if bootstrapper.calls != 0 {
		t.Fatalf("official bootstrap calls = %d, want 0", bootstrapper.calls)
	}
}

func TestAvailableModels_ReturnsCustomModelLoadError(t *testing.T) {
	wantErr := errors.New("custom repo down")
	svc := NewAvailableModelsService(
		&availableModelRepoFake{},
		&availableConfigRepoFake{},
		&availableCustomRepoFake{err: wantErr},
		&availableRouteRepoFake{routes: []*channelmodel.LLMRoute{{Models: []string{"*"}}}},
	)

	_, err := svc.ListAvailable(context.Background(), uuid.New(), "", "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("ListAvailable error = %v, want %v", err, wantErr)
	}
}

func TestAvailableModels_ReturnsConfigRefreshErrorInsteadOfStaleCache(t *testing.T) {
	organizationID := uuid.New()
	wantErr := errors.New("config repo down")
	svc := &availableModelsService{
		globalRepo: &availableModelRepoFake{},
		configRepo: &availableConfigRepoFake{err: wantErr},
		customRepo: &availableCustomRepoFake{},
		routeRepo:  &availableRouteRepoFake{routes: []*channelmodel.LLMRoute{{Models: []string{"*"}}}},
		tenantCache: map[uuid.UUID]*tenantCacheEntry{
			organizationID: {
				configs:   map[uuid.UUID]*llmmodel.ModelConfig{},
				updatedAt: time.Now().Add(-time.Hour),
			},
		},
		tenantCacheTTL:    time.Minute,
		availableCache:    make(map[availableModelsCacheKey]*availableModelsCacheEntry),
		availableCacheTTL: time.Minute,
	}

	_, err := svc.ListAvailable(context.Background(), organizationID, "", "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("ListAvailable error = %v, want %v", err, wantErr)
	}
}

func TestAvailableModels_ListAvailableCachesResponseAndInvalidatesTenant(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	modelRepo := &availableModelRepoFake{models: []*llmmodel.LLMModel{{
		ID:                modelID,
		Provider:          "openai",
		Model:             "gpt-test",
		ModelName:         "GPT Test",
		IsActive:          true,
		UseCases:          types.StringArray{"text-chat"},
		ChatCompletions:   true,
		SupportsStreaming: true,
	}}}
	configRepo := &availableConfigRepoFake{}
	customRepo := &availableCustomRepoFake{}
	routeRepo := &availableRouteRepoFake{routes: []*channelmodel.LLMRoute{{
		Models: []string{"gpt-test"},
	}}}
	svc := NewAvailableModelsService(modelRepo, configRepo, customRepo, routeRepo)

	first, err := svc.ListAvailable(context.Background(), organizationID, " openai ", " text-chat ")
	if err != nil {
		t.Fatalf("first ListAvailable returned error: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first ListAvailable length = %d, want 1", len(first))
	}

	first[0].DisplayName = "mutated caller copy"
	second, err := svc.ListAvailable(context.Background(), organizationID, "openai", "text-chat")
	if err != nil {
		t.Fatalf("second ListAvailable returned error: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second ListAvailable length = %d, want 1", len(second))
	}
	if second[0].DisplayName != "GPT Test" {
		t.Fatalf("cached model display name = %q, want cloned cache value", second[0].DisplayName)
	}
	if routeRepo.enabledCalls != 1 {
		t.Fatalf("enabled route calls = %d, want 1", routeRepo.enabledCalls)
	}
	if modelRepo.listAvailableByNames != 1 {
		t.Fatalf("ListAvailableByNames calls = %d, want 1", modelRepo.listAvailableByNames)
	}
	if configRepo.listAvailableConfigs != 1 {
		t.Fatalf("ListAvailableConfigs calls = %d, want 1", configRepo.listAvailableConfigs)
	}
	if customRepo.listCalls != 1 {
		t.Fatalf("custom List calls = %d, want 1", customRepo.listCalls)
	}

	svc.InvalidateTenantCache(organizationID)
	_, err = svc.ListAvailable(context.Background(), organizationID, "openai", "text-chat")
	if err != nil {
		t.Fatalf("ListAvailable after invalidation returned error: %v", err)
	}
	if routeRepo.enabledCalls != 2 {
		t.Fatalf("enabled route calls after invalidation = %d, want 2", routeRepo.enabledCalls)
	}
	if configRepo.listAvailableConfigs != 2 {
		t.Fatalf("ListAvailableConfigs after invalidation = %d, want 2", configRepo.listAvailableConfigs)
	}
}

func TestAvailableModels_ListAvailableJSONCachesEncodedResponse(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	modelRepo := &availableModelRepoFake{models: []*llmmodel.LLMModel{{
		ID:                modelID,
		Provider:          "openai",
		Model:             "gpt-test",
		ModelName:         "GPT Test",
		IsActive:          true,
		UseCases:          types.StringArray{"text-chat"},
		ChatCompletions:   true,
		SupportsStreaming: true,
	}}}
	configRepo := &availableConfigRepoFake{}
	customRepo := &availableCustomRepoFake{}
	routeRepo := &availableRouteRepoFake{routes: []*channelmodel.LLMRoute{{
		Models: []string{"gpt-test"},
	}}}
	svc := NewAvailableModelsService(modelRepo, configRepo, customRepo, routeRepo)
	jsonSvc, ok := svc.(interface {
		ListAvailableJSON(context.Context, uuid.UUID, string, string) ([]byte, error)
	})
	if !ok {
		t.Fatal("available models service does not implement ListAvailableJSON")
	}

	first, err := jsonSvc.ListAvailableJSON(context.Background(), organizationID, " openai ", " text-chat ")
	if err != nil {
		t.Fatalf("first ListAvailableJSON returned error: %v", err)
	}
	var decoded struct {
		Code string `json:"code"`
		Data struct {
			Items []*AvailableModel `json:"items"`
			Total int               `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}
	if decoded.Code != "0" || decoded.Data.Total != 1 || len(decoded.Data.Items) != 1 {
		t.Fatalf("decoded response = code %q total %d items %d, want code 0 total 1 items 1", decoded.Code, decoded.Data.Total, len(decoded.Data.Items))
	}

	second, err := jsonSvc.ListAvailableJSON(context.Background(), organizationID, "openai", "text-chat")
	if err != nil {
		t.Fatalf("second ListAvailableJSON returned error: %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("cached JSON response changed between identical requests")
	}
	if routeRepo.enabledCalls != 1 {
		t.Fatalf("enabled route calls = %d, want 1", routeRepo.enabledCalls)
	}
	if modelRepo.listAvailableByNames != 1 {
		t.Fatalf("ListAvailableByNames calls = %d, want 1", modelRepo.listAvailableByNames)
	}
	if configRepo.listAvailableConfigs != 1 {
		t.Fatalf("ListAvailableConfigs calls = %d, want 1", configRepo.listAvailableConfigs)
	}
	if customRepo.listCalls != 1 {
		t.Fatalf("custom List calls = %d, want 1", customRepo.listCalls)
	}

	svc.InvalidateTenantCache(organizationID)
	if _, err := jsonSvc.ListAvailableJSON(context.Background(), organizationID, "openai", "text-chat"); err != nil {
		t.Fatalf("ListAvailableJSON after invalidation returned error: %v", err)
	}
	if routeRepo.enabledCalls != 2 {
		t.Fatalf("enabled route calls after invalidation = %d, want 2", routeRepo.enabledCalls)
	}
	if configRepo.listAvailableConfigs != 2 {
		t.Fatalf("ListAvailableConfigs after invalidation = %d, want 2", configRepo.listAvailableConfigs)
	}
}

func TestAvailableModels_ListAvailableJSONDoesNotExtendStructCacheTTL(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	modelRepo := &availableModelRepoFake{models: []*llmmodel.LLMModel{{
		ID:              modelID,
		Provider:        "openai",
		Model:           "gpt-test",
		ModelName:       "GPT Test",
		IsActive:        true,
		UseCases:        types.StringArray{"text-chat"},
		ChatCompletions: true,
	}}}
	configRepo := &availableConfigRepoFake{}
	customRepo := &availableCustomRepoFake{}
	routeRepo := &availableRouteRepoFake{routes: []*channelmodel.LLMRoute{{
		Models: []string{"gpt-test"},
	}}}
	svc := NewAvailableModelsService(modelRepo, configRepo, customRepo, routeRepo)
	concrete, ok := svc.(*availableModelsService)
	if !ok {
		t.Fatal("available models service has unexpected implementation")
	}
	concrete.availableCacheTTL = time.Hour

	if _, err := svc.ListAvailable(context.Background(), organizationID, "openai", "text-chat"); err != nil {
		t.Fatalf("ListAvailable returned error: %v", err)
	}
	if routeRepo.enabledCalls != 1 {
		t.Fatalf("enabled route calls after ListAvailable = %d, want 1", routeRepo.enabledCalls)
	}

	cacheKey := availableModelsCacheKey{
		organizationID: organizationID,
		provider:       "openai",
		useCase:        "text-chat",
	}
	sourceUpdatedAt := time.Now().Add(-30 * time.Minute)
	concrete.availableCacheMu.Lock()
	concrete.availableCache[cacheKey].updatedAt = sourceUpdatedAt
	concrete.availableCacheMu.Unlock()

	if _, err := concrete.ListAvailableJSON(context.Background(), organizationID, "openai", "text-chat"); err != nil {
		t.Fatalf("ListAvailableJSON returned error: %v", err)
	}
	if routeRepo.enabledCalls != 1 {
		t.Fatalf("enabled route calls after JSON cache fill = %d, want 1", routeRepo.enabledCalls)
	}

	concrete.availableResponseCacheMu.RLock()
	responseUpdatedAt := concrete.availableResponseCache[cacheKey].updatedAt
	concrete.availableResponseCacheMu.RUnlock()
	if !responseUpdatedAt.Equal(sourceUpdatedAt) {
		t.Fatalf("response cache updatedAt = %s, want source updatedAt %s", responseUpdatedAt, sourceUpdatedAt)
	}

	expiredUpdatedAt := time.Now().Add(-2 * time.Hour)
	concrete.availableCacheMu.Lock()
	concrete.availableCache[cacheKey].updatedAt = expiredUpdatedAt
	concrete.availableCacheMu.Unlock()
	concrete.availableResponseCacheMu.Lock()
	concrete.availableResponseCache[cacheKey].updatedAt = expiredUpdatedAt
	concrete.availableResponseCacheMu.Unlock()

	if _, err := concrete.ListAvailableJSON(context.Background(), organizationID, "openai", "text-chat"); err != nil {
		t.Fatalf("ListAvailableJSON after source TTL returned error: %v", err)
	}
	if routeRepo.enabledCalls != 2 {
		t.Fatalf("enabled route calls after source TTL = %d, want 2", routeRepo.enabledCalls)
	}
}

func TestCreateCustomInvalidatesAvailableModelsCache(t *testing.T) {
	organizationID := uuid.New()
	providerID := uuid.New()
	availableSvc := &modelServiceAvailableModelsFake{}
	svc := &modelService{
		customRepo:      &availableCustomRepoFake{},
		availableModels: availableSvc,
	}

	_, err := svc.CreateCustom(context.Background(), organizationID, &llmmodeldto.CreateCustomModelRequest{
		Provider:    "custom-openai",
		ProviderID:  &providerID,
		Name:        "custom-model",
		DisplayName: "Custom Model",
		UseCases:    []string{"chat"},
	})
	if err != nil {
		t.Fatalf("CreateCustom returned error: %v", err)
	}
	if len(availableSvc.invalidated) != 1 || availableSvc.invalidated[0] != organizationID {
		t.Fatalf("invalidated tenants = %v, want [%s]", availableSvc.invalidated, organizationID)
	}
}

func TestUpdateCustomInvalidatesAvailableModelsCache(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	availableSvc := &modelServiceAvailableModelsFake{}
	svc := &modelService{
		customRepo: &availableCustomRepoFake{model: &llmmodel.CustomModel{
			ID:             modelID,
			OrganizationID: organizationID,
			Name:           "custom-model",
			IsActive:       true,
		}},
		availableModels: availableSvc,
	}

	displayName := "Renamed Model"
	_, err := svc.UpdateCustom(context.Background(), organizationID, modelID, &llmmodeldto.UpdateCustomModelRequest{
		DisplayName: &displayName,
	})
	if err != nil {
		t.Fatalf("UpdateCustom returned error: %v", err)
	}
	if len(availableSvc.invalidated) != 1 || availableSvc.invalidated[0] != organizationID {
		t.Fatalf("invalidated tenants = %v, want [%s]", availableSvc.invalidated, organizationID)
	}
}

func TestDeleteCustomInvalidatesAvailableModelsCache(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	availableSvc := &modelServiceAvailableModelsFake{}
	customRepo := &availableCustomRepoFake{}
	svc := &modelService{
		customRepo:      customRepo,
		availableModels: availableSvc,
	}

	if err := svc.DeleteCustom(context.Background(), organizationID, modelID); err != nil {
		t.Fatalf("DeleteCustom returned error: %v", err)
	}
	if !customRepo.deleted {
		t.Fatalf("custom model was not deleted")
	}
	if len(availableSvc.invalidated) != 1 || availableSvc.invalidated[0] != organizationID {
		t.Fatalf("invalidated tenants = %v, want [%s]", availableSvc.invalidated, organizationID)
	}
}

func TestBatchToggleModelsPreservesPriceOverrides(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	inputPrice := decimal.RequireFromString("1.23")
	outputPrice := decimal.RequireFromString("4.56")
	configRepo := &availableConfigRepoFake{
		configs: map[uuid.UUID]*llmmodel.ModelConfig{
			modelID: {
				OrganizationID:      organizationID,
				ModelID:             modelID,
				IsEnabled:           true,
				AccessScope:         llmmodel.AccessScopeAll,
				InputPriceOverride:  &inputPrice,
				OutputPriceOverride: &outputPrice,
			},
		},
	}
	availableSvc := &modelServiceAvailableModelsFake{}
	svc := &modelService{
		configRepo:      configRepo,
		availableModels: availableSvc,
	}

	if err := svc.BatchToggleModels(context.Background(), organizationID, []uuid.UUID{modelID}, false); err != nil {
		t.Fatalf("BatchToggleModels returned error: %v", err)
	}
	if len(configRepo.upserts) != 1 {
		t.Fatalf("upsert count = %d, want 1", len(configRepo.upserts))
	}
	got := configRepo.upserts[0]
	if got.IsEnabled {
		t.Fatalf("is_enabled = true, want false")
	}
	if got.InputPriceOverride == nil || !got.InputPriceOverride.Equal(inputPrice) {
		t.Fatalf("input override = %v, want %s", got.InputPriceOverride, inputPrice)
	}
	if got.OutputPriceOverride == nil || !got.OutputPriceOverride.Equal(outputPrice) {
		t.Fatalf("output override = %v, want %s", got.OutputPriceOverride, outputPrice)
	}
	if len(availableSvc.invalidated) != 1 || availableSvc.invalidated[0] != organizationID {
		t.Fatalf("invalidated tenants = %v, want [%s]", availableSvc.invalidated, organizationID)
	}
}

func TestConfigureModelRejectsInvalidOverridePrice(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	badPrice := "abc"
	svc := &modelService{
		globalRepo: &availableModelRepoFake{models: []*llmmodel.LLMModel{{
			ID:       modelID,
			Provider: "openai",
			Model:    "gpt-test",
		}}},
		configRepo: &availableConfigRepoFake{},
	}

	_, err := svc.ConfigureModel(context.Background(), organizationID, &llmmodeldto.ConfigureModelRequest{
		ModelID:            modelID,
		InputPriceOverride: &badPrice,
	})
	if err == nil {
		t.Fatalf("ConfigureModel error = nil, want invalid price error")
	}
	if !strings.Contains(err.Error(), "input_price_override") {
		t.Fatalf("ConfigureModel error = %v, want input_price_override error", err)
	}
}

func TestModelAvailabilityBatchMatchesSingleTenantDisabledConfig(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	modelName := "gpt-test"
	modelRepo := &availableModelRepoFake{models: []*llmmodel.LLMModel{{
		ID:       modelID,
		Provider: "openai",
		Model:    modelName,
		IsActive: true,
		Status:   llmmodel.ModelStatusActive,
	}}}
	configRepo := &availableConfigRepoFake{
		configs: map[uuid.UUID]*llmmodel.ModelConfig{
			modelID: {
				ModelID:   modelID,
				IsEnabled: false,
			},
		},
	}
	routeRepo := &availableRouteRepoFake{
		routes: []*channelmodel.LLMRoute{{Models: []string{modelName}}},
	}
	svc := NewModelAvailabilityService(modelRepo, configRepo, routeRepo)

	single, err := svc.CheckModelAvailable(context.Background(), organizationID, modelID)
	if err != nil {
		t.Fatalf("CheckModelAvailable returned error: %v", err)
	}
	if single.Available {
		t.Fatalf("single availability = true, want false")
	}

	batch, err := svc.BatchCheckAvailability(context.Background(), organizationID, []string{modelName})
	if err != nil {
		t.Fatalf("BatchCheckAvailability returned error: %v", err)
	}
	got := batch.Items[modelName]
	if got == nil {
		t.Fatalf("batch availability missing item %q", modelName)
	}
	if got.Available != single.Available {
		t.Fatalf("batch availability = %v, want single availability %v", got.Available, single.Available)
	}
	if got.Message != single.Message {
		t.Fatalf("batch message = %q, want single message %q", got.Message, single.Message)
	}
}

func TestModelAvailabilityBatchMatchesSingleDeprecatedModel(t *testing.T) {
	organizationID := uuid.New()
	modelID := uuid.New()
	modelName := "gpt-old"
	modelRepo := &availableModelRepoFake{models: []*llmmodel.LLMModel{{
		ID:       modelID,
		Provider: "openai",
		Model:    modelName,
		IsActive: true,
		Status:   llmmodel.ModelStatusDeprecated,
	}}}
	routeRepo := &availableRouteRepoFake{
		routes: []*channelmodel.LLMRoute{{Models: []string{modelName}}},
	}
	svc := NewModelAvailabilityService(modelRepo, &availableConfigRepoFake{}, routeRepo)

	single, err := svc.CheckModelAvailable(context.Background(), organizationID, modelID)
	if err != nil {
		t.Fatalf("CheckModelAvailable returned error: %v", err)
	}
	if single.Available {
		t.Fatalf("single availability = true, want false")
	}

	batch, err := svc.BatchCheckAvailability(context.Background(), organizationID, []string{modelName})
	if err != nil {
		t.Fatalf("BatchCheckAvailability returned error: %v", err)
	}
	got := batch.Items[modelName]
	if got == nil {
		t.Fatalf("batch availability missing item %q", modelName)
	}
	if got.Available != single.Available {
		t.Fatalf("batch availability = %v, want single availability %v", got.Available, single.Available)
	}
	if got.Message != single.Message {
		t.Fatalf("batch message = %q, want single message %q", got.Message, single.Message)
	}
}

func TestContainsUseCaseKeepsImageModelsOutOfTextChat(t *testing.T) {
	if containsUseCase(types.StringArray{"image-gen"}, "text-chat") {
		t.Fatal("image-gen model must not match text-chat")
	}
	if !containsUseCase(types.StringArray{"image-gen"}, "image-gen") {
		t.Fatal("image-gen model must match image-gen")
	}
	if containsUseCase(types.StringArray{"text-chat"}, "image-gen") {
		t.Fatal("text-chat model must not match image-gen")
	}
}

func TestAvailableModelsObserveGenerationClearsLocalCaches(t *testing.T) {
	organizationID := uuid.New()
	cacheKey := availableModelsCacheKey{organizationID: organizationID}
	svc := &availableModelsService{
		tenantCache: map[uuid.UUID]*tenantCacheEntry{
			organizationID: {updatedAt: time.Now()},
		},
		availableCache: map[availableModelsCacheKey]*availableModelsCacheEntry{
			cacheKey: {updatedAt: time.Now()},
		},
		availableResponseCache: map[availableModelsCacheKey]*availableModelsResponseCacheEntry{
			cacheKey: {response: []byte(`{}`), updatedAt: time.Now()},
		},
	}

	svc.observeGeneration(organizationID, "0", "0")
	if len(svc.tenantCache) != 1 || len(svc.availableCache) != 1 || len(svc.availableResponseCache) != 1 {
		t.Fatal("first generation observation unexpectedly cleared local caches")
	}

	svc.observeGeneration(organizationID, "1", "0")
	if len(svc.tenantCache) != 0 || len(svc.availableCache) != 0 || len(svc.availableResponseCache) != 0 {
		t.Fatal("generation change did not clear local caches")
	}
}
