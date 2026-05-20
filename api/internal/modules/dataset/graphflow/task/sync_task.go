package task

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/sync"
	"github.com/zgiai/ginext/pkg/logger"
)

// SyncResult represents the combined result of all sync operations
type SyncResult struct {
	EntityResult       *sync.EntitySyncResult       `json:"entity_result"`
	RelationshipResult *sync.RelationshipSyncResult `json:"relationship_result"`
	VectorResult       *sync.VectorSyncResult       `json:"vector_result,omitempty"`
}

// SyncTask orchestrates the synchronization of entities, relationships, and vectors
type SyncTask struct {
	entitySync       *sync.EntitySync
	relationshipSync *sync.RelationshipSync
	vectorSync       *sync.VectorSync
}

// NewSyncTask creates a new SyncTask instance
func NewSyncTask(
	entitySync *sync.EntitySync,
	relationshipSync *sync.RelationshipSync,
	vectorSync *sync.VectorSync,
) *SyncTask {
	return &SyncTask{
		entitySync:       entitySync,
		relationshipSync: relationshipSync,
		vectorSync:       vectorSync,
	}
}

// Run executes the full synchronization pipeline for a KB
// Order: Entities first (required for relationships), then Relationships, then Vectors
func (t *SyncTask) Run(ctx context.Context, tenantID string, kbID uuid.UUID) (*SyncResult, error) {
	result := &SyncResult{}

	logger.Info("Starting sync task", map[string]interface{}{
		"kb_id": kbID.String(),
	})

	// Step 1: Sync entities to Neo4j
	if t.entitySync != nil {
		entityResult, err := t.entitySync.SyncPendingEntities(ctx, kbID)
		if err != nil {
			logger.Error("Entity sync failed", err)
			return nil, fmt.Errorf("entity sync failed: %w", err)
		}
		result.EntityResult = entityResult

		logger.Info("Entity sync completed", map[string]interface{}{
			"nodes_synced": entityResult.NodesSynced,
			"nodes_failed": entityResult.NodesFailed,
		})
	}

	// Step 2: Sync relationships to Neo4j (after entities are synced)
	if t.relationshipSync != nil {
		relResult, err := t.relationshipSync.SyncPendingRelationships(ctx, kbID)
		if err != nil {
			logger.Error("Relationship sync failed", err)
			return nil, fmt.Errorf("relationship sync failed: %w", err)
		}
		result.RelationshipResult = relResult

		logger.Info("Relationship sync completed", map[string]interface{}{
			"edges_synced": relResult.EdgesSynced,
			"edges_failed": relResult.EdgesFailed,
		})
	}

	// Step 3: Sync vectors (can run in parallel with graph sync in future)
	if t.vectorSync != nil {
		vectorResult, err := t.vectorSync.SyncPendingVectors(ctx, tenantID, kbID)
		if err != nil {
			logger.Error("Vector sync failed", err)
			// Vector sync failure is non-critical, continue
		} else {
			result.VectorResult = vectorResult

			logger.Info("Vector sync completed", map[string]interface{}{
				"entities_synced": vectorResult.EntitiesSynced,
				"entities_failed": vectorResult.EntitiesFailed,
			})
		}
	}

	logger.Info("Sync task completed", map[string]interface{}{
		"kb_id": kbID.String(),
	})

	return result, nil
}

// RunGraphSyncOnly executes only entity and relationship sync (no vectors)
func (t *SyncTask) RunGraphSyncOnly(ctx context.Context, kbID uuid.UUID) (*SyncResult, error) {
	result := &SyncResult{}

	// Step 1: Sync entities
	if t.entitySync != nil {
		entityResult, err := t.entitySync.SyncPendingEntities(ctx, kbID)
		if err != nil {
			return nil, fmt.Errorf("entity sync failed: %w", err)
		}
		result.EntityResult = entityResult
	}

	// Step 2: Sync relationships
	if t.relationshipSync != nil {
		relResult, err := t.relationshipSync.SyncPendingRelationships(ctx, kbID)
		if err != nil {
			return nil, fmt.Errorf("relationship sync failed: %w", err)
		}
		result.RelationshipResult = relResult
	}

	return result, nil
}

// RunVectorSyncOnly executes only vector synchronization
func (t *SyncTask) RunVectorSyncOnly(ctx context.Context, tenantID string, kbID uuid.UUID) (*SyncResult, error) {
	result := &SyncResult{}

	if t.vectorSync != nil {
		vectorResult, err := t.vectorSync.SyncPendingVectors(ctx, tenantID, kbID)
		if err != nil {
			return nil, fmt.Errorf("vector sync failed: %w", err)
		}
		result.VectorResult = vectorResult
	}

	return result, nil
}
