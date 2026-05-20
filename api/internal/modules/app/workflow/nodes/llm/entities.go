package llm

import (
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
)

const (
	AppType = "agent"
)

// NodeData represents the data structure for LLM nodes
type NodeData struct {
	base.NodeData
	Model                    ModelConfig      `json:"model"`
	PromptTemplate           any              `json:"prompt_template"` // Can be []NodeChatModelMessage or NodeCompletionModelPromptTemplate
	PromptSource             string           `json:"prompt_source,omitempty"`
	PromptReference          *PromptReference `json:"prompt_reference,omitempty"`
	PromptConfig             PromptConfig     `json:"prompt_config"`
	Memory                   *MemoryConfig    `json:"memory,omitempty"`
	Context                  ContextConfig    `json:"context"`
	Vision                   VisionConfig     `json:"vision"`
	StructuredOutput         map[string]any   `json:"structured_output,omitempty"`
	StructuredOutputSwitchOn bool             `json:"structured_output_switch_on"`
	ModelOverrideVariable    *string          `json:"model_override_variable,omitempty"` // Reference to variable pool variable for model override
}

type PromptReference struct {
	PromptID   string  `json:"prompt_id"`
	PromptName *string `json:"prompt_name,omitempty"`
	Version    *int    `json:"version,omitempty"`
	Label      *string `json:"label,omitempty"`
	Locale     *string `json:"locale,omitempty"`
	Source     *string `json:"source,omitempty"`
}

// NewLLMNodeData creates a new NodeData with default values
func NewLLMNodeData(
	bnd base.NodeData,
	m ModelConfig,
	p any,
	pt PromptConfig,
) *NodeData {
	return &NodeData{
		NodeData:       bnd,
		Model:          m,
		PromptTemplate: p,
		PromptConfig:   pt,
	}
}

// IsStructuredOutputEnabled returns whether structured output is enabled
func (d *NodeData) IsStructuredOutputEnabled() bool {
	return d.StructuredOutputSwitchOn && d.StructuredOutput != nil
}
