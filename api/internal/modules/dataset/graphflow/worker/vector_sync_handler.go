package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/ratelimit"
)

// getVectorSyncBatchSize reads the batch size from the loaded .env configuration.
func getVectorSyncBatchSize(defaultVal int) int {
	if n := config.Current().GraphFlow.VectorSyncBatchSize; n > 0 {
		return n
	}
	return defaultVal
}

// getVectorSyncConcurrency reads the concurrency from the loaded .env configuration.
func getVectorSyncConcurrency(defaultVal int) int {
	if n := config.Current().GraphFlow.VectorSyncConcurrency; n > 0 {
		return n
	}
	return defaultVal
}

// NewVectorSyncHandler creates a handler for entity vector embedding sync tasks
func NewVectorSyncHandler(svc *graphflow.Service, taskManager *queue.TaskManager, limiter *ratelimit.KBLimiter) func(context.Context, *asynq.Task) error {
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

		// Read tunable parameters from centralized configuration.
		batchSize := getVectorSyncBatchSize(50)
		concurrency := getVectorSyncConcurrency(10)

		logger.Info("Starting GraphFlow vector sync", map[string]interface{}{
			"task_id":     taskID.String(),
			"batch_size":  batchSize,
			"concurrency": concurrency,
		})

		// Panic recovery
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("vector sync worker panicked: %v", r)
				logger.Error("Vector sync panic recovery", err)
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, err.Error())
			}
		}()

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
		tenantIDStr := graphFlowTask.TenantID.String()

		// Check if LLM client is available for embeddings
		if svc.GetLLMClient() == nil {
			logger.Warn("LLM client not configured, skipping vector sync", nil)
			svc.TaskRepo.UpdateTaskCompleted(ctx, taskID)
			checkAndUpdateDocumentStatus(ctx, svc, graphFlowTask)
			return nil
		}

		// Get pending entities for vector sync
		pendingEntities, err := svc.EntityRepo.FindPendingVectorSync(ctx, kbID)
		if err != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to get pending entities: %v", err))
			return fmt.Errorf("failed to get pending entities: %v: %w", err, asynq.SkipRetry)
		}

		if len(pendingEntities) == 0 {
			logger.Info("No pending entities for vector sync", map[string]interface{}{
				"kb_id": kbID.String(),
			})
			svc.TaskRepo.UpdateTaskCompleted(ctx, taskID)
			checkAndUpdateDocumentStatus(ctx, svc, graphFlowTask)
			return nil
		}

		logger.Info("Processing pending entities for vector sync", map[string]interface{}{
			"kb_id":       kbID.String(),
			"total":       len(pendingEntities),
			"batch_size":  batchSize,
			"concurrency": concurrency,
		})

		// ── Concurrent batch processing ──
		var (
			entitiesSynced int32
			entitiesFailed int32
			firstErr       error // capture the first fatal error
			errMu          sync.Mutex
			wg             sync.WaitGroup
			semaphore      = make(chan struct{}, concurrency)
		)

		for i := 0; i < len(pendingEntities); i += batchSize {
			end := i + batchSize
			if end > len(pendingEntities) {
				end = len(pendingEntities)
			}
			batch := pendingEntities[i:end]

			wg.Add(1)
			semaphore <- struct{}{} // block if concurrency limit reached

			go func(batch []*model.Entity) {
				defer wg.Done()
				defer func() { <-semaphore }()

				if err := processVectorSyncBatch(ctx, svc, taskID, tenantIDStr, batch, &entitiesSynced, &entitiesFailed); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
				}
			}(batch)
		}

		wg.Wait()

		// If a fatal error occurred (e.g. Neo4j batch update failure), propagate it
		if firstErr != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("vector sync failed: %v", firstErr))
			return fmt.Errorf("vector sync failed: %w", firstErr)
		}

		// Update current task status to completed
		if err := svc.TaskRepo.UpdateTaskCompleted(ctx, taskID); err != nil {
			logger.Error("Failed to update task status to completed", err)
		}

		// Check if both sync tasks are completed and update document status
		checkAndUpdateDocumentStatus(ctx, svc, graphFlowTask)

		logger.Info("GraphFlow vector sync completed", map[string]interface{}{
			"kb_id":           kbID.String(),
			"entities_synced": atomic.LoadInt32(&entitiesSynced),
			"entities_failed": atomic.LoadInt32(&entitiesFailed),
		})

		// Preserve taskManager for potential future use (chained tasks)
		_ = taskManager

		return nil
	}
}

// processVectorSyncBatch handles embedding generation and DB updates for a single batch.
// It is safe to call concurrently from multiple goroutines.
func processVectorSyncBatch(
	ctx context.Context,
	svc *graphflow.Service,
	taskID uuid.UUID,
	tenantIDStr string,
	batch []*model.Entity,
	entitiesSynced *int32,
	entitiesFailed *int32,
) error {
	// Prepare texts for embedding
	texts := make([]string, len(batch))
	for j, entity := range batch {
		texts[j] = fmt.Sprintf("%s (%s)", entity.Name, entity.Type)
		if entity.Description != "" {
			texts[j] = fmt.Sprintf("%s: %s", texts[j], entity.Description)
		}
	}

	// Resolve embedding service for this dataset
	embedSvc, err := svc.GetEmbeddingService(ctx, batch[0].KBID.String())
	if err != nil {
		logger.Error("Failed to resolve embedding service for vector sync", err)
		for _, entity := range batch {
			svc.EntityRepo.UpdateVectorState(ctx, entity.ID, "failed", "", "embedding service resolution failed")
		}
		atomic.AddInt32(entitiesFailed, int32(len(batch)))
		return nil
	}

	// Generate embeddings using the appropriate service
	vecs, err := embedSvc.EmbedTexts(ctx, texts)
	if err != nil {
		logger.Error("Failed to generate embeddings via embedding service", err)
		// Mark entities as failed
		for _, entity := range batch {
			svc.EntityRepo.UpdateVectorState(ctx, entity.ID, "failed", "", "embedding generation failed")
		}
		atomic.AddInt32(entitiesFailed, int32(len(batch)))
		return nil // non-fatal: skip this batch and continue
	}

	// Prepare batch updates for Neo4j and PG
	neo4jUpdates := make([]map[string]interface{}, 0, len(batch))
	syncedIDs := make([]uuid.UUID, 0, len(batch))

	for j, entity := range batch {
		if j < len(vecs) {
			v := vecs[j]
			embedding32 := make([]float32, len(v))
			for k, val := range v {
				embedding32[k] = float32(val)
			}

			neo4jUpdates = append(neo4jUpdates, map[string]interface{}{
				"id":        entity.ID.String(),
				"embedding": embedding32,
			})
			syncedIDs = append(syncedIDs, entity.ID)
		}
	}

	// Batch update Neo4j if client configured
	if svc.Neo4jClient != nil && len(neo4jUpdates) > 0 {
		if err := svc.Neo4jClient.UpdateNodeEmbeddingsBatch(ctx, neo4jUpdates); err != nil {
			logger.Error("Failed to batch update Neo4j node embeddings", err)
			return fmt.Errorf("failed to update Neo4j embeddings: %w", err) // fatal for this batch
		}
	}

	// Batch update PG vector state
	if len(syncedIDs) > 0 {
		if err := svc.EntityRepo.UpdateVectorStateBatch(ctx, syncedIDs, "synced"); err != nil {
			logger.Error("Failed to batch update entity vector state in PG", err)
			atomic.AddInt32(entitiesFailed, int32(len(syncedIDs)))
		} else {
			atomic.AddInt32(entitiesSynced, int32(len(syncedIDs)))
		}
	}

	return nil
}
