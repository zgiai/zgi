package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RunStore is the durable truth for manual DigitalHuman execution. Queue state
// is intentionally not part of this contract.
type RunStore interface {
	CreateRunWithOutbox(context.Context, *Run, *RunOutbox) error
	RunByID(context.Context, Scope, uuid.UUID, uuid.UUID) (*Run, error)
	RunByIdempotency(context.Context, uuid.UUID, uuid.UUID, string) (*Run, error)
	ListRuns(context.Context, Scope, uuid.UUID, ListQuery) ([]*Run, int64, error)
	ClaimRun(context.Context, uuid.UUID, time.Time) (*Run, bool, error)
	LinkRunMessage(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, time.Time) error
	ListRecoverableRuns(context.Context, time.Time, int) ([]*Run, error)
	CompleteRun(context.Context, uuid.UUID, string, datatypes.JSON, time.Time) error
	CompleteRunWithDelivery(context.Context, uuid.UUID, string, datatypes.JSON, time.Time) error
	FailRun(context.Context, uuid.UUID, string, string, time.Time) error
	RequestRunCancel(context.Context, Scope, uuid.UUID, uuid.UUID, time.Time) (*Run, error)
	ClaimRunOutboxBatch(context.Context, int, time.Time) ([]*RunOutbox, error)
	ConfirmRunOutbox(context.Context, uuid.UUID, time.Time) error
	RetryRunOutbox(context.Context, uuid.UUID, time.Time, string) error
	RequeueStaleRunOutbox(context.Context, time.Time, time.Time) error
	MarkStaleRunsUnknown(context.Context, time.Time, time.Time) error
}

// runWaitingStore is an optional persistence extension so focused RunStore
// fakes do not need to implement API-only waiting semantics.
type runWaitingStore interface {
	MarkRunWaiting(context.Context, uuid.UUID, string, time.Time) error
	ResumeWaitingRun(context.Context, uuid.UUID, time.Time) error
	CancelWaitingRun(context.Context, uuid.UUID, time.Time) error
}

// runtimeInputFollowUpStore is an optional persistence extension used by the
// durable runtime-input continuation path. Keeping it separate avoids widening
// the core run-store contract implemented by focused tests and alternative stores.
type runtimeInputFollowUpStore interface {
	RunByMessageID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*Run, error)
	ChannelInboundEventByID(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) (*channelruntime.InboundEvent, error)
}

func (r *GormRepository) RunByMessageID(ctx context.Context, organizationID, workspaceID, digitalHumanID, messageID uuid.UUID) (*Run, error) {
	if organizationID == uuid.Nil || workspaceID == uuid.Nil || digitalHumanID == uuid.Nil || messageID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var run Run
	err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND message_id = ?",
		organizationID, workspaceID, digitalHumanID, messageID,
	).Order("created_at DESC").Take(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get DigitalHuman run by message: %w", err)
	}
	return &run, nil
}

func (r *GormRepository) CreateRunWithOutbox(ctx context.Context, run *Run, outbox *RunOutbox) error {
	return r.Transaction(ctx, func(tx *GormRepository) error {
		if err := tx.CreateRun(ctx, run); err != nil {
			return err
		}
		return tx.CreateRunOutbox(ctx, outbox)
	})
}

func (r *GormRepository) CreateRun(ctx context.Context, run *Run) error {
	if run == nil || run.ID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(run).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrRunConflict
		}
		return fmt.Errorf("create DigitalHuman run: %w", err)
	}
	return nil
}

func (r *GormRepository) CreateRunOutbox(ctx context.Context, outbox *RunOutbox) error {
	if outbox == nil || outbox.ID == uuid.Nil || outbox.RunID == uuid.Nil {
		return ErrInvalidRequest
	}
	if err := r.db.WithContext(ctx).Create(outbox).Error; err != nil {
		if isUniqueViolation(err) {
			return ErrRunConflict
		}
		return fmt.Errorf("create DigitalHuman run outbox: %w", err)
	}
	return nil
}

func (r *GormRepository) RunByID(ctx context.Context, scope Scope, digitalHumanID, runID uuid.UUID) (*Run, error) {
	if !validScopeBoundary(scope) || digitalHumanID == uuid.Nil || runID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	var run Run
	err := r.db.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?",
		runID, scope.OrganizationID, scope.WorkspaceID, digitalHumanID,
	).Take(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("get DigitalHuman run: %w", err)
	}
	return &run, nil
}

func (r *GormRepository) RunByIdempotency(ctx context.Context, organizationID, digitalHumanID uuid.UUID, key string) (*Run, error) {
	var run Run
	err := r.db.WithContext(ctx).Where(
		"organization_id = ? AND digital_human_id = ? AND idempotency_key = ?", organizationID, digitalHumanID, key,
	).Take(&run).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRunNotFound
		}
		return nil, fmt.Errorf("get idempotent DigitalHuman run: %w", err)
	}
	return &run, nil
}

func (r *GormRepository) ListRuns(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query ListQuery) ([]*Run, int64, error) {
	db := r.db.WithContext(ctx).Model(&Run{}).Where(
		"organization_id = ? AND workspace_id = ? AND digital_human_id = ?", scope.OrganizationID, scope.WorkspaceID, digitalHumanID,
	).Where("surface <> ?", SurfacePreview) // Configuration previews are not work the DigitalHuman did.
	db, err := applyRunRequesterFilter(db, query)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count DigitalHuman runs: %w", err)
	}
	items := make([]*Run, 0, query.PageSize)
	if err := db.Order("created_at DESC, id DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list DigitalHuman runs: %w", err)
	}
	return items, total, nil
}

func (r *GormRepository) ClaimRun(ctx context.Context, runID uuid.UUID, now time.Time) (*Run, bool, error) {
	result := r.db.WithContext(ctx).Model(&Run{}).Where("id = ? AND status = ?", runID, RunStatusQueued).
		Updates(map[string]any{"status": RunStatusRunning, "started_at": now, "updated_at": now})
	if result.Error != nil {
		return nil, false, fmt.Errorf("claim DigitalHuman run: %w", result.Error)
	}
	var run Run
	if err := r.db.WithContext(ctx).Where("id = ?", runID).Take(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrRunNotFound
		}
		return nil, false, fmt.Errorf("load claimed DigitalHuman run: %w", err)
	}
	return &run, result.RowsAffected == 1, nil
}

func (r *GormRepository) LinkRunMessage(ctx context.Context, runID, conversationID, messageID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&Run{}).Where("id = ? AND status IN ?", runID, []RunStatus{RunStatusRunning, RunStatusCanceling}).
		Updates(map[string]any{"conversation_id": conversationID, "message_id": messageID, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("link DigitalHuman run message: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRunConflict
	}
	return r.LinkChannelRunConversation(ctx, runID, conversationID, now)
}

func (r *GormRepository) ResumeWaitingRun(ctx context.Context, runID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&Run{}).
		Where("id = ? AND status = ?", runID, RunStatusWaiting).
		Updates(map[string]any{"status": RunStatusRunning, "waiting_reason": "", "waiting_receipt": nil, "waiting_approval_origin": "", "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("resume DigitalHuman run: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRunConflict
	}
	return nil
}

func (r *GormRepository) CancelWaitingRun(ctx context.Context, runID uuid.UUID, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&Run{}).
		Where("id = ? AND status = ?", runID, RunStatusWaiting).
		Updates(map[string]any{"status": RunStatusCanceled, "waiting_reason": "", "completed_at": now, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("cancel waiting DigitalHuman run: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrRunConflict
	}
	return nil
}

func (r *GormRepository) ListRecoverableRuns(ctx context.Context, updatedBefore time.Time, limit int) ([]*Run, error) {
	if limit <= 0 {
		limit = defaultOutboxBatch
	}
	items := make([]*Run, 0, limit)
	if err := r.db.WithContext(ctx).Where(
		"status IN ? AND message_id IS NOT NULL AND actor_id IS NOT NULL AND updated_at < ?",
		[]RunStatus{RunStatusRunning, RunStatusCanceling}, updatedBefore,
	).Order("updated_at ASC, id ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list recoverable DigitalHuman runs: %w", err)
	}
	return items, nil
}

// ListRunningRunsWithMessage lists running runs of any surface whose turn has
// a message, oldest first, for resuming turns a stopped process left behind.
func (r *GormRepository) ListRunningRunsWithMessage(ctx context.Context, updatedBefore time.Time, limit int) ([]*Run, error) {
	if limit <= 0 {
		limit = defaultOutboxBatch
	}
	items := make([]*Run, 0, limit)
	if err := r.db.WithContext(ctx).Where(
		"status = ? AND message_id IS NOT NULL AND conversation_id IS NOT NULL AND updated_at < ?", RunStatusRunning, updatedBefore,
	).Order("updated_at ASC, id ASC").Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list running DigitalHuman runs: %w", err)
	}
	return items, nil
}

func (r *GormRepository) CompleteRun(ctx context.Context, runID uuid.UUID, summary string, trace datatypes.JSON, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run Run
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).Take(&run).Error; err != nil {
			return err
		}
		status := RunStatusSucceeded
		if run.Status == RunStatusCanceling || run.CancelRequestedAt != nil {
			status = RunStatusCanceled
		} else if run.Status != RunStatusRunning {
			waitingPlatform := run.Status == RunStatusWaiting && run.WaitingReason == runtimemodel.MessageStatusWaitingApproval && run.WaitingApprovalOrigin == platformCLIApprovalOrigin
			if (!waitingPlatform && run.Status != RunStatusSucceeded) || run.WaitingReceipt != nil {
				return ErrRunConflict
			}
			matched, err := runHasPlatformGovernanceMessage(tx, &run, runtimemodel.MessageStatusCompleted)
			if err != nil {
				return err
			}
			if !matched {
				return ErrRunConflict
			}
			if run.Status == RunStatusSucceeded {
				return nil // Repeated approval must not reopen or complete the run again.
			}
		}
		return tx.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{
			"status": status, "result_summary": summary, "execution_trace": trace, "waiting_reason": "", "waiting_receipt": nil, "waiting_approval_origin": "", "completed_at": now, "updated_at": now,
		}).Error
	})
}

func (r *GormRepository) MarkRunWaiting(ctx context.Context, runID uuid.UUID, reason string, now time.Time) error {
	if reason != runtimemodel.MessageStatusWaitingApproval && reason != runtimemodel.MessageStatusWaitingQuestion && reason != runtimemodel.MessageStatusWaitingClientAction && reason != runtimemodel.MessageStatusWaitingBudget {
		return ErrInvalidRequest
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run Run
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", runID, RunStatusRunning).Take(&run).Error; err != nil {
			return ErrRunConflict
		}
		var receipt *uuid.UUID
		approvalOrigin := ""
		if reason == runtimemodel.MessageStatusWaitingApproval {
			var rows []struct{ RouteReceipt uuid.UUID }
			if err := tx.Table("connector_confirmation_continuations").
				Where("origin_version = ? AND origin_surface = ? AND run_id = ? AND digital_human_id = ? AND state = ?", 2, "digital_human", runID.String(), run.DigitalHumanID.String(), "pending").
				Limit(2).Find(&rows).Error; err != nil || len(rows) > 1 {
				return ErrRunConflict
			}
			if len(rows) == 1 {
				receipt = &rows[0].RouteReceipt
			} else {
				matched, err := runHasPlatformGovernanceMessage(tx, &run, runtimemodel.MessageStatusWaitingApproval)
				if err != nil {
					return err
				}
				if matched {
					approvalOrigin = platformCLIApprovalOrigin
				} else {
					workflowMatched, err := runHasWorkflowContinuationMessage(tx, &run, runtimemodel.MessageStatusWaitingApproval)
					if err != nil {
						return err
					}
					if workflowMatched {
						approvalOrigin = workflowApprovalOrigin
					} else {
						taskMatched, err := runHasTaskApprovalMessage(tx, &run, runtimemodel.MessageStatusWaitingApproval)
						if err != nil {
							return err
						}
						if !taskMatched {
							return ErrRunConflict
						}
						approvalOrigin = taskApprovalOrigin
					}
				}
			}
		}
		result := tx.Model(&Run{}).Where("id = ? AND status = ?", runID, RunStatusRunning).
			Updates(map[string]any{"status": RunStatusWaiting, "waiting_reason": reason, "waiting_receipt": receipt, "waiting_approval_origin": approvalOrigin, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrRunConflict
		}
		return nil
	})
}

func (r *GormRepository) FailRun(ctx context.Context, runID uuid.UUID, code, message string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var run Run
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", runID).Take(&run).Error; err != nil {
			return err
		}
		if terminalRunStatus(run.Status) {
			return nil
		}
		status := RunStatusFailed
		if run.Status == RunStatusCanceling || run.CancelRequestedAt != nil {
			status = RunStatusCanceled
		}
		return tx.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{
			"status": status, "error_code": code, "error_message": message, "waiting_reason": "", "waiting_receipt": nil, "waiting_approval_origin": "", "completed_at": now, "updated_at": now,
		}).Error
	})
}

func (r *GormRepository) RequestRunCancel(ctx context.Context, scope Scope, digitalHumanID, runID uuid.UUID, now time.Time) (*Run, error) {
	var result Run
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"id = ? AND organization_id = ? AND workspace_id = ? AND digital_human_id = ?",
			runID, scope.OrganizationID, scope.WorkspaceID, digitalHumanID,
		)
		if err := query.Take(&result).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRunNotFound
			}
			return err
		}
		switch result.Status {
		case RunStatusQueued, RunStatusWaiting:
			result.Status, result.CompletedAt = RunStatusCanceled, &now
		case RunStatusRunning:
			result.Status = RunStatusCanceling
		case RunStatusCanceling, RunStatusCanceled:
			return nil
		default:
			return ErrRunConflict
		}
		result.CancelRequestedAt, result.UpdatedAt = &now, now
		result.WaitingReason, result.WaitingReceipt, result.WaitingApprovalOrigin = "", nil, ""
		return tx.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{
			"status": result.Status, "cancel_requested_at": now, "waiting_reason": "", "waiting_receipt": nil, "waiting_approval_origin": "", "completed_at": result.CompletedAt, "updated_at": now,
		}).Error
	})
	return &result, err
}

func (r *GormRepository) ClaimRunOutboxBatch(ctx context.Context, limit int, now time.Time) ([]*RunOutbox, error) {
	items := make([]*RunOutbox, 0, limit)
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("status = ? AND available_at <= ?", RunOutboxPending, now).
			Order("created_at ASC").Limit(limit).Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			item.Status, item.ClaimedAt, item.UpdatedAt = RunOutboxProcessing, &now, now
			if err := tx.Model(&RunOutbox{}).Where("id = ? AND status = ?", item.ID, RunOutboxPending).Updates(map[string]any{
				"status": RunOutboxProcessing, "claimed_at": now, "attempt_count": gorm.Expr("attempt_count + 1"), "updated_at": now,
			}).Error; err != nil {
				return err
			}
			item.AttemptCount++
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim DigitalHuman run outbox: %w", err)
	}
	return items, nil
}

func (r *GormRepository) ConfirmRunOutbox(ctx context.Context, id uuid.UUID, now time.Time) error {
	return r.db.WithContext(ctx).Model(&RunOutbox{}).Where("id = ? AND status = ?", id, RunOutboxProcessing).
		Updates(map[string]any{"status": RunOutboxConfirmed, "confirmed_at": now, "updated_at": now, "error_message": ""}).Error
}

func (r *GormRepository) RetryRunOutbox(ctx context.Context, id uuid.UUID, availableAt time.Time, message string) error {
	return r.db.WithContext(ctx).Model(&RunOutbox{}).Where("id = ? AND status = ?", id, RunOutboxProcessing).
		Updates(map[string]any{"status": RunOutboxPending, "available_at": availableAt, "claimed_at": nil, "updated_at": time.Now().UTC(), "error_message": message}).Error
}

func (r *GormRepository) RequeueStaleRunOutbox(ctx context.Context, claimedBefore, now time.Time) error {
	return r.db.WithContext(ctx).Model(&RunOutbox{}).
		Where("status = ? AND claimed_at < ?", RunOutboxProcessing, claimedBefore).
		Updates(map[string]any{"status": RunOutboxPending, "available_at": now, "claimed_at": nil, "updated_at": now}).Error
}

func (r *GormRepository) MarkStaleRunsUnknown(ctx context.Context, updatedBefore, now time.Time) error {
	// A streaming message has a live owner: a resumed turn (ADR 0035) can run
	// past the stale window, and if its process stops, the chat runtime's
	// lease sweep moves the message to error within minutes.
	return r.db.WithContext(ctx).Model(&Run{}).
		Where("status IN ? AND updated_at < ?", []RunStatus{RunStatusRunning, RunStatusCanceling}, updatedBefore).
		Where("message_id IS NULL OR NOT EXISTS (?)", r.db.Model(&runtimemodel.Message{}).
			Select("1").Where("chat_runtime_messages.id = digital_human_runs.message_id AND chat_runtime_messages.status IN ?",
			[]string{runtimemodel.MessageStatusCompleted, runtimemodel.MessageStatusStreaming})).
		Updates(map[string]any{
			"status": RunStatusUnknown, "error_code": "digital_human.run.interrupted",
			"error_message": "Execution was interrupted and its external side effects may be unknown.",
			"completed_at":  now, "updated_at": now,
		}).Error
}

// RunHeldByWorkflowApproval returns the interactive run of this workflow run
// that a generic workflow approval holds: one still waiting on it, or one an
// external approval resume already claimed (retaken=true) whose worker may
// have been lost before the conversation settled. A claimed run may still be
// waiting: the message is claimed before the run, so a worker lost between
// the two leaves the run for the retake to start. It returns nil when no run
// is held.
func (r *GormRepository) RunHeldByWorkflowApproval(ctx context.Context, workflowRunID string) (*Run, bool, error) {
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return nil, false, nil
	}
	waiting, err := r.runHeldByWorkflow(ctx, workflowRunID,
		"digital_human_runs.status = ? AND digital_human_runs.waiting_reason = ? AND digital_human_runs.waiting_approval_origin = ? AND chat_runtime_messages.status = ?",
		RunStatusWaiting, runtimemodel.MessageStatusWaitingApproval, workflowApprovalOrigin, runtimemodel.MessageStatusWaitingApproval)
	if err != nil || waiting != nil {
		return waiting, false, err
	}
	claimed, err := r.runHeldByWorkflow(ctx, workflowRunID,
		"(digital_human_runs.status = ? OR (digital_human_runs.status = ? AND digital_human_runs.waiting_approval_origin = ?)) AND chat_runtime_messages.status = ?",
		RunStatusRunning, RunStatusWaiting, workflowApprovalOrigin, runtimemodel.MessageStatusStreaming)
	if err != nil || claimed == nil {
		return nil, false, err
	}
	// Only a continuation an external resume claimed is retaken; one the
	// conversation is running stays with it. The marker is read in Go because
	// JSON booleans compare differently across databases.
	var message runtimemodel.Message
	if err := r.db.WithContext(ctx).Select("id", "metadata").Where("id = ?", *claimed.MessageID).Take(&message).Error; err != nil {
		return nil, false, fmt.Errorf("load DigitalHuman workflow continuation message: %w", err)
	}
	record, _ := message.Metadata["agent_workflow_continuation"].(map[string]interface{})
	if externalResume, _ := record["external_resume"].(bool); !externalResume {
		return nil, false, nil
	}
	return claimed, true, nil
}

func (r *GormRepository) runHeldByWorkflow(ctx context.Context, workflowRunID, condition string, args ...interface{}) (*Run, error) {
	var runs []*Run
	if err := r.db.WithContext(ctx).Model(&Run{}).
		Joins("JOIN chat_runtime_messages ON chat_runtime_messages.id = digital_human_runs.message_id").
		Where("digital_human_runs.conversation_id IS NOT NULL AND chat_runtime_messages.deleted_at IS NULL AND chat_runtime_messages.metadata->'agent_workflow_continuation'->>'workflow_run_id' = ?", workflowRunID).
		Where(condition, args...).
		Limit(2).Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("find DigitalHuman run held by workflow approval: %w", err)
	}
	if len(runs) != 1 {
		// A workflow run belongs to one conversation message; anything else is
		// not a hold this host can resume.
		return nil, nil
	}
	return runs[0], nil
}

func terminalRunStatus(status RunStatus) bool {
	switch status {
	case RunStatusSucceeded, RunStatusFailed, RunStatusCanceled, RunStatusUnknown:
		return true
	default:
		return false
	}
}
