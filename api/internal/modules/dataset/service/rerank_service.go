package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

// RerankMode represents different reranking modes
type RerankMode string

const (
	RerankModeRerankingModel RerankMode = "reranking_model"
	RerankModeWeightedScore  RerankMode = "weighted_score"
)

// RerankModel represents reranking model configuration
type RerankModel struct {
	RerankingProviderName string `json:"reranking_provider_name"`
	RerankingModelName    string `json:"reranking_model_name"`
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

// RerankRunner interface for different reranking strategies
type RerankRunner interface {
	Run(ctx context.Context, query string, documents []*model.DocumentSegment, scoreThreshold *float64, topN *int) ([]*model.DocumentSegment, error)
}

// DataPostProcessor handles document post-processing including reranking
type DataPostProcessor struct {
	rerankRunner RerankRunner
	rerankMode   RerankMode
}

// NewDataPostProcessor creates a new DataPostProcessor instance
func NewDataPostProcessor(rerankMode RerankMode, rerankingModel *RerankModel, weights *Weights) *DataPostProcessor {
	var runner RerankRunner

	switch rerankMode {
	case RerankModeRerankingModel:
		if rerankingModel != nil {
			runner = NewRerankingModelRunner(rerankingModel)
		}
	case RerankModeWeightedScore:
		if weights != nil {
			runner = NewWeightedScoreRunner(weights)
		}
	}

	return &DataPostProcessor{
		rerankRunner: runner,
		rerankMode:   rerankMode,
	}
}

// Invoke processes documents with reranking and filtering
func (p *DataPostProcessor) Invoke(ctx context.Context, query string, documents []*model.DocumentSegment, scoreThreshold *float64, topN *int) ([]*model.DocumentSegment, error) {
	if p.rerankRunner != nil {
		var err error
		documents, err = p.rerankRunner.Run(ctx, query, documents, scoreThreshold, topN)
		if err != nil {
			return nil, fmt.Errorf("reranking failed: %w", err)
		}
	}

	return documents, nil
}

// RerankingModelRunner implements reranking using external reranking models
type RerankingModelRunner struct {
	model *RerankModel
}

// NewRerankingModelRunner creates a new RerankingModelRunner
func NewRerankingModelRunner(model *RerankModel) *RerankingModelRunner {
	return &RerankingModelRunner{
		model: model,
	}
}

// Run performs reranking using the reranking model
func (r *RerankingModelRunner) Run(ctx context.Context, query string, documents []*model.DocumentSegment, scoreThreshold *float64, topN *int) ([]*model.DocumentSegment, error) {
	// TODO: Implement actual reranking model integration
	// For now, return documents as-is with mock reranking scores
	for i, doc := range documents {
		// Mock reranking score based on content similarity
		similarity := calculateContentSimilarity(query, doc.Content)
		doc.HitCount = int(similarity * 100) // Use hit_count field to store reranking score
		documents[i] = doc
	}

	// Sort by reranking score (hit_count)
	sort.Slice(documents, func(i, j int) bool {
		return documents[i].HitCount > documents[j].HitCount
	})

	// Apply score threshold
	if scoreThreshold != nil {
		filtered := make([]*model.DocumentSegment, 0)
		for _, doc := range documents {
			if float64(doc.HitCount)/100.0 >= *scoreThreshold {
				filtered = append(filtered, doc)
			}
		}
		documents = filtered
	}

	// Apply top N limit
	if topN != nil && *topN < len(documents) {
		documents = documents[:*topN]
	}

	return documents, nil
}

// WeightedScoreRunner implements weighted scoring for hybrid search
type WeightedScoreRunner struct {
	weights *Weights
}

// NewWeightedScoreRunner creates a new WeightedScoreRunner
func NewWeightedScoreRunner(weights *Weights) *WeightedScoreRunner {
	return &WeightedScoreRunner{
		weights: weights,
	}
}

// Run performs weighted scoring
func (w *WeightedScoreRunner) Run(ctx context.Context, query string, documents []*model.DocumentSegment, scoreThreshold *float64, topN *int) ([]*model.DocumentSegment, error) {
	// TODO: Implement weighted scoring with vector and keyword components
	// For now, apply simple weighted scoring
	for i, doc := range documents {
		// Mock weighted score calculation
		vectorScore := calculateVectorSimilarity(query, doc.Content) * w.weights.VectorSetting.VectorWeight
		keywordScore := calculateKeywordSimilarity(query, doc.Content) * w.weights.KeywordSetting.KeywordWeight
		totalScore := vectorScore + keywordScore

		doc.HitCount = int(totalScore * 100)
		documents[i] = doc
	}

	// Sort by weighted score
	sort.Slice(documents, func(i, j int) bool {
		return documents[i].HitCount > documents[j].HitCount
	})

	// Apply score threshold
	if scoreThreshold != nil {
		filtered := make([]*model.DocumentSegment, 0)
		for _, doc := range documents {
			if float64(doc.HitCount)/100.0 >= *scoreThreshold {
				filtered = append(filtered, doc)
			}
		}
		documents = filtered
	}

	// Apply top N limit
	if topN != nil && *topN < len(documents) {
		documents = documents[:*topN]
	}

	return documents, nil
}

// Helper functions for similarity calculations
func calculateContentSimilarity(query, content string) float64 {
	// Simple content similarity calculation
	queryWords := splitIntoWords(query)
	contentWords := splitIntoWords(content)

	matches := 0
	for _, qw := range queryWords {
		for _, cw := range contentWords {
			if strings.Contains(cw, qw) || strings.Contains(qw, cw) {
				matches++
				break
			}
		}
	}

	if len(queryWords) == 0 {
		return 0.0
	}

	return float64(matches) / float64(len(queryWords))
}

func calculateVectorSimilarity(query, content string) float64 {
	// TODO: Implement actual vector similarity calculation
	// For now, return content similarity as proxy
	return calculateContentSimilarity(query, content)
}

func calculateKeywordSimilarity(query, content string) float64 {
	// TODO: Implement keyword-based similarity calculation
	// For now, return content similarity as proxy
	return calculateContentSimilarity(query, content)
}

func splitIntoWords(text string) []string {
	// Simple word splitting
	words := strings.Fields(text)
	result := make([]string, 0)
	for _, word := range words {
		if len(word) > 1 {
			result = append(result, word)
		}
	}
	return result
}
