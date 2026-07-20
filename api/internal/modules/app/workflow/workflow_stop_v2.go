package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const workflowLocalStopFlushGrace = 1500 * time.Millisecond

func (s *WorkflowService) waitForLocalWorkflowStopFinalization(ctx context.Context, run *WorkflowRunLog) *WorkflowRunLog {
	if s == nil || run == nil || s.workflowRunLogRepo == nil {
		return run
	}
	deadline := time.NewTimer(workflowLocalStopFlushGrace)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return run
		case <-deadline.C:
			return run
		case <-ticker.C:
			current, err := s.workflowRunLogRepo.GetByID(ctx, run.ID)
			if err != nil || current == nil {
				continue
			}
			run = current
			if run.ActiveExecutionID == nil || *run.ActiveExecutionID == "" {
				return run
			}
		}
	}
}

func (s *WorkflowService) finalizeStoppedWorkflowRunV2(ctx context.Context, run *WorkflowRunLog, stoppedAt time.Time) error {
	if run == nil {
		return nil
	}
	if run.ActiveExecutionID == nil || *run.ActiveExecutionID == "" {
		return s.finalizeUnownedStoppedWorkflowRunV2(ctx, run.ID, stoppedAt)
	}
	owner := workflowExecutionOwnerFromRun(run)
	if owner.ExecutionID == "" {
		return workflowpause.ErrExecutionOwnershipLost
	}

	finalAnswer := ""
	messageStatus := ""
	messageEnd := map[string]interface{}(nil)
	var message conversation.AgentMessage
	messageErr := database.GetDB().WithContext(ctx).
		Where("workflow_run_id = ? AND deleted_at IS NULL", run.ID).
		First(&message).Error
	if messageErr == nil {
		finalAnswer = message.Answer
		messageStatus = conversation.AgentMessageStatusStopped
		messageEnd = map[string]interface{}{
			"id":              message.ID.String(),
			"message_id":      message.ID.String(),
			"conversation_id": message.ConversationID.String(),
			"status":          conversation.AgentMessageStatusStopped,
			"created_at":      stoppedAt.Unix(),
		}
	} else if !errors.Is(messageErr, gorm.ErrRecordNotFound) {
		return messageErr
	}

	outputs := run.GetOutputsDict()
	workflowFinished := map[string]interface{}{
		"id":               run.ID,
		"workflow_id":      run.WorkflowID,
		"status":           "stopped",
		"outputs":          outputs,
		"error":            nil,
		"elapsed_time":     run.ElapsedTime,
		"total_tokens":     run.TotalTokens,
		"total_steps":      run.TotalSteps,
		"created_at":       run.CreatedAt.Unix(),
		"finished_at":      stoppedAt.Unix(),
		"exceptions_count": run.ExceptionsCount,
		"files":            []interface{}{},
	}
	finalizeCtx := withWorkflowExecutionOwner(ctx, owner)
	err := finalizeWorkflowRun(finalizeCtx, finalizeWorkflowRunParams{
		WorkflowRunID: run.ID, Status: "stopped", Outputs: outputs,
		ElapsedTime: run.ElapsedTime, TotalTokens: run.TotalTokens, TotalSteps: run.TotalSteps,
		ExceptionsCount: run.ExceptionsCount, FinalAnswer: finalAnswer, MessageStatus: messageStatus,
		MessageEnd: messageEnd, WorkflowFinished: workflowFinished,
	})
	if !errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
		return err
	}
	current, loadErr := s.workflowRunLogRepo.GetByID(ctx, run.ID)
	if loadErr == nil && current != nil && (current.ActiveExecutionID == nil || *current.ActiveExecutionID == "") {
		return nil
	}
	return err
}

func (s *WorkflowService) finalizeUnownedStoppedWorkflowRunV2(ctx context.Context, workflowRunID string, stoppedAt time.Time) error {
	return database.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run WorkflowRunLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND deleted_at IS NULL", workflowRunID).First(&run).Error; err != nil {
			return err
		}
		if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
			return nil
		}
		if run.ActiveExecutionID != nil && *run.ActiveExecutionID != "" {
			return workflowpause.ErrExecutionOwnershipLost
		}
		switch string(run.Status) {
		case "succeeded", "failed", "stopped":
			return nil
		}

		result := tx.Model(&WorkflowRunLog{}).
			Where("id = ? AND execution_generation = ? AND active_execution_id IS NULL", run.ID, run.ExecutionGeneration).
			Updates(map[string]interface{}{
				"status":                     "stopped",
				"finished_at":                stoppedAt,
				"execution_lease_expires_at": nil,
				"state_revision":             gorm.Expr("state_revision + 1"),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return workflowpause.ErrExecutionOwnershipLost
		}

		messageEnd := map[string]interface{}(nil)
		var message conversation.AgentMessage
		messageErr := tx.Where("workflow_run_id = ? AND deleted_at IS NULL", run.ID).First(&message).Error
		if messageErr == nil {
			if err := tx.Model(&conversation.AgentMessage{}).Where("id = ?", message.ID).
				Updates(map[string]interface{}{
					"status":              conversation.AgentMessageStatusStopped,
					"projection_revision": gorm.Expr("projection_revision + 1"),
					"updated_at":          stoppedAt,
				}).Error; err != nil {
				return err
			}
			messageEnd = map[string]interface{}{
				"id": message.ID.String(), "message_id": message.ID.String(),
				"conversation_id": message.ConversationID.String(),
				"status":          conversation.AgentMessageStatusStopped, "created_at": stoppedAt.Unix(),
			}
		} else if !errors.Is(messageErr, gorm.ErrRecordNotFound) {
			return messageErr
		}

		if err := tx.Model(&workflowpause.RunPause{}).
			Where("workflow_run_id = ? AND status <> ?", run.ID, workflowpause.RunPauseStatusClosed).
			Updates(map[string]interface{}{
				"status": workflowpause.RunPauseStatusClosed, "revision": gorm.Expr("revision + 1"),
				"resumed_at": stoppedAt, "lease_expires_at": nil,
			}).Error; err != nil {
			return err
		}
		if err := releaseWorkflowConversationTx(tx, &run); err != nil {
			return workflowpause.ErrExecutionOwnershipLost
		}

		pauseService := workflowpause.NewService(tx)
		eventDrafts := make([]workflowpause.EventDraft, 0, 2)
		if len(messageEnd) > 0 {
			eventDrafts = append(eventDrafts, workflowpause.EventDraft{
				EventType: workflowEventMessageEnd, Category: workflowpause.EventCategoryControl,
				IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:message_end", run.ID, run.ExecutionGeneration), OccurredAt: stoppedAt,
				EventData: messageEnd,
			})
		}
		eventDrafts = append(eventDrafts, workflowpause.EventDraft{
			EventType: workflowpause.EventWorkflowFinished, Category: workflowpause.EventCategoryControl,
			IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:stopped", run.ID, run.ExecutionGeneration), OccurredAt: stoppedAt,
			EventData: map[string]interface{}{
				"id": run.ID, "workflow_id": run.WorkflowID, "status": "stopped",
				"outputs": run.GetOutputsDict(), "error": nil, "elapsed_time": run.ElapsedTime,
				"total_tokens": run.TotalTokens, "total_steps": run.TotalSteps,
				"created_at": run.CreatedAt.Unix(), "finished_at": stoppedAt.Unix(),
				"exceptions_count": run.ExceptionsCount, "files": []interface{}{},
			},
		})
		_, err := pauseService.AppendEventBatchTx(ctx, tx, workflowpause.AppendEventBatchRequest{
			TenantID: run.TenantID, AppID: run.AgentID, WorkflowRunID: run.ID,
			FlushReason: "stop_barrier",
			Events:      eventDrafts,
		})
		return err
	})
}
