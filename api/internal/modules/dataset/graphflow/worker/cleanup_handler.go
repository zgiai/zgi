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

type documentProjectionCleanup struct {
	Relationships []*graphmodel.Relationship
	Entities      []*graphmodel.Entity
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
			graphFlowTask, loadErr := svc.TaskRepo.GetByID(ctx, taskID)
			if loadErr != nil {
				return fmt.Errorf("failed to load cleanup task: %v: %w", loadErr, asynq.SkipRetry)
			}
			if graphFlowTask == nil {
				return fmt.Errorf("cleanup task not found: %s: %w", taskID, asynq.SkipRetry)
			}
			if graphFlowTask.Status == "completed" || graphFlowTask.Status == "failed" {
				return nil
			}
			if err := validateActiveRunTask(ctx, svc, graphFlowTask); err != nil {
				return fmt.Errorf("cleanup task belongs to an inactive run: %v: %w", err, asynq.SkipRetry)
			}

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
			cleanup, err := cleanupDocumentEvidence(ctx, svc.DB, kbID, documentID)
			if err != nil {
				logger.Error("Failed to clean document graph evidence", err)
				errors = append(errors, err)
			} else if err := cleanupDocumentProjections(ctx, svc, cleanup); err != nil {
				logger.Error("Failed to clean document graph projections", err)
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
				return fmt.Errorf("%s", errorMsg)
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

func cleanupDocumentEvidence(ctx context.Context, db *gorm.DB, kbID uuid.UUID, documentID uuid.UUID) (*documentProjectionCleanup, error) {
	if db == nil || kbID == uuid.Nil {
		return nil, fmt.Errorf("graph cleanup database and kb_id are required")
	}
	cleanup := &documentProjectionCleanup{}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		mentionScope := "kb_id = ? AND is_deleted = ? AND (document_id = ? OR (document_id IS NULL AND segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)))"
		evidenceScope := "kb_id = ? AND (document_id = ? OR (document_id IS NULL AND segment_id IN (SELECT id FROM document_segments WHERE document_id = ?)))"

		entityIDs := make([]uuid.UUID, 0)
		if err := tx.Model(&graphmodel.EntityMention{}).
			Where(evidenceScope+" AND entity_id IS NOT NULL", kbID, documentID, documentID).
			Distinct("entity_id").
			Pluck("entity_id", &entityIDs).Error; err != nil {
			return err
		}

		relationshipIDs := make([]uuid.UUID, 0)
		if err := tx.Model(&graphmodel.TripleMention{}).
			Where(evidenceScope+" AND relationship_id IS NOT NULL", kbID, documentID, documentID).
			Distinct("relationship_id").
			Pluck("relationship_id", &relationshipIDs).Error; err != nil {
			return err
		}
		for _, column := range []string{"head_entity_id", "tail_entity_id"} {
			var tripleEntityIDs []uuid.UUID
			if err := tx.Model(&graphmodel.TripleMention{}).
				Where(evidenceScope+" AND "+column+" IS NOT NULL", kbID, documentID, documentID).
				Distinct(column).
				Pluck(column, &tripleEntityIDs).Error; err != nil {
				return err
			}
			entityIDs = appendUniqueUUIDs(entityIDs, tripleEntityIDs...)
		}

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
		if len(relationshipIDs) > 0 {
			if err := tx.Model(&graphmodel.Relationship{}).
				Where("kb_id = ? AND id IN ? AND is_deleted = ? AND weight = 0", kbID, relationshipIDs, false).
				Updates(map[string]any{
					"is_deleted":  true,
					"deleted_at":  now,
					"graph_state": "pending_delete",
				}).Error; err != nil {
				return err
			}
		}
		if len(entityIDs) > 0 {
			if err := tx.Model(&graphmodel.Entity{}).
				Where(`kb_id = ? AND id IN ? AND is_deleted = ? AND source_count = 0 AND NOT EXISTS (
				SELECT 1 FROM kb_relationships relationship
				WHERE relationship.kb_id = kb_entities.kb_id
				  AND relationship.is_deleted = false
				  AND (relationship.head_entity_id = kb_entities.id OR relationship.tail_entity_id = kb_entities.id)
			)`, kbID, entityIDs, false).
				Updates(map[string]any{
					"is_deleted":   true,
					"deleted_at":   now,
					"graph_state":  "pending_delete",
					"vector_state": "pending_delete",
				}).Error; err != nil {
				return err
			}
		}

		if len(relationshipIDs) > 0 {
			if err := tx.Where("kb_id = ? AND id IN ? AND graph_state = ?", kbID, relationshipIDs, "pending_delete").
				Find(&cleanup.Relationships).Error; err != nil {
				return err
			}
		}
		if len(entityIDs) > 0 {
			if err := tx.Where("kb_id = ? AND id IN ? AND graph_state = ?", kbID, entityIDs, "pending_delete").
				Find(&cleanup.Entities).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cleanup, nil
}

func appendUniqueUUIDs(existing []uuid.UUID, values ...uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(existing)+len(values))
	for _, id := range existing {
		seen[id] = struct{}{}
	}
	for _, id := range values {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		existing = append(existing, id)
	}
	return existing
}

func cleanupDocumentProjections(ctx context.Context, svc *graphflow.Service, cleanup *documentProjectionCleanup) error {
	if cleanup == nil {
		return nil
	}
	for _, relationship := range cleanup.Relationships {
		if svc.Neo4jClient != nil {
			if err := svc.Neo4jClient.DeleteRelationship(ctx, relationship.ID.String()); err != nil {
				return fmt.Errorf("delete relationship %s from Neo4j: %w", relationship.ID, err)
			}
		}
		if err := svc.RelationshipRepo.UpdateGraphState(ctx, relationship.ID, "deleted"); err != nil {
			return fmt.Errorf("mark relationship %s projection deleted: %w", relationship.ID, err)
		}
	}

	for _, entity := range cleanup.Entities {
		if svc.Neo4jClient != nil {
			if err := svc.Neo4jClient.DeleteNode(ctx, entity.ID.String()); err != nil {
				return fmt.Errorf("delete entity %s from Neo4j: %w", entity.ID, err)
			}
		}
		if svc.WeaviateClient != nil && entity.EmbeddingID != "" {
			className := fmt.Sprintf("Entity_%s", entity.KBID.String())
			if err := svc.WeaviateClient.DeleteObjectByID(ctx, className, entity.ID.String()); err != nil {
				return fmt.Errorf("delete entity %s from Weaviate: %w", entity.ID, err)
			}
		}
		if err := svc.EntityRepo.UpdateGraphState(ctx, entity.ID, "deleted", ""); err != nil {
			return fmt.Errorf("mark entity %s graph projection deleted: %w", entity.ID, err)
		}
		if err := svc.EntityRepo.UpdateVectorState(ctx, entity.ID, "deleted", "", ""); err != nil {
			return fmt.Errorf("mark entity %s vector projection deleted: %w", entity.ID, err)
		}
	}
	return nil
}
