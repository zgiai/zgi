package service

// RetrievalMethod represents different retrieval methods
type RetrievalMethod string

const (
	SemanticSearch  RetrievalMethod = "semantic_search"
	GraphSearch     RetrievalMethod = "graph_search"
	FullTextSearch  RetrievalMethod = "full_text_search"
	KeywordSearch   RetrievalMethod = "keyword_search"
	HybridSearch    RetrievalMethod = "hybrid_search"
)

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
