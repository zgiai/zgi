package shared

import (
	"errors"
)

// ---------------------------------------------------------- node ----------------------------------------------------------

type NodeType string

const (
	Start               NodeType = "start"
	End                 NodeType = "end"
	Answer              NodeType = "answer"
	LLM                 NodeType = "llm"
	KnowledgeRetrieval  NodeType = "knowledge-retrieval"
	IfElse              NodeType = "if-else"
	Code                NodeType = "code"
	TemplateTransform   NodeType = "template-transform"
	QuestionClassifier  NodeType = "question-classifier"
	HTTPRequest         NodeType = "http-request"
	Tools               NodeType = "tools" // New plugin runner system
	CallDatabase        NodeType = "call-database"
	SQLGenerator        NodeType = "sql-generator"
	VariableAggregator  NodeType = "variable-aggregator"
	Loop                NodeType = "loop"
	LoopStart           NodeType = "loop-start"
	LoopEnd             NodeType = "loop-end"
	Iteration           NodeType = "iteration"
	IterationStart      NodeType = "iteration-start"
	ParameterExtractor  NodeType = "parameter-extractor"
	VariableAssigner    NodeType = "assigner"
	DocumentExtractor   NodeType = "document-extractor"
	ImageGen            NodeType = "image-gen"
	ListOperator        NodeType = "list-operator"
	Agent               NodeType = "agent"
	JSONParser          NodeType = "json-parser"
	CreateScheduledTask NodeType = "create-scheduled-task"
	Approval            NodeType = "approval"
	Announcement        NodeType = "announcement"
	QuestionAnswer      NodeType = "question-answer"
	NotificationSMS     NodeType = "notification-sms"
)

var executableNodeTypes = map[NodeType]struct{}{
	Start:               {},
	End:                 {},
	Answer:              {},
	LLM:                 {},
	KnowledgeRetrieval:  {},
	IfElse:              {},
	Code:                {},
	HTTPRequest:         {},
	Tools:               {},
	CallDatabase:        {},
	SQLGenerator:        {},
	VariableAggregator:  {},
	Loop:                {},
	LoopStart:           {},
	LoopEnd:             {},
	Iteration:           {},
	IterationStart:      {},
	ParameterExtractor:  {},
	VariableAssigner:    {},
	DocumentExtractor:   {},
	ImageGen:            {},
	JSONParser:          {},
	CreateScheduledTask: {},
	Approval:            {},
	Announcement:        {},
	QuestionAnswer:      {},
	NotificationSMS:     {},
}

func IsExecutableNodeType(nodeType NodeType) bool {
	_, exists := executableNodeTypes[nodeType]
	return exists
}

type ErrorStrategy string

const (
	FailBranch ErrorStrategy = "fail-branch"   // Route to fail branch
	DefaultVal ErrorStrategy = "default-value" // Use configured default values
)

type FailBranchSourceHandle string

const (
	FailedBranch FailBranchSourceHandle = "fail-branch"
	SUCCESS      FailBranchSourceHandle = "success-branch"
)

type DefaultValueType string

const (
	TypeString      DefaultValueType = "string"
	TypeNumber      DefaultValueType = "number"
	TypeObject      DefaultValueType = "object"
	TypeArrayNumber DefaultValueType = "array[number]"
	TypeArrayString DefaultValueType = "array[string]"
	TypeArrayObject DefaultValueType = "array[object]"
	TypeArrayFiles  DefaultValueType = "array[file]"
)

var (
	NodeErr         = errors.New("node error")
	DefaultValueErr = errors.New("default value error")
)

type WorkflowNodeExecutionStatus string

const (
	PENDING   WorkflowNodeExecutionStatus = "pending"
	RUNNING   WorkflowNodeExecutionStatus = "running"
	SUCCEEDED WorkflowNodeExecutionStatus = "succeeded"
	FAILED    WorkflowNodeExecutionStatus = "failed"
	EXCEPTION WorkflowNodeExecutionStatus = "exception"
	RETRY     WorkflowNodeExecutionStatus = "retry"
	SKIPPED   WorkflowNodeExecutionStatus = "skipped"
	PAUSED    WorkflowNodeExecutionStatus = "paused"
)

type WorkflowNodeExecutionMetadataKey string

const (
	TotalTokens               WorkflowNodeExecutionMetadataKey = "total_tokens"
	TotalPrice                WorkflowNodeExecutionMetadataKey = "total_price"
	Currency                  WorkflowNodeExecutionMetadataKey = "currency"
	ResolvedModelProvider     WorkflowNodeExecutionMetadataKey = "resolved_model_provider"
	ResolvedModelName         WorkflowNodeExecutionMetadataKey = "resolved_model_name"
	ResolvedModelSource       WorkflowNodeExecutionMetadataKey = "resolved_model_source"
	ToolInfo                  WorkflowNodeExecutionMetadataKey = "tool_info"
	AgentLog                  WorkflowNodeExecutionMetadataKey = "agent_log"
	ITERATION_ID              WorkflowNodeExecutionMetadataKey = "iteration_id"
	IterationIndex            WorkflowNodeExecutionMetadataKey = "iteration_index"
	IterationItem             WorkflowNodeExecutionMetadataKey = "iteration_item"
	LoopId                    WorkflowNodeExecutionMetadataKey = "loop_id"
	LoopIndex                 WorkflowNodeExecutionMetadataKey = "loop_index"
	ParallelId                WorkflowNodeExecutionMetadataKey = "parallel_id"
	ParallelStartNodeId       WorkflowNodeExecutionMetadataKey = "parallel_start_node_id"
	ParentParallelId          WorkflowNodeExecutionMetadataKey = "parent_parallel_id"
	ParentParallelStartNodeId WorkflowNodeExecutionMetadataKey = "parent_parallel_start_node_id"
	ParallelModeRunId         WorkflowNodeExecutionMetadataKey = "parallel_mode_run_id"
	IterationDurationMap      WorkflowNodeExecutionMetadataKey = "iteration_duration_map"  // per-iteration duration in milliseconds
	IterationDurationList     WorkflowNodeExecutionMetadataKey = "iteration_duration_list" // per-iteration duration list in milliseconds
	IterationExecutionTrace   WorkflowNodeExecutionMetadataKey = "iteration_execution_trace"
	LoopDurationMap           WorkflowNodeExecutionMetadataKey = "loop_duration_map" // single loop duration if loop node runs
	ErrStrategy               WorkflowNodeExecutionMetadataKey = "error_strategy"    //  node in continue on error mode return the field
	LoopVariableMap           WorkflowNodeExecutionMetadataKey = "loop_variable_map" // single loop variable output
)

const (
	ResolvedModelSourceNodeDefault   = "node_default"
	ResolvedModelSourceInputOverride = "input_override"
)

type NodeEventType string

const (
	EventTypeRunCompleted         NodeEventType = "run_completed"
	EventTypeRunFailed            NodeEventType = "run_failed"
	EventTypeRunStarted           NodeEventType = "run_started"
	EventTypeRunStreamChunk       NodeEventType = "run_stream_chunk"
	EventTypeModelInvokeCompleted NodeEventType = "model_invoke_completed"
	EventTypeRetrieverResource    NodeEventType = "run_retriever_resource"
	EventTypeLLMStructuredOutput  NodeEventType = "llm_structured_output"
	EventTypeIterationStarted     NodeEventType = "iteration_started"
	EventTypeIterationNext        NodeEventType = "iteration_next"
	EventTypeIterationSucceeded   NodeEventType = "iteration_succeeded"
	EventTypeIterationFailed      NodeEventType = "iteration_failed"
	EventTypeLoopStarted          NodeEventType = "loop_started"
	EventTypeLoopNext             NodeEventType = "loop_next"
	EventTypeLoopSucceeded        NodeEventType = "loop_succeeded"
	EventTypeLoopFailed           NodeEventType = "loop_failed"
	EventTypeInternalNodeStarted  NodeEventType = "internal_node_started"  // Internal node started within subgraph
	EventTypeInternalNodeFinished NodeEventType = "internal_node_finished" // Internal node finished within subgraph
	EventTypeInternalReadyBatch   NodeEventType = "internal_ready_batch"
	// Other event types...
)

// SegmentType enumeration
type SegmentType string

const (
	SegmentTypeAny     SegmentType = "any"
	SegmentTypeNumber  SegmentType = "number"
	SegmentTypeInteger SegmentType = "integer"
	SegmentTypeFloat   SegmentType = "float"
	SegmentTypeString  SegmentType = "string"
	SegmentTypeObject  SegmentType = "object"
	SegmentTypeSecret  SegmentType = "secret"

	SegmentTypeFile    SegmentType = "file"
	SegmentTypeBoolean SegmentType = "boolean"

	SegmentTypeArrayAny     SegmentType = "array[any]"
	SegmentTypeArrayString  SegmentType = "array[string]"
	SegmentTypeArrayNumber  SegmentType = "array[number]"
	SegmentTypeArrayObject  SegmentType = "array[object]"
	SegmentTypeArrayFile    SegmentType = "array[file]"
	SegmentTypeArrayBoolean SegmentType = "array[boolean]"

	SegmentTypeNone SegmentType = "none"

	SegmentTypeGroup SegmentType = "group"
)

// RouteNodeStatus enumeration for route node status
type RouteNodeStatus string

const (
	RouteNodeStatusRunning   RouteNodeStatus = "running"
	RouteNodeStatusSuccess   RouteNodeStatus = "success"
	RouteNodeStatusFailed    RouteNodeStatus = "failed"
	RouteNodeStatusPaused    RouteNodeStatus = "paused"
	RouteNodeStatusException RouteNodeStatus = "exception"
)

// ----------------------------------------------------------node ----------------------------------------------------------
