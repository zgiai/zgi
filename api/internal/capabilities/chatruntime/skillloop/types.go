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
	EventMessage              = "message"
	EventMessageRetract       = "message_retract"
	EventAgentProgress        = "agent_progress"
	EventIntermediateAnswer   = "agent_intermediate_answer"
	EventUserInputRequested   = "user_input_requested"
	EventSkillCallStart       = "skill_call_start"
	EventSkillCallEnd         = "skill_call_end"
	EventSkillCallError       = "skill_call_error"
	EventSkillLoadStart       = "skill_load_start"
	EventSkillLoadEnd         = "skill_load_end"
	EventSkillReferenceRead   = "skill_reference_read"
	EventSkillArtifactCreated = "skill_artifact_created"
	EventWorkflowStarted      = "workflow_started"
	EventWorkflowNodeStarted  = "node_started"
	EventWorkflowNodeFinished = "node_finished"
	EventWorkflowPaused       = "workflow_paused"
	EventWorkflowApproval     = "approval_requested"
	EventWorkflowFinished     = "workflow_finished"
	EventWorkflowFailed       = "workflow_failed"
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

type Event struct {
	Type    string
	Payload map[string]interface{}
}

type Runner struct {
	LLMClient     llmclient.LLMClient
	SkillRuntime  *skills.Runtime
	AppContext    *llmclient.AppContext
	OnEvent       func(Event) error
	OnTrace       func([]skills.SkillTrace, skills.SkillTrace)
	OnArtifact    func(map[string]interface{})
	FallbackDelay time.Duration
}

type RunRequest struct {
	Prepared         *PreparedChat
	Resolved         *skills.ResolvedSkills
	ExecutionContext skills.ExecutionContext
	OnChunk          func(string) error
}

type PreparedChat struct {
	Conversation *Conversation
	Message      *Message
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
