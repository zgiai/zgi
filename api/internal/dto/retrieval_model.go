package dto

// RetrievalConfig represents the retrieval configuration
type RetrievalConfig struct {
	SearchMethod          string          `json:"search_method"`
	TopK                  int             `json:"top_k"`
	ScoreThresholdEnabled bool            `json:"score_threshold_enabled"`
	ScoreThreshold        float64         `json:"score_threshold"`
	RerankingEnable       bool            `json:"reranking_enable"`
	RerankingModel        *RerankingModel `json:"reranking_model"`
}

// RerankingModel represents the reranking model configuration
type RerankingModel struct {
	RerankingProviderName string `json:"reranking_provider_name"`
	RerankingModelName    string `json:"reranking_model_name"`
}

// DefaultRetrievalConfig returns the default retrieval configuration
func DefaultRetrievalConfig() *RetrievalConfig {
	return &RetrievalConfig{
		SearchMethod:          "hybrid_search",
		TopK:                  10,
		ScoreThresholdEnabled: true,
		ScoreThreshold:        0.35,
		RerankingEnable:       true,
		RerankingModel: &RerankingModel{
			RerankingProviderName: "",
			RerankingModelName:    "",
		},
	}
}

// IsSemanticSearch checks if the search method is semantic search
func (rc *RetrievalConfig) IsSemanticSearch() bool {
	return rc.SearchMethod == "semantic_search"
}

// IsGraphSearch checks if the search method is graph search
func (rc *RetrievalConfig) IsGraphSearch() bool {
	return rc.SearchMethod == "graph_search"
}

// GetEffectiveScoreThreshold returns the effective score threshold
func (rc *RetrievalConfig) GetEffectiveScoreThreshold() float64 {
	if rc.ScoreThresholdEnabled {
		return rc.ScoreThreshold
	}
	return 0.0
}

// GetEffectiveTopK returns the effective top k value
func (rc *RetrievalConfig) GetEffectiveTopK() int {
	if rc.TopK <= 0 {
		return 10 // default value
	}
	return rc.TopK
}
