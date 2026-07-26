package pause

import (
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

const (
	StateVersion = "2"

	ReasonTypeApprovalRequired       = "approval_required"
	ReasonTypeQuestionAnswerRequired = "question_answer_required"

	EventWorkflowStarted         = "workflow_started"
	EventWorkflowPaused          = "workflow_paused"
	EventWorkflowResumed         = "workflow_resumed"
	EventWorkflowFinished        = "workflow_finished"
	EventNodeStarted             = "node_started"
	EventNodeFinished            = "node_finished"
	EventApprovalRequested       = "approval_requested"
	EventApprovalResultFilled    = "approval_result_filled"
	EventApprovalExpired         = "approval_expired"
	EventQuestionAnswerRequested = "question_answer_requested"
	EventQuestionAnswerSubmitted = "question_answer_submitted"
	EventError                   = "error"

	EventCategoryControl          = "control"
	EventCategoryExecution        = "execution"
	EventCategoryInteraction      = "interaction"
	EventCategoryAnswerCheckpoint = "answer_checkpoint"

	RunPauseStatusPaused      = "paused"
	RunPauseStatusResumeReady = "resume_ready"
	RunPauseStatusResuming    = "resuming"
	RunPauseStatusClosed      = "closed"

	RunPauseReasonStatusPending   = "pending"
	RunPauseReasonStatusCompleted = "completed"

	RuntimeOutboxKindResume = "resume"
	RuntimeOutboxPending    = "pending"
	RuntimeOutboxPublished  = "published"
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
	TenantID            string
	AppID               string
	WorkflowRunID       string
	NodeID              string
	Reason              string
	ConversationID      string
	State               State
	Reasons             []Reason
	ExecutionID         string
	Generation          int64
	RunOutputsJSON      string
	RunElapsedTime      float64
	RunTotalTokens      int64
	RunTotalSteps       int
	MessageStatus       string
	MessageAnswer       string
	UpdateMessageAnswer bool
	MessageProjection   *conversation.AgentMessage
	NodeUpdates         []NodePauseUpdate
	Events              []AppendEventParams
}

type NodePauseUpdate struct {
	NodeLogID         string
	Outputs           map[string]interface{}
	ProcessData       map[string]interface{}
	ExecutionMetadata map[string]interface{}
	ElapsedTime       float64
}

type ExecutionClaim struct {
	WorkflowRunID   string
	PauseID         string
	Generation      int64
	PauseGeneration int64
	ExecutionID     string
	LeaseExpires    time.Time
	EventCursor     int
	Event           *RunEventPayload
}

type InteractionSubmission struct {
	Event         *RunEventPayload
	PendingEvents []*RunEventPayload
	Outbox        *RuntimeOutbox
	ResumeReady   bool
}

type RuntimeOutboxPayload struct {
	WorkflowRunID   string                 `json:"workflow_run_id"`
	PauseID         string                 `json:"pause_id"`
	Generation      int64                  `json:"generation"`
	TriggerID       string                 `json:"trigger_id"`
	InteractionType string                 `json:"interaction_type,omitempty"`
	ResumeInputs    map[string]interface{} `json:"resume_inputs,omitempty"`
}

type AppendEventParams struct {
	TenantID        string
	AppID           string
	WorkflowRunID   string
	EventType       string
	EventData       map[string]interface{}
	SchemaVersion   int
	Category        string
	ExecutionID     string
	PauseID         string
	PauseGeneration *int64
	IdempotencyKey  string
	OccurredAt      time.Time

	// ExpectedExecutionID and ExpectedExecutionGeneration fence writes from a
	// stale workflow worker after a lease takeover. They are intentionally
	// separate from ExecutionID: ExecutionID describes the event envelope,
	// while the expected fields authorize the mutation that allocates sequence.
	ExpectedExecutionID         string
	ExpectedExecutionGeneration int64
	// ExpectedPause* fences interaction events that are produced outside an
	// active execution, such as approval and question submissions.
	ExpectedPauseID         string
	ExpectedPauseGeneration *int64
	ExpectedPauseRevision   *int64
}

// RuntimeFence authorizes a batch of event mutations against one workflow
// execution or pause generation. All events in a batch share the same fence so
// the run row and pause row only need to be validated once.
type RuntimeFence struct {
	ExpectedExecutionID         string
	ExpectedExecutionGeneration int64
	ExpectedPauseID             string
	ExpectedPauseGeneration     *int64
	ExpectedPauseRevision       *int64
}

// EventDraft is the event-specific portion of a durable batch append.
type EventDraft struct {
	EventType       string
	EventData       map[string]interface{}
	SchemaVersion   int
	Category        string
	ExecutionID     string
	PauseID         string
	PauseGeneration *int64
	IdempotencyKey  string
	OccurredAt      time.Time
}

// AppendEventBatchRequest appends events for one workflow run in request order.
// Callers retain ownership of EventData; the persistence layer serializes it
// before returning and does not retain the map.
type AppendEventBatchRequest struct {
	TenantID      string
	AppID         string
	WorkflowRunID string
	FlushReason   string
	Fence         RuntimeFence
	Events        []EventDraft
}

// StoredEvent reports the durable event associated with a draft. Inserted is
// false when an idempotent retry resolved to an existing event.
type StoredEvent struct {
	Payload  *RunEventPayload
	Inserted bool
}

type RunEventPayload struct {
	EventID         string                 `json:"event_id,omitempty"`
	Sequence        int                    `json:"sequence"`
	Event           string                 `json:"event"`
	Category        string                 `json:"category,omitempty"`
	SchemaVersion   int                    `json:"schema_version,omitempty"`
	PayloadVersion  int                    `json:"payload_version,omitempty"`
	ExecutionID     string                 `json:"execution_id,omitempty"`
	PauseID         string                 `json:"pause_id,omitempty"`
	PauseGeneration *int64                 `json:"pause_generation,omitempty"`
	IdempotencyKey  string                 `json:"idempotency_key,omitempty"`
	Data            map[string]interface{} `json:"data"`
	CreatedAt       int64                  `json:"created_at"`
	OccurredAtMS    int64                  `json:"occurred_at_ms,omitempty"`
	RecordedAtMS    int64                  `json:"recorded_at_ms,omitempty"`
}

type RunEventsPayload struct {
	WorkflowRunID string            `json:"workflow_run_id"`
	Events        []RunEventPayload `json:"events"`
}
