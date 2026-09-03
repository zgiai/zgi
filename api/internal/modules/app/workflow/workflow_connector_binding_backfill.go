package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type workflowConnectorBindingBackfillItem struct {
	WorkflowID uuid.UUID `gorm:"column:workflow_id"`
}

// workflowConnectorBindingBackfillIncompleteError distinguishes malformed
// historical Workflow items from an unavailable backfill mechanism. The
// former remain queued and fail closed without disabling unrelated surfaces;
// schema/query failures must still abort startup so new code never serves
// against a pre-migration binding ledger.
type workflowConnectorBindingBackfillIncompleteError struct {
	count int
	first error
}

func (err *workflowConnectorBindingBackfillIncompleteError) Error() string {
	return fmt.Sprintf("%d workflow connector binding backfill(s) remain pending; first failure: %v", err.count, err.first)
}

func (err *workflowConnectorBindingBackfillIncompleteError) Unwrap() error {
	return err.first
}

func (err *workflowConnectorBindingBackfillIncompleteError) WorkflowConnectorBindingBackfillIncomplete() bool {
	return true
}

// ReconcileWorkflowConnectorBindingBackfills derives Workflow-consumer ledger
// rows for graphs that existed when consumer identities were introduced. The
// migration queue is finite: ordinary saves and publishes maintain the ledger
// transactionally and mark their queued item complete. Completed tombstones
// prevent a rollback/reapply from recreating deliberately revoked bindings.
func (s *WorkflowService) ReconcileWorkflowConnectorBindingBackfills(ctx context.Context) error {
	if s == nil || s.workflowDB == nil || s.workflowBindings == nil {
		return fmt.Errorf("workflow connector binding backfill is not configured")
	}

	startedAt := time.Now()
	failures := make([]error, 0)
	for {
		var (
			done    bool
			itemErr error
		)
		err := s.workflowDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var item workflowConnectorBindingBackfillItem
			result := tx.Raw(`
				SELECT workflow_id
				FROM workflow_connector_binding_backfills
				WHERE completed_at IS NULL
				  AND (last_attempted_at IS NULL OR last_attempted_at < ?)
				ORDER BY created_at ASC, workflow_id ASC
				LIMIT 1
				FOR UPDATE SKIP LOCKED
			`, startedAt).Scan(&item)
			if result.Error != nil {
				return fmt.Errorf("claim workflow connector binding backfill: %w", result.Error)
			}
			if result.RowsAffected == 0 || item.WorkflowID == uuid.Nil {
				done = true
				return nil
			}
			var workflow Workflow
			if err := tx.WithContext(ctx).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", item.WorkflowID).
				Take(&workflow).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					return completeWorkflowConnectorBindingBackfill(ctx, tx, item.WorkflowID, true)
				}
				return fmt.Errorf("load queued workflow %s: %w", item.WorkflowID, err)
			}

			actorID := workflow.CreatedBy
			if workflow.UpdatedBy != nil && strings.TrimSpace(*workflow.UpdatedBy) != "" {
				actorID = *workflow.UpdatedBy
			}
			bindings, err := s.workflowConnectorBindingRows(ctx, &workflow, workflow.GetGraphDict(), actorID)
			if err != nil {
				itemErr = fmt.Errorf("workflow %s: %w", item.WorkflowID, err)
				return failWorkflowConnectorBindingBackfill(ctx, tx, item.WorkflowID, itemErr)
			}
			ref, err := workflowBindingScopeRef(&workflow)
			if err != nil {
				itemErr = fmt.Errorf("workflow %s: %w", item.WorkflowID, err)
				return failWorkflowConnectorBindingBackfill(ctx, tx, item.WorkflowID, itemErr)
			}

			const savepoint = "workflow_connector_binding_backfill_write"
			if err := tx.SavePoint(savepoint).Error; err != nil {
				return fmt.Errorf("create workflow connector binding backfill savepoint: %w", err)
			}
			bindingRepo := agentbindings.NewRepository(tx)
			if ref.Scope == agentbindings.ScopePublished {
				err = bindingRepo.ReplaceWorkflowPublishedHead(ctx, tx, ref, bindings)
			} else {
				err = bindingRepo.ReplaceWorkflowScope(ctx, tx, ref, bindings)
			}
			if err != nil {
				if rollbackErr := tx.RollbackTo(savepoint).Error; rollbackErr != nil {
					return fmt.Errorf("rollback workflow connector binding backfill: %v (original error: %w)", rollbackErr, err)
				}
				// Repository failures are infrastructure failures, not malformed
				// historical Workflow data. Abort startup rather than recording the
				// item as retryable and allowing the lifecycle to swallow the error.
				return fmt.Errorf("replace workflow %s connector bindings: %w", item.WorkflowID, err)
			}
			return completeWorkflowConnectorBindingBackfill(ctx, tx, item.WorkflowID, true)
		})
		if err != nil {
			return err
		}
		if done {
			break
		}
		if itemErr != nil {
			failures = append(failures, itemErr)
		}
	}
	if len(failures) > 0 {
		return &workflowConnectorBindingBackfillIncompleteError{count: len(failures), first: failures[0]}
	}
	return nil
}

func completeWorkflowConnectorBindingBackfill(ctx context.Context, tx *gorm.DB, workflowID uuid.UUID, requireRow bool) error {
	result := tx.WithContext(ctx).Exec(`
		UPDATE workflow_connector_binding_backfills
		SET completed_at = COALESCE(completed_at, CURRENT_TIMESTAMP),
			last_error = NULL,
			updated_at = CURRENT_TIMESTAMP
		WHERE workflow_id = ?
	`, workflowID)
	if result.Error != nil {
		return fmt.Errorf("complete workflow connector binding backfill: %w", result.Error)
	}
	if requireRow && result.RowsAffected != 1 {
		return fmt.Errorf("complete workflow connector binding backfill: queue item not found")
	}
	return nil
}

func failWorkflowConnectorBindingBackfill(ctx context.Context, tx *gorm.DB, workflowID uuid.UUID, cause error) error {
	message := "workflow connector binding backfill failed"
	if cause != nil {
		message = cause.Error()
	}
	runes := []rune(message)
	if len(runes) > 2048 {
		message = string(runes[:2048])
	}
	result := tx.WithContext(ctx).Exec(`
		UPDATE workflow_connector_binding_backfills
		SET attempts = attempts + 1,
			last_error = ?,
			last_attempted_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE workflow_id = ? AND completed_at IS NULL
	`, message, workflowID)
	if result.Error != nil {
		return fmt.Errorf("record workflow connector binding backfill failure: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("record workflow connector binding backfill failure: queue item not found")
	}
	return nil
}
