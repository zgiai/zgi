package iter

import (
	"errors"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
)

// ErrorHandleMode defines how the iteration node deals with failures that
// occur while running child sub-graphs.
type ErrorHandleMode string

const (
	ErrorHandleModeTerminated           ErrorHandleMode = "terminated"
	ErrorHandleModeContinueOnError      ErrorHandleMode = "continue-on-error"
	ErrorHandleModeRemoveAbnormalOutput ErrorHandleMode = "remove-abnormal-output"
)

type NodeData struct {
	base.NodeData
	StartNodeID      *string         `json:"start_node_id,omitempty"`
	ParentLoopID     *string         `json:"parent_loop_id,omitempty"`
	IteratorSelector []string        `json:"iterator_selector"`
	OutputSelector   []string        `json:"output_selector"`
	IsParallel       bool            `json:"is_parallel"`
	ParallelNums     int             `json:"parallel_nums"`
	ErrorHandleMode  ErrorHandleMode `json:"error_handle_mode"`
}

// StartNodeData  currently does not add extra fields
// but keeps the structure for parity and forward compatibility.
type StartNodeData struct {
	base.NodeData
}

// StateMetadata stores per-run metadata collected for an iteration execution.
type StateMetadata struct {
	IteratorLength int `json:"iterator_length"`
}

// State keeps the runtime information generated during iteration execution.
type State struct {
	IterationNodeID string         `json:"iteration_node_id"`
	Index           int            `json:"index"`
	Inputs          map[string]any `json:"inputs"`
	Outputs         []any          `json:"outputs"`
	CurrentOutput   any            `json:"current_output,omitempty"`
	Metadata        StateMetadata  `json:"metadata"`
}

type Node struct {
	base.NodeStruct
	nodeData NodeData

	now              func() time.Time
	llmClient        interface{}
	contentExtractor interface{}
	toolEngine       interface{}
	engineFactory    subgraph.EngineFactory
}

var (
	errIteratorVariableNotFound = errors.New("iterator variable not found")
	errInvalidIteratorValue     = errors.New("invalid iterator value, expect an array-like value")
	errStartNodeIDMissing       = errors.New("start_node_id is required for iteration node")
)

type iterationRunOutcome struct {
	Output               any
	Tokens               int
	ConversationSnapshot map[string]any
}
