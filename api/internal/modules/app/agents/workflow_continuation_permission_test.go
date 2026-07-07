package agents

import (
	"errors"
	"testing"

	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
)

func TestAgentWorkflowContinuationApprovalFormRequiresSameRun(t *testing.T) {
	err := ensureAgentWorkflowContinuationApprovalForm(&runtimeservice.WorkflowApprovalContinuation{
		WorkflowRunID: "owned-run",
	}, &approvalruntime.Form{
		WorkflowRunID: "other-run",
	})

	if !errors.Is(err, runtimeservice.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestAgentWorkflowContinuationApprovalFormAllowsSameRun(t *testing.T) {
	err := ensureAgentWorkflowContinuationApprovalForm(&runtimeservice.WorkflowApprovalContinuation{
		WorkflowRunID: " owned-run ",
	}, &approvalruntime.Form{
		WorkflowRunID: "owned-run",
	})

	if err != nil {
		t.Fatalf("ensureAgentWorkflowContinuationApprovalForm returned error: %v", err)
	}
}

func TestAgentWorkflowContinuationApprovalFormRejectsMissingForm(t *testing.T) {
	err := ensureAgentWorkflowContinuationApprovalForm(&runtimeservice.WorkflowApprovalContinuation{
		WorkflowRunID: "owned-run",
	}, nil)

	if !errors.Is(err, runtimeservice.ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}
