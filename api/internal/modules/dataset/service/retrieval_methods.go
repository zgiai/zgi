package service

import (
	"fmt"
	"strings"
)

// RetrievalMethod represents different retrieval methods
type RetrievalMethod string

const (
	SemanticSearch RetrievalMethod = "semantic_search"
	GraphSearch    RetrievalMethod = "graph_search"
	FullTextSearch RetrievalMethod = "full_text_search"
	KeywordSearch  RetrievalMethod = "keyword_search"
	HybridSearch   RetrievalMethod = "hybrid_search"
)

const (
	RetrievalModeVector = "vector"
	RetrievalModeGraph  = "graph"
	RetrievalModeHybrid = "hybrid"

	FallbackPolicyNone   = "none"
	FallbackPolicyVector = "vector"
)

func defaultDatasetSearchMethod(graphEnabled bool) string {
	if graphEnabled {
		return string(GraphSearch)
	}
	return string(HybridSearch)
}

type GraphRetrievalConfig struct {
	RequestedMethod string `json:"requested_method"`
	ActualMode      string `json:"actual_mode"`
	FallbackPolicy  string `json:"fallback_policy"`
	MaxHops         int    `json:"max_hops"`
}

func NormalizeGraphRetrievalConfig(searchMethod string, retrievalMode string, fallbackPolicy string) (GraphRetrievalConfig, error) {
	method := strings.ToLower(strings.TrimSpace(searchMethod))
	mode := strings.ToLower(strings.TrimSpace(retrievalMode))
	if mode == "" {
		if method == string(GraphSearch) {
			mode = RetrievalModeHybrid
		} else {
			mode = RetrievalModeVector
		}
	}
	if mode != RetrievalModeVector && mode != RetrievalModeGraph && mode != RetrievalModeHybrid {
		return GraphRetrievalConfig{}, fmt.Errorf("unsupported retrieval mode %q", mode)
	}
	policy := strings.ToLower(strings.TrimSpace(fallbackPolicy))
	if policy == "" {
		policy = FallbackPolicyNone
	}
	if policy != FallbackPolicyNone && policy != FallbackPolicyVector {
		return GraphRetrievalConfig{}, fmt.Errorf("unsupported fallback policy %q", policy)
	}
	return GraphRetrievalConfig{
		RequestedMethod: method,
		ActualMode:      mode,
		FallbackPolicy:  policy,
		MaxHops:         3,
	}, nil
}

func ShouldPropagateRetrievalError(config GraphRetrievalConfig, graphFailed bool) bool {
	if !graphFailed {
		return false
	}
	graphRequested := config.RequestedMethod == string(GraphSearch) || config.ActualMode == RetrievalModeGraph
	return graphRequested && config.FallbackPolicy == FallbackPolicyNone
}

// IsSupportSemanticSearch checks if the retrieval method supports semantic search
func IsSupportSemanticSearch(retrievalMethod string) bool {
	return retrievalMethod == string(SemanticSearch)
}

// IsSupportGraphSearch checks if the retrieval method supports graph search
func IsSupportGraphSearch(retrievalMethod string) bool {
	return retrievalMethod == string(GraphSearch)
}

// IsSupportFullTextSearch checks if the retrieval method supports full text search
func IsSupportFullTextSearch(retrievalMethod string) bool {
	return retrievalMethod == string(FullTextSearch)
}

// IsSupportKeywordSearch checks if the retrieval method supports keyword search
func IsSupportKeywordSearch(retrievalMethod string) bool {
	return retrievalMethod == string(KeywordSearch)
}

// IsValidRetrievalMethod checks if the retrieval method is valid
func IsValidRetrievalMethod(retrievalMethod string) bool {
	switch RetrievalMethod(retrievalMethod) {
	case SemanticSearch, GraphSearch, FullTextSearch, KeywordSearch, HybridSearch:
		return true
	default:
		return false
	}
}
