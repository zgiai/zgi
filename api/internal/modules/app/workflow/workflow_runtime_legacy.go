package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func (s *WorkflowService) pauseLegacyWorkflowRunLog(ctx context.Context, workflowRunLogID string, outputs map[string]interface{}, elapsedTime float64, totalTokens int64, totalSteps int) error {
	if s.workflowRunLogRepo == nil {
		return fmt.Errorf("workflow run log repository not initialized")
	}

	outputsJSON := "{}"
	if outputs != nil {
		if encoded, err := json.Marshal(outputs); err == nil {
			outputsJSON = string(encoded)
		}
	}
	if err := s.workflowRunLogRepo.UpdateStatus(ctx, workflowRunLogID, "paused", nil); err != nil {
		return fmt.Errorf("failed to pause workflow run log: %w", err)
	}
	if err := s.workflowRunLogRepo.UpdateOutputsAndTokens(ctx, workflowRunLogID, outputsJSON, totalTokens, elapsedTime); err != nil {
		return fmt.Errorf("failed to update paused workflow outputs: %w", err)
	}
	if run, err := s.workflowRunLogRepo.GetByID(ctx, workflowRunLogID); err == nil && run != nil {
		run.TotalSteps = totalSteps
		if updateErr := s.workflowRunLogRepo.Update(ctx, run); updateErr != nil {
			return fmt.Errorf("failed to update paused workflow details: %w", updateErr)
		}
	}
	return nil
}

func (s *WorkflowService) resumeLegacyWorkflowRunLog(ctx context.Context, workflowRunLogID string) error {
	if s.workflowRunLogRepo == nil {
		return fmt.Errorf("workflow run log repository not initialized")
	}
	if err := s.workflowRunLogRepo.UpdateStatus(ctx, workflowRunLogID, "running", nil); err != nil {
		return fmt.Errorf("failed to resume workflow run log: %w", err)
	}
	return nil
}

// resumeLegacyWorkflowContinuation contains the remaining V1 state mutations.
// New V2 executions must never call this adapter.
func (h *WorkflowHandler) resumeLegacyWorkflowContinuation(
	ctx context.Context,
	workflowService *WorkflowService,
	pauseService *workflowpause.Service,
	run *WorkflowRunLog,
	kind string,
) error {
	recordWorkflowV1Continuation(ctx, kind)
	if err := workflowService.resumeLegacyWorkflowRunLog(ctx, run.ID); err != nil {
		return err
	}
	if err := pauseService.MarkResumed(ctx, run.ID); err != nil {
		logger.WarnContext(ctx, "failed to mark legacy workflow pause resumed", "workflow_run_id", run.ID, err)
	}
	h.updateApprovalConversationMessageStatus(ctx, run.ID, conversation.AgentMessageStatusRunning, nil)
	return nil
}

func (s *WorkflowService) stopLegacyWorkflowRun(ctx context.Context, workflowRunID string, stoppedAt time.Time) error {
	if err := s.workflowRunLogRepo.UpdateStatus(ctx, workflowRunID, "stopped", &stoppedAt); err != nil {
		return fmt.Errorf("failed to stop workflow run: %w", err)
	}
	return nil
}

// appendLegacyWorkflowStopEvents preserves the V1 event contract while V1
// paused runs drain. V2 stop finalization is owned by StopWorkflowTask.
func appendLegacyWorkflowStopEvents(ctx context.Context, run *WorkflowRunLog, payload map[string]interface{}) {
	recordWorkflowV1Continuation(ctx, "stop")
	appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, "workflow_stopped", payload)
	appendWorkflowRunEvent(ctx, run.TenantID, run.AgentID, run.ID, workflowpause.EventWorkflowFinished, payload)
}
