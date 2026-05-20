package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/ratelimit"
)

// NewSyncHandler creates a handler for Neo4j sync tasks
func NewSyncHandler(svc *graphflow.Service, taskManager *queue.TaskManager, limiter *ratelimit.KBLimiter) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		// Parse payload
		var payload GraphFlowTaskPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
		}

		taskID, err := uuid.Parse(payload.TaskID)
		if err != nil {
			return fmt.Errorf("failed to parse task_id: %v: %w", err, asynq.SkipRetry)
		}

		// Panic recovery
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("sync worker panicked: %v", r)
				logger.Error("Sync panic recovery", err)
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, err.Error())
			}
		}()

		logger.Info("Starting GraphFlow Neo4j sync", map[string]interface{}{
			"task_id": taskID.String(),
		})

		// 1. Get the GraphFlow task from DB
		graphFlowTask, err := svc.TaskRepo.GetByID(ctx, taskID)
		if err != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to get graphflow task: %v", err))
			return fmt.Errorf("failed to get graphflow task: %v: %w", err, asynq.SkipRetry)
		}
		if graphFlowTask == nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("graphflow task not found: %s", taskID))
			return fmt.Errorf("graphflow task not found: %s: %w", taskID, asynq.SkipRetry)
		}

		// Check for idempotency - skip if already completed or failed
		if graphFlowTask.Status == "completed" || graphFlowTask.Status == "failed" {
			logger.Info("Task already processed, skipping", map[string]interface{}{
				"task_id": taskID.String(),
				"status":  graphFlowTask.Status,
			})
			return nil
		}

		// 2. Update task status to processing
		if err := svc.TaskRepo.UpdateTaskProcessing(ctx, taskID); err != nil {
			logger.Error("Failed to update task status to processing", err)
		}

		// Apply Rate Limiting
		kbID := graphFlowTask.KBID
		kbIDStr := kbID.String()
		if allowed, err := limiter.Allow(ctx, kbIDStr); err != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("rate limit check failed: %v", err))
			return fmt.Errorf("rate limit check failed: %w", err)
		} else if !allowed {
			return fmt.Errorf("rate limit exceeded for KB %s, retrying later", kbIDStr)
		}
		defer func() {
			if err := limiter.Release(ctx, kbIDStr); err != nil {
				logger.Error("Failed to release rate limit token", err)
			}
		}()

		kbID = graphFlowTask.KBID

		// Check if Neo4j client is configured
		if svc.Neo4jClient == nil {
			logger.Warn("Neo4j client not configured, skipping sync", nil)
			svc.TaskRepo.UpdateTaskCompleted(ctx, taskID)
			checkAndUpdateDocumentStatus(ctx, svc, graphFlowTask)
			return nil
		}

		// 3. Sync pending entities in batches
		nodesSynced := 0
		edgesSynced := 0

		pendingEntities, err := svc.EntityRepo.FindPendingSync(ctx, kbID)
		if err != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to get pending entities: %v", err))
			return fmt.Errorf("failed to get pending entities: %v: %w", err, asynq.SkipRetry)
		}

		if len(pendingEntities) > 0 {
			// Initialize embedding service
			embedSvc, _ := svc.GetEmbeddingService(ctx, kbID.String())

			// Batch entities for Neo4j creation
			entityBatchSize := 50
			for i := 0; i < len(pendingEntities); i += entityBatchSize {
				end := i + entityBatchSize
				if end > len(pendingEntities) {
					end = len(pendingEntities)
				}
				batch := pendingEntities[i:end]

				// Prepare Neo4j data
				nodesData := make([]map[string]interface{}, 0, len(batch))
				syncedIDs := make([]uuid.UUID, 0, len(batch))

				// SameAs relationships to creating
				sameAsRels := make([]map[string]interface{}, 0)

				// If embedding service is available, batch embed the entity names
				embeddings := make([][]float32, len(batch))
				if embedSvc != nil {
					texts := make([]string, len(batch))
					for j, e := range batch {
						texts[j] = fmt.Sprintf("%s (%s)", e.Name, e.Type)
						if e.Description != "" {
							texts[j] = fmt.Sprintf("%s: %s", texts[j], e.Description)
						}
					}
					// EmbedTexts uses batch API internally
					vecs, err := embedSvc.EmbedTexts(ctx, texts)
					if err == nil && len(vecs) == len(batch) {
						for j, v := range vecs {
							embeddings[j] = make([]float32, len(v))
							for k, val := range v {
								embeddings[j][k] = float32(val)
							}
						}
					}
				}

				for j, entity := range batch {
					props := map[string]interface{}{
						"id":             entity.ID.String(),
						"name":           entity.Name,
						"canonical_name": entity.CanonicalName,
						"kb_id":          entity.KBID.String(),
						"source_count":   entity.SourceCount,
					}
					if len(embeddings[j]) > 0 {
						props["embedding"] = embeddings[j]

						// Check for similar entity to merge
						// Threshold 0.92 for high confidence
						similarID, err := svc.Neo4jClient.FindSimilarEntity(ctx, entity.KBID.String(), embeddings[j], 0.92)
						if err == nil && similarID != "" {
							logger.Info("Found similar entity, creating SAME_AS link", map[string]interface{}{
								"new_entity":  entity.Name,
								"existing_id": similarID,
							})
							sameAsRels = append(sameAsRels, map[string]interface{}{
								"from": entity.ID.String(),
								"to":   similarID,
							})
						}
					}
					nodesData = append(nodesData, props)
					syncedIDs = append(syncedIDs, entity.ID)
				}

				// Unified Batch Creation in Neo4j
				// Actually, the label in CreateNode was entity.Type.
				// Let's refine the loop to group by type.
				batchTypeGroups := make(map[string][]map[string]interface{})
				for j, entity := range batch {
					batchTypeGroups[entity.Type] = append(batchTypeGroups[entity.Type], nodesData[j])
				}

				for label, labelNodes := range batchTypeGroups {
					if err := svc.Neo4jClient.CreateNodesBatch(ctx, label, labelNodes); err != nil {
						logger.Error(fmt.Sprintf("Failed to batch create Neo4j nodes for label %s", label), err)
						svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to create Neo4j nodes: %v", err))
						return fmt.Errorf("failed to create Neo4j nodes: %w", err)
					}
				}

				// Create SAME_AS relationships
				if len(sameAsRels) > 0 {
					// Use SAME_AS relationship type
					// Since CreateRelationshipsBatch expects a specific format, we can adapt it or call a specific method
					// But our map structure is different ("from", "to"). Let's map it.
					// Or better: Create a direct batch call for SAME_AS using transaction/query in Neo4jClient?
					// For now, let's use the valid format for CreateRelationshipsBatch: "head_id", "tail_id", "properties"

					sameAsBatch := make([]map[string]interface{}, len(sameAsRels))
					for k, r := range sameAsRels {
						sameAsBatch[k] = map[string]interface{}{
							"head_id": r["from"],
							"tail_id": r["to"],
							"properties": map[string]interface{}{
								"weight": 1.0,
								"type":   "semantic_merge",
							},
						}
					}

					if err := svc.Neo4jClient.CreateRelationshipsBatch(ctx, "SAME_AS", sameAsBatch); err != nil {
						logger.Error("Failed to create SAME_AS relationships", err)
					}
				}

				// Update PG in one batch
				if err := svc.EntityRepo.UpdateGraphStateBatch(ctx, syncedIDs, "synced"); err != nil {
					logger.Error("Failed to update entity batch graph state", err)
				}
				nodesSynced += len(syncedIDs)
			}
		}

		// 4. Sync pending relationships in batches
		pendingRelationships, err := svc.RelationshipRepo.FindPendingSync(ctx, kbID)
		if err != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to get pending relationships: %v", err))
			return fmt.Errorf("failed to get pending relationships: %v: %w", err, asynq.SkipRetry)
		}

		if len(pendingRelationships) > 0 {
			relBatchSize := 100
			for i := 0; i < len(pendingRelationships); i += relBatchSize {
				end := i + relBatchSize
				if end > len(pendingRelationships) {
					end = len(pendingRelationships)
				}
				batch := pendingRelationships[i:end]

				// Group by relation type for UNWIND
				typeGroups := make(map[string][]map[string]interface{})
				syncedIDs := make([]uuid.UUID, 0, len(batch))

				for _, rel := range batch {
					typeGroups[rel.RelationType] = append(typeGroups[rel.RelationType], map[string]interface{}{
						"head_id": rel.HeadEntityID.String(),
						"tail_id": rel.TailEntityID.String(),
						"kb_id":   rel.KBID.String(),
						"properties": map[string]interface{}{
							"id":     rel.ID.String(),
							"weight": rel.Weight,
						},
					})
					syncedIDs = append(syncedIDs, rel.ID)
				}

				for rType, relsData := range typeGroups {
					if err := svc.Neo4jClient.CreateRelationshipsBatch(ctx, rType, relsData); err != nil {
						logger.Error(fmt.Sprintf("Failed to batch create Neo4j relationships of type %s", rType), err)
						svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to create Neo4j relationships: %v", err))
						return fmt.Errorf("failed to create Neo4j relationships: %w", err)
					}
				}

				// Update PG in one batch
				if err := svc.RelationshipRepo.UpdateGraphStateBatch(ctx, syncedIDs, "synced"); err != nil {
					logger.Error("Failed to update relationship batch graph state", err)
				}
				edgesSynced += len(syncedIDs)
			}

		}

		// 4.5 Sync Aligned Mentions (creating provenance links)
		// Fetch mentions that are 'aligned' but not yet 'synced'
		// Note: We use a limit to batch processing if there are too many, but here we try to do a reasonable chunk
		// Since we want to clear the queue, we loop until empty or hit a safety limit.
		mentionsSynced := 0
		for {
			pendingMentions, err := svc.EntityMentionRepo.FindAlignedSync(ctx, kbID, 500) // Batch 500
			if err != nil {
				logger.Error("Failed to fetching aligned mentions", err)
				// Don't fail the whole task, just log? Or strictly fail?
				// Strict fail is safer for data consistency.
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to get aligned mentions: %v", err))
				return fmt.Errorf("failed to get aligned mentions: %w", err)
			}

			if len(pendingMentions) == 0 {
				break
			}

			// Prepare Neo4j Batch
			mentionsBatch := make([]map[string]interface{}, len(pendingMentions))
			idsToUpdate := make([]uuid.UUID, len(pendingMentions))

			for i, m := range pendingMentions {
				if m.EntityID == nil {
					continue // Should not happen for 'aligned' status
				}
				mentionsBatch[i] = map[string]interface{}{
					"segment_id": m.SegmentID.String(),
					"entity_id":  m.EntityID.String(),
					"properties": map[string]interface{}{
						"confidence": m.Confidence,
						// "id": m.ID.String(), // relationship ID optional
					},
				}
				idsToUpdate[i] = m.ID
			}

			// Exec Neo4j Sync
			if err := svc.Neo4jClient.CreateMentionsBatch(ctx, mentionsBatch); err != nil {
				logger.Error("Failed to sync mentions batch to Neo4j", err)
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to sync mentions: %v", err))
				return fmt.Errorf("failed to sync mentions: %w", err)
			}

			// Update Status to 'synced'
			if err := svc.EntityMentionRepo.UpdateBatchStatus(ctx, idsToUpdate, "synced", nil); err != nil {
				logger.Error("Failed to update mention status to synced", err)
			}

			mentionsSynced += len(pendingMentions)

			// Safety break if too many? For now just rely on loop.
			if len(pendingMentions) < 500 {
				break
			}
		}

		// 5. Update current task status to completed
		if err := svc.TaskRepo.UpdateTaskCompleted(ctx, taskID); err != nil {
			logger.Error("Failed to update task status to completed", err)
		}

		logger.Info("GraphFlow Neo4j sync completed", map[string]interface{}{
			"kb_id":           kbID.String(),
			"nodes_synced":    nodesSynced,
			"edges_synced":    edgesSynced,
			"mentions_synced": mentionsSynced,
		})

		// 6. Check if both sync tasks are completed and update document status
		checkAndUpdateDocumentStatus(ctx, svc, graphFlowTask)

		// Preserve taskManager for potential future use (chained tasks)
		_ = taskManager

		return nil
	}
}

// checkAndUpdateDocumentStatus checks if both sync tasks are completed and updates document status
func checkAndUpdateDocumentStatus(ctx context.Context, svc *graphflow.Service, currentTask *model.GraphFlowTask) {
	// Check if both graph_sync and vector_sync tasks are completed
	allCompleted, err := svc.TaskRepo.AreTasksCompletedByTypes(ctx, currentTask.DocumentID, []string{"graph_sync", "vector_sync"})
	if err != nil {
		logger.Error("Failed to check task completion status", err)
		return
	}

	if allCompleted {
		// Update document indexing status to completed
		if err := svc.DocumentRepo.UpdateDocumentIndexingStatus(ctx, currentTask.DocumentID.String(), "completed"); err != nil {
			logger.Error("Failed to update document indexing status", err)
			return
		}

		// Update segment graph indexing status to indexed ONLY if it was extracted (to prevent overwriting failed segments)
		if err := svc.DocumentRepo.UpdateSegmentGraphIndexingStatusByDocumentID(ctx, currentTask.DocumentID.String(), "indexed", "extracted"); err != nil {
			logger.Error("Failed to update segment graph indexing status to indexed", err)
		}

		logger.Info("All GraphFlow tasks completed, document marked as completed", map[string]interface{}{
			"document_id": currentTask.DocumentID.String(),
		})
	} else {
		// Get incomplete tasks for debugging
		incompleteTasks, err := svc.TaskRepo.GetIncompleteTasksByTypes(ctx, currentTask.DocumentID, []string{"graph_sync", "vector_sync"})
		if err != nil {
			logger.Error("Failed to get incomplete tasks", err)
		}

		incompleteTaskInfo := make([]map[string]interface{}, 0, len(incompleteTasks))
		for _, task := range incompleteTasks {
			incompleteTaskInfo = append(incompleteTaskInfo, map[string]interface{}{
				"task_id":   task.ID.String(),
				"task_type": task.TaskType,
				"status":    task.Status,
			})
		}

		logger.Info("Not all GraphFlow tasks completed yet", map[string]interface{}{
			"document_id":      currentTask.DocumentID.String(),
			"task_type":        currentTask.TaskType,
			"incomplete_tasks": incompleteTaskInfo,
		})
	}
}
