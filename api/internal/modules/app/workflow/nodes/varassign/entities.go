package varassign

import "github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"

const updatedVariablesKey = "updated_variables"

type InputType string

const (
	InputTypeVariable InputType = "variable"
	InputTypeConstant InputType = "constant"
)

type Operation string

const (
	OperationOverWrite   Operation = "over-write"
	OperationClear       Operation = "clear"
	OperationAppend      Operation = "append"
	OperationExtend      Operation = "extend"
	OperationSet         Operation = "set"
	OperationAdd         Operation = "+="
	OperationSubtract    Operation = "-="
	OperationMultiply    Operation = "*="
	OperationDivide      Operation = "/="
	OperationRemoveFirst Operation = "remove-first"
	OperationRemoveLast  Operation = "remove-last"
)

type VariableOperationItem struct {
	VariableSelector []string  `json:"variable_selector"`
	InputType        InputType `json:"input_type"`
	Operation        Operation `json:"operation"`
	Value            any       `json:"value,omitempty"`
}

type NodeData struct {
	base.NodeData
	Items []VariableOperationItem `json:"items,omitempty"`
}

type Node struct {
	base.NodeStruct
	NodeData          NodeData
	conversationSaver conversationVariablePersister
}
