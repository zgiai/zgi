package pause

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPauseNotFound = errors.New("workflow pause not found")
var ErrExecutionOwnershipLost = errors.New("workflow execution ownership lost")
var ErrResumeAlreadyRunning = errors.New("workflow resume already running")
var ErrPauseNotResumeReady = errors.New("workflow pause is not ready to resume")

var runEventLocks sync.Map

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

func (s *Service) Save(ctx context.Context, params SaveParams) (*RunPause, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if params.State.Version == "" {
		params.State.Version = StateVersion
	}
	stateJSON, err := json.Marshal(params.State)
	if err != nil {
		return nil, fmt.Errorf("marshal workflow pause state: %w", err)
	}

	var pause RunPause
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run struct {
			RuntimeProtocolVersion int     `gorm:"column:runtime_protocol_version"`
			ExecutionGeneration    int64   `gorm:"column:execution_generation"`
			ActiveExecutionID      *string `gorm:"column:active_execution_id"`
			ConversationID         *string `gorm:"column:conversation_id"`
		}
		if err := tx.Table("workflow_run_logs").
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("runtime_protocol_version, execution_generation, active_execution_id, conversation_id").
			Where("id = ? AND deleted_at IS NULL", params.WorkflowRunID).
			Take(&run).Error; err != nil {
			return fmt.Errorf("lock workflow run for pause: %w", err)
		}
		if run.RuntimeProtocolVersion >= 2 {
			if params.ExecutionID == "" || params.Generation <= 0 {
				return ErrExecutionOwnershipLost
			}
			if stringValue(run.ActiveExecutionID) != params.ExecutionID {
				return ErrExecutionOwnershipLost
			}
			if run.ExecutionGeneration != params.Generation {
				return ErrExecutionOwnershipLost
			}
		}

		var existing RunPause
		loadErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workflow_run_id = ? AND resumed_at IS NULL AND status <> ?", params.WorkflowRunID, RunPauseStatusClosed).
			First(&existing).Error
		if loadErr == nil && existing.Status == RunPauseStatusResuming {
			if params.ExecutionID == "" || stringValue(existing.ResumeExecutionID) != params.ExecutionID {
				return ErrExecutionOwnershipLost
			}
			now := time.Now()
			result := tx.Model(&RunPause{}).
				Where("id = ? AND revision = ? AND resume_execution_id = ?", existing.ID, existing.Revision, params.ExecutionID).
				Updates(map[string]interface{}{
					"status":           RunPauseStatusClosed,
					"revision":         gorm.Expr("revision + 1"),
					"resumed_at":       now,
					"lease_expires_at": nil,
				})
			if result.Error != nil {
				return fmt.Errorf("close prior workflow pause: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrExecutionOwnershipLost
			}
			loadErr = gorm.ErrRecordNotFound
		}
		if loadErr == nil {
			pause = existing
			now := time.Now()
			conversationID := nullableString(params.ConversationID)
			if err := tx.Model(&pause).Updates(map[string]interface{}{
				"tenant_id":           params.TenantID,
				"app_id":              params.AppID,
				"node_id":             params.NodeID,
				"reason":              params.Reason,
				"conversation_id":     conversationID,
				"state_json":          string(stateJSON),
				"created_at":          now,
				"resumed_at":          nil,
				"status":              RunPauseStatusPaused,
				"revision":            gorm.Expr("revision + 1"),
				"resume_execution_id": nil,
				"lease_expires_at":    nil,
			}).Error; err != nil {
				return fmt.Errorf("update workflow pause: %w", err)
			}
			pause.TenantID = params.TenantID
			pause.AppID = params.AppID
			pause.NodeID = params.NodeID
			pause.Reason = params.Reason
			pause.ConversationID = conversationID
			pause.StateJSON = string(stateJSON)
			pause.CreatedAt = now
			pause.ResumedAt = nil
			pause.Status = RunPauseStatusPaused
			pause.Revision++
			pause.ResumeExecutionID = nil
			pause.LeaseExpiresAt = nil
			if err := tx.Where("pause_id = ?", pause.ID).Delete(&RunPauseReason{}).Error; err != nil {
				return fmt.Errorf("delete workflow pause reasons: %w", err)
			}
		} else if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			var maximumGeneration int64
			if err := tx.Model(&RunPause{}).
				Where("workflow_run_id = ?", params.WorkflowRunID).
				Select("COALESCE(MAX(generation), 0)").
				Scan(&maximumGeneration).Error; err != nil {
				return fmt.Errorf("load workflow pause generation: %w", err)
			}
			pause = RunPause{
				ID:             uuid.NewString(),
				TenantID:       params.TenantID,
				AppID:          params.AppID,
				WorkflowRunID:  params.WorkflowRunID,
				NodeID:         params.NodeID,
				Reason:         params.Reason,
				ConversationID: nullableString(params.ConversationID),
				StateJSON:      string(stateJSON),
				CreatedAt:      time.Now(),
				Generation:     maximumGeneration + 1,
				Status:         RunPauseStatusPaused,
			}
			if err := tx.Create(&pause).Error; err != nil {
				return fmt.Errorf("create workflow pause: %w", err)
			}
		} else {
			return fmt.Errorf("load workflow pause: %w", loadErr)
		}

		reasons := make([]RunPauseReason, 0, len(params.Reasons))
		for _, reason := range params.Reasons {
			reasons = append(reasons, RunPauseReason{
				ID:        uuid.NewString(),
				PauseID:   pause.ID,
				Type:      reason.Type,
				NodeID:    reason.NodeID,
				FormID:    reason.FormID,
				Status:    RunPauseReasonStatusPending,
				CreatedAt: time.Now(),
			})
		}
		if len(reasons) > 0 {
			if err := tx.Create(&reasons).Error; err != nil {
				return fmt.Errorf("create workflow pause reasons: %w", err)
			}
		}
		if run.RuntimeProtocolVersion >= 2 {
			runUpdates := map[string]interface{}{
				"status":         "paused",
				"outputs":        params.RunOutputsJSON,
				"elapsed_time":   params.RunElapsedTime,
				"total_tokens":   params.RunTotalTokens,
				"total_steps":    params.RunTotalSteps,
				"state_revision": gorm.Expr("state_revision + 1"),
			}
			if params.RunOutputsJSON == "" {
				delete(runUpdates, "outputs")
			}
			result := tx.Table("workflow_run_logs").
				Where("id = ? AND execution_generation = ? AND active_execution_id = ?", params.WorkflowRunID, run.ExecutionGeneration, stringValue(run.ActiveExecutionID)).
				Updates(runUpdates)
			if result.Error != nil {
				return fmt.Errorf("pause workflow run: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrExecutionOwnershipLost
			}
			for _, nodeUpdate := range params.NodeUpdates {
				if nodeUpdate.NodeLogID == "" {
					continue
				}
				outputsJSON, err := json.Marshal(nodeUpdate.Outputs)
				if err != nil {
					return fmt.Errorf("marshal paused node outputs: %w", err)
				}
				processDataJSON, err := json.Marshal(nodeUpdate.ProcessData)
				if err != nil {
					return fmt.Errorf("marshal paused node process data: %w", err)
				}
				executionMetadataJSON, err := json.Marshal(nodeUpdate.ExecutionMetadata)
				if err != nil {
					return fmt.Errorf("marshal paused node execution metadata: %w", err)
				}
				nodeResult := tx.Table("workflow_node_runtime_logs").
					Where("id = ? AND workflow_run_id = ? AND deleted_at IS NULL", nodeUpdate.NodeLogID, params.WorkflowRunID).
					Updates(map[string]interface{}{
						"status":             "paused",
						"outputs":            string(outputsJSON),
						"process_data":       string(processDataJSON),
						"execution_metadata": string(executionMetadataJSON),
						"elapsed_time":       nodeUpdate.ElapsedTime,
					})
				if nodeResult.Error != nil {
					return fmt.Errorf("pause workflow node projection: %w", nodeResult.Error)
				}
				if nodeResult.RowsAffected != 1 {
					return fmt.Errorf("pause workflow node projection %s: %w", nodeUpdate.NodeLogID, ErrExecutionOwnershipLost)
				}
			}
			if params.MessageStatus != "" {
				messageUpdates := map[string]interface{}{
					"status":               params.MessageStatus,
					"execution_generation": params.Generation,
					"projection_revision":  gorm.Expr("projection_revision + 1"),
					"updated_at":           time.Now(),
				}
				if params.UpdateMessageAnswer {
					messageUpdates["answer"] = params.MessageAnswer
				}
				messageResult := tx.Table("agents_messages").
					Where("workflow_run_id = ? AND deleted_at IS NULL AND execution_generation <= ?", params.WorkflowRunID, params.Generation).
					Updates(messageUpdates)
				if messageResult.Error != nil {
					return fmt.Errorf("pause workflow message projection: %w", messageResult.Error)
				}
				if messageResult.RowsAffected == 0 {
					if params.MessageProjection == nil {
						return fmt.Errorf("pause workflow message projection missing")
					}
					projection := *params.MessageProjection
					projection.Status = params.MessageStatus
					projection.ExecutionGeneration = params.Generation
					projection.ProjectionRevision = 1
					if params.UpdateMessageAnswer {
						projection.Answer = params.MessageAnswer
					}
					if err := tx.Create(&projection).Error; err != nil {
						return fmt.Errorf("create paused workflow message projection: %w", err)
					}
					if err := tx.Table("agents_conversations").Where("id = ?", projection.ConversationID).
						UpdateColumn("dialogue_count", gorm.Expr("dialogue_count + 1")).Error; err != nil {
						return fmt.Errorf("increment paused workflow conversation dialogue count: %w", err)
					}
				} else if messageResult.RowsAffected != 1 {
					return fmt.Errorf("pause workflow message projection conflict")
				}
			}
		}
		if len(params.Events) > 0 {
			pauseGeneration := pause.Generation
			pauseRevision := pause.Revision
			drafts := make([]EventDraft, 0, len(params.Events))
			for index := range params.Events {
				event := &params.Events[index]
				event.ExecutionID = stringValue(run.ActiveExecutionID)
				event.PauseID = pause.ID
				event.PauseGeneration = &pauseGeneration
				if event.IdempotencyKey != "" {
					event.IdempotencyKey = fmt.Sprintf("%s:%d", event.IdempotencyKey, pause.Generation)
				}
				drafts = append(drafts, eventDraftFromAppendParams(*event))
			}
			request := AppendEventBatchRequest{
				TenantID: params.TenantID, AppID: params.AppID, WorkflowRunID: params.WorkflowRunID,
				FlushReason: "pause_barrier",
				Fence: RuntimeFence{
					ExpectedExecutionID: params.ExecutionID, ExpectedExecutionGeneration: params.Generation,
					ExpectedPauseID: pause.ID, ExpectedPauseGeneration: &pauseGeneration, ExpectedPauseRevision: &pauseRevision,
				},
				Events: drafts,
			}
			storedEvents, err := NewService(tx).AppendEventBatchTx(ctx, tx, request)
			if err != nil {
				return fmt.Errorf("append workflow pause event batch: %w", err)
			}
			for index, stored := range storedEvents {
				if params.Events[index].EventData == nil || stored.Payload == nil {
					continue
				}
				params.Events[index].EventData["__stored_sequence"] = stored.Payload.Sequence
				params.Events[index].EventData["__stored_event_id"] = stored.Payload.EventID
				params.Events[index].EventData["__stored_event_payload"] = stored.Payload
			}
		}
		if run.RuntimeProtocolVersion >= 2 && run.ConversationID != nil && strings.TrimSpace(*run.ConversationID) != "" {
			runtimeStatus := "pending_approval"
			if strings.Contains(strings.ToLower(params.MessageStatus), "question") {
				runtimeStatus = "pending_question"
			}
			conversationResult := tx.Table("agents_conversations").
				Where("id = ? AND active_workflow_run_id = ?", strings.TrimSpace(*run.ConversationID), params.WorkflowRunID).
				Updates(map[string]interface{}{
					"runtime_status":   runtimeStatus,
					"runtime_revision": gorm.Expr("runtime_revision + 1"),
				})
			if conversationResult.Error != nil {
				return fmt.Errorf("pause workflow conversation: %w", conversationResult.Error)
			}
			if conversationResult.RowsAffected != 1 {
				return ErrExecutionOwnershipLost
			}
		}
		if run.RuntimeProtocolVersion >= 2 {
			clearOwner := tx.Table("workflow_run_logs").
				Where("id = ? AND execution_generation = ? AND active_execution_id = ?", params.WorkflowRunID, params.Generation, params.ExecutionID).
				Updates(map[string]interface{}{
					"active_execution_id":        nil,
					"execution_lease_expires_at": nil,
				})
			if clearOwner.Error != nil {
				return fmt.Errorf("clear paused workflow execution owner: %w", clearOwner.Error)
			}
			if clearOwner.RowsAffected != 1 {
				return ErrExecutionOwnershipLost
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &pause, nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (s *Service) GetActiveByConversationID(ctx context.Context, tenantID, appID, conversationID, reason string) (*RunPause, []RunPauseReason, *State, error) {
	if s == nil || s.db == nil {
		return nil, nil, nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if conversationID == "" {
		return nil, nil, nil, ErrPauseNotFound
	}
	var pause RunPause
	query := s.db.WithContext(ctx).
		Where("conversation_id = ? AND resumed_at IS NULL", conversationID)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if appID != "" {
		query = query.Where("app_id = ?", appID)
	}
	if reason != "" {
		query = query.Where("reason = ?", reason)
	}
	if err := query.Order("created_at DESC").First(&pause).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, ErrPauseNotFound
		}
		return nil, nil, nil, fmt.Errorf("load workflow pause: %w", err)
	}

	var state State
	if err := json.Unmarshal([]byte(pause.StateJSON), &state); err != nil {
		return nil, nil, nil, fmt.Errorf("decode workflow pause state: %w", err)
	}
	var reasons []RunPauseReason
	if err := s.db.WithContext(ctx).
		Where("pause_id = ?", pause.ID).
		Order("created_at ASC").
		Find(&reasons).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("load workflow pause reasons: %w", err)
	}
	return &pause, reasons, &state, nil
}

func (s *Service) GetActiveByWorkflowRunID(ctx context.Context, workflowRunID string) (*RunPause, []RunPauseReason, *State, error) {
	if s == nil || s.db == nil {
		return nil, nil, nil, fmt.Errorf("workflow pause service is not initialized")
	}
	var pause RunPause
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND resumed_at IS NULL", workflowRunID).
		First(&pause).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil, ErrPauseNotFound
		}
		return nil, nil, nil, fmt.Errorf("load workflow pause: %w", err)
	}

	var state State
	if err := json.Unmarshal([]byte(pause.StateJSON), &state); err != nil {
		return nil, nil, nil, fmt.Errorf("decode workflow pause state: %w", err)
	}
	var reasons []RunPauseReason
	if err := s.db.WithContext(ctx).
		Where("pause_id = ?", pause.ID).
		Order("created_at ASC").
		Find(&reasons).Error; err != nil {
		return nil, nil, nil, fmt.Errorf("load workflow pause reasons: %w", err)
	}
	return &pause, reasons, &state, nil
}

func (s *Service) MarkResumed(ctx context.Context, workflowRunID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("workflow pause service is not initialized")
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&RunPause{}).
		Where("workflow_run_id = ? AND resumed_at IS NULL", workflowRunID).
		Updates(map[string]interface{}{
			"resumed_at": now,
			"status":     RunPauseStatusClosed,
			"revision":   gorm.Expr("revision + 1"),
		}).Error; err != nil {
		return fmt.Errorf("mark workflow pause resumed: %w", err)
	}
	return nil
}

func (s *Service) PrepareResume(ctx context.Context, workflowRunID, pauseID, triggerID string) (*RuntimeOutbox, error) {
	return s.prepareResume(ctx, workflowRunID, pauseID, triggerID, "", nil)
}

func (s *Service) prepareResume(ctx context.Context, workflowRunID, pauseID, triggerID, interactionType string, resumeInputs map[string]interface{}) (*RuntimeOutbox, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	var outbox *RuntimeOutbox
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		outbox, err = NewService(tx).prepareResumeTx(ctx, tx, workflowRunID, pauseID, triggerID, interactionType, resumeInputs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return outbox, nil
}

func (s *Service) prepareResumeTx(ctx context.Context, tx *gorm.DB, workflowRunID, pauseID, triggerID, interactionType string, resumeInputs map[string]interface{}) (*RuntimeOutbox, error) {
	var pause RunPause
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workflow_run_id = ? AND resumed_at IS NULL", workflowRunID)
	if pauseID != "" {
		query = query.Where("id = ?", pauseID)
	}
	if err := query.First(&pause).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPauseNotFound
		}
		return nil, fmt.Errorf("lock workflow pause for resume readiness: %w", err)
	}
	if pause.Status == RunPauseStatusClosed {
		return nil, ErrPauseNotFound
	}
	if pause.Status == RunPauseStatusPaused {
		result := tx.Model(&RunPause{}).Where("id = ? AND status = ? AND revision = ?", pause.ID, RunPauseStatusPaused, pause.Revision).
			Updates(map[string]interface{}{"status": RunPauseStatusResumeReady, "revision": gorm.Expr("revision + 1")})
		if result.Error != nil {
			return nil, fmt.Errorf("mark workflow pause resume ready: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return nil, ErrPauseNotResumeReady
		}
		pause.Status = RunPauseStatusResumeReady
		pause.Revision++
	}
	if pause.Status != RunPauseStatusResumeReady && pause.Status != RunPauseStatusResuming {
		return nil, ErrPauseNotResumeReady
	}
	payload, err := json.Marshal(RuntimeOutboxPayload{
		WorkflowRunID: workflowRunID, PauseID: pause.ID, Generation: pause.Generation,
		TriggerID: triggerID, InteractionType: interactionType, ResumeInputs: resumeInputs,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal workflow resume outbox payload: %w", err)
	}
	now := time.Now()
	idempotencyKey := fmt.Sprintf("workflow-resume:%s:%d", pause.ID, pause.Generation)
	outbox := RuntimeOutbox{
		ID: uuid.NewString(), TenantID: pause.TenantID, WorkflowRunID: workflowRunID, PauseID: &pause.ID,
		Kind: RuntimeOutboxKindResume, IdempotencyKey: idempotencyKey, PayloadJSON: string(payload),
		Status: RuntimeOutboxPending, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	result := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&outbox)
	if result.Error != nil {
		return nil, fmt.Errorf("create workflow resume outbox: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		if err := tx.Where("idempotency_key = ?", idempotencyKey).First(&outbox).Error; err != nil {
			return nil, fmt.Errorf("load workflow resume outbox: %w", err)
		}
	}
	return &outbox, nil
}

// LoadResumePayload returns the durable inputs prepared for one exact pause
// generation. The outbox is the authoritative handoff between interaction
// submission and synchronous or queued workflow resumption.
func (s *Service) LoadResumePayload(ctx context.Context, workflowRunID, pauseID string, generation int64) (*RuntimeOutboxPayload, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	workflowRunID = strings.TrimSpace(workflowRunID)
	pauseID = strings.TrimSpace(pauseID)
	if workflowRunID == "" || pauseID == "" {
		return nil, ErrPauseNotResumeReady
	}

	idempotencyKey := fmt.Sprintf("workflow-resume:%s:%d", pauseID, generation)
	var outbox RuntimeOutbox
	if err := s.db.WithContext(ctx).
		Where(
			"workflow_run_id = ? AND pause_id = ? AND kind = ? AND idempotency_key = ? AND status IN ?",
			workflowRunID,
			pauseID,
			RuntimeOutboxKindResume,
			idempotencyKey,
			[]string{RuntimeOutboxPending, RuntimeOutboxPublished},
		).
		First(&outbox).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPauseNotResumeReady
		}
		return nil, fmt.Errorf("load workflow resume outbox: %w", err)
	}

	var payload RuntimeOutboxPayload
	if err := json.Unmarshal([]byte(outbox.PayloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("decode workflow resume outbox payload: %w", err)
	}
	if payload.WorkflowRunID != workflowRunID || payload.PauseID != pauseID || payload.Generation != generation {
		return nil, ErrPauseNotResumeReady
	}
	return &payload, nil
}

// SubmitInteraction durably records a question/client interaction and makes the
// paused run resumable in the same transaction. Callers may publish Event only
// after this method returns successfully.
func (s *Service) SubmitInteraction(ctx context.Context, workflowRunID, pauseID, triggerID, eventType string, eventData map[string]interface{}, idempotencyKey string) (*InteractionSubmission, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	result := &InteractionSubmission{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		service := NewService(tx)
		pauseRecord, _, _, err := service.GetActiveByWorkflowRunID(ctx, workflowRunID)
		if err != nil {
			if errors.Is(err, ErrPauseNotFound) && idempotencyKey != "" {
				replay, replayErr := service.loadInteractionSubmissionReplay(ctx, workflowRunID, idempotencyKey)
				if replayErr == nil && replay != nil {
					*result = *replay
					return nil
				}
				if replayErr != nil && !errors.Is(replayErr, gorm.ErrRecordNotFound) {
					return replayErr
				}
			}
			return err
		}
		if pauseID != "" && pauseRecord.ID != pauseID {
			return ErrPauseNotFound
		}
		stored, err := service.AppendEventPayloadTx(ctx, tx, AppendEventParams{
			TenantID:                pauseRecord.TenantID,
			AppID:                   pauseRecord.AppID,
			WorkflowRunID:           workflowRunID,
			EventType:               eventType,
			EventData:               eventData,
			PauseID:                 pauseRecord.ID,
			PauseGeneration:         &pauseRecord.Generation,
			ExpectedPauseID:         pauseRecord.ID,
			ExpectedPauseGeneration: &pauseRecord.Generation,
			ExpectedPauseRevision:   &pauseRecord.Revision,
			IdempotencyKey:          idempotencyKey,
		})
		if err != nil {
			return err
		}
		nodeID, _ := eventData["node_id"].(string)
		outbox, ready, err := service.CompleteReasonsTx(ctx, tx, CompleteReasonsParams{
			WorkflowRunID:     workflowRunID,
			PauseID:           pauseRecord.ID,
			ReasonType:        ReasonTypeQuestionAnswerRequired,
			NodeID:            strings.TrimSpace(nodeID),
			SubmissionEventID: stored.EventID,
			TriggerID:         triggerID,
			ResumeInputs:      eventData,
		})
		if err != nil {
			return err
		}
		result.Event = stored
		result.Outbox = outbox
		result.ResumeReady = ready
		if !ready {
			pendingEvents, pendingErr := service.AppendNextPendingInteractionProjectionTx(
				ctx,
				tx,
				pauseRecord,
				stored.EventID,
			)
			if pendingErr != nil {
				return pendingErr
			}
			result.PendingEvents = pendingEvents
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AppendNextPendingInteractionProjectionTx durably re-projects the next
// unanswered approval or question for a pause that is not ready to resume.
// The caller must invoke it in the same transaction as the interaction
// submission that completed the preceding pause reason.
func (s *Service) AppendNextPendingInteractionProjectionTx(
	ctx context.Context,
	tx *gorm.DB,
	pauseRecord *RunPause,
	submissionEventID string,
) ([]*RunEventPayload, error) {
	if pauseRecord == nil {
		return nil, ErrPauseNotFound
	}

	var pendingReasons []RunPauseReason
	if err := tx.WithContext(ctx).
		Where("pause_id = ? AND status = ?", pauseRecord.ID, RunPauseReasonStatusPending).
		Order("created_at ASC, id ASC").
		Find(&pendingReasons).Error; err != nil {
		return nil, fmt.Errorf("load pending workflow pause reasons: %w", err)
	}
	pendingReasonIdentities := make(map[string]struct{}, len(pendingReasons))
	for _, reason := range pendingReasons {
		switch reason.Type {
		case ReasonTypeQuestionAnswerRequired, ReasonTypeApprovalRequired:
			pendingReasonIdentities[pauseReasonIdentity(reason.Type, reason.NodeID, reason.FormID)] = struct{}{}
		}
	}
	if len(pendingReasonIdentities) == 0 {
		return nil, nil
	}

	var storedEvents []RunEvent
	if err := tx.WithContext(ctx).
		Where(
			"workflow_run_id = ? AND pause_id = ? AND pause_generation = ? AND event_type IN ?",
			pauseRecord.WorkflowRunID,
			pauseRecord.ID,
			pauseRecord.Generation,
			[]string{EventQuestionAnswerRequested, EventApprovalRequested, EventWorkflowPaused},
		).
		Order("sequence ASC").
		Find(&storedEvents).Error; err != nil {
		return nil, fmt.Errorf("load pending workflow interaction evidence: %w", err)
	}

	var interactionSource *RunEventPayload
	var pausedSource *RunEventPayload
	for index := range storedEvents {
		payload, err := runEventPayloadFromModel(storedEvents[index])
		if err != nil {
			return nil, err
		}
		switch payload.Event {
		case EventQuestionAnswerRequested:
			identity := pauseReasonIdentity(
				ReasonTypeQuestionAnswerRequired,
				stringFromEventData(payload.Data["node_id"]),
				"",
			)
			if _, pending := pendingReasonIdentities[identity]; pending && interactionSource == nil {
				interactionSource = payload
			}
		case EventApprovalRequested:
			identity := pauseReasonIdentity(
				ReasonTypeApprovalRequired,
				stringFromEventData(payload.Data["node_id"]),
				stringFromEventData(payload.Data["form_id"]),
			)
			if _, pending := pendingReasonIdentities[identity]; pending && interactionSource == nil {
				interactionSource = payload
			}
		case EventWorkflowPaused:
			pausedSource = payload
		}
	}
	if interactionSource == nil {
		return nil, fmt.Errorf("pending workflow interaction has no durable request event")
	}

	now := time.Now()
	interactionData := copyEventData(interactionSource.Data)
	interactionData["created_at"] = now.Unix()
	interactionNodeID := stringFromEventData(interactionData["node_id"])

	pausedData := map[string]interface{}{
		"id":           pauseRecord.WorkflowRunID,
		"status":       "paused",
		"paused_nodes": pendingPauseNodeIDs(pendingReasons),
		"reasons":      pendingPauseReasonData(pendingReasons, pausedSource),
		"created_at":   now.Unix(),
	}
	if pausedSource != nil {
		pausedData = copyEventData(pausedSource.Data)
		pausedData["status"] = "paused"
		pausedData["paused_nodes"] = pendingPauseNodeIDs(pendingReasons)
		pausedData["reasons"] = pendingPauseReasonData(pendingReasons, pausedSource)
		pausedData["created_at"] = now.Unix()
	}

	pauseGeneration := pauseRecord.Generation
	pauseRevision := pauseRecord.Revision
	idempotencySuffix := fmt.Sprintf(
		"%s:%d:%s:%s",
		pauseRecord.ID,
		pauseRecord.Generation,
		interactionSource.EventID,
		submissionEventID,
	)
	stored, err := s.AppendEventBatchTx(ctx, tx, AppendEventBatchRequest{
		TenantID:      pauseRecord.TenantID,
		AppID:         pauseRecord.AppID,
		WorkflowRunID: pauseRecord.WorkflowRunID,
		FlushReason:   "interaction_pending",
		Fence: RuntimeFence{
			ExpectedPauseID:         pauseRecord.ID,
			ExpectedPauseGeneration: &pauseGeneration,
			ExpectedPauseRevision:   &pauseRevision,
		},
		Events: []EventDraft{
			{
				EventType:       interactionSource.Event,
				EventData:       interactionData,
				PauseID:         pauseRecord.ID,
				PauseGeneration: &pauseGeneration,
				IdempotencyKey:  "interaction-pending:" + idempotencySuffix,
				OccurredAt:      now,
			},
			{
				EventType:       EventWorkflowPaused,
				EventData:       pausedData,
				PauseID:         pauseRecord.ID,
				PauseGeneration: &pauseGeneration,
				IdempotencyKey:  "pause-pending:" + idempotencySuffix,
				OccurredAt:      now,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("append pending workflow interaction projection for node %s: %w", interactionNodeID, err)
	}
	result := make([]*RunEventPayload, 0, len(stored))
	for _, event := range stored {
		if event.Payload != nil {
			result = append(result, event.Payload)
		}
	}
	if len(result) != 2 {
		return nil, fmt.Errorf("pending workflow interaction projection returned %d events", len(result))
	}
	return result, nil
}

func copyEventData(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return map[string]interface{}{}
	}
	result := make(map[string]interface{}, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func pendingPauseNodeIDs(reasons []RunPauseReason) []string {
	result := make([]string, 0, len(reasons))
	seen := make(map[string]struct{}, len(reasons))
	for _, reason := range reasons {
		nodeID := strings.TrimSpace(reason.NodeID)
		if nodeID == "" {
			continue
		}
		if _, ok := seen[nodeID]; ok {
			continue
		}
		seen[nodeID] = struct{}{}
		result = append(result, nodeID)
	}
	return result
}

func pendingPauseReasonData(reasons []RunPauseReason, pausedSource *RunEventPayload) []interface{} {
	originalByIdentity := map[string]map[string]interface{}{}
	if pausedSource != nil {
		if original, ok := pausedSource.Data["reasons"].([]interface{}); ok {
			for _, item := range original {
				data, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				identity := pauseReasonIdentity(
					stringFromEventData(data["type"]),
					stringFromEventData(data["node_id"]),
					stringFromEventData(data["form_id"]),
				)
				originalByIdentity[identity] = data
			}
		}
	}

	result := make([]interface{}, 0, len(reasons))
	for _, reason := range reasons {
		identity := pauseReasonIdentity(reason.Type, reason.NodeID, reason.FormID)
		if original, ok := originalByIdentity[identity]; ok {
			result = append(result, copyEventData(original))
			continue
		}
		data := map[string]interface{}{"type": reason.Type}
		if reason.NodeID != "" {
			data["node_id"] = reason.NodeID
		}
		if reason.FormID != "" {
			data["form_id"] = reason.FormID
		}
		result = append(result, data)
	}
	return result
}

func pauseReasonIdentity(reasonType, nodeID, formID string) string {
	return strings.TrimSpace(reasonType) + "\x00" + strings.TrimSpace(nodeID) + "\x00" + strings.TrimSpace(formID)
}

func stringFromEventData(value interface{}) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func (s *Service) loadInteractionSubmissionReplay(ctx context.Context, workflowRunID, idempotencyKey string) (*InteractionSubmission, error) {
	var event RunEvent
	if err := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND idempotency_key = ?", workflowRunID, idempotencyKey).
		First(&event).Error; err != nil {
		return nil, err
	}
	payload, err := runEventPayloadFromModel(event)
	if err != nil {
		return nil, err
	}
	result := &InteractionSubmission{Event: payload}
	if event.PauseID == nil || event.PauseGeneration == nil {
		return result, nil
	}
	outboxKey := fmt.Sprintf("workflow-resume:%s:%d", *event.PauseID, *event.PauseGeneration)
	var outbox RuntimeOutbox
	if err := s.db.WithContext(ctx).Where("idempotency_key = ?", outboxKey).First(&outbox).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, nil
		}
		return nil, err
	}
	result.Outbox = &outbox
	result.ResumeReady = true
	return result, nil
}

func runEventPayloadFromModel(event RunEvent) (*RunEventPayload, error) {
	data := map[string]interface{}{}
	if err := json.Unmarshal([]byte(event.EventData), &data); err != nil {
		return nil, fmt.Errorf("decode workflow event data: %w", err)
	}
	return &RunEventPayload{
		EventID: event.ID, Sequence: event.Sequence, Event: event.EventType, Category: event.Category,
		SchemaVersion: event.SchemaVersion, PayloadVersion: 1, ExecutionID: stringValue(event.ExecutionID),
		PauseID: stringValue(event.PauseID), PauseGeneration: event.PauseGeneration,
		IdempotencyKey: stringValue(event.IdempotencyKey), Data: data, CreatedAt: event.CreatedAt.Unix(),
		OccurredAtMS: event.OccurredAt.UnixMilli(), RecordedAtMS: event.CreatedAt.UnixMilli(),
	}, nil
}

type CompleteReasonsParams struct {
	WorkflowRunID     string
	PauseID           string
	ReasonType        string
	NodeID            string
	FormID            string
	SubmissionEventID string
	TriggerID         string
	ResumeInputs      map[string]interface{}
}

// CompleteReasons marks the matching reasons as completed and only makes the
// pause resumable when every reason in the same pause generation is complete.
// It is safe to call repeatedly for the same submission event.
func (s *Service) CompleteReasons(ctx context.Context, params CompleteReasonsParams) (*RuntimeOutbox, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, fmt.Errorf("workflow pause service is not initialized")
	}
	var outbox *RuntimeOutbox
	ready := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		outbox, ready, err = NewService(tx).CompleteReasonsTx(ctx, tx, params)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	return outbox, ready, nil
}

// CompleteReasonsTx performs the reason transition in the caller's lifecycle
// transaction. It must not publish events before that transaction commits.
func (s *Service) CompleteReasonsTx(ctx context.Context, tx *gorm.DB, params CompleteReasonsParams) (*RuntimeOutbox, bool, error) {
	if s == nil || tx == nil {
		return nil, false, fmt.Errorf("workflow pause reason transaction is not initialized")
	}
	var pause RunPause
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workflow_run_id = ? AND resumed_at IS NULL", params.WorkflowRunID)
	if params.PauseID != "" {
		query = query.Where("id = ?", params.PauseID)
	}
	if err := query.First(&pause).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrPauseNotFound
		}
		return nil, false, fmt.Errorf("lock workflow pause reasons: %w", err)
	}
	var reasons []RunPauseReason
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pause_id = ?", pause.ID).
		Order("created_at ASC").Find(&reasons).Error; err != nil {
		return nil, false, fmt.Errorf("lock workflow pause reasons: %w", err)
	}
	matched := false
	now := time.Now()
	for i := range reasons {
		reason := &reasons[i]
		if reason.Type != params.ReasonType || (params.NodeID != "" && reason.NodeID != params.NodeID) || (params.FormID != "" && reason.FormID != params.FormID) {
			continue
		}
		matched = true
		if reason.Status == RunPauseReasonStatusCompleted {
			continue
		}
		updates := map[string]interface{}{
			"status": RunPauseReasonStatusCompleted, "revision": gorm.Expr("revision + 1"), "completed_at": now,
		}
		if params.SubmissionEventID != "" {
			updates["submission_event_id"] = params.SubmissionEventID
		}
		result := tx.Model(&RunPauseReason{}).
			Where("id = ? AND revision = ? AND status = ?", reason.ID, reason.Revision, RunPauseReasonStatusPending).
			Updates(updates)
		if result.Error != nil {
			return nil, false, fmt.Errorf("complete workflow pause reason: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return nil, false, ErrPauseNotResumeReady
		}
		reason.Status = RunPauseReasonStatusCompleted
		reason.CompletedAt = &now
		if params.SubmissionEventID != "" {
			submissionEventID := params.SubmissionEventID
			reason.SubmissionEventID = &submissionEventID
		}
	}
	if !matched {
		return nil, false, ErrPauseNotResumeReady
	}
	var pending int64
	if err := tx.Model(&RunPauseReason{}).Where("pause_id = ? AND status <> ?", pause.ID, RunPauseReasonStatusCompleted).
		Count(&pending).Error; err != nil {
		return nil, false, fmt.Errorf("count pending workflow pause reasons: %w", err)
	}
	if pending != 0 {
		return nil, false, nil
	}
	triggerID, interactionType, resumeInputs, err := completedReasonResumePayloadTx(ctx, tx, reasons, params)
	if err != nil {
		return nil, false, err
	}
	prepared, err := NewService(tx).prepareResumeTx(ctx, tx, params.WorkflowRunID, pause.ID, triggerID, interactionType, resumeInputs)
	if err != nil {
		return nil, false, err
	}
	return prepared, true, nil
}

func completedReasonResumePayloadTx(
	ctx context.Context,
	tx *gorm.DB,
	reasons []RunPauseReason,
	params CompleteReasonsParams,
) (string, string, map[string]interface{}, error) {
	triggerID := params.TriggerID
	interactionType := params.ReasonType
	resumeInputs := copyEventData(params.ResumeInputs)

	eventIDs := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if reason.Status != RunPauseReasonStatusCompleted || reason.SubmissionEventID == nil {
			continue
		}
		eventID := strings.TrimSpace(*reason.SubmissionEventID)
		if eventID != "" {
			eventIDs = append(eventIDs, eventID)
		}
	}
	if len(eventIDs) == 0 {
		return triggerID, interactionType, resumeInputs, nil
	}

	var events []RunEvent
	if err := tx.WithContext(ctx).Where("id IN ?", eventIDs).Find(&events).Error; err != nil {
		return "", "", nil, fmt.Errorf("load completed workflow interaction submissions: %w", err)
	}
	eventsByID := make(map[string]RunEvent, len(events))
	for _, event := range events {
		eventsByID[event.ID] = event
	}

	submissions := make([]interface{}, 0, len(eventIDs))
	for _, reason := range reasons {
		if reason.Status != RunPauseReasonStatusCompleted || reason.SubmissionEventID == nil {
			continue
		}
		event, ok := eventsByID[strings.TrimSpace(*reason.SubmissionEventID)]
		if !ok {
			return "", "", nil, fmt.Errorf("completed workflow pause reason %s has no submission event", reason.ID)
		}
		eventData := map[string]interface{}{}
		if err := json.Unmarshal([]byte(event.EventData), &eventData); err != nil {
			return "", "", nil, fmt.Errorf("decode completed workflow interaction submission %s: %w", event.ID, err)
		}
		submissions = append(submissions, map[string]interface{}{
			"reason_type": reason.Type,
			"node_id":     reason.NodeID,
			"form_id":     reason.FormID,
			"event_id":    event.ID,
			"data":        copyEventData(eventData),
		})
		if reason.Type != ReasonTypeQuestionAnswerRequired {
			continue
		}
		interactionType = ReasonTypeQuestionAnswerRequired
		if strings.TrimSpace(reason.NodeID) != "" {
			triggerID = strings.TrimSpace(reason.NodeID)
		}
		for key, value := range eventData {
			resumeInputs[key] = value
		}
	}
	resumeInputs["interaction_submissions"] = submissions
	return triggerID, interactionType, resumeInputs, nil
}

func (s *Service) ClaimResume(ctx context.Context, workflowRunID, pauseID string, leaseDuration time.Duration) (*ExecutionClaim, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}
	now := time.Now()
	claim := &ExecutionClaim{}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run struct {
			RuntimeProtocolVersion int        `gorm:"column:runtime_protocol_version"`
			ExecutionGeneration    int64      `gorm:"column:execution_generation"`
			NextEventSequence      int        `gorm:"column:next_event_sequence"`
			ConversationID         *string    `gorm:"column:conversation_id"`
			Status                 string     `gorm:"column:status"`
			FinishedAt             *time.Time `gorm:"column:finished_at"`
		}
		if err := tx.Table("workflow_run_logs").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("runtime_protocol_version, execution_generation, next_event_sequence, conversation_id, status, finished_at").
			Where("id = ? AND deleted_at IS NULL", workflowRunID).Take(&run).Error; err != nil {
			return fmt.Errorf("lock workflow run for resume claim: %w", err)
		}
		if run.RuntimeProtocolVersion < 2 || run.FinishedAt != nil || workflowRunStatusTerminal(run.Status) {
			return ErrPauseNotResumeReady
		}
		var pause RunPause
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workflow_run_id = ? AND resumed_at IS NULL", workflowRunID)
		if pauseID != "" {
			query = query.Where("id = ?", pauseID)
		}
		if err := query.First(&pause).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPauseNotFound
			}
			return fmt.Errorf("lock workflow pause for resume claim: %w", err)
		}
		if pause.Status == RunPauseStatusResuming && pause.LeaseExpiresAt != nil && pause.LeaseExpiresAt.After(now) {
			recordResumeClaimConflict(ctx, "active_lease")
			return ErrResumeAlreadyRunning
		}
		if pause.Status != RunPauseStatusResumeReady && pause.Status != RunPauseStatusResuming {
			recordResumeClaimConflict(ctx, "not_ready")
			return ErrPauseNotResumeReady
		}
		if pause.Status == RunPauseStatusResuming {
			recordLeaseTakeover(ctx)
		}

		executionID := uuid.NewString()
		leaseExpires := now.Add(leaseDuration)
		generation := run.ExecutionGeneration + 1
		pauseResult := tx.Model(&RunPause{}).
			Where("id = ? AND revision = ?", pause.ID, pause.Revision).
			Updates(map[string]interface{}{
				"status":              RunPauseStatusResuming,
				"revision":            gorm.Expr("revision + 1"),
				"resume_execution_id": executionID,
				"lease_expires_at":    leaseExpires,
			})
		if pauseResult.Error != nil {
			return fmt.Errorf("claim workflow pause: %w", pauseResult.Error)
		}
		if pauseResult.RowsAffected != 1 {
			recordResumeClaimConflict(ctx, "pause_cas")
			return ErrResumeAlreadyRunning
		}
		runResult := tx.Table("workflow_run_logs").
			Where("id = ? AND execution_generation = ? AND finished_at IS NULL AND status NOT IN ?", workflowRunID, run.ExecutionGeneration, terminalWorkflowRunStatuses()).
			Updates(map[string]interface{}{
				"status":                     "running",
				"execution_generation":       generation,
				"active_execution_id":        executionID,
				"execution_lease_expires_at": leaseExpires,
				"state_revision":             gorm.Expr("state_revision + 1"),
			})
		if runResult.Error != nil {
			return fmt.Errorf("claim workflow execution: %w", runResult.Error)
		}
		if runResult.RowsAffected != 1 {
			return ErrExecutionOwnershipLost
		}
		if pause.ConversationID != nil && strings.TrimSpace(*pause.ConversationID) != "" {
			messageResult := tx.Table("agents_messages").
				Where("workflow_run_id = ? AND deleted_at IS NULL AND execution_generation <= ?", workflowRunID, generation).
				Updates(map[string]interface{}{
					"status":               "running",
					"execution_generation": generation,
					"projection_revision":  gorm.Expr("projection_revision + 1"),
					"updated_at":           now,
				})
			if messageResult.Error != nil {
				return fmt.Errorf("claim workflow message projection: %w", messageResult.Error)
			}
			if messageResult.RowsAffected != 1 {
				return fmt.Errorf("claim workflow message projection missing")
			}
			if run.ConversationID != nil && strings.TrimSpace(*run.ConversationID) != "" {
				conversationResult := tx.Table("agents_conversations").
					Where("id = ? AND active_workflow_run_id = ?", strings.TrimSpace(*run.ConversationID), workflowRunID).
					Updates(map[string]interface{}{
						"runtime_status":   "running",
						"runtime_revision": gorm.Expr("runtime_revision + 1"),
					})
				if conversationResult.Error != nil {
					return fmt.Errorf("claim workflow conversation: %w", conversationResult.Error)
				}
				if conversationResult.RowsAffected != 1 {
					return ErrExecutionOwnershipLost
				}
			}
		}
		claimedPauseRevision := pause.Revision + 1
		claimedPauseGeneration := pause.Generation
		stored, err := NewService(tx).AppendEventPayloadTx(ctx, tx, AppendEventParams{
			TenantID: pause.TenantID, AppID: pause.AppID, WorkflowRunID: workflowRunID,
			EventType: EventWorkflowResumed, Category: EventCategoryControl, ExecutionID: executionID,
			PauseID: pause.ID, PauseGeneration: &claimedPauseGeneration,
			ExpectedExecutionID: executionID, ExpectedExecutionGeneration: generation,
			ExpectedPauseID: pause.ID, ExpectedPauseGeneration: &claimedPauseGeneration, ExpectedPauseRevision: &claimedPauseRevision,
			// A lease takeover creates a new execution generation for the same pause.
			// Key the durable transition by that generation so the takeover records
			// its own resume boundary without colliding with the abandoned owner.
			IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:resumed", workflowRunID, generation),
			OccurredAt:     now,
			EventData: map[string]interface{}{
				"workflow_run_id":      workflowRunID,
				"execution_id":         executionID,
				"execution_generation": generation,
				"pause_id":             pause.ID,
				"pause_generation":     pause.Generation,
				"status":               "running",
			},
		})
		if err != nil {
			return fmt.Errorf("append workflow resumed event: %w", err)
		}
		*claim = ExecutionClaim{
			WorkflowRunID:   workflowRunID,
			PauseID:         pause.ID,
			Generation:      generation,
			PauseGeneration: pause.Generation,
			ExecutionID:     executionID,
			LeaseExpires:    leaseExpires,
			EventCursor:     stored.Sequence,
			Event:           stored,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claim, nil
}

func workflowRunStatusTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "failed", "stopped", "expired", "partial-succeeded":
		return true
	default:
		return false
	}
}

func terminalWorkflowRunStatuses() []string {
	return []string{"succeeded", "failed", "stopped", "expired", "partial-succeeded"}
}

func (s *Service) RenewExecutionLease(ctx context.Context, claim ExecutionClaim, leaseDuration time.Duration) (time.Time, error) {
	if s == nil || s.db == nil {
		return time.Time{}, fmt.Errorf("workflow pause service is not initialized")
	}
	if leaseDuration <= 0 {
		leaseDuration = 2 * time.Minute
	}
	leaseExpires := time.Now().Add(leaseDuration)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		runResult := tx.Table("workflow_run_logs").
			Where("id = ? AND execution_generation = ? AND active_execution_id = ?", claim.WorkflowRunID, claim.Generation, claim.ExecutionID).
			Updates(map[string]interface{}{
				"execution_lease_expires_at": leaseExpires,
				"state_revision":             gorm.Expr("state_revision + 1"),
			})
		if runResult.Error != nil {
			return fmt.Errorf("renew workflow execution lease: %w", runResult.Error)
		}
		if runResult.RowsAffected != 1 {
			return ErrExecutionOwnershipLost
		}
		if claim.PauseID == "" {
			return nil
		}
		pauseResult := tx.Model(&RunPause{}).
			Where("id = ? AND generation = ? AND status = ? AND resume_execution_id = ?", claim.PauseID, claim.PauseGeneration, RunPauseStatusResuming, claim.ExecutionID).
			Updates(map[string]interface{}{
				"lease_expires_at": leaseExpires,
				"revision":         gorm.Expr("revision + 1"),
			})
		if pauseResult.Error != nil {
			return fmt.Errorf("renew workflow pause lease: %w", pauseResult.Error)
		}
		if pauseResult.RowsAffected != 1 {
			return ErrExecutionOwnershipLost
		}
		return nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return leaseExpires, nil
}

// FinalizeExpiredExecutions safely fails V2 executions whose owner stopped
// renewing its lease. It deliberately does not resume or replay work because a
// node may already have produced an external side effect.
func (s *Service) FinalizeExpiredExecutions(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	type expiredCandidate struct {
		ID string `gorm:"column:id"`
	}
	var candidates []expiredCandidate
	if err := s.db.WithContext(ctx).Table("workflow_run_logs").
		Select("id").
		Where("runtime_protocol_version >= ? AND status = ? AND active_execution_id IS NOT NULL AND execution_lease_expires_at < ? AND deleted_at IS NULL", 2, "running", cutoff).
		Order("execution_lease_expires_at ASC").
		Limit(limit).
		Scan(&candidates).Error; err != nil {
		return nil, fmt.Errorf("list expired workflow executions: %w", err)
	}
	finalized := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		completed, err := s.finalizeExpiredExecution(ctx, candidate.ID, cutoff)
		if err != nil {
			return finalized, err
		}
		if completed {
			finalized = append(finalized, candidate.ID)
			recordOrphanFinalized(ctx)
		}
	}
	return finalized, nil
}

func (s *Service) ObserveActiveV1Runs(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("workflow pause service is not initialized")
	}
	var count int64
	if err := s.db.WithContext(ctx).Table("workflow_run_logs").
		Where("runtime_protocol_version < ? AND status IN ? AND deleted_at IS NULL", 2, []string{"running", "paused", "resuming"}).
		Count(&count).Error; err != nil {
		return fmt.Errorf("count active V1 workflow runs: %w", err)
	}
	recordActiveV1Runs(ctx, count)
	return nil
}

func (s *Service) finalizeExpiredExecution(ctx context.Context, workflowRunID string, cutoff time.Time) (bool, error) {
	finalized := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run struct {
			ID              string     `gorm:"column:id"`
			TenantID        string     `gorm:"column:tenant_id"`
			AppID           string     `gorm:"column:agent_id"`
			WorkflowID      string     `gorm:"column:workflow_id"`
			ExecutionID     *string    `gorm:"column:active_execution_id"`
			Generation      int64      `gorm:"column:execution_generation"`
			LeaseExpiresAt  *time.Time `gorm:"column:execution_lease_expires_at"`
			RuntimeProtocol int        `gorm:"column:runtime_protocol_version"`
			ConversationID  *string    `gorm:"column:conversation_id"`
		}
		if err := tx.Table("workflow_run_logs").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id, tenant_id, agent_id, workflow_id, conversation_id, active_execution_id, execution_generation, execution_lease_expires_at, runtime_protocol_version").
			Where("id = ? AND status = ? AND deleted_at IS NULL", workflowRunID, "running").
			Take(&run).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("lock expired workflow execution: %w", err)
		}
		if run.RuntimeProtocol < 2 || run.ExecutionID == nil || run.LeaseExpiresAt == nil || !run.LeaseExpiresAt.Before(cutoff) {
			return nil
		}
		now := time.Now()
		code := "workflow_execution_interrupted"
		message := "Workflow execution was interrupted after its execution lease expired."
		result := tx.Table("workflow_run_logs").
			Where("id = ? AND status = ? AND execution_generation = ? AND active_execution_id = ? AND execution_lease_expires_at < ?", run.ID, "running", run.Generation, *run.ExecutionID, cutoff).
			Updates(map[string]interface{}{
				"status": "failed", "error": code, "finished_at": now,
				"exceptions_count": gorm.Expr("exceptions_count + 1"),
				"state_revision":   gorm.Expr("state_revision + 1"),
			})
		if result.Error != nil {
			return fmt.Errorf("finalize expired workflow execution: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return nil
		}

		var messageProjection struct {
			ID             string `gorm:"column:id"`
			ConversationID string `gorm:"column:conversation_id"`
		}
		messageExists := false
		messageQuery := tx.Table("agents_messages").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id, conversation_id").Where("workflow_run_id = ? AND deleted_at IS NULL", run.ID).Take(&messageProjection)
		if messageQuery.Error == nil {
			messageExists = true
			messageResult := tx.Table("agents_messages").
				Where("workflow_run_id = ? AND deleted_at IS NULL AND execution_generation <= ?", run.ID, run.Generation).
				Updates(map[string]interface{}{
					"status": "error", "error": code, "execution_generation": run.Generation,
					"projection_revision": gorm.Expr("projection_revision + 1"), "updated_at": now,
				})
			if messageResult.Error != nil {
				return fmt.Errorf("fail expired workflow message projection: %w", messageResult.Error)
			}
			if messageResult.RowsAffected != 1 {
				return ErrExecutionOwnershipLost
			}
		} else if !errors.Is(messageQuery.Error, gorm.ErrRecordNotFound) {
			return fmt.Errorf("load expired workflow message projection: %w", messageQuery.Error)
		}
		conversationID := stringValue(run.ConversationID)
		if conversationID != "" {
			conversationResult := tx.Table("agents_conversations").
				Where("id = ? AND active_workflow_run_id = ?", conversationID, run.ID).
				Updates(map[string]interface{}{
					"runtime_status":         "idle",
					"active_workflow_run_id": nil,
					"runtime_revision":       gorm.Expr("runtime_revision + 1"),
				})
			if conversationResult.Error != nil {
				return fmt.Errorf("release interrupted workflow conversation: %w", conversationResult.Error)
			}
			if conversationResult.RowsAffected != 1 {
				return ErrExecutionOwnershipLost
			}
		}

		if err := tx.Model(&RunPause{}).
			Where("workflow_run_id = ? AND status = ? AND resume_execution_id = ?", run.ID, RunPauseStatusResuming, *run.ExecutionID).
			Updates(map[string]interface{}{
				"status": RunPauseStatusClosed, "resumed_at": now, "lease_expires_at": nil,
				"revision": gorm.Expr("revision + 1"),
			}).Error; err != nil {
			return fmt.Errorf("close interrupted workflow pause: %w", err)
		}

		service := NewService(tx)
		errorData := map[string]interface{}{
			"workflow_run_id": run.ID, "code": code, "message": message,
			"execution_id": *run.ExecutionID, "execution_generation": run.Generation,
		}
		eventDrafts := []EventDraft{{
			EventType: EventError, Category: EventCategoryControl, ExecutionID: *run.ExecutionID,
			IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:interrupted", run.ID, run.Generation),
			OccurredAt:     now, EventData: errorData,
		}}
		if messageExists {
			eventDrafts = append(eventDrafts, EventDraft{
				EventType: "message_end", Category: EventCategoryControl, ExecutionID: *run.ExecutionID,
				IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:message_end", run.ID, run.Generation),
				OccurredAt:     now,
				EventData: map[string]interface{}{
					"id": messageProjection.ID, "message_id": messageProjection.ID,
					"conversation_id": messageProjection.ConversationID, "status": "error", "error": code,
				},
			})
		}
		eventDrafts = append(eventDrafts, EventDraft{
			EventType: EventWorkflowFinished, Category: EventCategoryControl, ExecutionID: *run.ExecutionID,
			IdempotencyKey: fmt.Sprintf("run:%s:generation:%d:failed", run.ID, run.Generation), OccurredAt: now,
			EventData: map[string]interface{}{
				"id": run.ID, "workflow_run_id": run.ID, "workflow_id": run.WorkflowID,
				"status": "failed", "error": errorData, "finished_at": now.Unix(),
			},
		})
		if _, err := service.AppendEventBatchTx(ctx, tx, AppendEventBatchRequest{
			TenantID: run.TenantID, AppID: run.AppID, WorkflowRunID: run.ID,
			FlushReason: "orphan_terminal_barrier",
			Fence:       RuntimeFence{ExpectedExecutionID: *run.ExecutionID, ExpectedExecutionGeneration: run.Generation},
			Events:      eventDrafts,
		}); err != nil {
			return fmt.Errorf("append interrupted workflow terminal event batch: %w", err)
		}
		clearOwner := tx.Table("workflow_run_logs").
			Where("id = ? AND status = ? AND execution_generation = ? AND active_execution_id = ?", run.ID, "failed", run.Generation, *run.ExecutionID).
			Updates(map[string]interface{}{
				"active_execution_id":        nil,
				"execution_lease_expires_at": nil,
			})
		if clearOwner.Error != nil {
			return fmt.Errorf("clear interrupted workflow execution owner: %w", clearOwner.Error)
		}
		if clearOwner.RowsAffected != 1 {
			return ErrExecutionOwnershipLost
		}
		finalized = true
		return nil
	})
	return finalized, err
}

func (s *Service) ClosePause(ctx context.Context, claim ExecutionClaim) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("workflow pause service is not initialized")
	}
	now := time.Now()
	result := s.db.WithContext(ctx).Model(&RunPause{}).
		Where("id = ? AND status = ? AND resume_execution_id = ?", claim.PauseID, RunPauseStatusResuming, claim.ExecutionID).
		Updates(map[string]interface{}{
			"status":           RunPauseStatusClosed,
			"revision":         gorm.Expr("revision + 1"),
			"resumed_at":       now,
			"lease_expires_at": nil,
		})
	if result.Error != nil {
		return fmt.Errorf("close workflow pause: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrExecutionOwnershipLost
	}
	return nil
}

func (s *Service) ListPendingOutbox(ctx context.Context, limit int) ([]RuntimeOutbox, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var items []RuntimeOutbox
	if err := s.db.WithContext(ctx).
		Where("status = ? AND next_attempt_at <= ?", RuntimeOutboxPending, time.Now()).
		Order("created_at ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list workflow runtime outbox: %w", err)
	}
	return items, nil
}

func (s *Service) RuntimeOutboxDispatchable(ctx context.Context, id string) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("workflow pause service is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	pauseRunJoin := "pause.workflow_run_id = outbox.workflow_run_id::text"
	if s.db.Dialector.Name() != "postgres" {
		pauseRunJoin = "pause.workflow_run_id = CAST(outbox.workflow_run_id AS text)"
	}
	var count int64
	err := s.db.WithContext(ctx).
		Table("workflow_runtime_outbox AS outbox").
		Joins("JOIN workflow_run_logs AS run ON run.id = outbox.workflow_run_id AND run.deleted_at IS NULL").
		Joins("JOIN workflow_run_pauses AS pause ON pause.id = outbox.pause_id AND "+pauseRunJoin).
		Where("outbox.id = ? AND outbox.kind = ? AND outbox.status = ?", id, RuntimeOutboxKindResume, RuntimeOutboxPending).
		Where("run.finished_at IS NULL AND run.status NOT IN ?", terminalWorkflowRunStatuses()).
		Where("pause.resumed_at IS NULL AND pause.status IN ?", []string{RunPauseStatusResumeReady, RunPauseStatusResuming}).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check workflow runtime outbox dispatchability: %w", err)
	}
	return count == 1, nil
}

func (s *Service) MarkOutboxPublished(ctx context.Context, id string) error {
	now := time.Now()
	var outbox RuntimeOutbox
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&outbox).Error; err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Model(&RuntimeOutbox{}).Where("id = ? AND status = ?", id, RuntimeOutboxPending).
		Updates(map[string]interface{}{"status": RuntimeOutboxPublished, "published_at": now, "updated_at": now})
	if result.Error == nil && result.RowsAffected == 1 {
		recordOutboxLag(ctx, outbox.CreatedAt)
	}
	return result.Error
}

func (s *Service) MarkOutboxRetry(ctx context.Context, id string, dispatchErr error) error {
	now := time.Now()
	message := ""
	if dispatchErr != nil {
		message = dispatchErr.Error()
	}
	return s.db.WithContext(ctx).Model(&RuntimeOutbox{}).Where("id = ? AND status = ?", id, RuntimeOutboxPending).
		Updates(map[string]interface{}{
			"attempts":        gorm.Expr("attempts + 1"),
			"next_attempt_at": now.Add(10 * time.Second),
			"last_error":      message,
			"updated_at":      now,
		}).Error
}

func (s *Service) AppendEvent(ctx context.Context, params AppendEventParams) error {
	_, err := s.AppendEventPayload(ctx, params)
	return err
}

func (s *Service) AppendEventPayload(ctx context.Context, params AppendEventParams) (*RunEventPayload, error) {
	stored, err := s.AppendEventBatch(ctx, eventBatchRequestFromAppendParams(params))
	if err != nil {
		return nil, err
	}
	if len(stored) != 1 || stored[0].Payload == nil {
		return nil, fmt.Errorf("workflow event batch returned no event")
	}
	return stored[0].Payload, nil
}

func allocateRunEventSequence(tx *gorm.DB, workflowRunID, expectedExecutionID string, expectedExecutionGeneration int64) (int, int, string, bool, error) {
	if tx.Dialector.Name() == "postgres" {
		var run struct {
			NextEventSequence      int     `gorm:"column:next_event_sequence"`
			RuntimeProtocolVersion int     `gorm:"column:runtime_protocol_version"`
			ActiveExecutionID      *string `gorm:"column:active_execution_id"`
		}
		query := `
			UPDATE workflow_run_logs
			SET next_event_sequence = next_event_sequence + 1
			WHERE id = ?`
		args := []interface{}{workflowRunID}
		if expectedExecutionID != "" && expectedExecutionGeneration > 0 {
			query += ` AND active_execution_id = ? AND execution_generation = ?`
			args = append(args, expectedExecutionID, expectedExecutionGeneration)
		}
		query += `
			RETURNING next_event_sequence, runtime_protocol_version, active_execution_id
		`
		result := tx.Raw(query, args...).Scan(&run)
		if result.Error != nil {
			return 0, 0, "", false, fmt.Errorf("allocate workflow event sequence: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			if expectedExecutionID != "" && expectedExecutionGeneration > 0 {
				return 0, 0, "", false, ErrExecutionOwnershipLost
			}
			return 0, 0, "", false, nil
		}
		return run.NextEventSequence, run.RuntimeProtocolVersion, stringValue(run.ActiveExecutionID), true, nil
	}
	query := tx.Table("workflow_run_logs").Where("id = ?", workflowRunID)
	if expectedExecutionID != "" && expectedExecutionGeneration > 0 {
		query = query.Where("active_execution_id = ? AND execution_generation = ?", expectedExecutionID, expectedExecutionGeneration)
	}
	result := query.
		UpdateColumn("next_event_sequence", gorm.Expr("next_event_sequence + 1"))
	if result.Error != nil {
		return 0, 0, "", false, fmt.Errorf("allocate workflow event sequence: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		if expectedExecutionID != "" && expectedExecutionGeneration > 0 {
			return 0, 0, "", false, ErrExecutionOwnershipLost
		}
		return 0, 0, "", false, nil
	}
	var run struct {
		NextEventSequence      int     `gorm:"column:next_event_sequence"`
		RuntimeProtocolVersion int     `gorm:"column:runtime_protocol_version"`
		ActiveExecutionID      *string `gorm:"column:active_execution_id"`
	}
	if err := tx.Table("workflow_run_logs").Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("next_event_sequence, runtime_protocol_version, active_execution_id").
		Where("id = ?", workflowRunID).
		Scan(&run).Error; err != nil {
		return 0, 0, "", false, fmt.Errorf("load allocated workflow event sequence: %w", err)
	}
	return run.NextEventSequence, run.RuntimeProtocolVersion, stringValue(run.ActiveExecutionID), true, nil
}

func validateExpectedExecutionEventOwner(ctx context.Context, tx *gorm.DB, params AppendEventParams) error {
	if params.ExpectedExecutionID == "" || params.ExpectedExecutionGeneration <= 0 {
		return nil
	}
	var runID string
	if err := tx.Table("workflow_run_logs").
		Where("id = ? AND active_execution_id = ? AND execution_generation = ?", params.WorkflowRunID, params.ExpectedExecutionID, params.ExpectedExecutionGeneration).
		Pluck("id", &runID).Error; err != nil {
		return fmt.Errorf("validate workflow event execution owner: %w", err)
	}
	if runID == "" {
		recordStaleAppendRejected(ctx, "execution_owner")
		return ErrExecutionOwnershipLost
	}
	return nil
}

func validateExpectedPauseEventOwner(ctx context.Context, tx *gorm.DB, params AppendEventParams) error {
	if params.ExpectedPauseID == "" {
		return nil
	}
	query := tx.Model(&RunPause{}).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND workflow_run_id = ?", params.ExpectedPauseID, params.WorkflowRunID)
	if params.ExpectedPauseGeneration != nil {
		query = query.Where("generation = ?", *params.ExpectedPauseGeneration)
	}
	if params.ExpectedPauseRevision != nil {
		query = query.Where("revision = ?", *params.ExpectedPauseRevision)
	}
	var pauseID string
	if err := query.Pluck("id", &pauseID).Error; err != nil {
		return fmt.Errorf("validate workflow pause event owner: %w", err)
	}
	if pauseID == "" {
		recordStaleAppendRejected(ctx, "pause_revision")
		return ErrPauseNotResumeReady
	}
	return nil
}

func allocateLegacyRunEventSequence(tx *gorm.DB, workflowRunID string) (int, error) {
	var lastSequence int
	if err := tx.Model(&RunEvent{}).
		Where("workflow_run_id = ?", workflowRunID).
		Select("COALESCE(MAX(sequence), 0)").
		Scan(&lastSequence).Error; err != nil {
		return 0, fmt.Errorf("load legacy workflow event sequence: %w", err)
	}
	return lastSequence + 1, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func eventCategory(eventType string) string {
	switch eventType {
	case EventWorkflowStarted, EventWorkflowPaused, EventWorkflowResumed, EventWorkflowFinished, EventError:
		return EventCategoryControl
	case EventApprovalRequested, EventApprovalResultFilled, EventApprovalExpired,
		EventQuestionAnswerRequested, EventQuestionAnswerSubmitted:
		return EventCategoryInteraction
	case "message":
		return EventCategoryAnswerCheckpoint
	default:
		return EventCategoryExecution
	}
}

func (s *Service) LatestEventSequence(ctx context.Context, tenantID, workflowRunID string) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("workflow pause service is not initialized")
	}
	var sequence int
	query := s.db.WithContext(ctx).Model(&RunEvent{}).
		Where("workflow_run_id = ?", workflowRunID)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if err := query.Select("COALESCE(MAX(sequence), 0)").Scan(&sequence).Error; err != nil {
		return 0, fmt.Errorf("load workflow event sequence: %w", err)
	}
	return sequence, nil
}

func (s *Service) ListEvents(ctx context.Context, tenantID, workflowRunID string, afterSequence, limit int) (*RunEventsPayload, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("workflow pause service is not initialized")
	}
	if afterSequence < 0 {
		afterSequence = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}

	query := s.db.WithContext(ctx).
		Where("workflow_run_id = ? AND sequence > ?", workflowRunID, afterSequence)
	if tenantID != "" {
		query = query.Where("tenant_id = ?", tenantID)
	}

	var events []RunEvent
	if err := query.Order("sequence ASC").Limit(limit).Find(&events).Error; err != nil {
		return nil, fmt.Errorf("load workflow run events: %w", err)
	}

	payload := &RunEventsPayload{
		WorkflowRunID: workflowRunID,
		Events:        make([]RunEventPayload, 0, len(events)),
	}
	for _, event := range events {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(event.EventData), &data); err != nil {
			return nil, fmt.Errorf("decode workflow run event %s: %w", event.ID, err)
		}
		payload.Events = append(payload.Events, RunEventPayload{
			EventID:         event.ID,
			Sequence:        event.Sequence,
			Event:           event.EventType,
			Category:        event.Category,
			SchemaVersion:   event.SchemaVersion,
			PayloadVersion:  1,
			ExecutionID:     stringValue(event.ExecutionID),
			PauseID:         stringValue(event.PauseID),
			PauseGeneration: event.PauseGeneration,
			IdempotencyKey:  stringValue(event.IdempotencyKey),
			Data:            data,
			CreatedAt:       event.CreatedAt.Unix(),
			OccurredAtMS:    event.OccurredAt.UnixMilli(),
			RecordedAtMS:    event.CreatedAt.UnixMilli(),
		})
	}
	return payload, nil
}
