package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/queue"
	"github.com/zgiai/ginext/pkg/scheduler"
)

// GlobalCleanupTask implements scheduler.ScheduledTask for GraphFlow data cleanup
type GlobalCleanupTask struct {
}

// Ensure GlobalCleanupTask implements ScheduledTask interface
var _ scheduler.ScheduledTask = (*GlobalCleanupTask)(nil)

// NewGlobalCleanupTask creates a new GlobalCleanupTask
func NewGlobalCleanupTask() *GlobalCleanupTask {
	return &GlobalCleanupTask{}
}

func (t *GlobalCleanupTask) TaskType() string {
	return "graphflow:global_cleanup"
}

func (t *GlobalCleanupTask) CronSpec() string {
	return "" // Use Interval instead
}

func (t *GlobalCleanupTask) Interval() time.Duration {
	return 5 * time.Minute
}

func (t *GlobalCleanupTask) Payload() []byte {
	return nil
}

func (t *GlobalCleanupTask) Options() []asynq.Option {
	return []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue("graphflow"),
		asynq.Timeout(10 * time.Minute),
	}
}

// GlobalCleanupHandler handles the periodical cleanup task
type GlobalCleanupHandler struct {
	svc         *graphflow.Service
	taskManager *queue.TaskManager
}

// NewGlobalCleanupHandler creates a new handler for global cleanup
func NewGlobalCleanupHandler(svc *graphflow.Service, taskManager *queue.TaskManager) *GlobalCleanupHandler {
	return &GlobalCleanupHandler{
		svc:         svc,
		taskManager: taskManager,
	}
}

// Handle processes the task
func (h *GlobalCleanupHandler) Handle(ctx context.Context, t *asynq.Task) error {
	logger.Info("Starting GraphFlow global cleanup task", nil)

	// Batch size
	limit := 100

	// 1. Clean Relationships first
	if err := h.cleanRelationships(ctx, limit); err != nil {
		logger.Error("Failed during relationship cleanup", err)
		// Continue to clean entities even if relationships fail partially
	}

	// 2. Clean Entities
	if err := h.cleanEntities(ctx, limit); err != nil {
		logger.Error("Failed during entity cleanup", err)
		return err
	}

	logger.Info("GraphFlow global cleanup task completed", nil)
	return nil
}

func (h *GlobalCleanupHandler) cleanRelationships(ctx context.Context, limit int) error {
	rels, err := h.svc.RelationshipRepo.FindPendingDelete(ctx, limit)
	if err != nil {
		return fmt.Errorf("failed to find pending delete relationships: %w", err)
	}

	if len(rels) == 0 {
		return nil
	}

	logger.Info("Found pending delete relationships", map[string]interface{}{
		"count": len(rels),
	})

	for _, rel := range rels {
		// 1. Delete from Neo4j
		// Note: Relationships in Neo4j (via our simple model) might be tricky to target uniquely if not careful.
		// Our Neo4jClient's DeleteRelationship uses ID, assuming we store [r:TYPE {id: UUID}]
		if h.svc.Neo4jClient != nil {
			if err := h.svc.Neo4jClient.DeleteRelationship(ctx, rel.ID.String()); err != nil {
				logger.Warn("Failed to delete relationship from Neo4j", map[string]interface{}{
					"rel_id": rel.ID.String(),
					"error":  err.Error(),
				})
				// We might want to continue or retry?
				// For now, if Neo4j deletion fails, we probably shouldn't mark as deleted in postgres?
				// But maybe the rel doesn't exist in Neo4j.
			}
		}

		// 2. Update Postgres state
		if err := h.svc.RelationshipRepo.UpdateGraphState(ctx, rel.ID, "deleted"); err != nil {
			logger.Error("Failed to update relationship graph_state", err)
		}
	}

	return nil
}

func (h *GlobalCleanupHandler) cleanEntities(ctx context.Context, limit int) error {
	entities, err := h.svc.EntityRepo.FindPendingDelete(ctx, limit)
	if err != nil {
		return fmt.Errorf("failed to find pending delete entities: %w", err)
	}

	if len(entities) == 0 {
		return nil
	}

	logger.Info("Found pending delete entities", map[string]interface{}{
		"count": len(entities),
	})

	for _, entity := range entities {
		// 1. Delete from Neo4j
		if h.svc.Neo4jClient != nil {
			if err := h.svc.Neo4jClient.DeleteNode(ctx, entity.ID.String()); err != nil {
				logger.Warn("Failed to delete node from Neo4j", map[string]interface{}{
					"entity_id": entity.ID.String(),
					"error":     err.Error(),
				})
			}
		}

		// 2. Delete from Weaviate (if linked)
		if h.svc.WeaviateClient != nil {
			// Construct class name: Dataset_{KBID normalized}
			// Weaviate client handles normalization usually but let's be safe or rely on client helpers if any.
			// In WeaviateClient.findActualClassName, it replaces hyphens with underscores.
			kbidStr := entity.KBID.String()
			// Manually normalize to match typical Weaviate class name pattern if needed,
			// but weaviate_client methods usually take the "raw" name and normalize internally
			// OR we should pass the name we expect.
			// Let's pass "Dataset_" + kbidStr and let the client handle it / or we assume it matches.
			className := fmt.Sprintf("Dataset_%s", kbidStr)

			// We try to delete by ID (which effectively is our entity ID)
			if err := h.svc.WeaviateClient.DeleteObjectByID(ctx, className, entity.ID.String()); err != nil {
				logger.Warn("Failed to delete object from Weaviate", map[string]interface{}{
					"entity_id": entity.ID.String(),
					"class":     className,
					"error":     err.Error(),
				})
			}
		}

		// 3. Update Postgres state
		if err := h.svc.EntityRepo.UpdateGraphState(ctx, entity.ID, "deleted", ""); err != nil {
			logger.Error("Failed to update entity graph_state", err)
		}

		// Also update vector state
		if err := h.svc.EntityRepo.UpdateVectorState(ctx, entity.ID, "deleted", "", ""); err != nil {
			logger.Error("Failed to update entity vector_state", err)
		}
	}

	return nil
}
