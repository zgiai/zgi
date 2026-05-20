package loop

import (
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/condbranch"
)

type VariableValueType string

const (
	ValueTypeVariable VariableValueType = "variable"
	ValueTypeConstant VariableValueType = "constant"
)

type LoopVariableSpec struct {
	Label     string            `json:"label"`
	VarType   string            `json:"var_type"`
	ValueType VariableValueType `json:"value_type"`
	Value     any               `json:"value"`
}

type NodeData struct {
	base.NodeData
	StartNodeID     *string                    `json:"start_node_id,omitempty"`
	LoopCount       int                        `json:"loop_count"`
	ParallelNums    int                        `json:"parallel_nums,omitempty"`
	BreakConditions []condbranch.Condition     `json:"break_conditions,omitempty"`
	LogicalOperator condbranch.LogicalOperator `json:"logical_operator,omitempty"`
	LoopVariables   []LoopVariableSpec         `json:"loop_variables,omitempty"`
	Outputs         map[string]any             `json:"outputs,omitempty"`
}

type StartNodeData struct {
	base.NodeData
}

type EndNodeData struct {
	base.NodeData
}
