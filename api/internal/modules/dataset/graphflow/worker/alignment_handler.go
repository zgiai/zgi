package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/aligner"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	"github.com/zgiai/zgi/api/pkg/lock"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/ratelimit"
	pkgredis "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/gorm"
)

// NewAlignmentHandler creates a handler for alignment tasks
// This handler uses KB-level distributed locking to prevent race conditions
// when multiple alignment tasks for the same KB run concurrently
func NewAlignmentHandler(svc *graphflow.Service, taskManager *queue.TaskManager, limiter *ratelimit.KBLimiter) func(context.Context, *asynq.Task) error {
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

		// Panic recovery to ensure task status is updated if the worker crashes
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("alignment worker panicked: %v", r)
				logger.Error("Alignment panic recovery", err)
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, err.Error())
			}
		}()

		logger.Info("Starting GraphFlow alignment", map[string]interface{}{
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

		// Apply Rate Limiting
		kbID := graphFlowTask.KBID
		kbIDStr := kbID.String()

		if allowed, err := limiter.Allow(ctx, kbIDStr); err != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("rate limit check failed: %v", err))
			return fmt.Errorf("rate limit check failed: %w", err)
		} else if !allowed {
			// Return error to trigger Asynq retry (backoff)
			return fmt.Errorf("rate limit exceeded for KB %s, retrying later", kbIDStr)
		}
		defer func() {
			if err := limiter.Release(ctx, kbIDStr); err != nil {
				logger.Error("Failed to release rate limit token", err)
			}
		}()

		tenantID := graphFlowTask.TenantID

		// 2. Acquire KB-level distributed lock to prevent concurrent alignment
		lockKey := lock.GraphFlowLockKey(kbID.String(), lock.LockOpAlignment)
		kbLock := lock.NewRedisLock(pkgredis.GetClient(), lockKey, lock.DefaultLockTTL)

		acquired, err := kbLock.Acquire(ctx)
		if err != nil {
			logger.Error("Failed to acquire alignment lock", err)
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to acquire lock: %v", err))
			return fmt.Errorf("failed to acquire lock: %v", err)
		}
		if !acquired {
			logger.Info("Alignment lock held by another worker, retrying", map[string]interface{}{
				"task_id": taskID.String(),
				"kb_id":   kbID.String(),
			})
			// Return error WITHOUT SkipRetry to trigger Asynq's exponential backoff
			return fmt.Errorf("alignment lock for KB %s is held by another worker", kbID)
		}

		// Ensure lock is released when we're done
		defer func() {
			if err := kbLock.Release(ctx); err != nil {
				logger.Error("Failed to release alignment lock", err)
			}
		}()

		logger.Info("Acquired alignment lock", map[string]interface{}{
			"task_id": taskID.String(),
			"kb_id":   kbID.String(),
		})

		// 3. Update task status to processing
		if err := svc.TaskRepo.UpdateTaskProcessing(ctx, taskID); err != nil {
			logger.Error("Failed to update task status to processing", err)
		}

		// Local cache to reduce DB lookups (persists across batches)
		entityCache := make(map[string]uuid.UUID)
		batchSize := 2000

		// Global stats counters
		entitiesCreated := 0
		mentionsAligned := 0
		relationshipsCreated := 0

		// 4-6. Process pending entity mentions in batches
		for {
			pendingMentions, err := svc.EntityMentionRepo.FindPendingByKBID(ctx, kbID, batchSize)
			if err != nil {
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to get pending mentions: %v", err))
				return fmt.Errorf("failed to get pending mentions: %v: %w", err, asynq.SkipRetry)
			}

			if len(pendingMentions) == 0 {
				break
			}

			// Group mentions by raw_name for alignment
			mentionGroups := make(map[string][]*model.EntityMention)
			for _, mention := range pendingMentions {
				canonicalName := aligner.Canonicalize(mention.RawName)
				mentionGroups[canonicalName] = append(mentionGroups[canonicalName], mention)
			}

			// Process each group
			groupCount := len(mentionGroups)
			currentGroup := 0

			for canonicalName, mentions := range mentionGroups {
				currentGroup++

				// Check context before processing group
				if ctx.Err() != nil {
					svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("context canceled during alignment: %v", ctx.Err()))
					return fmt.Errorf("context canceled during alignment: %w", ctx.Err())
				}

				// Check cache first
				if id, ok := entityCache[canonicalName]; ok {
					// In cache, just update mentions
					mentionIDs := make([]uuid.UUID, len(mentions))
					for i, m := range mentions {
						mentionIDs[i] = m.ID
					}
					// Increment source count? If in cache, we assume it exists.
					// We should increment source count for the *new* mentions of this existing entity.
					svc.EntityRepo.IncrementSourceCount(ctx, id)
					if err := svc.EntityMentionRepo.UpdateBatchStatus(ctx, mentionIDs, "aligned", &id); err != nil {
						logger.Error("Failed to update mentions", err)
						svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to update mentions for cached entity %s: %v", canonicalName, err))
						return fmt.Errorf("failed to update mentions for cached entity %s: %w", canonicalName, err)
					}
					mentionsAligned += len(mentions)
					// Verify context again after DB op
					if ctx.Err() != nil {
						svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("context canceled: %v", ctx.Err()))
						return fmt.Errorf("context canceled: %w", ctx.Err())
					}
				} else {
					// Not in cache, perform transactional update
					var entityID uuid.UUID

					// Wrap in transaction to ensure atomicity
					// Panic safe: Transaction handles panics by rollback
					err := svc.DB.Transaction(func(tx *gorm.DB) error {
						txEntityRepo := repository.NewEntityRepository(tx)
						txMentionRepo := repository.NewEntityMentionRepository(tx)

						existingEntity, err := txEntityRepo.FindByCanonicalName(ctx, kbID, canonicalName)
						if err != nil {
							return fmt.Errorf("failed to find entity: %w", err)
						}

						if existingEntity != nil {
							// Entity exists, increment source count
							entityID = existingEntity.ID
							if err := txEntityRepo.IncrementSourceCount(ctx, entityID); err != nil {
								return fmt.Errorf("failed to increment source count: %w", err)
							}
						} else {
							// Create new entity
							firstMention := mentions[0]
							newEntity := &model.Entity{
								KBID:          kbID,
								TenantID:      tenantID,
								Name:          firstMention.RawName,
								CanonicalName: canonicalName,
								Type:          firstMention.RawType,
								SourceCount:   len(mentions),
								VectorState:   "pending",
								GraphState:    "pending",
							}
							if err := txEntityRepo.Create(ctx, newEntity); err != nil {
								return fmt.Errorf("failed to create entity: %w", err)
							}
							entityID = newEntity.ID
							entitiesCreated++
						}

						// Update all mentions to point to this entity
						mentionIDs := make([]uuid.UUID, len(mentions))
						for i, m := range mentions {
							mentionIDs[i] = m.ID
						}
						if err := txMentionRepo.UpdateBatchStatus(ctx, mentionIDs, "aligned", &entityID); err != nil {
							return fmt.Errorf("failed to update mentions batch: %w", err)
						}

						return nil
					})

					if err != nil {
						logger.Error(fmt.Sprintf("Failed to process entity group %s (transaction rolled back)", canonicalName), err)
						svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("transaction failed for entity %s: %v", canonicalName, err))
						return fmt.Errorf("transaction failed for entity %s: %w", canonicalName, err)
					}

					// Cache the resolved ID
					entityCache[canonicalName] = entityID
					mentionsAligned += len(mentions)
				}

				// Update progress periodically (every 50 groups or last one)
				if currentGroup%50 == 0 || currentGroup == groupCount {
					// Progress calc needs to account for total?
					// We can just keep setting 25%? Or accumulating?
					// Simple hack: toggle 10-40% for mentions phase.
					svc.TaskRepo.UpdateTaskProgress(ctx, taskID, 25)
				}

				// Extend lock TTL periodically
				if currentGroup%100 == 0 {
					if _, err := kbLock.Extend(ctx, lock.DefaultLockTTL); err != nil {
						logger.Warn("Failed to extend lock TTL", map[string]interface{}{
							"task_id": taskID.String(),
							"error":   err.Error(),
						})
					}
				}
			}
		}

		// 7. Process triple mentions in batches
		for {
			pendingTriples, err := svc.TripleMentionRepo.FindPendingByKBID(ctx, kbID, batchSize)
			if err != nil {
				logger.Error("Failed to get pending triples", err)
				break
			}

			if len(pendingTriples) == 0 {
				break
			}

			totalTriples := len(pendingTriples)
			for i, triple := range pendingTriples {
				// Find head and tail entities by canonical name using CACHE first
				headCanonical := aligner.Canonicalize(triple.RawSubject)
				tailCanonical := aligner.Canonicalize(triple.RawObject)

				headID, okHead := entityCache[headCanonical]
				tailID, okTail := entityCache[tailCanonical]

				// fallback to DB if not in cache
				if !okHead {
					ent, _ := svc.EntityRepo.FindByCanonicalName(ctx, kbID, headCanonical)
					if ent != nil {
						headID = ent.ID
						entityCache[headCanonical] = headID // Cache it
						okHead = true
					}
				}
				if !okTail {
					ent, _ := svc.EntityRepo.FindByCanonicalName(ctx, kbID, tailCanonical)
					if ent != nil {
						tailID = ent.ID
						entityCache[tailCanonical] = tailID // Cache it
						okTail = true
					}
				}

				if !okHead || !okTail {
					logger.Warn("Triple header or tail entity not found, skipping triple", map[string]interface{}{
						"triple_id": triple.ID,
						"subject":   triple.RawSubject,
						"object":    triple.RawObject,
						"okHead":    okHead,
						"okTail":    okTail,
					})
					// Mark triple mention status as skipped to avoid infinite loop
					if err := svc.TripleMentionRepo.UpdateStatus(ctx, triple.ID, "skipped", nil, nil); err != nil {
						logger.Error("Failed to update triple status to skipped", err)
					}
					continue
				}

				// Check relationship
				existing, _ := svc.RelationshipRepo.FindExisting(ctx, kbID, headID, tailID, triple.RawPredicate)
				if existing != nil {
					svc.RelationshipRepo.IncrementWeight(ctx, existing.ID)
				} else {
					rel := &model.Relationship{
						KBID:         kbID,
						TenantID:     tenantID,
						HeadEntityID: headID,
						TailEntityID: tailID,
						RelationType: triple.RawPredicate,
						Weight:       1,
						GraphState:   "pending",
					}
					if err := svc.RelationshipRepo.Create(ctx, rel); err != nil {
						logger.Error("Failed to create relationship", err)
						return fmt.Errorf("failed to create relationship for triple %s: %w", triple.ID, err)
					}
					relationshipsCreated++
				}

				// Update triple mention status
				if err := svc.TripleMentionRepo.UpdateStatus(ctx, triple.ID, "aligned", &headID, &tailID); err != nil {
					logger.Error("Failed to update triple status", err)
					return fmt.Errorf("failed to update triple status for %s: %w", triple.ID, err)
				}

				// Update progress
				if (i+1)%50 == 0 || i+1 == totalTriples {
					svc.TaskRepo.UpdateTaskProgress(ctx, taskID, 75)
				}
			}
		}

		// 8. Create and enqueue sync tasks (parallel: graph_sync and vector_sync) - Moved before completion
		if err := enqueueNextSyncTasks(ctx, svc, taskManager, graphFlowTask); err != nil {
			logger.Error("Failed to create sync tasks", err)
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to enqueue sync tasks: %v", err))
			return fmt.Errorf("failed to enqueue sync tasks: %w", err)
		}

		// 9. Update current task status to completed
		if err := svc.TaskRepo.UpdateTaskCompleted(ctx, taskID); err != nil {
			logger.Error("Failed to update task status to completed", err)
		}

		logger.Info("GraphFlow alignment completed", map[string]interface{}{
			"kb_id":                 kbID.String(),
			"entities_created":      entitiesCreated,
			"mentions_aligned":      mentionsAligned,
			"relationships_created": relationshipsCreated,
		})

		return nil
	}
}

// enqueueNextSyncTasks creates both sync tasks (graph_sync and vector_sync) and enqueues them in parallel
func enqueueNextSyncTasks(ctx context.Context, svc *graphflow.Service, taskManager *queue.TaskManager, currentTask *model.GraphFlowTask) error {
	_ = time.Now() // Preserve time import for potential future use

	// Create graph_sync task
	graphSyncTask := &model.GraphFlowTask{
		TenantID:           currentTask.TenantID,
		KBID:               currentTask.KBID,
		DocumentID:         currentTask.DocumentID,
		TaskType:           "graph_sync",
		ExtractionStrategy: currentTask.ExtractionStrategy,
		Status:             "pending",
		Progress:           0,
		Metadata:           currentTask.Metadata,
	}

	graphSyncTaskID, err := svc.TaskRepo.CreateTaskAndReturnID(ctx, graphSyncTask)
	if err != nil {
		return fmt.Errorf("failed to create graph_sync task: %w", err)
	}

	// Create and enqueue graph_sync task using asynq
	task, err := NewGraphFlowTask(TypeGraphFlowSync, graphSyncTaskID.String(), taskManager)
	if err != nil {
		svc.TaskRepo.UpdateTaskFailed(ctx, graphSyncTaskID, fmt.Sprintf("failed to create task: %v", err))
		logger.Error("Failed to create graph_sync task", err)
		return fmt.Errorf("failed to create graph_sync task: %w", err)
	}

	_, err = taskManager.EnqueueTask(task, asynq.Queue("graphflow"))
	if err != nil {
		svc.TaskRepo.UpdateTaskFailed(ctx, graphSyncTaskID, fmt.Sprintf("failed to enqueue: %v", err))
		logger.Error("Failed to enqueue graph_sync task", err)
		return fmt.Errorf("failed to enqueue graph_sync task: %w", err)
	}

	logger.Info("Graph sync task created and enqueued", map[string]interface{}{
		"task_id":     graphSyncTaskID.String(),
		"document_id": currentTask.DocumentID.String(),
	})

	// Create vector_sync task
	vectorSyncTask := &model.GraphFlowTask{
		TenantID:           currentTask.TenantID,
		KBID:               currentTask.KBID,
		DocumentID:         currentTask.DocumentID,
		TaskType:           "vector_sync",
		ExtractionStrategy: currentTask.ExtractionStrategy,
		Status:             "pending",
		Progress:           0,
		Metadata:           currentTask.Metadata,
	}

	vectorSyncTaskID, err := svc.TaskRepo.CreateTaskAndReturnID(ctx, vectorSyncTask)
	if err != nil {
		return fmt.Errorf("failed to create vector_sync task: %w", err)
	}

	// Create and enqueue vector_sync task using asynq
	vectorTask, err := NewGraphFlowTask(TypeGraphFlowVectorSync, vectorSyncTaskID.String(), taskManager)
	if err != nil {
		svc.TaskRepo.UpdateTaskFailed(ctx, vectorSyncTaskID, fmt.Sprintf("failed to create task: %v", err))
		logger.Error("Failed to create vector_sync task", err)
		return fmt.Errorf("failed to create vector_sync task: %w", err)
	}

	_, err = taskManager.EnqueueTask(vectorTask, asynq.Queue("graphflow"))
	if err != nil {
		svc.TaskRepo.UpdateTaskFailed(ctx, vectorSyncTaskID, fmt.Sprintf("failed to enqueue: %v", err))
		logger.Error("Failed to enqueue vector_sync task", err)
		return fmt.Errorf("failed to enqueue vector_sync task: %w", err)
	}

	logger.Info("Vector sync task created and enqueued", map[string]interface{}{
		"task_id":     vectorSyncTaskID.String(),
		"document_id": currentTask.DocumentID.String(),
	})

	return nil
}
