package dto

type ProcessRuleResponse struct {
	Mode   string                 `json:"mode"`
	Rules  map[string]interface{} `json:"rules"`
	Limits map[string]interface{} `json:"limits"`
}

type DocumentListRequest struct {
	Page           int    `form:"page" binding:"omitempty,min=1"`
	Limit          int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Keyword        string `form:"keyword"`
	Sort           string `form:"sort"`
	Fetch          string `form:"fetch"`
	IndexingStatus string `form:"indexing_status"`
}

type DocumentCreateRequest struct {
	Type                      string   `json:"type" binding:"required"`
	FileIDs                   []string `json:"file_ids" binding:"required"`
	ExtractionStrategy        string   `json:"extraction_strategy,omitempty"` // Extraction strategy: mineru|reducto|local|unstructured|landingai
	ExtractionFallbackEnabled *bool    `json:"extraction_fallback_enabled,omitempty"`
}

type DocumentResponse struct {
	ID                   string                 `json:"id"`
	Position             int                    `json:"position"`
	DataSourceType       string                 `json:"data_source_type"`
	DataSourceInfo       map[string]interface{} `json:"data_source_info"`
	DatasetProcessRuleID string                 `json:"dataset_process_rule_id"`
	DatasetProcessRule   map[string]interface{} `json:"dataset_process_rule,omitempty"`
	Name                 string                 `json:"name"`
	CreatedFrom          string                 `json:"created_from"`
	CreatedBy            string                 `json:"created_by"`
	CreatedAt            int64                  `json:"created_at"`
	Tokens               *int                   `json:"tokens"`
	IndexingStatus       string                 `json:"indexing_status"`
	CompletedAt          *int64                 `json:"completed_at"`
	UpdatedAt            *int64                 `json:"updated_at"`
	IndexingLatency      *float64               `json:"indexing_latency"`
	Error                *string                `json:"error"`
	Enabled              bool                   `json:"enabled"`
	DisabledAt           *int64                 `json:"disabled_at"`
	DisabledBy           *string                `json:"disabled_by"`
	Archived             bool                   `json:"archived"`
	DocType              *string                `json:"doc_type,omitempty"`
	DocMetadata          map[string]interface{} `json:"doc_metadata,omitempty"`
	SegmentCount         int                    `json:"segment_count"`
	AverageSegmentLength float64                `json:"average_segment_length"`
	HitCount             int                    `json:"hit_count"`
	DisplayStatus        string                 `json:"display_status"`
	DocForm              string                 `json:"doc_form"`
	DocLanguage          string                 `json:"doc_language"`
	GraphIndexingStatus  string                 `json:"graph_indexing_status,omitempty"`
	WordCount            *int                   `json:"word_count,omitempty"`
	Progress             int                    `json:"progress"` // 0-100 indexing progress percentage
	// For status endpoints
	CompletedSegments *int `json:"completed_segments,omitempty"`
	TotalSegments     *int `json:"total_segments,omitempty"`
}

type DocumentDetailResponse struct {
	DocumentResponse
	DatasetProcessRule  map[string]interface{} `json:"dataset_process_rule,omitempty"`
	DocumentProcessRule map[string]interface{} `json:"document_process_rule,omitempty"`
}

type DocumentListResponse struct {
	Data    []DocumentResponse `json:"data"`
	HasMore bool               `json:"has_more"`
	Limit   int                `json:"limit"`
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
}

type DocumentCreateResponse struct {
	Documents []DocumentResponse `json:"documents"`
	Batch     string             `json:"batch"`
}

type DocumentBatchStatusResponse struct {
	Data []DocumentResponse `json:"data"`
}

type EnterpriseGroupInitRequest struct {
	IndexingTechnique string                 `json:"indexing_technique" binding:"required"`
	DataSource        map[string]interface{} `json:"data_source" binding:"required"`
	ProcessRule       map[string]interface{} `json:"process_rule" binding:"required"`
	DocForm           string                 `json:"doc_form"`
	DocLanguage       string                 `json:"doc_language"`
}

type EnterpriseGroupInitResponse struct {
	Dataset   interface{}        `json:"dataset"`
	Documents []DocumentResponse `json:"documents"`
	Batch     string             `json:"batch"`
}

type DocumentDeleteResponse struct {
	Result string `json:"result"`
}

// DocumentIndexingStatus represents the indexing status of a document
type DocumentIndexingStatus struct {
	ID                   string  `json:"id"`
	IndexingStatus       string  `json:"indexing_status"`
	GraphIndexingStatus  string  `json:"graph_indexing_status,omitempty"`
	ProcessingStartedAt  *int64  `json:"processing_started_at"`
	ParsingCompletedAt   *int64  `json:"parsing_completed_at"`
	CleaningCompletedAt  *int64  `json:"cleaning_completed_at"`
	SplittingCompletedAt *int64  `json:"splitting_completed_at"`
	CompletedAt          *int64  `json:"completed_at"`
	PausedAt             *int64  `json:"paused_at"`
	Error                *string `json:"error"`
	StoppedAt            *int64  `json:"stopped_at"`
	CompletedSegments    int     `json:"completed_segments"`
	TotalSegments        int     `json:"total_segments"`
}

type DocumentUpdateRequest struct {
	Enabled *bool  `json:"enabled"`
	Name    string `json:"name"`
}

// DocumentProgressResponse represents the indexing progress of a document
type DocumentProgressResponse struct {
	DocumentID      string `json:"document_id"`
	Progress        int    `json:"progress"`          // 0-100 percentage
	Stage           string `json:"stage"`             // Current stage name (e.g., "parsing", "indexing", "extraction")
	StageDetail     string `json:"stage_detail"`      // Detailed stage info
	IsCompleted     bool   `json:"is_completed"`      // Whether all indexing is complete
	EnableGraphFlow bool   `json:"enable_graph_flow"` // Whether GraphFlow is enabled for this document's dataset
}
