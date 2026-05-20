package jsonparser

import (
	"encoding/json"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

var AllowedOutputTypes = map[shared.SegmentType]bool{
	shared.SegmentTypeString:       true,
	shared.SegmentTypeNumber:       true,
	shared.SegmentTypeObject:       true,
	shared.SegmentTypeBoolean:      true,
	shared.SegmentTypeArrayString:  true,
	shared.SegmentTypeArrayNumber:  true,
	shared.SegmentTypeArrayObject:  true,
	shared.SegmentTypeArrayBoolean: true,
}

func ValidateOutputType(segmentType shared.SegmentType) error {
	if !AllowedOutputTypes[segmentType] {
		return fmt.Errorf("invalid type for json parser output, expected one of %v, actual %s", getAllowedTypes(), segmentType)
	}
	return nil
}

func getAllowedTypes() []shared.SegmentType {
	types := make([]shared.SegmentType, 0, len(AllowedOutputTypes))
	for t := range AllowedOutputTypes {
		types = append(types, t)
	}
	return types
}

type VariableSelector struct {
	Variable      string   `json:"variable"`
	ValueSelector []string `json:"value_selector"`
}

type Output struct {
	Type     shared.SegmentType `json:"type"`
	Children map[string]*Output `json:"children,omitempty"`
}

func (o *Output) Validate() error {
	if err := ValidateOutputType(o.Type); err != nil {
		return err
	}
	for key, child := range o.Children {
		if child != nil {
			if err := child.Validate(); err != nil {
				return fmt.Errorf("validation failed for child '%s': %w", key, err)
			}
		}
	}
	return nil
}

type NodeData struct {
	base.NodeData
	Variables       []VariableSelector    `json:"variables,omitempty"`
	InputSelector   []string              `json:"input_selector,omitempty"`
	IsFlattenOutput bool                  `json:"is_flatten_output,omitempty"`
	Outputs         map[string]Output     `json:"outputs"`
	ErrorStrategy   shared.ErrorStrategy  `json:"error_strategy,omitempty"`
	DefaultValue    []shared.DefaultValue `json:"default_value,omitempty"`
	RetryConfig     shared.RetryConfig    `json:"retry_config,omitempty"`
}

func (nd *NodeData) ValidateOutputs() error {
	for outputName, output := range nd.Outputs {
		if err := output.Validate(); err != nil {
			return fmt.Errorf("validation failed for output '%s': %w", outputName, err)
		}
	}
	return nil
}

type Node struct {
	base.NodeStruct
	NodeData
}

func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nd, nodeID, err := parseNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	if err := nd.ValidateOutputs(); err != nil {
		return nil, fmt.Errorf("invalid outputs configuration: %w", err)
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.JSONParser,

			TenantID:          graphInitParams.TenantID,
			APPID:             graphInitParams.AppID,
			WorkflowType:      string(graphInitParams.WorkflowType),
			WorkflowID:        graphInitParams.WorkflowID,
			UserFrom:          string(graphInitParams.UserFrom),
			UserID:            graphInitParams.UserID,
			GraphConfig:       graphInitParams.GraphConfig,
			InvokeFrom:        string(graphInitParams.InvokeFrom),
			WorkflowCallDepth: graphInitParams.CallDepth,

			Graph:             graph,
			GraphRuntimeState: graphRuntimeState,
			PreviousNodeID:    previousNodeID,
		},
		NodeData: nd,
	}, nil
}

func parseNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}

	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(jsonBytes, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	// Normalize ErrorStrategy: only "fail-branch" and "default-value" are valid strategies.
	// "none" and any unknown/empty values are treated as no strategy (empty string),
	// which causes the workflow to terminate on error — same as http-request node behaviour.
	switch nodeData.ErrorStrategy {
	case shared.FailBranch, shared.DefaultVal:
		// keep as-is
	default:
		nodeData.ErrorStrategy = ""
	}

	return nodeData, nodeIDStr, nil
}
