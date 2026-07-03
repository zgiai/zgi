package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
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

func (s *stubVectorDB) DeleteVector(ctx context.Context, id, className string) error {
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

func TestSearchMethodRoutingForVectorBM25Hybrid(t *testing.T) {
	tests := []struct {
		name                 string
		method               string
		wantNormalizedMethod string
		wantHybrid           bool
		wantBM25             bool
		wantKnown            bool
	}{
		{name: "default", method: "", wantNormalizedMethod: "hybrid_search", wantHybrid: true, wantKnown: true},
		{name: "hybrid", method: "hybrid_search", wantNormalizedMethod: "hybrid_search", wantHybrid: true, wantKnown: true},
		{name: "unknown defaults to hybrid", method: "legacy", wantNormalizedMethod: "hybrid_search", wantHybrid: true, wantKnown: false},
		{name: "keyword maps to BM25", method: "keyword_search", wantNormalizedMethod: "keyword_search", wantBM25: true, wantKnown: true},
		{name: "full text maps to BM25", method: "full_text_search", wantNormalizedMethod: "full_text_search", wantBM25: true, wantKnown: true},
		{name: "semantic single source", method: "semantic_search", wantNormalizedMethod: "semantic_search", wantKnown: true},
		{name: "graph single source", method: "graph_search", wantNormalizedMethod: "graph_search", wantKnown: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := normalizeVectorSearchMethod(tt.method)
			if normalized != tt.wantNormalizedMethod {
				t.Fatalf("normalizeVectorSearchMethod(%q) = %q, want %q", tt.method, normalized, tt.wantNormalizedMethod)
			}
			if got := isHybridVectorBM25Method(normalized); got != tt.wantHybrid {
				t.Fatalf("isHybridVectorBM25Method(%q) = %v, want %v", normalized, got, tt.wantHybrid)
			}
			if got := isBM25OnlyMethod(normalized); got != tt.wantBM25 {
				t.Fatalf("isBM25OnlyMethod(%q) = %v, want %v", normalized, got, tt.wantBM25)
			}
			if got := isKnownSearchMethod(tt.method); got != tt.wantKnown {
				t.Fatalf("isKnownSearchMethod(%q) = %v, want %v", tt.method, got, tt.wantKnown)
			}
		})
	}
}

func TestRetrievalSourceResponseForHybridEvidence(t *testing.T) {
	doc := retrieval.SearchResult{
		ID:      "chunk-1",
		Content: "invoice 2026",
		Metadata: map[string]interface{}{
			"retrieval_sources": []string{"vector", "bm25"},
			"matched_terms":     []string{"invoice", "2026"},
			"vector_score":      0.87,
			"bm25_score":        5.25,
			"vector_rank":       2,
			"bm25_rank":         1,
			"best_rank":         1,
			"score":             0.064,
			"fusion_score":      0.064,
			"rerank_score":      0.91,
			"final_score":       0.91,
		},
	}

	source := retrievalSourceResponseForDoc(doc, "semantic_search", "fallback")
	if source.Method != "hybrid_search" {
		t.Fatalf("Method = %q, want hybrid_search", source.Method)
	}
	if source.Reason == "fallback" || source.Reason == "" {
		t.Fatalf("expected hybrid reason, got %q", source.Reason)
	}
	if len(source.MatchedTerms) != 2 || source.MatchedTerms[0] != "invoice" || source.MatchedTerms[1] != "2026" {
		t.Fatalf("MatchedTerms = %#v, want invoice+2026", source.MatchedTerms)
	}
	if len(source.RetrievalSources) != 2 || source.RetrievalSources[0] != "vector" || source.RetrievalSources[1] != "bm25" {
		t.Fatalf("RetrievalSources = %#v, want vector+bm25", source.RetrievalSources)
	}
	if source.VectorScore == nil || *source.VectorScore != 0.87 {
		t.Fatalf("VectorScore = %#v, want 0.87", source.VectorScore)
	}
	if source.BM25Score == nil || *source.BM25Score != 5.25 {
		t.Fatalf("BM25Score = %#v, want 5.25", source.BM25Score)
	}
	if source.VectorRank == nil || *source.VectorRank != 2 {
		t.Fatalf("VectorRank = %#v, want 2", source.VectorRank)
	}
	if source.BM25Rank == nil || *source.BM25Rank != 1 {
		t.Fatalf("BM25Rank = %#v, want 1", source.BM25Rank)
	}
	if source.BestRank == nil || *source.BestRank != 1 {
		t.Fatalf("BestRank = %#v, want 1", source.BestRank)
	}
	if source.FusionScore == nil || *source.FusionScore != 0.064 {
		t.Fatalf("FusionScore = %#v, want 0.064", source.FusionScore)
	}
	if source.RerankScore == nil || *source.RerankScore != 0.91 {
		t.Fatalf("RerankScore = %#v, want 0.91", source.RerankScore)
	}
	if source.FinalScore == nil || *source.FinalScore != 0.91 {
		t.Fatalf("FinalScore = %#v, want 0.91", source.FinalScore)
	}
}

func TestHybridRecallCandidateLimit(t *testing.T) {
	if hybridRecallCandidateLimit != 50 {
		t.Fatalf("hybridRecallCandidateLimit = %d, want 50", hybridRecallCandidateLimit)
	}
}

func TestFilterAndLimitFinalRecordsSortsAndAppliesThresholdBeforeTopK(t *testing.T) {
	records := []dto.HitTestingRecordResponse{
		{Segment: dto.SegmentResponse{ID: "mid"}, Score: 0.72},
		{Segment: dto.SegmentResponse{ID: "below"}, Score: 0.49},
		{Segment: dto.SegmentResponse{ID: "high"}, Score: 0.91},
		{Segment: dto.SegmentResponse{ID: "low"}, Score: 0.30},
	}

	got := filterAndLimitFinalRecords(records, &RetrievalOptions{
		ScoreThresholdEnabled: true,
		ScoreThreshold:        0.5,
		TopK:                  2,
	})

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Segment.ID != "high" || got[1].Segment.ID != "mid" {
		t.Fatalf("got IDs = %q, %q; want high, mid", got[0].Segment.ID, got[1].Segment.ID)
	}
}

func TestFilterAndLimitFinalRecordsCanReturnFewerThanTopK(t *testing.T) {
	records := []dto.HitTestingRecordResponse{
		{Segment: dto.SegmentResponse{ID: "below-a"}, Score: 0.49},
		{Segment: dto.SegmentResponse{ID: "below-b"}, Score: 0.40},
	}

	got := filterAndLimitFinalRecords(records, &RetrievalOptions{
		ScoreThresholdEnabled: true,
		ScoreThreshold:        0.5,
		TopK:                  5,
	})

	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestFilterAndLimitFinalRecordsSkipsThresholdWhenDisabled(t *testing.T) {
	records := []dto.HitTestingRecordResponse{
		{Segment: dto.SegmentResponse{ID: "below-a"}, Score: 0.49},
		{Segment: dto.SegmentResponse{ID: "below-b"}, Score: 0.40},
	}

	got := filterAndLimitFinalRecords(records, &RetrievalOptions{
		ScoreThresholdEnabled: false,
		ScoreThreshold:        0.5,
		TopK:                  1,
	})

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Segment.ID != "below-a" {
		t.Fatalf("got ID = %q, want below-a", got[0].Segment.ID)
	}
}

func TestFilterAndLimitFinalRecordsKeepsRRFFallbackBelowThreshold(t *testing.T) {
	fusionScore := 0.0164
	bestRank := 1
	records := []dto.HitTestingRecordResponse{
		{
			Segment: dto.SegmentResponse{ID: "rrf-fallback"},
			Score:   fusionScore,
			RetrievalSource: &dto.RetrievalSourceResponse{
				FusionScore: &fusionScore,
				BestRank:    &bestRank,
			},
		},
	}

	got := filterAndLimitFinalRecords(records, &RetrievalOptions{
		ScoreThresholdEnabled: true,
		ScoreThreshold:        0.35,
		TopK:                  5,
	})

	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if got[0].Segment.ID != "rrf-fallback" {
		t.Fatalf("got ID = %q, want rrf-fallback", got[0].Segment.ID)
	}
}

func TestFilterAndLimitFinalRecordsAppliesThresholdToRerankScores(t *testing.T) {
	fusionScore := 0.0164
	rerankScore := 0.25
	bestRank := 1
	records := []dto.HitTestingRecordResponse{
		{
			Segment: dto.SegmentResponse{ID: "reranked-below-threshold"},
			Score:   rerankScore,
			RetrievalSource: &dto.RetrievalSourceResponse{
				FusionScore: &fusionScore,
				RerankScore: &rerankScore,
				BestRank:    &bestRank,
			},
		},
	}

	got := filterAndLimitFinalRecords(records, &RetrievalOptions{
		ScoreThresholdEnabled: true,
		ScoreThreshold:        0.35,
		TopK:                  5,
	})

	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestSplitRerankableSearchResultsPassesThroughGraphResults(t *testing.T) {
	docBacked := retrieval.SearchResult{
		ID: "doc-backed",
		Metadata: map[string]interface{}{
			"doc_id": "doc-backed",
		},
	}
	graphDoc := retrieval.SearchResult{
		ID: "graph",
		Metadata: map[string]interface{}{
			"source": "graph_knowledge",
		},
	}

	rerankable, passthrough := splitRerankableSearchResults([]retrieval.SearchResult{docBacked, graphDoc})

	if len(rerankable) != 1 || rerankable[0].ID != "doc-backed" {
		t.Fatalf("rerankable = %#v, want doc-backed only", rerankable)
	}
	if len(passthrough) != 1 || passthrough[0].ID != "graph" {
		t.Fatalf("passthrough = %#v, want graph only", passthrough)
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
