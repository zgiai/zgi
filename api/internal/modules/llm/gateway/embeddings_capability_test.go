package gateway

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	_ "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters/provider"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

func TestCreateEmbeddingsRejectsModelWithoutEmbeddingsCapabilityBeforeDispatch(t *testing.T) {
	organizationID := uuid.New()
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()
	setSpeechGatewayTestConfig(t, server.URL)

	service := newEmbeddingsGatewayTestService(t, organizationID, "gpt-4o", false)
	_, err := service.CreateEmbeddings(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &adapter.EmbeddingsRequest{Model: "gpt-4o", Input: "hello"})
	if !adapter.IsCapabilityUnsupported(err) {
		t.Fatalf("CreateEmbeddings() error = %v, want capability unsupported", err)
	}
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("request count = %d, want fail before dispatch", got)
	}
}

func TestCreateEmbeddingsAllowsEmbeddingsCapableModel(t *testing.T) {
	organizationID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	setSpeechGatewayTestConfig(t, server.URL)

	service := newEmbeddingsGatewayTestService(t, organizationID, "text-embedding-3-small", true)
	_, err := service.CreateEmbeddings(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &adapter.EmbeddingsRequest{Model: "text-embedding-3-small", Input: "hello"})
	if adapter.IsCapabilityUnsupported(err) {
		t.Fatalf("CreateEmbeddings() error = %v, want embeddings-capable model to pass the capability check", err)
	}
}

func newEmbeddingsGatewayTestService(t *testing.T, organizationID uuid.UUID, modelName string, embeddings bool) *llmGatewayServiceImpl {
	t.Helper()
	db := openGatewayCatalogDB(t)
	insertGatewayCatalogModel(t, db, uuid.New(), "openai", modelName)
	if err := db.Model(&llmmodel.LLMModel{}).
		Where("name = ?", modelName).
		Updates(map[string]any{
			"embeddings":       embeddings,
			"chat_completions": !embeddings,
		}).Error; err != nil {
		t.Fatalf("update embeddings model: %v", err)
	}

	route := &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  organizationID,
		Type:            shared.RouteTypeZGICloud,
		ChannelProvider: "zgi-cloud",
		IsOfficial:      true,
		IsEnabled:       true,
		Models:          []string{modelName},
		OfficialProviderModels: []channelmodel.ProviderModel{{
			Provider: "openai",
			Model:    modelName,
		}},
	}
	router := &ChannelRouter{
		db:                      db,
		organizationIDRouteRepo: &fakeCandidateRouteRepo{routes: []*channelmodel.LLMRoute{route}},
		cryptoService:           stubCryptoService{},
		strategyFactory:         NewStrategyFactory(),
		privateModels:           &fakePrivateModelLookup{},
	}
	return &llmGatewayServiceImpl{
		db:             db,
		adapterFactory: adapter.GlobalFactory,
		channelRouter:  router,
		tokenEstimator: NewTokenEstimator(),
	}
}

func TestCreateEmbeddingsKeepsPassthroughModelWithoutCatalogCapabilities(t *testing.T) {
	organizationID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	setSpeechGatewayTestConfig(t, server.URL)

	modelName := "tenant-embedding-model"
	route := &channelmodel.LLMRoute{
		ID:                     uuid.New(),
		OrganizationID:         organizationID,
		Type:                   shared.RouteTypeZGICloud,
		ChannelProvider:        "zgi-cloud",
		IsOfficial:             true,
		IsEnabled:              true,
		Models:                 []string{modelName},
		OfficialProviderModels: []channelmodel.ProviderModel{{Provider: "openai", Model: modelName}},
	}
	db := openGatewayCatalogDB(t)
	service := &llmGatewayServiceImpl{
		db:             db,
		adapterFactory: adapter.GlobalFactory,
		channelRouter: &ChannelRouter{
			db:                      db,
			organizationIDRouteRepo: &fakeCandidateRouteRepo{routes: []*channelmodel.LLMRoute{route}},
			cryptoService:           stubCryptoService{},
			strategyFactory:         NewStrategyFactory(),
			privateModels:           &fakePrivateModelLookup{},
		},
		tokenEstimator: NewTokenEstimator(),
	}
	_, err := service.CreateEmbeddings(t.Context(), &apikeymodel.TenantAPIKey{
		ID:             uuid.NewString(),
		OrganizationID: organizationID.String(),
	}, &adapter.EmbeddingsRequest{Model: modelName, Provider: "openai", Input: "hello"})
	if adapter.IsCapabilityUnsupported(err) {
		t.Fatalf("CreateEmbeddings() error = %v, want passthrough model to skip the catalog capability check", err)
	}
}
