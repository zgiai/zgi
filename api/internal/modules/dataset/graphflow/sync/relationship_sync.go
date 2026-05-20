package sync

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/graph"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// RelationshipSyncResult contains the result of relationship synchronization
type RelationshipSyncResult struct {
	EdgesSynced int      `json:"edges_synced"`
	EdgesFailed int      `json:"edges_failed"`
	FailedIDs   []string `json:"failed_ids,omitempty"`
}

// RelationshipSync handles synchronization of relationships to Neo4j
type RelationshipSync struct {
	relationshipRepo *repository.RelationshipRepository
	entityRepo       *repository.EntityRepository
	neo4jClient      *graph.Neo4jClient
}

// NewRelationshipSync creates a new RelationshipSync instance
func NewRelationshipSync(
	relationshipRepo *repository.RelationshipRepository,
	entityRepo *repository.EntityRepository,
	neo4jClient *graph.Neo4jClient,
) *RelationshipSync {
	return &RelationshipSync{
		relationshipRepo: relationshipRepo,
		entityRepo:       entityRepo,
		neo4jClient:      neo4jClient,
	}
}

// SyncPendingRelationships synchronizes all pending relationships for a KB to Neo4j
func (s *RelationshipSync) SyncPendingRelationships(ctx context.Context, kbID uuid.UUID) (*RelationshipSyncResult, error) {
	result := &RelationshipSyncResult{
		FailedIDs: make([]string, 0),
	}

	// Check if Neo4j client is available
	if s.neo4jClient == nil {
		logger.Warn("Neo4j client not configured, skipping relationship sync", nil)
		return result, nil
	}

	// Get pending relationships
	pendingRelationships, err := s.relationshipRepo.FindPendingSync(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending relationships: %w", err)
	}

	if len(pendingRelationships) == 0 {
		logger.Info("No pending relationships to sync", map[string]interface{}{
			"kb_id": kbID.String(),
		})
		return result, nil
	}

	// Group relationships by Type for batch processing
	batches := make(map[string][]map[string]interface{})
	relIDsByType := make(map[string][]uuid.UUID)

	for _, rel := range pendingRelationships {
		// Properties map
		props := map[string]interface{}{
			"weight": rel.Weight,
			"kb_id":  rel.KBID.String(),
		}

		// Item map for Neo4j UNWIND
		item := map[string]interface{}{
			"head_id":    rel.HeadEntityID.String(),
			"tail_id":    rel.TailEntityID.String(),
			"kb_id":      rel.KBID.String(), // Critical for robust MATCH in Neo4j
			"properties": props,
		}

		batches[rel.RelationType] = append(batches[rel.RelationType], item)
		relIDsByType[rel.RelationType] = append(relIDsByType[rel.RelationType], rel.ID)
	}

	// Execute batches
	for relType, batch := range batches {
		logger.Info(fmt.Sprintf("[Batch Sync] Syncing %d relationships of type %s", len(batch), relType), nil)

		err := s.neo4jClient.CreateRelationshipsBatch(ctx, relType, batch)
		ids := relIDsByType[relType]

		if err != nil {
			logger.Error(fmt.Sprintf("Failed to sync batch for type %s", relType), err)
			result.EdgesFailed += len(ids)
			for _, id := range ids {
				result.FailedIDs = append(result.FailedIDs, id.String())
			}
			// Mark batch as failed
			if updateErr := s.relationshipRepo.UpdateGraphStateBatch(ctx, ids, "failed"); updateErr != nil {
				logger.Error("Failed to update batch graph state to failed", updateErr)
			}
		} else {
			result.EdgesSynced += len(ids)
			// Mark batch as synced
			if updateErr := s.relationshipRepo.UpdateGraphStateBatch(ctx, ids, "synced"); updateErr != nil {
				logger.Error("Failed to update batch graph state to synced", updateErr)
			}
		}
	}

	logger.Info("Relationship sync completed", map[string]interface{}{
		"kb_id":        kbID.String(),
		"edges_synced": result.EdgesSynced,
		"edges_failed": result.EdgesFailed,
	})

	return result, nil
}

// syncRelationship creates a single relationship in Neo4j
func (s *RelationshipSync) syncRelationship(ctx context.Context, rel *model.Relationship) error {
	// Build properties for the relationship
	properties := map[string]interface{}{
		"weight": rel.Weight,
		"kb_id":  rel.KBID.String(),
	}

	// Create relationship in Neo4j
	// We use entity IDs as identifiers (stored as properties on nodes)
	err := s.neo4jClient.CreateRelationship(
		ctx,
		rel.HeadEntityID.String(),
		rel.TailEntityID.String(),
		rel.RelationType,
		properties,
	)
	if err != nil {
		return fmt.Errorf("neo4j create relationship failed: %w", err)
	}

	return nil
}

// SyncWithRetry attempts to sync relationships with retry logic
func (s *RelationshipSync) SyncWithRetry(ctx context.Context, kbID uuid.UUID, maxRetries int) (*RelationshipSyncResult, error) {
	var lastResult *RelationshipSyncResult
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		result, err := s.SyncPendingRelationships(ctx, kbID)
		if err != nil {
			lastErr = err
			continue
		}

		lastResult = result

		// If no failures, we're done
		if result.EdgesFailed == 0 {
			return result, nil
		}

		// If there are failures, they might be due to entities not yet synced
		// Wait and retry
		logger.Info("Retrying relationship sync", map[string]interface{}{
			"attempt":      attempt + 1,
			"edges_failed": result.EdgesFailed,
		})
	}

	if lastResult != nil {
		return lastResult, nil
	}

	return nil, lastErr
}
