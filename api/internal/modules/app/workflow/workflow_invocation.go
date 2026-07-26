package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"gorm.io/gorm"
)

var (
	errWorkflowInvocationConflict = errors.New("workflow_invocation_conflict")
	errWorkflowInvocationInvalid  = errors.New("workflow_invocation_invalid")
)

type workflowInvocationRunReader interface {
	GetByParentInvocationID(ctx context.Context, invocationID string) (*WorkflowRunLog, error)
}

func validateWorkflowInvocation(target *Workflow, invocation *automationaction.WorkflowInvocationContext) error {
	if invocation == nil {
		return nil
	}
	invocation.InvocationID = strings.TrimSpace(invocation.InvocationID)
	invocation.Mode = strings.TrimSpace(invocation.Mode)
	invocation.ParentConversationID = strings.TrimSpace(invocation.ParentConversationID)
	invocation.ParentMessageID = strings.TrimSpace(invocation.ParentMessageID)
	invocation.BindingID = strings.TrimSpace(invocation.BindingID)
	invocation.ContextDigest = strings.TrimSpace(invocation.ContextDigest)
	if invocation.ProtocolVersion <= 0 {
		invocation.ProtocolVersion = 1
	}
	if invocation.InvocationID == "" || invocation.ParentConversationID == "" || invocation.ParentMessageID == "" || invocation.BindingID == "" {
		return fmt.Errorf("%w: invocation_id, parent conversation/message and binding_id are required", errWorkflowInvocationInvalid)
	}
	expectedMode := automationaction.WorkflowInvocationModeAgentTaskTool
	if target != nil && target.Type == dto.WorkflowTypeChat {
		expectedMode = automationaction.WorkflowInvocationModeAgentDelegate
	}
	if invocation.Mode != expectedMode {
		return fmt.Errorf("%w: workflow requires mode %s, got %s", errWorkflowInvocationConflict, expectedMode, invocation.Mode)
	}
	return nil
}

func applyWorkflowInvocationToRunLog(run *WorkflowRunLog, invocation *automationaction.WorkflowInvocationContext) {
	if run == nil || invocation == nil {
		return
	}
	run.InvocationProtocolVersion = invocation.ProtocolVersion
	run.InvocationMode = optionalStringPointer(invocation.Mode)
	run.ParentConversationID = optionalStringPointer(invocation.ParentConversationID)
	run.ParentMessageID = optionalStringPointer(invocation.ParentMessageID)
	run.ParentInvocationID = optionalStringPointer(invocation.InvocationID)
	run.InvocationBindingID = optionalStringPointer(invocation.BindingID)
	run.InvocationContextDigest = optionalStringPointer(invocation.ContextDigest)
}

func (s *WorkflowService) findWorkflowInvocationRun(ctx context.Context, invocation *automationaction.WorkflowInvocationContext) (*WorkflowRunLog, error) {
	if s == nil || invocation == nil || strings.TrimSpace(invocation.InvocationID) == "" || s.workflowRunLogRepo == nil {
		return nil, nil
	}
	reader, ok := s.workflowRunLogRepo.(workflowInvocationRunReader)
	if !ok {
		return nil, nil
	}
	run, err := reader.GetByParentInvocationID(ctx, strings.TrimSpace(invocation.InvocationID))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find workflow invocation: %w", err)
	}
	return run, nil
}

func validateWorkflowInvocationRun(run *WorkflowRunLog, target *Workflow, invocation *automationaction.WorkflowInvocationContext) error {
	if run == nil || invocation == nil {
		return nil
	}
	if target == nil || run.WorkflowID != target.ID || run.AgentID != target.AgentID ||
		strings.TrimSpace(getStringValue(run.InvocationMode)) != invocation.Mode ||
		strings.TrimSpace(getStringValue(run.ParentConversationID)) != invocation.ParentConversationID ||
		strings.TrimSpace(getStringValue(run.ParentMessageID)) != invocation.ParentMessageID ||
		strings.TrimSpace(getStringValue(run.InvocationBindingID)) != invocation.BindingID ||
		strings.TrimSpace(getStringValue(run.InvocationContextDigest)) != invocation.ContextDigest {
		return fmt.Errorf("%w: invocation identity no longer matches its workflow run", errWorkflowInvocationConflict)
	}
	return nil
}

func workflowInvocationResult(run *WorkflowRunLog) *automationaction.WorkflowRunResult {
	if run == nil {
		return nil
	}
	return &automationaction.WorkflowRunResult{
		WorkflowRunID:  run.ID,
		WorkflowID:     run.WorkflowID,
		AgentID:        run.AgentID,
		Version:        run.Version,
		Status:         string(run.Status),
		Outputs:        run.GetOutputsDict(),
		ElapsedTime:    workflowRunElapsedMilliseconds(*run),
		InvocationID:   strings.TrimSpace(getStringValue(run.ParentInvocationID)),
		InvocationMode: strings.TrimSpace(getStringValue(run.InvocationMode)),
	}
}
