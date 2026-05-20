package knowledgeretrieval

import (
	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/llm"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	"gorm.io/gorm"
)

type retrievalMode string

const (
	single   retrievalMode = "single"
	multiple retrievalMode = "multiple"
)

const AppType = "agent"

type metadataFilteringMode string

const (
	disabled  metadataFilteringMode = "disabled"
	automatic metadataFilteringMode = "automatic"
	manual    metadataFilteringMode = "manual"
)

// Reranking mode constants
type rerankingMode string

const (
	RerankingModeNone          rerankingMode = "none"
	RerankingModeModel         rerankingMode = "reranking_model"
	RerankingModeWeightedScore rerankingMode = "weighted_score"
)

type SingleRetrievalConfig struct {
	TopK                  int      `json:"top_k"`
	ScoreThreshold        *float64 `json:"score_threshold"`
	ScoreThresholdEnabled *bool    `json:"score_threshold_enabled"`
	SearchMethod          string   `json:"search_method"` // keyword_search | full_text_search | semantic_search | hybrid_search
	RerankingEnable       bool     `json:"reranking_enable"`
	ContextSeparator      string   `json:"context_separator"` // e.g., "\n\n"
	MaxContextChars       int      `json:"max_context_chars"` // character cap for context

	llm.ModelConfig
}

type rerankModelConfig struct {
	ModelSlug string `json:"model,omitempty"`
	Provider  string `json:"provider,omitempty"`
}

type MultipleRetrievalConfig struct {
	TopK            int                `json:"top_k"`
	ScoreThreshold  *float64           `json:"score_threshold"`
	RerankingMode   rerankingMode      `json:"reranking_mode"`
	RerankingModel  *rerankModelConfig `json:"reranking_model,omitempty"`
	RerankingEnable bool               `json:"reranking_enable"`
	Weights         *WeightConfig      `json:"weights,omitempty"`
}

type WeightConfig struct {
	VectorSetting  VectorWeightSetting  `json:"vector_setting"`
	KeywordSetting KeywordWeightSetting `json:"keyword_setting"`
}

type VectorWeightSetting struct {
	VectorWeight          float64 `json:"vector_weight"`
	EmbeddingProviderName string  `json:"embedding_provider_name"`
	EmbeddingModelName    string  `json:"embedding_model_name"`
}

type KeywordWeightSetting struct {
	KeywordWeight float64 `json:"keyword_weight"`
}

// Condition represents a single metadata filter condition
// name: metadata field name
// comparison_operator: one of contains, not contains, start with, end with, in, not in, =, is, is not, ≠, empty, not empty, before, after, ≤, ≥
// value: optional value (string, number or comma-separated string for in/not in)
type Condition struct {
	Name               string `json:"name"`
	ComparisonOperator string `json:"comparison_operator"`
	Value              any    `json:"value"`
}

// MetadataFilteringCondition describes logical operator and conditions
type MetadataFilteringCondition struct {
	LogicalOperator string      `json:"logical_operator"` // and | or
	Conditions      []Condition `json:"conditions"`
}

// RetrievalOptions contains configuration for retrieval operations
type RetrievalOptions struct {
	SearchMethod          string
	TopK                  int
	ScoreThreshold        float64
	ScoreThresholdEnabled bool
	RerankingEnable       bool
	RerankingModel        map[string]any
	Weights               map[string]any
	DocumentIDsFilter     []string
}

type NodeData struct {
	base.NodeData
	QueryVariableSelector       []string                    `json:"query_variable_selector"`
	DatasetIds                  []string                    `json:"dataset_ids"`
	RetrievalMode               retrievalMode               `json:"retrieval_mode"`
	SingleRetrievalConfig       *SingleRetrievalConfig      `json:"single_retrieval_config"`
	MultipleRetrievalConfig     *MultipleRetrievalConfig    `json:"multiple_retrieval_config"`
	MetadataFilteringMode       *metadataFilteringMode      `json:"metadata_filtering_mode"`
	MetadataModelConfig         *llm.ModelConfig            `json:"metadata_model_config"`
	MetadataFilteringConditions *MetadataFilteringCondition `json:"metadata_filtering_conditions"`
	Vision                      llm.VisionConfig            `json:"vision"`
}

// Node represents the knowledge retrieval workflow node and its dependencies (DB, LLM invoker, etc.).
type Node struct {
	base.NodeStruct
	NodeData           NodeData
	organizationID     string
	billingSubjectType string
	fileOutputs        []*file.File
	llmFileSaver       llm.FileSaver
	db                 *gorm.DB
	llmInvoker         llmInvoker
	embInvoker         embeddingInvoker
	llmClient          llmclient.LLMClient
	graphFlowService   *graphflow.Service
}
