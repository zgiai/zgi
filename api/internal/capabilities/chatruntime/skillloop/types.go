package skillloop

import (
	"context"
	"errors"
	"fmt"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	EventMessage                = "message"
	EventMessageRetract         = "message_retract"
	EventAgentProgress          = "agent_progress"
	EventIntermediateAnswer     = "agent_intermediate_answer"
	EventUserInputRequested     = "user_input_requested"
	EventSkillCallStart         = "skill_call_start"
	EventSkillCallEnd           = "skill_call_end"
	EventSkillCallError         = "skill_call_error"
	EventClientActionRequired   = "client_action_required"
	EventToolGovernanceDecision = "tool_governance_decision"
	EventSkillLoadStart         = "skill_load_start"
	EventSkillLoadEnd           = "skill_load_end"
	EventSkillReferenceRead     = "skill_reference_read"
	EventSkillArtifactCreated   = "skill_artifact_created"
	EventWorkflowStarted        = "workflow_started"
	EventWorkflowNodeStarted    = "node_started"
	EventWorkflowNodeFinished   = "node_finished"
	EventWorkflowPaused         = "workflow_paused"
	EventWorkflowApproval       = "approval_requested"
	EventWorkflowFinished       = "workflow_finished"
	EventWorkflowFailed         = "workflow_failed"
)

var ErrInvalidInput = errors.New("invalid input")

type WorkflowApprovalPendingError struct {
	Payload map[string]interface{}
}

func (e *WorkflowApprovalPendingError) Error() string {
	if e == nil {
		return "workflow approval is pending"
	}
	workflowRunID := stringFromInterface(e.Payload["workflow_run_id"])
	if workflowRunID == "" {
		return "workflow approval is pending"
	}
	return fmt.Sprintf("workflow approval is pending for run %s", workflowRunID)
}

type WorkflowQuestionPendingError struct {
	Payload map[string]interface{}
}

func (e *WorkflowQuestionPendingError) Error() string {
	if e == nil {
		return "workflow question is pending"
	}
	workflowRunID := stringFromInterface(e.Payload["workflow_run_id"])
	if workflowRunID == "" {
		return "workflow question is pending"
	}
	return fmt.Sprintf("workflow question is pending for run %s", workflowRunID)
}

type ToolGovernancePendingError struct {
	Payload map[string]interface{}
}

func (e *ToolGovernancePendingError) Error() string {
	if e == nil {
		return "tool governance approval is pending"
	}
	correlationID := stringFromInterface(e.Payload["correlation_id"])
	if correlationID == "" {
		return "tool governance approval is pending"
	}
	return fmt.Sprintf("tool governance approval is pending for %s", correlationID)
}

type ClientActionPendingError struct {
	Payload map[string]interface{}
}

func (e *ClientActionPendingError) Error() string {
	if e == nil {
		return "client action is pending"
	}
	actionID := stringFromInterface(e.Payload["action_id"])
	if actionID == "" {
		return "client action is pending"
	}
	return fmt.Sprintf("client action is pending for %s", actionID)
}

type Event struct {
	Type    string
	Payload map[string]interface{}
}

type Runner struct {
	LLMClient         llmclient.LLMClient
	SkillRuntime      *skills.Runtime
	AppContext        *llmclient.AppContext
	OnEvent           func(Event) error
	OnTrace           func([]skills.SkillTrace, skills.SkillTrace)
	OnArtifact        func(map[string]interface{})
	OnModelInvocation func(ModelInvocationTrace)
	FallbackDelay     time.Duration
}

type RunRequest struct {
	Prepared                 *PreparedChat
	Resolved                 *skills.ResolvedSkills
	ExecutionContext         skills.ExecutionContext
	AdditionalSystemMessages []adapter.Message
	FinalAnswerGuard         FinalAnswerGuard
	UserInputGuard           UserInputGuard
	ToolCallGuard            ToolCallGuard
	PlanToolGuard            ToolCallGuard
	ToolArgumentResolver     ToolArgumentResolver
	CompletionEvidence       CompletionEvidenceFunc
	CurrentMetadata          func() map[string]interface{}
	OnCompletionVerification func(CompletionVerificationResult)
	OnChunk                  func(string) error
}

type FinalAnswerGuard func(FinalAnswerGuardRequest) (FinalAnswerGuardResult, bool)

type UserInputGuard func(UserInputGuardRequest) (FinalAnswerGuardResult, bool)

type ToolCallGuard func(ToolCallGuardRequest) (FinalAnswerGuardResult, bool)

type ToolArgumentResolver func(ToolCallGuardRequest) (map[string]interface{}, bool)

type CompletionEvidenceFunc func() map[string]interface{}

type CompletionVerificationResult struct {
	Status            string
	Reason            string
	MissingSteps      []string
	UnsupportedClaims []string
	NextActionHint    string
	FinalAnswer       string
}

type FinalAnswerGuardRequest struct {
	Answer              string
	Round               int
	SkillUsed           bool
	ToolCallCount       int
	AttemptedToolCalls  []SkillToolCallRef
	SuccessfulToolCalls []SkillToolCallRef
}

type FinalAnswerGuardResult struct {
	SkillID       string
	ToolName      string
	Message       string
	SystemMessage string
	Advisory      bool
}

type UserInputGuardRequest struct {
	Message             string
	Questions           []map[string]interface{}
	Round               int
	SkillUsed           bool
	ToolCallCount       int
	AttemptedToolCalls  []SkillToolCallRef
	SuccessfulToolCalls []SkillToolCallRef
}

type ToolCallGuardRequest struct {
	SkillID             string
	ToolName            string
	Arguments           map[string]interface{}
	Round               int
	SkillUsed           bool
	ToolCallCount       int
	AttemptedToolCalls  []SkillToolCallRef
	SuccessfulToolCalls []SkillToolCallRef
}

type SkillToolCallRef struct {
	SkillID   string
	ToolName  string
	Arguments map[string]interface{}
	Result    map[string]interface{}
}

type ModelInvocationTrace struct {
	Phase      string
	Round      int
	Streaming  bool
	StartedAt  time.Time
	DurationMS int64
	Request    *adapter.ChatRequest
	Response   *adapter.Message
	Usage      *adapter.Usage
	Error      string
}

type PreparedChat struct {
	Conversation *Conversation
	Message      *Message
	Query        string
	CurrentRoute string
	parts        *chatParts
	LLMRequest   *adapter.ChatRequest
}

type Conversation struct {
	ID stringID
}

type Message struct {
	ID stringID
}

type chatParts struct {
	Provider  string
	SkillMode string
}

type stringID string

func (id stringID) String() string { return string(id) }

func NewPreparedChat(conversationID string, messageID string, provider string, skillMode string, req *adapter.ChatRequest) *PreparedChat {
	return &PreparedChat{
		Conversation: &Conversation{ID: stringID(conversationID)},
		Message:      &Message{ID: stringID(messageID)},
		parts:        &chatParts{Provider: provider, SkillMode: skillMode},
		LLMRequest:   req,
	}
}

func (r *Runner) emitEvent(eventType string, payload map[string]interface{}) {
	if r == nil || r.OnEvent == nil {
		return
	}
	_ = r.OnEvent(Event{Type: eventType, Payload: payload})
}

func (r *Runner) recordTrace(traces []skills.SkillTrace, trace skills.SkillTrace) {
	if r == nil || r.OnTrace == nil {
		return
	}
	r.OnTrace(traces, trace)
}

func (r *Runner) recordArtifact(artifact map[string]interface{}) {
	if r == nil || r.OnArtifact == nil || len(artifact) == 0 {
		return
	}
	r.OnArtifact(artifact)
}

func (r *Runner) recordModelInvocation(trace ModelInvocationTrace) {
	if r == nil || r.OnModelInvocation == nil {
		return
	}
	if trace.StartedAt.IsZero() {
		trace.StartedAt = time.Now()
	}
	if trace.Request != nil {
		trace.Request = cloneChatRequest(trace.Request)
	}
	if trace.Response != nil {
		cloned := *trace.Response
		cloned.ToolCalls = append([]adapter.ToolCall{}, trace.Response.ToolCalls...)
		trace.Response = &cloned
	}
	if trace.Usage != nil {
		cloned := *trace.Usage
		trace.Usage = &cloned
	}
	r.OnModelInvocation(trace)
}

func (r *Runner) fallbackDelay() time.Duration {
	if r != nil && r.FallbackDelay > 0 {
		return r.FallbackDelay
	}
	return 800 * time.Millisecond
}

func backgroundContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
