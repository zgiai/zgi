package retrieval

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/tokenization"
)

// BaseRerankRunner defines the interface for rerank runners
type BaseRerankRunner interface {
	// Run executes the rerank model
	// query: search query
	// documents: documents for reranking
	// scoreThreshold: score threshold (optional)
	// topN: top n results (optional)
	// user: unique user id if needed (optional)
	Run(ctx context.Context, query string, documents []dto.Document, scoreThreshold *float64, topN *int, user *string) ([]dto.Document, error)
}

// BaseRerankRunnerImpl provides a base implementation of BaseRerankRunner
type BaseRerankRunnerImpl struct{}

// Run executes the rerank model
// query: search query
// documents: documents for reranking
// scoreThreshold: score threshold (optional)
// topN: top n results (optional)
// user: unique user id if needed (optional)
func (b *BaseRerankRunnerImpl) Run(ctx context.Context, query string, documents []dto.Document, scoreThreshold *float64, topN *int, user *string) ([]dto.Document, error) {
	// This is a base implementation that should be overridden by specific rerank runners
	// Return an explicit unsupported response instead of raising an exception
	// We'll just return the documents as is in the base implementation
	return documents, nil
}

// RerankRunnerFactory is a factory for creating rerank runners
type RerankRunnerFactory struct{}

// CreateRerankRunner creates a rerank runner based on the runner type
func (f *RerankRunnerFactory) CreateRerankRunner(runnerType RerankMode, args ...interface{}) (BaseRerankRunner, error) {
	switch runnerType {
	case WEIGHTED_SCORE:
		// Create WeightedScoreRunner
		// args[0] should be *Weights
		// args[1] should be tenantId (string)
		if len(args) < 1 {
			return nil, fmt.Errorf("insufficient arguments for WEIGHTED_SCORE runner")
		}

		weights, ok := args[0].(*Weights)
		if !ok {
			return nil, fmt.Errorf("invalid arguments for WEIGHTED_SCORE runner, first argument should be *Weights")
		}

		tenantId := ""
		if len(args) > 1 {
			if tid, ok := args[1].(string); ok {
				tenantId = tid
			}
		}

		return NewWeightedScoreRunner(weights, tenantId), nil
	default:
		return nil, fmt.Errorf("unknown runner type: %s", runnerType)
	}
}

// RerankModelRunner represents a rerank model runner
type RerankModelRunner struct {
	BaseRerankRunnerImpl
	llmClient   llmclient.LLMClient
	accountID   string
	appID       string
	modelName   string
	workspaceID string
}

// NewRerankModelRunner creates a new rerank model runner
func NewRerankModelRunner(
	llmClient llmclient.LLMClient,
	accountID string,
	appID string,
	modelName string,
	workspaceID string,
) *RerankModelRunner {
	return &RerankModelRunner{
		llmClient:   llmClient,
		accountID:   accountID,
		appID:       appID,
		modelName:   modelName,
		workspaceID: workspaceID,
	}
}

// Run executes the rerank model
// query: search query
// documents: documents for reranking
// scoreThreshold: score threshold (optional)
// topN: top n results (optional)
// user: unique user id if needed (optional)
func (r *RerankModelRunner) Run(ctx context.Context, query string, documents []dto.Document, scoreThreshold *float64, topN *int, _ *string) ([]dto.Document, error) {
	// Prepare unique documents and content for reranking
	docs := []string{}
	docIDs := make(map[string]bool)
	uniqueDocuments := []dto.Document{}

	for _, document := range documents {
		if document.Provider == "external" {
			docs = append(docs, document.PageContent)
			uniqueDocuments = append(uniqueDocuments, document)
		} else if document.Metadata != nil {
			if docID, ok := document.Metadata["doc_id"].(string); ok && !docIDs[docID] {
				docIDs[docID] = true
				docs = append(docs, document.PageContent)
				uniqueDocuments = append(uniqueDocuments, document)
			}
		}
	}

	documents = uniqueDocuments

	if r.llmClient == nil {
		return nil, fmt.Errorf("llm client is not configured")
	}
	if r.modelName == "" {
		return nil, fmt.Errorf("rerank model is empty")
	}

	gatewaySvc, err := NewGatewayRerankService(r.llmClient, r.accountID, r.appID, "dataset", r.modelName, r.workspaceID)
	if err != nil {
		return nil, err
	}

	rerankResults, err := gatewaySvc.Rerank(ctx, query, docs, scoreThreshold, topN)
	if err != nil {
		return nil, err
	}

	logger.Info("Using gateway rerank service", map[string]interface{}{
		"model": r.modelName,
	})

	for _, result := range rerankResults {
		if result.Index >= 0 && result.Index < len(documents) {
			if documents[result.Index].Metadata == nil {
				documents[result.Index].Metadata = map[string]interface{}{}
			}
			documents[result.Index].Metadata["score"] = result.Score
			documents[result.Index].Metadata["rerank_score"] = result.Score
		}
	}

	return documents, nil
}

// Weights represents weights for hybrid search
type Weights struct {
	VectorSetting  VectorSetting  `json:"vector_setting"`
	KeywordSetting KeywordSetting `json:"keyword_setting"`
}

// VectorSetting represents vector search settings
type VectorSetting struct {
	VectorWeight          float64 `json:"vector_weight"`
	EmbeddingProviderName string  `json:"embedding_provider_name"`
	EmbeddingModelName    string  `json:"embedding_model_name"`
}

// KeywordSetting represents keyword search settings
type KeywordSetting struct {
	KeywordWeight float64 `json:"keyword_weight"`
}

// WeightedScoreRunner implements weighted scoring for hybrid search
type WeightedScoreRunner struct {
	BaseRerankRunnerImpl
	weights  *Weights
	tenantId string
}

// NewWeightedScoreRunner creates a new WeightedScoreRunner
func NewWeightedScoreRunner(weights *Weights, tenantId string) *WeightedScoreRunner {
	return &WeightedScoreRunner{
		weights:  weights,
		tenantId: tenantId,
	}
}

// Run performs weighted scoring
func (w *WeightedScoreRunner) Run(ctx context.Context, query string, documents []dto.Document, scoreThreshold *float64, topN *int, user *string) ([]dto.Document, error) {
	// Prepare unique documents
	uniqueDocuments := []dto.Document{}
	docIds := make(map[string]bool)

	for _, document := range documents {
		if document.Metadata != nil {
			if docID, ok := document.Metadata["doc_id"].(string); ok && !docIds[docID] {
				docIds[docID] = true
				uniqueDocuments = append(uniqueDocuments, document)
			}
		} else {
			// If document has no metadata or doc_id, include it
			uniqueDocuments = append(uniqueDocuments, document)
		}
	}

	documents = uniqueDocuments

	// Calculate keyword scores and vector scores
	queryScores := w.calculateKeywordScore(query, documents)
	queryVectorScores := w.calculateCosine(ctx, w.tenantId, query, documents)

	// Calculate weighted scores
	rerankDocuments := []dto.Document{}
	for i, document := range documents {
		// Make sure we have scores for both keyword and vector
		if i >= len(queryScores) || i >= len(queryVectorScores) {
			continue
		}

		queryScore := queryScores[i]
		queryVectorScore := queryVectorScores[i]

		// Calculate weighted score
		score := w.weights.VectorSetting.VectorWeight*queryVectorScore +
			w.weights.KeywordSetting.KeywordWeight*queryScore

		// Apply score threshold
		if scoreThreshold != nil && score < *scoreThreshold {
			continue
		}

		// Set the new score in metadata
		if document.Metadata == nil {
			document.Metadata = make(map[string]interface{})
		}
		document.Metadata["score"] = score
		rerankDocuments = append(rerankDocuments, document)
	}

	// Sort by score in descending order
	for i := 0; i < len(rerankDocuments); i++ {
		for j := i + 1; j < len(rerankDocuments); j++ {
			scoreI, _ := rerankDocuments[i].Metadata["score"].(float64)
			scoreJ, _ := rerankDocuments[j].Metadata["score"].(float64)
			if scoreI < scoreJ {
				rerankDocuments[i], rerankDocuments[j] = rerankDocuments[j], rerankDocuments[i]
			}
		}
	}

	// Apply top N limit
	if topN != nil && *topN < len(rerankDocuments) {
		rerankDocuments = rerankDocuments[:*topN]
	}

	return rerankDocuments, nil
}

// calculateKeywordScore calculates BM25-like scores for documents based on keyword matching
func (w *WeightedScoreRunner) calculateKeywordScore(query string, documents []dto.Document) []float64 {
	// TODO: Implement proper BM25 algorithm
	// For now, we'll implement a simplified version of the BM25-like algorithm

	// Extract keywords from query
	// TODO: Use proper keyword extraction for CJK text.
	queryKeywords := w.extractKeywords(query)

	// Extract keywords from all documents
	documentsKeywords := make([][]string, len(documents))
	for i, document := range documents {
		keywords := w.extractKeywords(document.PageContent)
		documentsKeywords[i] = keywords

		// Store keywords in document metadata
		if document.Metadata == nil {
			document.Metadata = make(map[string]interface{})
		}
		document.Metadata["keywords"] = keywords
		// Note: We can't modify the original document in the slice, so this won't persist
		// The implementation modify the document object directly
	}

	// Count query keywords (TF)
	queryKeywordCounts := make(map[string]int)
	for _, keyword := range queryKeywords {
		queryKeywordCounts[keyword]++
	}

	// Total number of documents
	totalDocuments := len(documents)

	// Calculate IDF for all keywords
	allKeywords := make(map[string]bool)
	for _, docKeywords := range documentsKeywords {
		for _, keyword := range docKeywords {
			allKeywords[keyword] = true
		}
	}

	keywordIdf := make(map[string]float64)
	for keyword := range allKeywords {
		// Count documents containing this keyword
		docCountContainingKeyword := 0
		for _, docKeywords := range documentsKeywords {
			found := false
			for _, docKeyword := range docKeywords {
				if docKeyword == keyword {
					found = true
					break
				}
			}
			if found {
				docCountContainingKeyword++
			}
		}

		// Calculate IDF: log((1 + total_documents) / (1 + doc_count_containing_keyword)) + 1
		keywordIdf[keyword] = math.Log(float64(1+totalDocuments)/float64(1+docCountContainingKeyword)) + 1
	}

	// Calculate query TF-IDF
	queryTfidf := make(map[string]float64)
	for keyword, count := range queryKeywordCounts {
		tf := float64(count)
		idf := keywordIdf[keyword]
		queryTfidf[keyword] = tf * idf
	}

	// Calculate documents' TF-IDF
	documentsTfidf := make([]map[string]float64, len(documents))
	for i, docKeywords := range documentsKeywords {
		// Count document keywords (TF)
		docKeywordCounts := make(map[string]int)
		for _, keyword := range docKeywords {
			docKeywordCounts[keyword]++
		}

		// Calculate TF-IDF for document
		docTfidf := make(map[string]float64)
		for keyword, count := range docKeywordCounts {
			tf := float64(count)
			idf := keywordIdf[keyword]
			docTfidf[keyword] = tf * idf
		}

		documentsTfidf[i] = docTfidf
	}

	// Calculate cosine similarities
	similarities := make([]float64, len(documents))
	for i, docTfidf := range documentsTfidf {
		similarity := w.cosineSimilarity(queryTfidf, docTfidf)
		similarities[i] = similarity
	}

	return similarities
}

// extractKeywords extracts keywords from text
// TODO: Implement proper keyword extraction for CJK text.
// For now, we implement a more sophisticated approach based on the current retrieval rules
func (w *WeightedScoreRunner) extractKeywords(text string) []string {
	// Use the tokenization service for better keyword extraction
	tokenizer := tokenization.NewTokenizationService()

	// Extract keywords using the tokenizer's ExtractKeywords method
	// This is more sophisticated than simple whitespace tokenization
	// Use topK=10 as the default keyword extraction limit.
	keywords, err := tokenizer.ExtractKeywords(text, 10)
	if err != nil {
		// Fallback to simple whitespace-based tokenization if error occurs
		words := strings.Fields(text)

		// Remove duplicates while preserving order
		seen := make(map[string]bool)
		keywords := []string{}
		for _, word := range words {
			// Simple deduplication
			if !seen[word] {
				seen[word] = true
				keywords = append(keywords, word)
			}
		}
		return keywords
	}

	// Expand tokens with subtokens
	expandedKeywords := w.expandTokensWithSubtokens(keywords)

	return expandedKeywords
}

// expandTokensWithSubtokens expands tokens with subtokens
func (w *WeightedScoreRunner) expandTokensWithSubtokens(tokens []string) []string {
	// This is a placeholder implementation
	// This method expands tokens with subtokens
	// For now, we'll just return the tokens as is
	return tokens
}

// cosineSimilarity calculates cosine similarity between two TF-IDF vectors
func (w *WeightedScoreRunner) cosineSimilarity(vec1, vec2 map[string]float64) float64 {
	// Find intersection of keys
	intersection := make(map[string]bool)
	for k := range vec1 {
		if _, exists := vec2[k]; exists {
			intersection[k] = true
		}
	}

	// Calculate numerator
	numerator := 0.0
	for k := range intersection {
		numerator += vec1[k] * vec2[k]
	}

	// Calculate denominators
	sum1 := 0.0
	for _, v := range vec1 {
		sum1 += v * v
	}

	sum2 := 0.0
	for _, v := range vec2 {
		sum2 += v * v
	}

	denominator := math.Sqrt(sum1) * math.Sqrt(sum2)

	if denominator == 0 {
		return 0.0
	}

	return numerator / denominator
}

// calculateCosine calculates cosine similarities using embedding model
func (w *WeightedScoreRunner) calculateCosine(ctx context.Context, tenantId string, query string, documents []dto.Document) []float64 {
	// TODO: Implement cosine similarity calculation using embedding model
	// This would require integration with the embedding model system
	// tenantId is now properly passed from the runner context
	queryVectorScores := make([]float64, len(documents))

	// For now, we'll extract existing scores from document metadata if available
	// or return default scores
	for i, document := range documents {
		if document.Metadata != nil {
			if score, ok := document.Metadata["score"].(float64); ok {
				queryVectorScores[i] = score
			} else {
				// Default score if none exists
				queryVectorScores[i] = 0.5
			}
		} else {
			// Default score if no metadata
			queryVectorScores[i] = 0.5
		}
	}

	return queryVectorScores
}

// calculateKeywordSimilarity calculates a mock keyword similarity score
func calculateKeywordSimilarity(query, content string) float64 {
	// This is a placeholder implementation
	// A real implementation would use proper text similarity algorithms
	if query == content {
		return 1.0
	}
	return 0.5
}

// RerankResultDoc represents a single rerank result document
type RerankResultDoc struct {
	Text  string  `json:"text"`
	Index int     `json:"index"`
	Score float64 `json:"score"`
}

// RerankModelResult represents the result of a rerank operation
type RerankModelResult struct {
	Docs []RerankResultDoc `json:"docs"`
}
