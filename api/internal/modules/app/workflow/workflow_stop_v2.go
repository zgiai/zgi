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

func (s *WorkflowService) finalizeStoppedWorkflowRunV2(ctx context.Context, run *WorkflowRunLog, stoppedAt time.Time) error {
	if run == nil {
		return nil
	}
	err := database.GetDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return stopWorkflowRunV2Tx(ctx, tx, run.ID, stoppedAt)
	})
	if err == nil {
		publishWorkflowRuntimeEventSignal(run.ID)
	}
	return err
}

func (s *WorkflowService) finalizeUnownedStoppedWorkflowRunV2(ctx context.Context, workflowRunID string, stoppedAt time.Time) error {
	return s.finalizeStoppedWorkflowRunV2(ctx, &WorkflowRunLog{ID: workflowRunID}, stoppedAt)
}

func stopWorkflowRunV2Tx(ctx context.Context, tx *gorm.DB, workflowRunID string, stoppedAt time.Time) error {
	var run WorkflowRunLog
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND deleted_at IS NULL", workflowRunID).First(&run).Error; err != nil {
		return fmt.Errorf("lock workflow run for stop: %w", err)
	}
	if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
		return nil
	}
	switch string(run.Status) {
	case "succeeded", "failed", "expired", "partial-succeeded":
		return nil
	}

	nodeLogs, err := lockWorkflowStopRowsV2Tx(tx, run.ID)
	if err != nil {
		return err
	}
	if string(run.Status) == "stopped" {
		return cleanupStoppedWorkflowRunV2Tx(tx, &run, stoppedAt)
	}
	if len(nodeLogs) > 0 {
		if err := tx.Model(&WorkflowNodeRuntimeLog{}).
			Where("workflow_run_id = ? AND deleted_at IS NULL AND status IN ?", run.ID, []string{"running", "paused"}).
			Updates(map[string]interface{}{"status": "stopped", "finished_at": stoppedAt}).Error; err != nil {
			return fmt.Errorf("stop workflow node projections: %w", err)
		}
	}

	eventDrafts := make([]workflowpause.EventDraft, 0, len(nodeLogs)+2)
	executionID := ""
	if run.ActiveExecutionID != nil {
		executionID = *run.ActiveExecutionID
	}
	for _, nodeLog := range nodeLogs {
		nodeExecutionID := nodeLog.ID
		if nodeLog.NodeExecutionID != nil && *nodeLog.NodeExecutionID != "" {
			nodeExecutionID = *nodeLog.NodeExecutionID
		}
		createdAt := nodeLog.CreatedAt
		if createdAt.IsZero() {
			createdAt = stoppedAt
		}
		eventDrafts = append(eventDrafts, workflowpause.EventDraft{
			EventType: workflowpause.EventNodeFinished, Category: workflowpause.EventCategoryExecution,
			ExecutionID: executionID, IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:node:%s:stopped", run.ID, run.ExecutionGeneration, nodeLog.ID),
			OccurredAt: stoppedAt,
			EventData: map[string]interface{}{
				"id": nodeExecutionID, "node_execution_id": nodeExecutionID, "node_id": nodeLog.NodeID,
				"node_type": nodeLog.NodeType, "title": nodeLog.Title, "index": nodeLog.Index,
				"status": "stopped", "error": nil, "elapsed_time": nodeLog.ElapsedTime,
				"created_at": createdAt.Unix(), "finished_at": stoppedAt.Unix(), "files": []interface{}{},
			},
		})
	}

	var message conversation.AgentMessage
	messageErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workflow_run_id = ? AND deleted_at IS NULL", run.ID).First(&message).Error
	if messageErr == nil {
		if err := tx.Model(&conversation.AgentMessage{}).Where("id = ?", message.ID).
			Updates(map[string]interface{}{
				"status": conversation.AgentMessageStatusStopped, "projection_revision": gorm.Expr("projection_revision + 1"),
				"updated_at": stoppedAt,
			}).Error; err != nil {
			return fmt.Errorf("stop workflow message projection: %w", err)
		}
		eventDrafts = append(eventDrafts, workflowpause.EventDraft{
			EventType: workflowEventMessageEnd, Category: workflowpause.EventCategoryControl,
			ExecutionID: executionID, IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:message_end:stopped", run.ID, run.ExecutionGeneration),
			OccurredAt: stoppedAt,
			EventData: map[string]interface{}{
				"id": message.ID.String(), "message_id": message.ID.String(), "conversation_id": message.ConversationID.String(),
				"status": conversation.AgentMessageStatusStopped, "created_at": stoppedAt.Unix(),
			},
		})
	} else if !errors.Is(messageErr, gorm.ErrRecordNotFound) {
		return fmt.Errorf("lock workflow message projection for stop: %w", messageErr)
	}

	eventDrafts = append(eventDrafts, workflowpause.EventDraft{
		EventType: workflowpause.EventWorkflowFinished, Category: workflowpause.EventCategoryControl,
		ExecutionID: executionID, IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:stopped", run.ID, run.ExecutionGeneration),
		OccurredAt: stoppedAt,
		EventData: map[string]interface{}{
			"id": run.ID, "workflow_id": run.WorkflowID, "status": "stopped", "outputs": run.GetOutputsDict(),
			"error": nil, "elapsed_time": run.ElapsedTime, "total_tokens": run.TotalTokens, "total_steps": run.TotalSteps,
			"created_at": run.CreatedAt.Unix(), "finished_at": stoppedAt.Unix(), "exceptions_count": run.ExceptionsCount,
			"files": []interface{}{},
		},
	})
	fence := workflowpause.RuntimeFence{}
	if executionID != "" {
		fence.ExpectedExecutionID = executionID
		fence.ExpectedExecutionGeneration = run.ExecutionGeneration
	}
	if _, err := workflowpause.NewService(tx).AppendEventBatchTx(ctx, tx, workflowpause.AppendEventBatchRequest{
		TenantID: run.TenantID, AppID: run.AgentID, WorkflowRunID: run.ID,
		FlushReason: "stop_barrier", Fence: fence, Events: eventDrafts,
	}); err != nil {
		return fmt.Errorf("append workflow stop event batch: %w", err)
	}

	if err := cleanupStoppedWorkflowRunV2Tx(tx, &run, stoppedAt); err != nil {
		return err
	}
	result := tx.Model(&WorkflowRunLog{}).
		Where("id = ? AND execution_generation = ?", run.ID, run.ExecutionGeneration).
		Updates(map[string]interface{}{
			"status": "stopped", "finished_at": stoppedAt, "active_execution_id": nil,
			"execution_lease_expires_at": nil, "execution_generation": gorm.Expr("execution_generation + 1"),
			"state_revision": gorm.Expr("state_revision + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("stop workflow run: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return workflowpause.ErrExecutionOwnershipLost
	}
	return nil
}

// lockWorkflowStopRowsV2Tx uses the same order as resume and interaction
// transactions: run (locked by the caller), pauses, resume outbox, then nodes.
// Holding the run row serializes the state transition; the remaining locks make
// the stop barrier explicit and keep future callers from introducing an
// inverted lock order.
func lockWorkflowStopRowsV2Tx(tx *gorm.DB, workflowRunID string) ([]WorkflowNodeRuntimeLog, error) {
	var pauses []workflowpause.RunPause
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workflow_run_id = ? AND (resumed_at IS NULL OR status <> ?)", workflowRunID, workflowpause.RunPauseStatusClosed).
		Order("id ASC").Find(&pauses).Error; err != nil {
		return nil, fmt.Errorf("lock workflow pauses for stop: %w", err)
	}

	var outbox []workflowpause.RuntimeOutbox
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workflow_run_id = ? AND kind = ? AND status IN ?", workflowRunID, workflowpause.RuntimeOutboxKindResume, []string{workflowpause.RuntimeOutboxPending, workflowpause.RuntimeOutboxPublished}).
		Order("id ASC").Find(&outbox).Error; err != nil {
		return nil, fmt.Errorf("lock workflow resume outbox for stop: %w", err)
	}

	var nodeLogs []WorkflowNodeRuntimeLog
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workflow_run_id = ? AND deleted_at IS NULL AND status IN ?", workflowRunID, []string{"running", "paused"}).
		Order("index ASC, created_at ASC").Find(&nodeLogs).Error; err != nil {
		return nil, fmt.Errorf("lock running workflow nodes for stop: %w", err)
	}
	return nodeLogs, nil
}

func cleanupStoppedWorkflowRunV2Tx(tx *gorm.DB, run *WorkflowRunLog, stoppedAt time.Time) error {
	if err := tx.Model(&WorkflowNodeRuntimeLog{}).
		Where("workflow_run_id = ? AND deleted_at IS NULL AND status IN ?", run.ID, []string{"running", "paused"}).
		Updates(map[string]interface{}{"status": "stopped", "finished_at": stoppedAt}).Error; err != nil {
		return fmt.Errorf("stop remaining workflow node projections: %w", err)
	}
	if err := tx.Model(&workflowpause.RunPause{}).
		Where("workflow_run_id = ? AND (resumed_at IS NULL OR status <> ?)", run.ID, workflowpause.RunPauseStatusClosed).
		Updates(map[string]interface{}{
			"status": workflowpause.RunPauseStatusClosed, "revision": gorm.Expr("revision + 1"), "resumed_at": stoppedAt,
			"resume_execution_id": nil, "lease_expires_at": nil,
		}).Error; err != nil {
		return fmt.Errorf("close workflow pauses for stop: %w", err)
	}
	if err := tx.Model(&workflowpause.RuntimeOutbox{}).
		Where("workflow_run_id = ? AND kind = ? AND status IN ?", run.ID, workflowpause.RuntimeOutboxKindResume, []string{workflowpause.RuntimeOutboxPending, workflowpause.RuntimeOutboxPublished}).
		Updates(map[string]interface{}{"status": workflowpause.RuntimeOutboxObsolete, "updated_at": stoppedAt}).Error; err != nil {
		return fmt.Errorf("obsolete workflow resume outbox: %w", err)
	}
	if err := releaseWorkflowConversationTx(tx, run); err != nil {
		if string(run.Status) != "stopped" {
			return workflowpause.ErrExecutionOwnershipLost
		}
	}
	if string(run.Status) != "stopped" || run.ActiveExecutionID == nil || *run.ActiveExecutionID == "" {
		return nil
	}
	result := tx.Model(&WorkflowRunLog{}).
		Where("id = ? AND execution_generation = ?", run.ID, run.ExecutionGeneration).
		Updates(map[string]interface{}{
			"active_execution_id": nil, "execution_lease_expires_at": nil,
			"execution_generation": gorm.Expr("execution_generation + 1"), "state_revision": gorm.Expr("state_revision + 1"),
		})
	if result.Error != nil {
		return fmt.Errorf("clear stopped workflow execution owner: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return workflowpause.ErrExecutionOwnershipLost
	}
	return nil
}
