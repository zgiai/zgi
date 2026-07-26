package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type finalizeWorkflowRunParams struct {
	WorkflowRunID    string
	Status           string
	Outputs          map[string]interface{}
	ErrorMessage     string
	ElapsedTime      float64
	TotalTokens      int64
	TotalSteps       int
	ExceptionsCount  int
	FinalAnswer      string
	MessageStatus    string
	ErrorEvent       map[string]interface{}
	MessageEnd       map[string]interface{}
	WorkflowFinished map[string]interface{}
}

func finalizeWorkflowRun(ctx context.Context, params finalizeWorkflowRunParams) error {
	if params.WorkflowRunID == "" {
		return fmt.Errorf("workflow run id is empty")
	}
	owner, hasOwner := workflowExecutionOwnerFromContext(ctx)
	outputsJSON, err := json.Marshal(params.Outputs)
	if err != nil {
		return fmt.Errorf("marshal workflow terminal outputs: %w", err)
	}
	now := time.Now()
	db := database.GetDB()
	finishTransactionMetric := beginWorkflowDBTransaction(ctx, "finalize")
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run WorkflowRunLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND deleted_at IS NULL", params.WorkflowRunID).First(&run).Error; err != nil {
			return fmt.Errorf("lock workflow run for finalization: %w", err)
		}
		if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 {
			return fmt.Errorf("workflow run does not use runtime protocol v2")
		}
		if !hasOwner || run.ActiveExecutionID == nil || *run.ActiveExecutionID != owner.ExecutionID || run.ExecutionGeneration != owner.Generation {
			return workflowpause.ErrExecutionOwnershipLost
		}
		eventDrafts := make([]workflowpause.EventDraft, 0, 4)
		storedTargets := make(map[int]map[string]interface{})
		updates := map[string]interface{}{
			"status":           params.Status,
			"outputs":          string(outputsJSON),
			"elapsed_time":     params.ElapsedTime,
			"total_tokens":     params.TotalTokens,
			"total_steps":      params.TotalSteps,
			"exceptions_count": params.ExceptionsCount,
			"finished_at":      now,
			"state_revision":   gorm.Expr("state_revision + 1"),
		}
		if params.ErrorMessage != "" {
			updates["error"] = params.ErrorMessage
		} else {
			updates["error"] = nil
		}
		result := tx.Model(&WorkflowRunLog{}).
			Where("id = ? AND execution_generation = ? AND active_execution_id = ?", params.WorkflowRunID, owner.Generation, owner.ExecutionID).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("finalize workflow run: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return workflowpause.ErrExecutionOwnershipLost
		}

		if params.MessageStatus != "" {
			var previousMessage conversation.AgentMessage
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("workflow_run_id = ? AND deleted_at IS NULL AND execution_generation <= ?", params.WorkflowRunID, owner.Generation).
				First(&previousMessage).Error; err != nil {
				return fmt.Errorf("lock workflow message projection for finalization: %w", err)
			}
			messageUpdates := map[string]interface{}{
				"answer":               params.FinalAnswer,
				"status":               params.MessageStatus,
				"execution_generation": owner.Generation,
				"projection_revision":  gorm.Expr("projection_revision + 1"),
				"updated_at":           now,
			}
			projectedMessage := conversation.AgentMessage{}
			messageResult := tx.Model(&projectedMessage).
				Clauses(clause.Returning{Columns: []clause.Column{{Name: "id"}, {Name: "conversation_id"}, {Name: "projection_revision"}}}).
				Where("workflow_run_id = ? AND deleted_at IS NULL AND execution_generation <= ?", params.WorkflowRunID, owner.Generation).
				Updates(messageUpdates)
			if messageResult.Error != nil {
				return fmt.Errorf("finalize workflow message projection: %w", messageResult.Error)
			}
			if messageResult.RowsAffected != 1 {
				return fmt.Errorf("workflow message projection missing during finalization")
			}
			delta, replace := workflowAnswerDelta(previousMessage.Answer, params.FinalAnswer)
			digest := sha256.Sum256([]byte(params.FinalAnswer))
			eventDrafts = append(eventDrafts, workflowpause.EventDraft{
				EventType: workflowEventMessage, Category: workflowpause.EventCategoryAnswerCheckpoint,
				ExecutionID:    owner.ExecutionID,
				IdempotencyKey: fmt.Sprintf("answer-checkpoint:%s:%d:%d", run.ID, owner.Generation, projectedMessage.ProjectionRevision),
				OccurredAt:     now,
				EventData: map[string]interface{}{
					"id": run.ID, "message_id": projectedMessage.ID.String(), "conversation_id": projectedMessage.ConversationID.String(),
					"answer_delta": delta, "answer_revision": projectedMessage.ProjectionRevision,
					"answer_length": len(params.FinalAnswer), "answer_digest": hex.EncodeToString(digest[:]),
					"replace": replace, "projection_generation": owner.Generation, "terminal": true,
				},
			})
		}

		pauseService := workflowpause.NewService(tx)
		if owner.PauseID != "" {
			claim := workflowpause.ExecutionClaim{
				WorkflowRunID:   params.WorkflowRunID,
				PauseID:         owner.PauseID,
				Generation:      owner.Generation,
				PauseGeneration: owner.PauseGeneration,
				ExecutionID:     owner.ExecutionID,
			}
			if err := pauseService.ClosePause(ctx, claim); err != nil {
				return err
			}
		}
		events := []struct {
			eventType string
			category  string
			data      map[string]interface{}
		}{
			{eventType: workflowpause.EventError, category: workflowpause.EventCategoryControl, data: params.ErrorEvent},
			{eventType: workflowEventMessageEnd, category: workflowpause.EventCategoryControl, data: params.MessageEnd},
			{eventType: workflowpause.EventWorkflowFinished, category: workflowpause.EventCategoryControl, data: params.WorkflowFinished},
		}
		for _, terminalEvent := range events {
			if len(terminalEvent.data) == 0 {
				continue
			}
			idempotencyKey := fmt.Sprintf("run:%s:generation:%d:%s", run.ID, owner.Generation, terminalEvent.eventType)
			if terminalEvent.eventType == workflowpause.EventWorkflowFinished {
				idempotencyKey = fmt.Sprintf("run:%s:generation:%d:%s", run.ID, owner.Generation, params.Status)
			}
			storedIndex := len(eventDrafts)
			eventDrafts = append(eventDrafts, workflowpause.EventDraft{
				EventType:      terminalEvent.eventType,
				EventData:      terminalEvent.data,
				Category:       terminalEvent.category,
				ExecutionID:    owner.ExecutionID,
				IdempotencyKey: idempotencyKey,
				OccurredAt:     now,
			})
			storedTargets[storedIndex] = terminalEvent.data
		}
		if len(eventDrafts) > 0 {
			storedEvents, err := pauseService.AppendEventBatchTx(ctx, tx, workflowpause.AppendEventBatchRequest{
				TenantID: run.TenantID, AppID: run.AgentID, WorkflowRunID: run.ID,
				FlushReason: "terminal_barrier",
				Fence: workflowpause.RuntimeFence{
					ExpectedExecutionID:         owner.ExecutionID,
					ExpectedExecutionGeneration: owner.Generation,
				},
				Events: eventDrafts,
			})
			if err != nil {
				return fmt.Errorf("append workflow terminal event batch: %w", err)
			}
			for index, target := range storedTargets {
				if index >= len(storedEvents) || storedEvents[index].Payload == nil {
					continue
				}
				target["__stored_sequence"] = storedEvents[index].Payload.Sequence
				target["__stored_event_id"] = storedEvents[index].Payload.EventID
				target["__stored_event_payload"] = storedEvents[index].Payload
			}
		}
		if err := releaseWorkflowConversationTx(tx, &run); err != nil {
			return workflowpause.ErrExecutionOwnershipLost
		}
		clearOwner := tx.Model(&WorkflowRunLog{}).
			Where("id = ? AND execution_generation = ? AND active_execution_id = ?", run.ID, owner.Generation, owner.ExecutionID).
			Updates(map[string]interface{}{
				"active_execution_id":        nil,
				"execution_lease_expires_at": nil,
			})
		if clearOwner.Error != nil {
			return fmt.Errorf("clear workflow execution owner after finalization: %w", clearOwner.Error)
		}
		if clearOwner.RowsAffected != 1 {
			return workflowpause.ErrExecutionOwnershipLost
		}
		return nil
	})
	finishTransactionMetric()
	if err == nil {
		recordWorkflowDBStatements(ctx, params.Status)
	}
	return err
}
