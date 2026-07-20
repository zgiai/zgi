package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const workflowConversationBusyCode = "workflow_conversation_busy"

// WorkflowConversationBusyError is returned when a conversational workflow
// already owns the conversation's active-run slot.  It is intentionally
// structured so HTTP/SSE callers can observe the existing run instead of
// presenting a generic network failure.
type WorkflowConversationBusyError struct {
	ConversationID string
	WorkflowRunID  string
	RuntimeStatus  string
}

func (e *WorkflowConversationBusyError) Error() string {
	return fmt.Sprintf("conversation %s already has an active workflow run", e.ConversationID)
}

func workflowConversationIDFromRun(run *WorkflowRunLog) string {
	if run == nil {
		return ""
	}
	if run.ConversationID != nil && strings.TrimSpace(*run.ConversationID) != "" {
		return strings.TrimSpace(*run.ConversationID)
	}
	if run.Inputs == nil || strings.TrimSpace(*run.Inputs) == "" {
		return ""
	}
	var inputs map[string]interface{}
	if err := json.Unmarshal([]byte(*run.Inputs), &inputs); err != nil {
		return ""
	}
	conversationID, _ := inputs["sys.conversation_id"].(string)
	return strings.TrimSpace(conversationID)
}

func isConversationWorkflowRun(run *WorkflowRunLog) bool {
	if run == nil {
		return false
	}
	if run.Type == dto.WorkflowTypeChat {
		return true
	}
	if run.Inputs == nil {
		return false
	}
	var inputs map[string]interface{}
	if err := json.Unmarshal([]byte(*run.Inputs), &inputs); err != nil {
		return false
	}
	workflowType, _ := inputs["sys.workflow_type"].(string)
	return strings.EqualFold(strings.TrimSpace(workflowType), string(dto.WorkflowTypeChat))
}

// createWorkflowRunLogWithConversationClaim creates the run and claims the
// conversation in one database transaction.  PostgreSQL row locking is the
// cross-tab/cross-device authority; frontend disabled states are only UX.
func (s *WorkflowService) createWorkflowRunLogWithConversationClaim(ctx context.Context, run *WorkflowRunLog) error {
	if s.workflowRunLogRepo == nil {
		return fmt.Errorf("workflow run log repository not initialized")
	}
	if !isConversationWorkflowRun(run) {
		return s.workflowRunLogRepo.Create(ctx, run)
	}

	conversationID := workflowConversationIDFromRun(run)
	if conversationID == "" {
		return s.workflowRunLogRepo.Create(ctx, run)
	}
	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return fmt.Errorf("invalid workflow conversation id: %w", err)
	}
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	run.ConversationID = &conversationID

	db := database.GetDB()
	if db == nil {
		return errors.New("database is not initialized")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conv conversation.AgentConversation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND agent_id = ? AND deleted_at IS NULL", conversationUUID, run.AgentID).
			First(&conv).Error; err != nil {
			return fmt.Errorf("failed to lock workflow conversation: %w", err)
		}

		status := strings.TrimSpace(conv.RuntimeStatus)
		if status == "" {
			status = conversation.ConversationRuntimeIdle
		}
		if conv.ActiveWorkflowRunID != nil || status != conversation.ConversationRuntimeIdle {
			activeRunID := ""
			if conv.ActiveWorkflowRunID != nil {
				activeRunID = conv.ActiveWorkflowRunID.String()
			}
			return &WorkflowConversationBusyError{
				ConversationID: conversationID,
				WorkflowRunID:  activeRunID,
				RuntimeStatus:  status,
			}
		}

		if err := NewWorkflowRunLogRepository(tx).Create(ctx, run); err != nil {
			return fmt.Errorf("failed to create workflow run log: %w", err)
		}
		runUUID, err := uuid.Parse(run.ID)
		if err != nil {
			return fmt.Errorf("invalid workflow run id: %w", err)
		}
		result := tx.Model(&conversation.AgentConversation{}).
			Where("id = ? AND active_workflow_run_id IS NULL AND (runtime_status = ? OR runtime_status = '')", conversationUUID, conversation.ConversationRuntimeIdle).
			Updates(map[string]interface{}{
				"runtime_status":         conversation.ConversationRuntimeRunning,
				"active_workflow_run_id": runUUID,
				"runtime_generation":     gorm.Expr("runtime_generation + 1"),
				"runtime_revision":       gorm.Expr("runtime_revision + 1"),
			})
		if result.Error != nil {
			return fmt.Errorf("failed to claim workflow conversation: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return &WorkflowConversationBusyError{ConversationID: conversationID, RuntimeStatus: status}
		}
		return nil
	})
}

func setWorkflowConversationRuntimeTx(
	tx *gorm.DB,
	conversationID string,
	workflowRunID string,
	status string,
) error {
	if tx == nil || strings.TrimSpace(conversationID) == "" || strings.TrimSpace(workflowRunID) == "" {
		return nil
	}
	result := tx.Model(&conversation.AgentConversation{}).
		Where("id = ? AND active_workflow_run_id = ?", conversationID, workflowRunID).
		Updates(map[string]interface{}{
			"runtime_status":   status,
			"runtime_revision": gorm.Expr("runtime_revision + 1"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return workflowpauseOwnershipError(workflowRunID)
	}
	return nil
}

func releaseWorkflowConversationTx(tx *gorm.DB, run *WorkflowRunLog) error {
	// ConversationID is populated only for runs created after the runtime-slot
	// migration. Historical V2 runs may still carry sys.conversation_id inside
	// Inputs but never claimed the slot, so they must not attempt to release it.
	if tx == nil || run == nil || run.ConversationID == nil || strings.TrimSpace(*run.ConversationID) == "" || run.ID == "" {
		return nil
	}
	conversationID := strings.TrimSpace(*run.ConversationID)
	result := tx.Model(&conversation.AgentConversation{}).
		Where("id = ? AND active_workflow_run_id = ?", conversationID, run.ID).
		Updates(map[string]interface{}{
			"runtime_status":         conversation.ConversationRuntimeIdle,
			"active_workflow_run_id": nil,
			"runtime_revision":       gorm.Expr("runtime_revision + 1"),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return workflowpauseOwnershipError(run.ID)
	}
	return nil
}

// Kept local to avoid importing the pause package into the conversation
// model. Coordinator callers translate this into their existing ownership
// sentinel where required.
func workflowpauseOwnershipError(workflowRunID string) error {
	return fmt.Errorf("workflow execution ownership lost for run %s", workflowRunID)
}
