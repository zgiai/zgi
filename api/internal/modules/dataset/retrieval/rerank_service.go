package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/zgiai/ginext/internal/dto"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/ginext/internal/modules/llm/defaultmodel/service"
	llmruntime "github.com/zgiai/ginext/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/ginext/internal/modules/shared/model"
	"github.com/zgiai/ginext/internal/observability"
)

// RerankMode represents the reranking mode
type RerankMode string

const (
	// RERANKING_MODEL represents reranking using a model
	RERANKING_MODEL RerankMode = "reranking_model"
	// WEIGHTED_SCORE represents weighted score reranking
	WEIGHTED_SCORE RerankMode = "weighted_score"
)

type RerankService struct {
	httpClient *http.Client
}

type RerankModel struct {
	RerankingProviderName string `json:"reranking_provider_name"`
	RerankingModelName    string `json:"reranking_model_name"`
	APIEndpoint           string `json:"api_endpoint,omitempty"`
	APIKey                string `json:"api_key,omitempty"`
}

type RerankRequest struct {
	Query     string      `json:"query"`
	Documents []RerankDoc `json:"documents"`
	Model     string      `json:"model"`
	TopN      int         `json:"top_n"`
}

type RerankDoc struct {
	ID      string  `json:"id"`
	Content string  `json:"content"`
	Score   float64 `json:"score,omitempty"`
}

type RerankResponse struct {
	Results []RerankResult `json:"results"`
}

type RerankResult struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// DataPostProcessor
type DataPostProcessor struct {
	tenantID        string
	defaultModelSvc llmdefaultservice.DefaultModelService
	rerankMode      RerankMode
	rerankModel     *RerankModel
	weights         map[string]interface{}
	reorderEnabled  bool
	rerankRunner    BaseRerankRunner
	runnerErr       error
	// Gateway support fields
	llmClient llmclient.LLMClient
	accountID string
	appID     string
}

// NewDataPostProcessor
func NewDataPostProcessor(
	ctx context.Context,
	tenantID string,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	rerankMode RerankMode,
	rerankModel *RerankModel,
	weights map[string]interface{},

) *DataPostProcessor {
	d := &DataPostProcessor{
		tenantID:        tenantID,
		defaultModelSvc: defaultModelSvc,
		rerankMode:      rerankMode,
		rerankModel:     rerankModel,
		weights:         weights,
		reorderEnabled:  false,
	}

	d.rerankRunner, d.runnerErr = d.getRerankRunner(ctx, rerankMode, tenantID, rerankModel, weights)
	return d
}

// NewDataPostProcessorWithGateway creates a DataPostProcessor with gateway support.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewDataPostProcessorWithGateway(
	ctx context.Context,
	tenantID string,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	rerankMode RerankMode,
	rerankModel *RerankModel,
	weights map[string]interface{},
	llmClient llmclient.LLMClient,
	accountID string,
	appID string,
) *DataPostProcessor {
	d := &DataPostProcessor{
		tenantID:        tenantID,
		defaultModelSvc: defaultModelSvc,
		rerankMode:      rerankMode,
		rerankModel:     rerankModel,
		weights:         weights,
		reorderEnabled:  false,
		llmClient:       llmClient,
		accountID:       accountID,
		appID:           appID,
	}

	d.rerankRunner, d.runnerErr = d.getRerankRunnerWithGateway(ctx, rerankMode, tenantID, rerankModel, weights)
	return d
}

// Invoke
func (d *DataPostProcessor) Invoke(
	ctx context.Context,
	query string,
	documents []dto.Document,
	scoreThreshold *float64,
	topN *int,
) ([]dto.Document, error) {
	if d.runnerErr != nil {
		return nil, d.runnerErr
	}

	// use reranker
	if d.rerankRunner != nil {
		rerankedDocs, err := d.rerankRunner.Run(ctx, query, documents, scoreThreshold, topN, nil)
		if err != nil {
			return nil, err
		}
		return rerankedDocs, nil
	}

	if d.rerankMode == RERANKING_MODEL {
		return nil, fmt.Errorf("rerank runner is not configured")
	}

	// score threshold
	if scoreThreshold != nil {
		filteredDocs := make([]dto.Document, 0)
		for _, doc := range documents {
			score, ok := doc.Metadata["score"].(float64)
			if ok && score >= *scoreThreshold {
				filteredDocs = append(filteredDocs, doc)
			}
		}
		documents = filteredDocs
	}

	// topN
	if topN != nil && *topN > 0 && len(documents) > *topN {
		documents = documents[:*topN]
	}

	return documents, nil
}

func (d *DataPostProcessor) resolveRerankModel(
	ctx context.Context,
	tenantID string,
) (*llmruntime.ResolvedModel, error) {
	if d.defaultModelSvc == nil {
		return nil, fmt.Errorf("model manager is not configured")
	}

	explicitProvider := ""
	explicitModel := ""
	if d.rerankModel != nil {
		explicitProvider = d.rerankModel.RerankingProviderName
		explicitModel = d.rerankModel.RerankingModelName
	}

	return llmruntime.NewModelResolver(d.defaultModelSvc).Resolve(
		ctx,
		tenantID,
		explicitProvider,
		explicitModel,
		shared_model.ModelTypeRerank,
	)
}

func (d *DataPostProcessor) getRerankRunner(
	ctx context.Context,
	rerankingMode RerankMode,
	tenantId string,
	rerankingModel *RerankModel,
	weights map[string]interface{},
) (BaseRerankRunner, error) {
	if rerankingMode == WEIGHTED_SCORE && weights != nil {
		factory := &RerankRunnerFactory{}

		// Convert weights map to Weights struct
		weightStruct := &Weights{}

		// Handle vector setting
		if vectorSetting, ok := weights["vector_setting"].(map[string]interface{}); ok {
			weightStruct.VectorSetting.VectorWeight = 0.3
			if weight, ok := vectorSetting["vector_weight"].(float64); ok {
				weightStruct.VectorSetting.VectorWeight = weight
			}

			if providerName, ok := vectorSetting["embedding_provider_name"].(string); ok {
				weightStruct.VectorSetting.EmbeddingProviderName = providerName
			}

			if modelName, ok := vectorSetting["embedding_model_name"].(string); ok {
				weightStruct.VectorSetting.EmbeddingModelName = modelName
			}
		} else {
			// Default vector settings
			weightStruct.VectorSetting = VectorSetting{
				VectorWeight:          0.3,
				EmbeddingProviderName: "openai",
				EmbeddingModelName:    "text-embedding-ada-002",
			}
		}

		// Handle keyword setting
		if keywordSetting, ok := weights["keyword_setting"].(map[string]interface{}); ok {
			weightStruct.KeywordSetting.KeywordWeight = 0.7
			if weight, ok := keywordSetting["keyword_weight"].(float64); ok {
				weightStruct.KeywordSetting.KeywordWeight = weight
			}
		} else {
			// Default keyword settings
			weightStruct.KeywordSetting = KeywordSetting{
				KeywordWeight: 0.7,
			}
		}

		runner, err := factory.CreateRerankRunner(WEIGHTED_SCORE, weightStruct, tenantId)
		if err != nil {
			return nil, err
		}
		return runner, nil
	} else if rerankingMode == RERANKING_MODEL {
		return nil, fmt.Errorf("gateway llm client is required for reranking")
	}
	return nil, nil
}

// getRerankRunnerWithGateway creates a rerank runner with gateway support
func (d *DataPostProcessor) getRerankRunnerWithGateway(
	ctx context.Context,
	rerankingMode RerankMode,
	tenantId string,
	rerankingModel *RerankModel,
	weights map[string]interface{},
) (BaseRerankRunner, error) {
	if rerankingMode == WEIGHTED_SCORE && weights != nil {
		// Use the same logic as getRerankRunner for WEIGHTED_SCORE
		return d.getRerankRunner(ctx, rerankingMode, tenantId, rerankingModel, weights)
	}

	if rerankingMode == RERANKING_MODEL {
		if d.llmClient == nil {
			return nil, fmt.Errorf("llm client is not configured")
		}

		resolvedModel, err := d.resolveRerankModel(ctx, tenantId)
		if err != nil {
			return nil, err
		}

		return NewRerankModelRunner(d.llmClient, d.accountID, d.appID, resolvedModel.Model, tenantId), nil
	}
	return nil, nil
}

// getTopNValue
func getTopNValue(topN *int, docLen int) int {
	if topN != nil {
		return *topN
	}
	return docLen
}

func NewRerankService() *RerankService {
	return &RerankService{
		httpClient: observability.HTTPClient(&http.Client{
			Timeout: 30 * time.Second,
		}),
	}
}

func (s *RerankService) Rerank(ctx context.Context, query string, documents []SearchResult, model *RerankModel, topN int) ([]SearchResult, error) {
	if model == nil || model.RerankingProviderName == "" || model.RerankingModelName == "" {
		return documents, nil
	}

	rerankDocs := make([]RerankDoc, len(documents))
	for i, doc := range documents {
		rerankDocs[i] = RerankDoc{
			ID:      doc.ID,
			Content: doc.Content,
			Score:   doc.Score,
		}
	}

	rerankReq := RerankRequest{
		Query:     query,
		Documents: rerankDocs,
		Model:     model.RerankingModelName,
		TopN:      topN,
	}

	rerankedDocs, err := s.callExternalRerankService(ctx, rerankReq, model)
	if err != nil {
		return s.localWeightedRerank(query, documents), nil
	}

	resultMap := make(map[string]SearchResult)
	for _, doc := range documents {
		resultMap[doc.ID] = doc
	}

	var rerankedResults []SearchResult
	for _, rerankedDoc := range rerankedDocs {
		if doc, exists := resultMap[rerankedDoc.ID]; exists {
			doc.Score = rerankedDoc.Score
			rerankedResults = append(rerankedResults, doc)
		}
	}

	sort.Slice(rerankedResults, func(i, j int) bool {
		return rerankedResults[i].Score > rerankedResults[j].Score
	})

	return rerankedResults, nil
}

func (s *RerankService) callExternalRerankService(ctx context.Context, req RerankRequest, model *RerankModel) ([]RerankDoc, error) {
	switch model.RerankingProviderName {
	case "openai":
		return s.callOpenAIRerank(ctx, req, model)
	case "cohere":
		return s.callCohereRerank(ctx, req, model)
	case "bge":
		return s.callBGERerank(ctx, req, model)
	default:
		return nil, fmt.Errorf("unsupported reranking provider: %s", model.RerankingProviderName)
	}
}

func (s *RerankService) callOpenAIRerank(ctx context.Context, req RerankRequest, model *RerankModel) ([]RerankDoc, error) {
	return nil, fmt.Errorf("OpenAI rerank not implemented yet")
}

func (s *RerankService) callCohereRerank(ctx context.Context, req RerankRequest, model *RerankModel) ([]RerankDoc, error) {
	return nil, fmt.Errorf("Cohere rerank not implemented yet")
}

func (s *RerankService) callBGERerank(ctx context.Context, req RerankRequest, model *RerankModel) ([]RerankDoc, error) {
	return nil, fmt.Errorf("BGE rerank not implemented yet")
}

func (s *RerankService) localWeightedRerank(query string, documents []SearchResult) []SearchResult {
	results := make([]SearchResult, len(documents))
	copy(results, documents)

	queryWords := tokenizeQuery(query)

	for i := range results {
		contentWords := tokenizeQuery(results[i].Content)

		matchScore := calculateWordMatchScore(queryWords, contentWords)

		results[i].Score = results[i].Score*0.7 + matchScore*0.3
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results
}

func tokenizeQuery(query string) []string {
	words := make([]string, 0)
	return words
}

func calculateWordMatchScore(queryWords, contentWords []string) float64 {
	if len(queryWords) == 0 || len(contentWords) == 0 {
		return 0.0
	}

	matched := 0
	for _, qw := range queryWords {
		for _, cw := range contentWords {
			if qw == cw {
				matched++
				break
			}
		}
	}

	return float64(matched) / float64(len(queryWords))
}

func (s *RerankService) makeHTTPRequest(ctx context.Context, method, url string, body interface{}, headers map[string]string) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
