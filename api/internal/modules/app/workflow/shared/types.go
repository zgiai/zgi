package shared

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type NodeInterface interface {
	Run(ctx context.Context, eventChan chan *NodeEventCh) error
}

type DefaultValue struct {
	Key   string           `json:"key"`
	Value string           `json:"value"`
	Type  DefaultValueType `json:"type"` // Provides basis for subsequent data type conversion
}

type RetryConfig struct {
	MaxTimes int  `json:"max_times"`
	Interval int  `json:"interval"` // milliseconds
	Enable   bool `json:"enable"`
}

type NodeEventCh struct {
	Type      NodeEventType
	NodeID    string
	Data      any
	Error     error
	Timestamp time.Time
}

type RunCompletedEvent struct {
	RunResult  *NodeRunResult
	StartedAt  time.Time
	FinishedAt time.Time
}

type RunFailedEvent struct {
	RunResult *NodeRunResult
}

type RunStreamChunkEvent struct {
	ChunkContent         string          `json:"chunk_content"`
	FromVariableSelector []string        `json:"from_variable_selector"`
	Scope                *RunStreamScope `json:"-"`
}

type RunStreamScope struct {
	Kind         string
	ParentNodeID string
	Index        int
}

type ReadyBatchEvent struct {
	ScopeKind    string
	ParentNodeID string
	Index        int
	NodeIDs      []string
}

type ModelInvokeCompletedEvent struct {
	Text         string    `json:"text"`
	Usage        *LLMUsage `json:"usage"`
	FinishReason string    `json:"finish_reason"`
}

// RetrievalSourceMetadata represents metadata for retrieval sources
type RetrievalSourceMetadata struct {
	Position        *int           `json:"position,omitempty"`
	DatasetID       *string        `json:"dataset_id,omitempty"`
	DatasetName     *string        `json:"dataset_name,omitempty"`
	DocumentID      *string        `json:"document_id,omitempty"`
	DocumentName    *string        `json:"document_name,omitempty"`
	DataSourceType  *string        `json:"data_source_type,omitempty"`
	SegmentID       *string        `json:"segment_id,omitempty"`
	RetrieverFrom   *string        `json:"retriever_from,omitempty"`
	Score           *float64       `json:"score,omitempty"`
	HitCount        *int           `json:"hit_count,omitempty"`
	WordCount       *int           `json:"word_count,omitempty"`
	SegmentPosition *int           `json:"segment_position,omitempty"`
	IndexNodeHash   *string        `json:"index_node_hash,omitempty"`
	Content         *string        `json:"content,omitempty"`
	Page            *int           `json:"page,omitempty"`
	DocMetadata     map[string]any `json:"doc_metadata,omitempty"`
	Title           *string        `json:"title,omitempty"`
}

type RunRetrieverResourceEvent struct {
	RetrieverResources []*RetrievalSourceMetadata `json:"retriever_resources"`
	Context            string                     `json:"context"`
}

type NodeRunResult struct {
	Status           WorkflowNodeExecutionStatus              // Node execution status
	Inputs           map[string]any                           // Node input parameters
	ProcessData      map[string]any                           // Data during node processing (intermediate data, state info, etc.)
	Outputs          map[string]any                           // Node output parameters
	Metadata         map[WorkflowNodeExecutionMetadataKey]any // Node execution metadata
	LLMUsage         *LLMUsage                                // LLM usage information
	EdgeSourceHandle string                                   // Records result source
	Err              error                                    // Keeps the original node error for downstream error handling
	ErrMsg           string                                   // Stores node execution failure error message
	ErrType          string                                   // Stores node execution failure error type
}

type LLMUsage struct {
	PromptTokens        int
	PromptUnitPrice     decimal.Decimal
	PromptPriceUnit     decimal.Decimal
	PromptPrice         decimal.Decimal
	CompletionTokens    int
	CompletionUnitPrice decimal.Decimal
	CompletionPriceUnit decimal.Decimal
	CompletionPrice     decimal.Decimal
	TotalTokens         int
	TotalPrice          decimal.Decimal
	Currency            string
	Latency             float64
}
