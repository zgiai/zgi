package dto

// BatchHitTestingRequest represents the request for batch hit testing
// type BatchHitTestingRequest struct {
// 	Queries                []string               `json:"queries" binding:"required"`
// 	RetrievalModel         map[string]interface{} `json:"retrieval_model,omitempty"`
// 	ExternalRetrievalModel map[string]interface{} `json:"external_retrieval_model,omitempty"`
// }

// BatchHitTestingResponse represents the response for batch hit testing
// type BatchHitTestingResponse struct {
// 	Results []HitTestingResponse `json:"results"`
// }

// AsyncBatchHitTestingRequest represents the request for async batch hit testing
type AsyncBatchHitTestingRequest struct {
	Queries                []string               `json:"queries" binding:"required"`
	RetrievalModel         map[string]interface{} `json:"retrieval_model,omitempty"`
	ExternalRetrievalModel map[string]interface{} `json:"external_retrieval_model,omitempty"`
}

// AsyncBatchHitTestingResponse represents the response for async batch hit testing initiation
type AsyncBatchHitTestingResponse struct {
	TaskID string `json:"task_id"`
}

// BatchHitTestingTaskStatus represents the status of a batch hit testing task
type BatchHitTestingTaskStatus struct {
	TaskID     string        `json:"task_id"`
	Status     string        `json:"status"` // pending, processing, completed, failed
	Progress   int           `json:"progress"`
	Total      int           `json:"total"`
	Completed  int           `json:"completed"`
	Failed     int           `json:"failed"`
	CreatedAt  int64         `json:"created_at"`
	StartedAt  *int64        `json:"started_at,omitempty"`
	FinishedAt *int64        `json:"finished_at,omitempty"`
	Results    []QueryResult `json:"results,omitempty"`
}

// BatchHitTestingTaskReport represents the report of a completed batch hit testing task
type BatchHitTestingTaskReport struct {
	TaskID               string  `json:"task_id"`
	TotalQueries         int     `json:"total_queries"`
	RetrievalSuccessRate float64 `json:"retrieval_success_rate"`
	AverageResponseTime  float64 `json:"average_response_time"`
	QuestionMatchRate    float64 `json:"question_match_rate"`
}

// QueryResult represents the result of a single query in batch testing
type QueryResult struct {
	Query      string              `json:"query"`
	Status     string              `json:"status"` // pending, processing, completed, failed
	Result     *HitTestingResponse `json:"result,omitempty"`
	Error      *string             `json:"error,omitempty"`
	StartedAt  *int64              `json:"started_at,omitempty"`
	FinishedAt *int64              `json:"finished_at,omitempty"`
}