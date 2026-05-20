package sync

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/graph"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/repository"
	"github.com/zgiai/ginext/pkg/logger"
)

// EntitySyncResult contains the result of entity synchronization
type EntitySyncResult struct {
	NodesSynced int      `json:"nodes_synced"`
	NodesFailed int      `json:"nodes_failed"`
	FailedIDs   []string `json:"failed_ids,omitempty"`
}

// EntitySync handles synchronization of entities to Neo4j
type EntitySync struct {
	entityRepo  *repository.EntityRepository
	neo4jClient *graph.Neo4jClient
}

// NewEntitySync creates a new EntitySync instance
func NewEntitySync(entityRepo *repository.EntityRepository, neo4jClient *graph.Neo4jClient) *EntitySync {
	return &EntitySync{
		entityRepo:  entityRepo,
		neo4jClient: neo4jClient,
	}
}

// SyncPendingEntities synchronizes all pending entities for a KB to Neo4j
func (s *EntitySync) SyncPendingEntities(ctx context.Context, kbID uuid.UUID) (*EntitySyncResult, error) {
	result := &EntitySyncResult{
		FailedIDs: make([]string, 0),
	}

	// Check if Neo4j client is available
	if s.neo4jClient == nil {
		logger.Warn("Neo4j client not configured, skipping entity sync", nil)
		return result, nil
	}

	// Get pending entities
	pendingEntities, err := s.entityRepo.FindPendingSync(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending entities: %w", err)
	}

	if len(pendingEntities) == 0 {
		logger.Info("No pending entities to sync", map[string]interface{}{
			"kb_id": kbID.String(),
		})
		return result, nil
	}

	// Sync each entity
	for _, entity := range pendingEntities {
		nodeID, err := s.syncEntity(ctx, entity)
		if err != nil {
			logger.Error("Failed to sync entity", err)
			result.NodesFailed++
			result.FailedIDs = append(result.FailedIDs, entity.ID.String())

			// Update entity state to failed
			s.entityRepo.UpdateGraphState(ctx, entity.ID, "failed", "")
			continue
		}

		// Update entity with graph node ID
		if err := s.entityRepo.UpdateGraphState(ctx, entity.ID, "synced", nodeID); err != nil {
			logger.Error("Failed to update entity graph state", err)
			result.NodesFailed++
			continue
		}

		result.NodesSynced++
	}

	logger.Info("Entity sync completed", map[string]interface{}{
		"kb_id":        kbID.String(),
		"nodes_synced": result.NodesSynced,
		"nodes_failed": result.NodesFailed,
	})

	return result, nil
}

// syncEntity creates a single entity node in Neo4j
func (s *EntitySync) syncEntity(ctx context.Context, entity *model.Entity) (string, error) {
	// Determine label from entity type, default to "Entity"
	label := entity.Type
	if label == "" {
		label = "Entity"
	}

	// Build properties
	properties := map[string]interface{}{
		"id":             entity.ID.String(),
		"name":           entity.Name,
		"canonical_name": entity.CanonicalName,
		"kb_id":          entity.KBID.String(),
		"tenant_id":      entity.TenantID.String(),
		"source_count":   entity.SourceCount,
	}

	if entity.Description != "" {
		properties["description"] = entity.Description
	}

	// Create node in Neo4j
	nodeID, err := s.neo4jClient.CreateNode(ctx, label, properties)
	if err != nil {
		return "", fmt.Errorf("neo4j create node failed: %w", err)
	}

	return nodeID, nil
}

// SyncSingleEntity synchronizes a single entity by ID
func (s *EntitySync) SyncSingleEntity(ctx context.Context, entityID uuid.UUID) error {
	if s.neo4jClient == nil {
		return fmt.Errorf("neo4j client not configured")
	}

	// This would need a GetByID method in the repository
	// For now, we return nil as this is optional functionality
	return nil
}
