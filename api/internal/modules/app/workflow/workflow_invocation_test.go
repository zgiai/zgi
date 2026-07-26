package workflow

import (
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
)

func TestValidateWorkflowInvocationUsesServerWorkflowType(t *testing.T) {
	base := automationaction.WorkflowInvocationContext{
		InvocationID:         "invocation-1",
		ParentConversationID: "conversation-1",
		ParentMessageID:      "message-1",
		BindingID:            "binding-1",
	}

	t.Run("chat workflow delegates the conversation", func(t *testing.T) {
		invocation := base
		invocation.Mode = automationaction.WorkflowInvocationModeAgentDelegate
		if err := validateWorkflowInvocation(&Workflow{Type: dto.WorkflowTypeChat}, &invocation); err != nil {
			t.Fatalf("validate chat invocation: %v", err)
		}
		if invocation.ProtocolVersion != 1 {
			t.Fatalf("expected default protocol version 1, got %d", invocation.ProtocolVersion)
		}
	})

	t.Run("task workflow rejects delegate mode", func(t *testing.T) {
		invocation := base
		invocation.Mode = automationaction.WorkflowInvocationModeAgentDelegate
		err := validateWorkflowInvocation(&Workflow{Type: dto.WorkflowTypeWorkflow}, &invocation)
		if !errors.Is(err, errWorkflowInvocationConflict) {
			t.Fatalf("expected invocation conflict, got %v", err)
		}
	})
}

func TestApplyWorkflowInvocationToRunLogPersistsParentIdentity(t *testing.T) {
	invocation := &automationaction.WorkflowInvocationContext{
		InvocationID:         "invocation-1",
		ProtocolVersion:      2,
		Mode:                 automationaction.WorkflowInvocationModeAgentTaskTool,
		ParentConversationID: "conversation-1",
		ParentMessageID:      "message-1",
		BindingID:            "binding-1",
		ContextDigest:        "digest-1",
	}
	run := &WorkflowRunLog{}

	applyWorkflowInvocationToRunLog(run, invocation)

	if run.InvocationProtocolVersion != 2 || getStringValue(run.ParentInvocationID) != "invocation-1" {
		t.Fatalf("unexpected invocation identity: %+v", run)
	}
	if getStringValue(run.InvocationMode) != automationaction.WorkflowInvocationModeAgentTaskTool {
		t.Fatalf("unexpected invocation mode: %q", getStringValue(run.InvocationMode))
	}
	if getStringValue(run.ParentConversationID) != "conversation-1" ||
		getStringValue(run.ParentMessageID) != "message-1" ||
		getStringValue(run.InvocationBindingID) != "binding-1" ||
		getStringValue(run.InvocationContextDigest) != "digest-1" {
		t.Fatalf("parent workflow identity was not persisted: %+v", run)
	}
}
