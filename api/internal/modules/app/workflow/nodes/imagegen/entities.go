package imagegen

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	llmnode "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/llm"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

const (
	defaultImageCount = 1
)

type NodeData struct {
	base.NodeData

	Model        ImageModelConfig     `json:"model"`
	Prompt       string               `json:"prompt"`
	PromptConfig llmnode.PromptConfig `json:"prompt_config"`
	Generation   GenerationConfig     `json:"generation"`
	Output       OutputConfig         `json:"output"`
}

type ImageModelConfig struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

type GenerationConfig struct {
	N       int    `json:"n"`
	Size    string `json:"size,omitempty"`
	Quality string `json:"quality,omitempty"`
	Style   string `json:"style,omitempty"`
}

type OutputConfig struct {
	Lifecycle string `json:"lifecycle,omitempty"`
}

func parseNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	rawNodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}

	nodeID, ok := rawNodeID.(string)
	if !ok || strings.TrimSpace(nodeID) == "" {
		return NodeData{}, "", fmt.Errorf("node ID must be non-empty string")
	}

	rawData, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	payload, err := json.Marshal(rawData)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	nodeData.ensureDefaults()
	if err := nodeData.validate(); err != nil {
		return NodeData{}, "", err
	}

	return nodeData, nodeID, nil
}

func (nd *NodeData) ensureDefaults() {
	if nd.PromptConfig.TemplateVariables == nil {
		nd.PromptConfig = llmnode.NewPromptConfig()
	}
	if nd.Generation.N <= 0 {
		nd.Generation.N = defaultImageCount
	}
	if strings.TrimSpace(nd.Output.Lifecycle) == "" {
		nd.Output.Lifecycle = string(tool_file.ToolFileLifecyclePersistent)
	}
}

func (nd *NodeData) validate() error {
	if strings.TrimSpace(nd.Model.Provider) == "" {
		return fmt.Errorf("model provider is required")
	}
	if strings.TrimSpace(nd.Model.Name) == "" {
		return fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(nd.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if nd.Generation.N < 1 {
		return fmt.Errorf("generation.n must be greater than or equal to 1")
	}

	switch strings.TrimSpace(nd.Output.Lifecycle) {
	case string(tool_file.ToolFileLifecyclePersistent), string(tool_file.ToolFileLifecycleTemporary):
		return nil
	default:
		return fmt.Errorf("unsupported output lifecycle: %s", nd.Output.Lifecycle)
	}
}
