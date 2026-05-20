package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultmodel "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/model"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmsharedtypes "github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/embedding"
)

type mockEmbeddingService struct {
	called bool
	vec    []float64
	err    error
}

func (m *mockEmbeddingService) EmbedText(ctx context.Context, text string) ([]float64, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	if m.vec != nil {
		return m.vec, nil
	}
	return []float64{0.1, 0.2}, nil
}

func (m *mockEmbeddingService) EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return [][]float64{m.vec}, nil
}

func (m *mockEmbeddingService) GetDimension() int { return 0 }
func (m *mockEmbeddingService) GetModel() string  { return "mock-model" }

type stubVectorDB struct {
	lastVector []float64
}

func (s *stubVectorDB) StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error {
	return nil
}

func (s *stubVectorDB) SearchVectors(ctx context.Context, className string, vector []float64, limit int) ([]map[string]interface{}, error) {
	s.lastVector = vector
	return []map[string]interface{}{
		{
			"id":     "doc-1",
			"text":   "hello",
			"score":  0.9,
			"_extra": "ignored",
		},
	}, nil
}

func (s *stubVectorDB) SearchVectorsWithQuestions(ctx context.Context, className, questionClassName string, vector []float64, limit int) ([]map[string]interface{}, error) {
	return s.SearchVectors(ctx, className, vector, limit)
}

func (s *stubVectorDB) SearchByFullText(ctx context.Context, className, query string, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (s *stubVectorDB) CreateClass(ctx context.Context, className string, properties []map[string]interface{}) error {
	return nil
}

func (s *stubVectorDB) HealthCheck(ctx context.Context) error { return nil }

// mockDocumentRepo is unused in embeddingSearch but required to build the retrieval service.
func TestEmbeddingSearchUsesFactory(t *testing.T) {
	ctx := context.Background()
	vecDB := &stubVectorDB{}
	mockEmb := &mockEmbeddingService{vec: []float64{1, 2, 3}}
	vectorRetrieval := retrieval.NewVectorRetrievalService(mockEmb, vecDB, "")

	// Factory returns our mock embedding service
	factory := func(dataset *dataset_model.Dataset) embedding.EmbeddingService {
		return mockEmb
	}

	rs := NewRetrievalServiceWithEmbeddingFactory(
		nil,
		vectorRetrieval,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		factory,
	)

	ds := &dataset_model.Dataset{
		ID:          "ds-1",
		WorkspaceID: "tenant-1",
	}

	opts := &RetrievalOptions{
		SearchMethod:    "semantic_search",
		TopK:            5,
		RerankingEnable: false,
	}

	results, err := rs.embeddingSearch(ctx, ds, "hello", 5, 0.0, opts)
	if err != nil {
		t.Fatalf("embeddingSearch returned error: %v", err)
	}
	if !mockEmb.called {
		t.Fatalf("expected embedding service from factory to be used")
	}
	if len(vecDB.lastVector) == 0 {
		t.Fatalf("expected vector DB to receive embedding vector")
	}
	if len(results) == 0 || results[0].ID != "doc-1" {
		t.Fatalf("unexpected search results: %#v", results)
	}
}

func TestEmbeddingSearchFallbackErrorWhenNoFactoryOrDefaultModelService(t *testing.T) {
	ctx := context.Background()
	vecDB := &stubVectorDB{}
	vectorRetrieval := retrieval.NewVectorRetrievalService(nil, vecDB, "")

	rs := NewRetrievalService(
		nil,
		vectorRetrieval,
		nil,
		nil,
		nil,
		nil,
		nil, // defaultModelService nil triggers fallback error path
		nil,
		nil,
	)
	rs.SetLLMClient(&mockGraphLLMClient{})

	ds := &dataset_model.Dataset{
		ID:             "ds-2",
		WorkspaceID:    "tenant-2",
		OrganizationID: "org-2",
	}

	opts := &RetrievalOptions{
		SearchMethod:    "semantic_search",
		TopK:            5,
		RerankingEnable: false,
	}

	_, err := rs.embeddingSearch(ctx, ds, "hello", 5, 0.0, opts)
	if err == nil {
		t.Fatalf("expected error when default model service is nil and no factory provided")
	}
	if !strings.Contains(err.Error(), "failed to resolve embedding service") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type mockGraphLLMClient struct {
	lastOrganizationID string
}

type stubDefaultModelService struct{}

func (s *stubDefaultModelService) ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType shared_model.ModelType) (*llmdefaultservice.ResolvedModel, error) {
	return &llmdefaultservice.ResolvedModel{
		UseCase:  string(llmmodelmodel.UseCaseTextChat),
		Provider: "openai",
		Model:    "test-model",
		Params:   llmsharedtypes.JSONObject{},
		Source:   llmdefaultservice.SourceExplicit,
	}, nil
}

func (s *stubDefaultModelService) ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*llmdefaultservice.ResolvedModel, error) {
	return &llmdefaultservice.ResolvedModel{
		UseCase:  string(useCase),
		Provider: "openai",
		Model:    "test-model",
		Params:   llmsharedtypes.JSONObject{},
		Source:   llmdefaultservice.SourceExplicit,
	}, nil
}

func (s *stubDefaultModelService) ListResolved(ctx context.Context, organizationID uuid.UUID) ([]*llmdefaultservice.ResolvedModel, error) {
	return []*llmdefaultservice.ResolvedModel{}, nil
}

func (s *stubDefaultModelService) Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, useCase llmmodelmodel.UseCase, provider string, modelName string, params llmsharedtypes.JSONObject) (*llmdefaultmodel.DefaultModel, error) {
	return nil, nil
}

func (s *stubDefaultModelService) Delete(ctx context.Context, organizationID uuid.UUID, useCase llmmodelmodel.UseCase) error {
	return nil
}

func (m *mockGraphLLMClient) Chat(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	m.lastOrganizationID = organizationID
	return &llmadapter.ChatResponse{
		Choices: []llmadapter.Choice{
			{
				Message: llmadapter.Message{Content: `{"entities":[]}`},
			},
		},
	}, nil
}

func (m *mockGraphLLMClient) ChatStream(ctx context.Context, organizationID string, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) CreateResponse(ctx context.Context, organizationID string, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) Embed(ctx context.Context, organizationID string, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) CreateImage(ctx context.Context, organizationID string, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) Rerank(ctx context.Context, organizationID string, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) AppChat(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ChatRequest) (*llmadapter.ChatResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) AppChatStream(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ChatRequest) (<-chan llmadapter.StreamResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) AppCreateResponse(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.CreateResponseRequest) (*llmadapter.CreateResponseResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) AppEmbed(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.EmbeddingsRequest) (*llmadapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) AppCreateImage(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.ImageRequest) (*llmadapter.ImageResponse, error) {
	return nil, nil
}

func (m *mockGraphLLMClient) AppRerank(ctx context.Context, appCtx *llmclient.AppContext, req *llmadapter.RerankRequest) (*llmadapter.RerankResponse, error) {
	return nil, nil
}

func TestGraphSearchUsesDatasetOrganizationIDForEntityExtraction(t *testing.T) {
	mockClient := &mockGraphLLMClient{}
	graphFlowService := graphflow.NewService(
		&config.Config{},
		nil,
		nil,
		nil,
		mockClient,
		&stubDefaultModelService{},
		nil,
	)

	rs := &RetrievalService{
		graphFlowService: graphFlowService,
	}

	dataset := &dataset_model.Dataset{
		ID:              "ds-graph",
		WorkspaceID:     "ws-1",
		OrganizationID:  "org-1",
		EnableGraphFlow: true,
	}

	_, _, err := rs.graphSearch(context.Background(), dataset, "你好", &RetrievalOptions{TopK: 4})
	if err != nil {
		t.Fatalf("graphSearch returned error: %v", err)
	}

	if mockClient.lastOrganizationID != dataset.OrganizationID {
		t.Fatalf("graphSearch used organizationID %q, want %q", mockClient.lastOrganizationID, dataset.OrganizationID)
	}
}
