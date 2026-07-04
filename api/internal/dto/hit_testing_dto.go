package dto

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// HitTestingRequest represents the request for hit testing
type HitTestingRequest struct {
	Query                  string                 `json:"query" binding:"required"`
	ExternalRetrievalModel map[string]interface{} `json:"external_retrieval_model,omitempty"`
	RetrievalMode          string                 `json:"retrieval_mode,omitempty"`
	RecordHistory          *bool                  `json:"record_history,omitempty"`
}

const (
	MatchTypeOriginal       = "original"        // Original content match
	MatchTypeQuestion       = "question"        // Question match (including both pregenerated and manually entered questions)
	MatchTypeGraphKnowledge = "graph_knowledge" // From knowledge graph
)

// HitTestingResponse represents the response for hit testing
type HitTestingResponse struct {
	Query             HitTestingQueryResponse    `json:"query"`
	Records           []HitTestingRecordResponse `json:"records"`
	ElapsedTime       float64                    `json:"elapsed_time,omitempty"`
	GraphExecution    *GraphExecution            `json:"graph_execution,omitempty"`
	RetrievalPipeline *RetrievalPipelineResponse `json:"retrieval_pipeline,omitempty"`
}

// GraphExecution represents the graph retrieval process and details
type GraphExecution struct {
	Entities  []string               `json:"entities"`             // Entities extracted from query
	Triples   []TripleResponse       `json:"triples"`              // Related triples found in graph
	Steps     []GraphExecutionStep   `json:"steps,omitempty"`      // Steps taken in graph retrieval
	Summary   string                 `json:"summary,omitempty"`    // Human-readable summary of graph reasoning
	Thinking  string                 `json:"thinking,omitempty"`   // Internal thinking/reasoning
	DebugInfo map[string]interface{} `json:"debug_info,omitempty"` // Internal debug information
}

// GraphExecutionStep represents a step in the graph retrieval process
type GraphExecutionStep struct {
	Step        int    `json:"step"`             // Step number
	Action      string `json:"action"`           // Action taken (e.g., "extract_entities", "query_graph", "find_relations")
	Description string `json:"description"`      // Human-readable description
	Result      string `json:"result,omitempty"` // Result of this step
}

// TripleResponse represents a single triple (S-P-O) from knowledge graph
type TripleResponse struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

// Value implements the driver.Valuer interface for database serialization
func (h *HitTestingResponse) Value() (driver.Value, error) {
	if h == nil {
		return nil, nil
	}
	return json.Marshal(h)
}

// Scan implements the sql.Scanner interface for database deserialization
func (h *HitTestingResponse) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	// Check if value is byte slice
	if bytes, ok := value.([]byte); ok {
		if len(bytes) == 0 {
			return nil
		}
		return json.Unmarshal(bytes, h)
	}

	// Check if value is string
	if str, ok := value.(string); ok {
		if str == "" {
			return nil
		}
		return json.Unmarshal([]byte(str), h)
	}

	return fmt.Errorf("cannot scan %T into HitTestingResponse", value)
}

// HitTestingQueryResponse represents the query part of the response
type HitTestingQueryResponse struct {
	Content      string                 `json:"content"`
	TSNEPosition map[string]interface{} `json:"tsne_position,omitempty"`
}

// HitTestingRecordResponse represents a single record in the hit testing response
type HitTestingRecordResponse struct {
	Segment         SegmentResponse          `json:"segment"`
	Score           float64                  `json:"score"`
	TSNEPosition    map[string]interface{}   `json:"tsne_position,omitempty"`
	ChildChunks     []ChildChunkResponse     `json:"child_chunks,omitempty"`
	MatchType       string                   `json:"match_type"`
	RetrievalSource *RetrievalSourceResponse `json:"retrieval_source,omitempty"`
}

// RetrievalSourceResponse explains why a segment was returned
type RetrievalSourceResponse struct {
	Method           string   `json:"method"`                      // "semantic_search", "full_text_search", "hybrid_search", "graph_knowledge"
	Reason           string   `json:"reason,omitempty"`            // Human-readable explanation
	RetrievalSources []string `json:"retrieval_sources,omitempty"` // Low-level retrieval sources such as "vector" and "bm25"
	MatchedTerms     []string `json:"matched_terms,omitempty"`     // For full-text: matched keywords
	MatchedEntities  []string `json:"matched_entities,omitempty"`  // For graph: matched entity names
	VectorScore      *float64 `json:"vector_score,omitempty"`      // Raw vector score before fusion
	BM25Score        *float64 `json:"bm25_score,omitempty"`        // Raw BM25 score before fusion
	VectorRank       *int     `json:"vector_rank,omitempty"`       // Rank in vector result list
	BM25Rank         *int     `json:"bm25_rank,omitempty"`         // Rank in BM25 result list
	BestRank         *int     `json:"best_rank,omitempty"`         // Best rank across retrieval sources
	FusionScore      *float64 `json:"fusion_score,omitempty"`      // Final fused retrieval score
	RerankScore      *float64 `json:"rerank_score,omitempty"`      // Rerank model score after fusion
	FinalScore       *float64 `json:"final_score,omitempty"`       // Final score returned to the UI
}

// RetrievalPipelineResponse shows the retrieval process summary
type RetrievalPipelineResponse struct {
	Methods []RetrievalMethodSummary `json:"methods"`
}

// RetrievalMethodSummary summarizes a single retrieval method's execution
type RetrievalMethodSummary struct {
	Method       string `json:"method"`                  // "semantic_search", "full_text_search", "graph_knowledge"
	ResultCount  int    `json:"result_count"`            // How many results from this method
	ElapsedMs    int64  `json:"elapsed_ms"`              // Time spent on this method
	Status       string `json:"status"`                  // "success", "skipped", "timeout", "error"
	ErrorMessage string `json:"error_message,omitempty"` // Error details if status is "error"
}

// SegmentResponse represents a document segment in the response
type SegmentResponse struct {
	ID                 string                     `json:"id"`
	Position           int                        `json:"position"`
	DocumentID         string                     `json:"document_id"`
	Content            string                     `json:"content"`
	SignContent        string                     `json:"sign_content"`
	Answer             *string                    `json:"answer"`
	WordCount          int                        `json:"word_count"`
	Tokens             int                        `json:"tokens"`
	Keywords           []string                   `json:"keywords"`
	IndexNodeID        *string                    `json:"index_node_id"`
	IndexNodeHash      *string                    `json:"index_node_hash"`
	HitCount           int                        `json:"hit_count"`
	Enabled            bool                       `json:"enabled"`
	DisabledAt         *int64                     `json:"disabled_at"` // Unix timestamp
	DisabledBy         *string                    `json:"disabled_by"`
	Status             string                     `json:"status"`
	CreatedBy          string                     `json:"created_by"`
	CreatedAt          int64                      `json:"created_at"`   // Unix timestamp
	IndexingAt         *int64                     `json:"indexing_at"`  // Unix timestamp
	CompletedAt        *int64                     `json:"completed_at"` // Unix timestamp
	Error              *string                    `json:"error"`
	StoppedAt          *int64                     `json:"stopped_at"` // Unix timestamp
	Document           HitTestingDocumentResponse `json:"document,omitempty"`
	DatasetProcessRule map[string]interface{}     `json:"dataset_process_rule,omitempty"` // Process rule used for segment creation
}

// HitTestingDocumentResponse represents document information in hit testing response
type HitTestingDocumentResponse struct {
	ID             string                 `json:"id"`
	DataSourceType string                 `json:"data_source_type"`
	Name           string                 `json:"name"`
	DocType        string                 `json:"doc_type"`
	DocMetadata    map[string]interface{} `json:"doc_metadata"`
}

// ChildChunkResponse represents a child chunk in the response
type ChildChunkResponse struct {
	ID            string                 `json:"id"`
	SegmentID     string                 `json:"segment_id"`
	Content       string                 `json:"content"`
	Position      int                    `json:"position"`
	WordCount     int                    `json:"word_count"`
	Type          string                 `json:"type"`
	IndexNodeID   *string                `json:"index_node_id,omitempty"`
	IndexNodeHash *string                `json:"index_node_hash,omitempty"`
	Score         float64                `json:"score"`              // Score field for hit testing
	CreatedAt     int64                  `json:"created_at"`         // Unix timestamp
	UpdatedAt     int64                  `json:"updated_at"`         // Unix timestamp
	Metadata      map[string]interface{} `json:"metadata,omitempty"` // Additional metadata
}

// ChildChunkListRequest represents the request parameters for getting child chunks list
type ChildChunkListRequest struct {
	Page    int    `form:"page" json:"page"`       // Page number, default: 1
	Limit   int    `form:"limit" json:"limit"`     // Items per page, default: 20
	Keyword string `form:"keyword" json:"keyword"` // Search keyword in content
}

// ChildChunkListResponse represents the response for child chunks list
type ChildChunkListResponse struct {
	Data       []ChildChunkResponse `json:"data"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	Limit      int                  `json:"limit"`
	TotalPages int                  `json:"total_pages"`
}

// SegmentListRequest represents the request parameters for getting document segments list
type SegmentListRequest struct {
	Limit        int      `form:"limit" json:"limit"`                 // Default: 20, Max: 100
	Status       []string `form:"status" json:"status"`               // Filter by status (can be multiple)
	HitCountGte  *int     `form:"hit_count_gte" json:"hit_count_gte"` // Filter by hit count greater than or equal
	Enabled      string   `form:"enabled" json:"enabled"`             // Filter by enabled status: "all", "true", "false"
	Keyword      string   `form:"keyword" json:"keyword"`             // Search keyword in content
	Page         int      `form:"page" json:"page"`                   // Page number, default: 1
	SearchMethod string   `form:"search_method" json:"search_method"` // Search method: "semantic_search", "keyword_search", "full_text_search"
}

// SegmentDetailResponse represents a detailed document segment in the response
type SegmentDetailResponse struct {
	ID            string               `json:"id"`
	Position      int                  `json:"position"`
	DocumentID    string               `json:"document_id"`
	Content       string               `json:"content"`
	SignContent   *string              `json:"sign_content"`
	Answer        *string              `json:"answer"`
	WordCount     int                  `json:"word_count"`
	Tokens        int                  `json:"tokens"`
	Keywords      []string             `json:"keywords"`
	IndexNodeID   *string              `json:"index_node_id"`
	IndexNodeHash *string              `json:"index_node_hash"`
	HitCount      int                  `json:"hit_count"`
	Enabled       bool                 `json:"enabled"`
	DisabledAt    *time.Time           `json:"disabled_at"`
	DisabledBy    *string              `json:"disabled_by"`
	Status        string               `json:"status"`
	CreatedBy     string               `json:"created_by"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
	UpdatedBy     *string              `json:"updated_by"`
	IndexingAt    *time.Time           `json:"indexing_at"`
	CompletedAt   *time.Time           `json:"completed_at"`
	Error         *string              `json:"error"`
	StoppedAt     *time.Time           `json:"stopped_at"`
	ChildChunks   []ChildChunkResponse `json:"child_chunks"`
}

// SegmentListResponse represents the paginated response for segment list
type SegmentListResponse struct {
	Data       []SegmentDetailResponse `json:"data"`
	Limit      int                     `json:"limit"`
	Total      int64                   `json:"total"`
	TotalPages int                     `json:"total_pages"`
	Page       int                     `json:"page"`
}

// SegmentDeleteRequest represents the request for deleting segments
type SegmentDeleteRequest struct {
	SegmentIds []string `form:"segment_id" json:"segment_id"` // List of segment IDs to delete
}

// SegmentCreateRequest represents the request for creating a new segment
type SegmentCreateRequest struct {
	Content string  `json:"content" binding:"required"` // Segment content (required)
	Answer  *string `json:"answer"`                     // Answer for Q&A model (optional)
}

// SegmentUpdateRequest represents the request for updating a segment
type SegmentUpdateRequest struct {
	Content               string  `json:"content" binding:"required"`        // Updated segment content
	Answer                *string `json:"answer"`                            // Updated answer for Q&A model (optional)
	RegenerateChildChunks bool    `json:"regenerate_child_chunks,omitempty"` // Whether to rebuild child chunks
}

// SegmentActionRequest represents the request for batch segment actions (enable/disable)
type SegmentActionRequest struct {
	SegmentIds []string `form:"segment_id" json:"segment_id"` // List of segment IDs to update
	Action     string   `json:"action" binding:"required"`    // Action: "enable" or "disable"
}

// SegmentBatchImportRequest represents the request for batch importing segments
type SegmentBatchImportRequest struct {
	Segments []SegmentImportItem `json:"segments" binding:"required"` // List of segments to import
}

// SegmentImportItem represents a single segment item for import
type SegmentImportItem struct {
	Content string  `json:"content" binding:"required"` // Segment content
	Answer  *string `json:"answer"`                     // Answer for Q&A model (optional)
}

// SegmentBatchImportResponse represents the response for batch import
type SegmentBatchImportResponse struct {
	JobID string `json:"job_id"` // Background job ID for tracking import progress
}

// SegmentBatchJobStatusResponse represents the status of a batch import job
type SegmentBatchJobStatusResponse struct {
	JobID        string                     `json:"job_id"`
	Status       string                     `json:"status"`   // "processing", "completed", "failed"
	Progress     float64                    `json:"progress"` // Progress percentage (0-100)
	ProcessedAt  *time.Time                 `json:"processed_at"`
	CompletedAt  *time.Time                 `json:"completed_at"`
	ErrorMessage *string                    `json:"error_message"`
	Results      *SegmentBatchImportResults `json:"results,omitempty"`
}

// SegmentBatchImportResults represents the results of batch import
type SegmentBatchImportResults struct {
	TotalCount      int                     `json:"total_count"`
	SuccessCount    int                     `json:"success_count"`
	FailedCount     int                     `json:"failed_count"`
	SuccessSegments []SegmentDetailResponse `json:"success_segments"`
	FailedSegments  []FailedSegment         `json:"failed_segments"`
}

// FailedSegment represents a failed segment import
type FailedSegment struct {
	Content      string  `json:"content"`
	Answer       *string `json:"answer"`
	ErrorMessage string  `json:"error_message"`
}

// BatchHitTestingRequest represents the request for batch hit testing
type BatchHitTestingRequest struct {
	Queries                []string               `json:"queries" binding:"required"`
	ExternalRetrievalModel map[string]interface{} `json:"external_retrieval_model,omitempty"`
}

// BatchHitTestingResponse represents the response for batch hit testing
type BatchHitTestingResponse struct {
	Results []HitTestingResponse `json:"results"`
}
