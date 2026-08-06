package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"gorm.io/gorm"
)

type evidenceGarbageCollectionPlan struct {
	DeleteRelationship bool
	DeleteEntity       bool
}

func planEvidenceGarbageCollection(remainingEvidence int, remainingRelationships int) evidenceGarbageCollectionPlan {
	deleteRelationship := remainingEvidence == 0
	return evidenceGarbageCollectionPlan{
		DeleteRelationship: deleteRelationship,
		DeleteEntity:       deleteRelationship && remainingRelationships == 0,
	}
}

// NewCleanupHandler creates a handler for cleaning up GraphFlow data when a document is deleted
func NewCleanupHandler(svc *graphflow.Service, taskManager *queue.TaskManager) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		// Parse payload
		var payload GraphFlowCleanupPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
		}

		var taskID uuid.UUID
		var hasTaskID bool
		var err error

		if payload.TaskID != "" {
			taskID, err = uuid.Parse(payload.TaskID)
			if err != nil {
				return fmt.Errorf("failed to parse task_id: %v: %w", err, asynq.SkipRetry)
			}
			hasTaskID = true

			// Update task status to processing
			if err := svc.TaskRepo.UpdateTaskProcessing(ctx, taskID); err != nil {
				logger.Error("Failed to update task status to processing", err)
			}
		}

		// Parse document ID
		if payload.DocumentID == "" {
			if hasTaskID {
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, "invalid document_id in payload")
			}
			return fmt.Errorf("invalid document_id in payload: %w", asynq.SkipRetry)
		}

		documentID, err := uuid.Parse(payload.DocumentID)
		if err != nil {
			if hasTaskID {
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to parse document_id: %v", err))
			}
			return fmt.Errorf("failed to parse document_id: %v: %w", err, asynq.SkipRetry)
		}

		var kbID uuid.UUID
		if payload.KBID != "" {
			kbID, _ = uuid.Parse(payload.KBID)
		}

		logger.Info("Starting GraphFlow cleanup", map[string]interface{}{
			"task_id":     taskID.String(),
			"document_id": documentID.String(),
			"kb_id":       kbID.String(),
		})

		var errors []error

		// 1. Soft delete document segments (20% progress)
		if svc.DocumentRepo != nil {
			if err := svc.DocumentRepo.SoftDeleteSegmentsByDocumentID(ctx, documentID.String()); err != nil {
				logger.Error("Failed to soft delete document segments", err)
				errors = append(errors, err)
			} else {
				logger.Info("Soft deleted document segments", map[string]interface{}{
					"document_id": documentID.String(),
				})
			}
		}

		if hasTaskID {
			svc.TaskRepo.UpdateTaskProgress(ctx, taskID, 20)
		}

		// 2. Remove concrete document evidence and derive garbage collection from remaining evidence.
		if svc.EntityMentionRepo != nil && svc.EntityRepo != nil && svc.RelationshipRepo != nil {
			if err := cleanupDocumentEvidence(ctx, svc.DB, kbID, documentID); err != nil {
				logger.Error("Failed to clean document graph evidence", err)
				errors = append(errors, err)
			}
			if hasTaskID {
				svc.TaskRepo.UpdateTaskProgress(ctx, taskID, 90)
			}
		}

		// Update final performance
		if hasTaskID {
			if len(errors) > 0 {
				errorMsg := fmt.Sprintf("cleanup completed with %d errors: %v", len(errors), errors[0])
				if err := svc.TaskRepo.UpdateTaskFailed(ctx, taskID, errorMsg); err != nil {
					logger.Error("Failed to update task status to failed", err)
				}
			} else {
				if err := svc.TaskRepo.UpdateTaskCompleted(ctx, taskID); err != nil {
					logger.Error("Failed to update task status to completed", err)
				} else if graphFlowTask, loadErr := svc.TaskRepo.GetByID(ctx, taskID); loadErr != nil {
					logger.Error("Failed to reload completed cleanup task", loadErr)
				} else if err := advanceBatchPipelineAfterItemTask(ctx, svc, taskManager, graphFlowTask); err != nil {
					logger.Error("Failed to advance batched graph run after cleanup", err)
					return err
				}
			}
		}

		return nil
	}
}

func cleanupDocumentEvidence(ctx context.Context, db *gorm.DB, kbID uuid.UUID, documentID uuid.UUID) error {
	if db == nil || kbID == uuid.Nil {
		return fmt.Errorf("graph cleanup database and kb_id are required")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		mentionScope := "kb_id = ? AND is_deleted = ? AND (document_id = ? OR (document_id IS NULL AND segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)))"
		if err := tx.Model(&graphmodel.EntityMention{}).
			Where(mentionScope, kbID, false, documentID, documentID).
			Updates(map[string]any{"is_deleted": true, "deleted_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&graphmodel.TripleMention{}).
			Where(mentionScope, kbID, false, documentID, documentID).
			Updates(map[string]any{"is_deleted": true, "deleted_at": now}).Error; err != nil {
			return err
		}
		if err := repository.NewRelationshipRepository(tx).RecalculateSourceCounts(ctx, kbID); err != nil {
			return err
		}
		if err := repository.NewEntityRepository(tx).RecalculateSourceCounts(ctx, kbID); err != nil {
			return err
		}
		if err := tx.Model(&graphmodel.Relationship{}).
			Where("kb_id = ? AND is_deleted = ? AND weight = 0", kbID, false).
			Updates(map[string]any{
				"is_deleted":  true,
				"deleted_at":  now,
				"graph_state": "pending_delete",
			}).Error; err != nil {
			return err
		}
		return tx.Model(&graphmodel.Entity{}).
			Where(`kb_id = ? AND is_deleted = ? AND source_count = 0 AND NOT EXISTS (
				SELECT 1 FROM kb_relationships relationship
				WHERE relationship.kb_id = kb_entities.kb_id
				  AND relationship.is_deleted = false
				  AND (relationship.head_entity_id = kb_entities.id OR relationship.tail_entity_id = kb_entities.id)
			)`, kbID, false).
			Updates(map[string]any{
				"is_deleted":   true,
				"deleted_at":   now,
				"graph_state":  "pending_delete",
				"vector_state": "pending_delete",
			}).Error
	})
}
