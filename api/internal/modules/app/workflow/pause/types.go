package pause

import graphentities "github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"

const (
	StateVersion = "2"

	ReasonTypeApprovalRequired       = "approval_required"
	ReasonTypeQuestionAnswerRequired = "question_answer_required"

	EventWorkflowStarted         = "workflow_started"
	EventWorkflowPaused          = "workflow_paused"
	EventWorkflowFinished        = "workflow_finished"
	EventNodeStarted             = "node_started"
	EventNodeFinished            = "node_finished"
	EventApprovalRequested       = "approval_requested"
	EventApprovalResultFilled    = "approval_result_filled"
	EventApprovalExpired         = "approval_expired"
	EventQuestionAnswerRequested = "question_answer_requested"
	EventQuestionAnswerSubmitted = "question_answer_submitted"
	EventError                   = "error"
)

type State struct {
	Version       string               `json:"version"`
	WorkflowRunID string               `json:"workflow_run_id"`
	WorkflowID    string               `json:"workflow_id"`
	AppID         string               `json:"app_id"`
	TenantID      string               `json:"tenant_id"`
	RunType       string               `json:"run_type"`
	TriggeredFrom string               `json:"triggered_from"`
	Request       RequestState         `json:"request"`
	ExecutorState ExecutorState        `json:"executor_state"`
	VariablePool  VariablePoolSnapshot `json:"variable_pool"`
	AnswerOutput  *AnswerOutputState   `json:"answer_output,omitempty"`
}

type RequestState struct {
	Inputs       map[string]interface{} `json:"inputs"`
	ResponseMode string                 `json:"response_mode"`
}

type ExecutorState struct {
	PausedNodeID      string                            `json:"paused_node_id"`
	PausedNodeIDs     []string                          `json:"paused_node_ids,omitempty"`
	NodeQueue         []string                          `json:"node_queue"`
	CompletedNodes    map[string]bool                   `json:"completed_nodes"`
	FailedNodes       map[string]string                 `json:"failed_nodes"`
	ExecutionOutputs  map[string]map[string]interface{} `json:"execution_outputs"`
	AllNodeOutputs    map[string]interface{}            `json:"all_node_outputs"`
	NodeIndex         int                               `json:"node_index"`
	TotalTokens       int                               `json:"total_tokens"`
	PredecessorNodeID *string                           `json:"predecessor_node_id"`
}

type VariablePoolSnapshot struct {
	Variables       map[string]map[string]interface{} `json:"variables"`
	UserInputs      map[string]interface{}            `json:"user_inputs"`
	SystemVariables *graphentities.SystemVariable     `json:"system_variables"`
}

type AnswerOutputState struct {
	FullAnswer  string                      `json:"full_answer,omitempty"`
	MessageSent bool                        `json:"message_sent,omitempty"`
	Emitters    []AnswerOutputEmitterState  `json:"emitters,omitempty"`
	Variables   []AnswerOutputVariableState `json:"variables,omitempty"`
}

type AnswerOutputEmitterState struct {
	NodeID              string `json:"node_id"`
	Lifecycle           string `json:"lifecycle"`
	CurrentIndex        int    `json:"current_index"`
	Drained             bool   `json:"drained"`
	TemplateFingerprint string `json:"template_fingerprint,omitempty"`
}

type AnswerOutputVariableState struct {
	StateKey         string `json:"state_key"`
	ReleasedText     string `json:"released_text,omitempty"`
	HasFinal         bool   `json:"has_final,omitempty"`
	FinalValue       string `json:"final_value,omitempty"`
	FinalOnly        bool   `json:"final_only,omitempty"`
	SourceSkipped    bool   `json:"source_skipped,omitempty"`
	SourceFailed     bool   `json:"source_failed,omitempty"`
	FinalizedSegment bool   `json:"finalized_segment,omitempty"`
}

type Reason struct {
	Type   string `json:"type"`
	NodeID string `json:"node_id"`
	FormID string `json:"form_id,omitempty"`
}

type SaveParams struct {
	TenantID       string
	AppID          string
	WorkflowRunID  string
	NodeID         string
	Reason         string
	ConversationID string
	State          State
	Reasons        []Reason
}

type AppendEventParams struct {
	TenantID      string
	AppID         string
	WorkflowRunID string
	EventType     string
	EventData     map[string]interface{}
}

type RunEventPayload struct {
	Sequence  int                    `json:"sequence"`
	Event     string                 `json:"event"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt int64                  `json:"created_at"`
}

type RunEventsPayload struct {
	WorkflowRunID string            `json:"workflow_run_id"`
	Events        []RunEventPayload `json:"events"`
}
