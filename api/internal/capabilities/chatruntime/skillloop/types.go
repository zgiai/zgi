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

const (
	clientActionContinuationPolicyResumeModel = "resume_model"
	clientActionContinuationPolicyRecordOnly  = "record_only"
)

var (
	ErrInvalidInput     = errors.New("invalid input")
	ErrModelIdleTimeout = errors.New("model idle timeout")
)

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

type UserInputPendingError struct {
	Payload map[string]interface{}
}

type PlanningTerminationError struct {
	Reason      string
	Recoverable bool
	Streaming   bool
}

func (e *PlanningTerminationError) Error() string {
	if e == nil {
		return "skill planning ended before a complete turn"
	}
	kind := "response"
	if e.Streaming {
		kind = "stream"
	}
	return fmt.Sprintf("skill planning %s ended before a complete turn: finish_reason=%s", kind, e.Reason)
}

func (e *UserInputPendingError) Error() string {
	if e == nil {
		return "user input is pending"
	}
	requestID := stringFromInterface(e.Payload["request_id"])
	if requestID == "" {
		return "user input is pending"
	}
	return fmt.Sprintf("user input is pending for %s", requestID)
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
	ModelIdleTimeout  time.Duration
	diagnostics       modelInvocationDiagnostics
	requestBudget     planningRequestBudget
}

type RunRequest struct {
	Prepared                       *PreparedChat
	Resolved                       *skills.ResolvedSkills
	ProtocolToolsOnly              bool
	LegacyToolChat                 bool
	ExecutionContext               skills.ExecutionContext
	PreferExplicitFinalAnswer      bool
	SuppressInitialNaturalProgress bool
	AdditionalSystemMessages       []adapter.Message
	RuntimeStateSnapshot           RuntimeStateSnapshotFunc
	CurrentMetadata                func() map[string]interface{}
	OnTerminalStateGuardDecision   func(TerminalStateGuardDecisionRecord)
	OnTerminalCompletion           func(TerminalCompletionResult)
	OnChunk                        func(string) error
	PlanningOutputTokenLimit       int
	AuthorizeSkillStep             func(context.Context, string) (bool, error)
	PreferredRestoredSkillID       string
	ContinuationType               string
	TerminalOnly                   bool
}

type TerminalStateGuardDecisionRecord struct {
	Path     string
	Reason   string
	Blockers []string
}

type RuntimeStateSnapshotFunc func() map[string]interface{}

type TerminalCompletionResult struct {
	Status   string
	Source   string
	Reason   string
	Blockers []string
}

type SkillToolCallRef struct {
	SkillID   string
	ToolName  string
	Arguments map[string]interface{}
	Result    map[string]interface{}
}

type ModelInvocationTrace struct {
	Phase                      string
	Round                      int
	Streaming                  bool
	StartedAt                  time.Time
	DurationMS                 int64
	Request                    *adapter.ChatRequest
	Response                   *adapter.Message
	Usage                      *adapter.Usage
	FinishReason               string
	StreamDoneReceived         bool
	TerminatedBy               string
	Error                      string
	PromptChars                int
	RequestChars               int
	EstimatedPromptTokens      int
	PromptEstimator            string
	PromptComponentTokens      map[string]int
	PromptComponentChars       map[string]int
	ActiveSkillIDs             []string
	LoadedSkillIDs             []string
	RestoredSkillIDs           []string
	ProjectedRefs              []string
	ProjectedChars             int
	ContinuationType           string
	TerminalOnly               bool
	BudgetSafeContextLimit     int
	BudgetPromptLimit          int
	BudgetOriginalPromptTokens int
	BudgetCompressionChars     map[string]int
	BudgetSavedChars           int
	BudgetMaxTokensClamped     bool
	BudgetOriginalMaxTokens    int
	BudgetEffectiveMaxTokens   int
	BudgetEstimateScale        float64
}

type modelInvocationDiagnostics struct {
	activeSkillIDs   []string
	loadedSkillIDs   []string
	restoredSkillIDs []string
	projectedRefs    []string
	projectedChars   int
	continuationType string
	terminalOnly     bool
	requestBudget    planningRequestBudgetDiagnostics
}

type PreparedChat struct {
	Conversation *Conversation
	Message      *Message
	Query        string
	CurrentRoute string
	Surface      string
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
	if trace.Request != nil && trace.PromptChars <= 0 {
		trace.PromptChars = chatRequestPromptChars(trace.Request)
	}
	if trace.Request != nil && trace.EstimatedPromptTokens <= 0 {
		estimate := chatRequestPromptEstimate(trace.Request)
		trace.RequestChars = estimate.Characters
		trace.EstimatedPromptTokens = estimate.Tokens
		trace.PromptEstimator = estimate.Tokenizer
		trace.PromptComponentTokens = make(map[string]int, len(estimate.Components))
		trace.PromptComponentChars = make(map[string]int, len(estimate.Components))
		for name, component := range estimate.Components {
			trace.PromptComponentTokens[name] = component.Tokens
			trace.PromptComponentChars[name] = component.Characters
		}
	}
	trace.PromptComponentTokens = cloneStringIntMap(trace.PromptComponentTokens)
	trace.PromptComponentChars = cloneStringIntMap(trace.PromptComponentChars)
	trace.ActiveSkillIDs = append([]string(nil), r.diagnostics.activeSkillIDs...)
	trace.LoadedSkillIDs = append([]string(nil), r.diagnostics.loadedSkillIDs...)
	trace.RestoredSkillIDs = append([]string(nil), r.diagnostics.restoredSkillIDs...)
	trace.ProjectedRefs = append([]string(nil), r.diagnostics.projectedRefs...)
	trace.ProjectedChars = r.diagnostics.projectedChars
	trace.ContinuationType = r.diagnostics.continuationType
	trace.TerminalOnly = r.diagnostics.terminalOnly
	trace.BudgetSafeContextLimit = r.diagnostics.requestBudget.safeContextLimit
	trace.BudgetPromptLimit = r.diagnostics.requestBudget.promptBudget
	trace.BudgetOriginalPromptTokens = r.diagnostics.requestBudget.originalPromptTokens
	trace.BudgetCompressionChars = cloneStringIntMap(r.diagnostics.requestBudget.compressionChars)
	trace.BudgetSavedChars = r.diagnostics.requestBudget.savedChars
	trace.BudgetMaxTokensClamped = r.diagnostics.requestBudget.maxTokensClamped
	trace.BudgetOriginalMaxTokens = r.diagnostics.requestBudget.originalMaxTokens
	trace.BudgetEffectiveMaxTokens = r.diagnostics.requestBudget.effectiveMaxTokens
	trace.BudgetEstimateScale = r.diagnostics.requestBudget.estimateScale
	r.OnModelInvocation(trace)
}

func cloneStringIntMap(source map[string]int) map[string]int {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]int, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (r *Runner) fallbackDelay() time.Duration {
	if r != nil && r.FallbackDelay > 0 {
		return r.FallbackDelay
	}
	return 800 * time.Millisecond
}

func (r *Runner) modelIdleTimeout() time.Duration {
	if r != nil && r.ModelIdleTimeout > 0 {
		return r.ModelIdleTimeout
	}
	return 5 * time.Minute
}

func backgroundContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
