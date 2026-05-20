package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
)

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

		// 2. Process entity mentions and handle cascade soft deletes
		if svc.EntityMentionRepo != nil && svc.EntityRepo != nil && svc.RelationshipRepo != nil {
			// Find all entity mentions for this document
			mentions, err := svc.EntityMentionRepo.FindByDocumentSegments(ctx, documentID)
			if err != nil {
				logger.Error("Failed to find entity mentions", err)
				errors = append(errors, err)
			} else {
				// Map to track processed entities to avoid race conditions/redundant checks within this task
				// though source count decrement should be atomic in DB.
				// However, if one doc mentions the same entity 10 times, we should decrement 10 times?
				// Requirement says: "Traverse every mention... a. entity.source_count -= 1"
				// So yes, for EACH mention, we decrement.

				totalMentions := len(mentions)
				for i, mention := range mentions {
					if mention.EntityID != nil {
						entityID := *mention.EntityID

						// a. Decrement source count
						if err := svc.EntityRepo.DecrementSourceCount(ctx, entityID); err != nil {
							logger.Error("Failed to decrement entity source count", err)
						} else {
							// b. Check if source count <= 0
							entity, err := svc.EntityRepo.GetByID(ctx, entityID)
							if err != nil {
								logger.Error("Failed to fetching entity to check source count", err)
								continue
							}

							if entity != nil && entity.SourceCount <= 0 {
								// Soft delete entity
								if err := svc.EntityRepo.SoftDelete(ctx, entityID); err != nil {
									logger.Error("Failed to soft delete entity", err)
									errors = append(errors, err)
								} else {
									logger.Info("Soft deleted entity due to zero source count", map[string]interface{}{
										"entity_id": entityID.String(),
									})

									// Soft delete associated relationships
									if err := svc.RelationshipRepo.SoftDeleteByEntityID(ctx, entityID); err != nil {
										logger.Error("Failed to soft delete relationships", err)
										errors = append(errors, err)
									}
								}
							}
						}
					}
					// Update progress incrementally from 20% to 80%
					if hasTaskID && totalMentions > 0 {
						progress := 20 + int(float64(i+1)/float64(totalMentions)*60)
						if progress%10 == 0 { // Update every 10% to avoid too many DB writes
							svc.TaskRepo.UpdateTaskProgress(ctx, taskID, progress)
						}
					}
				}

				// 3. Soft delete mentions (80% -> 90% progress)
				if err := svc.EntityMentionRepo.SoftDeleteByDocumentSegments(ctx, documentID); err != nil {
					logger.Error("Failed to soft delete entity mentions", err)
					errors = append(errors, err)
				}

				if hasTaskID {
					svc.TaskRepo.UpdateTaskProgress(ctx, taskID, 85)
				}

				if err := svc.TripleMentionRepo.SoftDeleteByDocumentSegments(ctx, documentID); err != nil {
					logger.Error("Failed to soft delete triple mentions", err)
					errors = append(errors, err)
				}

				if hasTaskID {
					svc.TaskRepo.UpdateTaskProgress(ctx, taskID, 90)
				}
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
				}
			}
		}

		// Preserve taskManager for potential future use
		_ = taskManager

		return nil
	}
}
