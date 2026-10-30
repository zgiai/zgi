package skillloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/contextmgr"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
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
	EventConnectorConfirmation  = "connector_confirmation_required"
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
	ErrInvalidInput           = errors.New("invalid input")
	ErrModelIdleTimeout       = errors.New("model idle timeout")
	ErrFinalAnswerUnavailable = errors.New("final answer unavailable")
	ErrAgentOutputTruncated   = errors.New("agent output truncated")
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

// ConnectorConfirmationPendingError is intentionally independent from the
// legacy client-action and governance continuations. Its opaque receipt is
// resolved only through the shared Connector confirmation endpoint.
type ConnectorConfirmationPendingError struct {
	Payload map[string]interface{}
}

func (e *ConnectorConfirmationPendingError) Error() string {
	if e == nil {
		return "Connector confirmation is pending"
	}
	receipt := stringFromInterface(e.Payload["route_receipt"])
	if receipt == "" {
		return "Connector confirmation is pending"
	}
	return fmt.Sprintf("Connector confirmation is pending for %s", receipt)
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
	OnModelRequest    func(phase string, round int, request *adapter.ChatRequest, decision *contextmgr.Decision)
	OnModelInvocation func(ModelInvocationTrace)
	FallbackDelay     time.Duration
	ModelIdleTimeout  time.Duration
	// ModelRetryDelays are the waits before each retry of a model call that
	// failed with a transient error; nil uses defaultModelRetryDelays.
	ModelRetryDelays []time.Duration
	// prefetchedCalls are the read-only calls of the current round already
	// started (prefetchReadOnlyCalls).
	prefetchedCalls       map[string]*prefetchedCall
	ModelProgressSchedule ModelProgressSchedule
	ContextManager        *contextmgr.Manager
	diagnostics           modelInvocationDiagnostics
	requestBudget         planningRequestBudget
	contextDecision       *contextmgr.Decision
}

type RunRequest struct {
	Prepared *PreparedChat
	Resolved *skills.ResolvedSkills
	// OnTurnUserContent receives the current question as it is sent, with the
	// reminders the turn adds after it merged in, so the transcript can store
	// what the next turn will replay (ADR 0043).
	OnTurnUserContent func(content interface{})
	// OmitPlanningTools leaves update_plan and submit_turn_state out of a
	// native run that has no business action to plan (ADR 0043).
	OmitPlanningTools bool
	// UnifiedSkillDispatch offers the tools of activatable skills through the
	// unified tool instead of as native functions (ADR 0044).
	UnifiedSkillDispatch           bool
	NativeToolSet                  *skills.NativeToolSet
	NativeSkillSession             *skills.NativeSkillSession
	RuntimeTools                   []RuntimeTool
	PlatformModel                  func(context.Context, string, int) (string, error)
	ExecutionContext               skills.ExecutionContext
	SuppressInitialNaturalProgress bool
	AdditionalSystemMessages       []adapter.Message
	// SystemPromptAdditions are guidance that stays the same from turn to
	// turn, such as the memory rules. The native loop appends them to the
	// system prompt so they are part of the cached prefix; other loops send
	// them as additional system messages.
	SystemPromptAdditions        []adapter.Message
	RuntimeStateSnapshot         RuntimeStateSnapshotFunc
	CurrentMetadata              func() map[string]interface{}
	OnTerminalStateGuardDecision func(TerminalStateGuardDecisionRecord)
	OnTerminalCompletion         func(TerminalCompletionResult)
	OnChunk                      func(string) error
	BeforeModelRound             func(context.Context, int) ([]adapter.Message, error)
	AfterModelRound              func(context.Context, int, bool) error
	PlanningOutputTokenLimit     int
	AuthorizeSkillStep           func(context.Context, string) (bool, error)
	PreferredRestoredSkillID     string
	ContinuationType             string
	TerminalOnly                 bool
}

// RuntimeTool is a first-party capability owned by the chat runtime rather
// than an installable Skill. Arguments and Result must be safe and
// content-free because they are persisted in traces. ModelResult, when set, is
// delivered to the current model turn only and is never retained in a trace.
type RuntimeTool struct {
	// PlatformOrigin and AuthorizePlatform are server-only continuation guards.
	PlatformOrigin    *toolgovernance.ExecutionOrigin `json:"-"`
	AuthorizePlatform func(context.Context) error     `json:"-"`
	Definition        adapter.Tool
	SkillID           string
	Handler           func(context.Context, adapter.ToolCall) RuntimeToolResult
	// BindPlatform is called once by the owning Runner before execution. Its
	// callbacks may only be called synchronously within this turn's Handler.
	BindPlatform func(RuntimePlatformServices)
	// PlatformInteractiveApproval is granted only by a server-owned interactive
	// execution factory; ordinary Skill calls retain their original policy.
	PlatformInteractiveApproval bool `json:"-"`
}

// RuntimePlatformServices exposes no routing credentials or arbitrary API
// access. Tool calls reenter the current loaded-skill and governance boundary.
type RuntimePlatformServices struct {
	AuthorizeSkill func(context.Context, string) (bool, error)
	Model          func(context.Context, string, int) (string, error)
	Tool           func(context.Context, string, string, json.RawMessage) (RuntimePlatformToolResult, error)
}

type RuntimePlatformToolResult struct {
	Data                json.RawMessage
	PendingGovernance   map[string]interface{}
	PendingConnector    map[string]interface{}
	PendingApproval     map[string]interface{}
	PendingQuestion     map[string]interface{}
	PendingClientAction map[string]interface{}
	PendingUserInput    map[string]interface{}
}

func (r RuntimePlatformToolResult) NeedsContinuation() bool {
	return r.PendingGovernance != nil || r.PendingConnector != nil || r.PendingApproval != nil || r.PendingQuestion != nil || r.PendingClientAction != nil || r.PendingUserInput != nil
}

type RuntimeToolResult struct {
	Status      string
	Arguments   map[string]interface{}
	Result      map[string]interface{}
	ModelResult map[string]interface{}
	// Artifacts are trusted, server-created file summaries for the existing
	// artifact event and conversation metadata pipeline. Never put file bytes
	// or unsigned filesystem paths here.
	Artifacts []map[string]interface{}
	// PendingApproval pauses the current run and is projected into the
	// durable workflow-continuation metadata. Runtime tools use this instead
	// of returning a normal success result so callers can render the approval
	// interaction in the composer.
	PendingApproval     map[string]interface{}
	PendingGovernance   map[string]interface{}
	PendingConnector    map[string]interface{}
	PendingQuestion     map[string]interface{}
	PendingClientAction map[string]interface{}
	PendingUserInput    map[string]interface{}
	// Events are runtime events the tool's work produced, such as a memory
	// change, recorded on the message's timeline like the chat runtime's own.
	Events      []RuntimeToolEvent
	Error       error
	Recoverable bool
}

// RuntimeToolEvent is one event a runtime tool reports. The conversation and
// message IDs are added when it is recorded.
type RuntimeToolEvent struct {
	Type    string
	Payload map[string]interface{}
}

type TerminalStateGuardDecisionRecord struct {
	Path     string
	Reason   string
	Blockers []string
}

type RuntimeStateSnapshotFunc func() map[string]interface{}

type TerminalCompletionResult struct {
	Status           string
	Source           string
	Reason           string
	Blockers         []string
	CompletionReason string
}

type SkillToolCallRef struct {
	ConnectorNative bool
	SkillID         string
	ToolName        string
	Arguments       map[string]interface{}
	Result          map[string]interface{}
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
	AgentRunID                 string
	ContextAPIRound            int
	ContextRequestType         string
	ContextDecision            string
	ContextModelWindowTokens   int
	ContextConfiguredWindowK   int
	ContextEffectiveWindow     int
	ContextWindowClamped       bool
	ContextSoftLimit           int
	ContextHardLimit           int
	ContextTargetTokens        int
	ContextFixedTokens         int
	ContextCompressibleTokens  int
	ContextToolTokensBefore    int
	ContextToolTokensAfter     int
	ContextToolProjectionCount int
	ContextLossyRecoveryRounds int
	ContextCompactedThrough    int
	ContextCompactionFailures  int
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
	eventState   presentationEventState
}

type presentationEventState struct {
	mu  sync.Mutex
	err error
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

func (r *Runner) emitEvent(prepared *PreparedChat, eventType string, payload map[string]interface{}) {
	if r == nil || r.OnEvent == nil {
		return
	}
	if prepared == nil {
		_ = r.OnEvent(Event{Type: eventType, Payload: payload})
		return
	}
	prepared.eventState.mu.Lock()
	defer prepared.eventState.mu.Unlock()
	if prepared.eventState.err != nil {
		return
	}
	if err := r.OnEvent(Event{Type: eventType, Payload: payload}); err != nil {
		prepared.eventState.err = err
	}
}

func (prepared *PreparedChat) resetPresentationEventError() {
	if prepared == nil {
		return
	}
	prepared.eventState.mu.Lock()
	prepared.eventState.err = nil
	prepared.eventState.mu.Unlock()
}

func (prepared *PreparedChat) presentationEventError() error {
	if prepared == nil {
		return nil
	}
	prepared.eventState.mu.Lock()
	defer prepared.eventState.mu.Unlock()
	return prepared.eventState.err
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
	if r.contextDecision != nil {
		decision := r.contextDecision
		trace.AgentRunID = decision.AgentRunID
		trace.ContextAPIRound = decision.APIRound
		trace.ContextRequestType = decision.RequestType
		trace.ContextDecision = decision.Action
		trace.ContextModelWindowTokens = decision.Budget.ModelContextWindow
		trace.ContextConfiguredWindowK = decision.Budget.ConfiguredAgentWindowK
		trace.ContextEffectiveWindow = decision.Budget.AgentContextWindow
		trace.ContextWindowClamped = decision.Budget.AgentContextWindowClamped
		trace.ContextSoftLimit = decision.Budget.SoftLimit
		trace.ContextHardLimit = decision.Budget.HardLimit
		trace.ContextTargetTokens = decision.Budget.TargetTokens
		trace.ContextFixedTokens = decision.FixedRequestTokens
		trace.ContextCompressibleTokens = decision.CompressibleTokens
		trace.ContextToolTokensBefore = decision.ToolResultOriginalTokens
		trace.ContextToolTokensAfter = decision.ToolResultProjectedTokens
		trace.ContextToolProjectionCount = decision.ToolProjectionCount
		trace.ContextLossyRecoveryRounds = decision.LossyRecoveryDroppedRounds
		trace.ContextCompactedThrough = decision.CompactedThroughRound
		trace.ContextCompactionFailures = decision.ConsecutiveCompactionFailures
	}
	r.OnModelInvocation(trace)
}

func (r *Runner) recordModelRequest(phase string, round int, request *adapter.ChatRequest) {
	if r == nil || r.OnModelRequest == nil || request == nil {
		return
	}
	var decision *contextmgr.Decision
	if r.contextDecision != nil {
		cloned := *r.contextDecision
		decision = &cloned
	}
	r.OnModelRequest(phase, round, cloneChatRequest(request), decision)
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
