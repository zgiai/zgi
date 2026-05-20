package retrieval

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
)

// BoundaryType defines the scope of the graph search.
type BoundaryType int

const (
	// BoundaryTypeGlobal performs a broad search (fallback)
	BoundaryTypeGlobal BoundaryType = iota
	// BoundaryTypeAnchored focuses on a confirmed relationship between multiple entities
	BoundaryTypeAnchored
	// BoundaryTypeSingle focuses on a single entity and its immediate neighborhood
	BoundaryTypeSingle
)

// Boundary represents the detected focus area.
type Boundary struct {
	Type      BoundaryType
	EntityIDs []uuid.UUID
}

// BoundaryDetector identifies the search scope based on query entities and metadata.
type BoundaryDetector struct {
	entityRepo *repository.EntityRepository
	relRepo    *repository.RelationshipRepository
}

// NewBoundaryDetector creates a new BoundaryDetector instance.
func NewBoundaryDetector(entityRepo *repository.EntityRepository, relRepo *repository.RelationshipRepository) *BoundaryDetector {
	return &BoundaryDetector{
		entityRepo: entityRepo,
		relRepo:    relRepo,
	}
}

// Detect analyzes extracted entities and determines the retrieval boundary.
func (bd *BoundaryDetector) Detect(ctx context.Context, kbID uuid.UUID, entityNames []string) (*Boundary, error) {
	if len(entityNames) == 0 {
		return &Boundary{Type: BoundaryTypeGlobal}, nil
	}

	// 1. Resolve names to canonical entities within the same knowledge base
	var entityIDs []uuid.UUID
	idMap := make(map[uuid.UUID]bool)

	for _, name := range entityNames {
		entities, err := bd.entityRepo.FindByNameOrAlias(ctx, kbID, name)
		if err == nil {
			for _, e := range entities {
				if !idMap[e.ID] {
					entityIDs = append(entityIDs, e.ID)
					idMap[e.ID] = true
				}
			}
		}
	}

	if len(entityIDs) == 0 {
		return &Boundary{Type: BoundaryTypeGlobal}, nil
	}

	// 2. Classify as "Anchored" based on semantic intent (multiple valid entities extracted from the query)
	if len(entityIDs) >= 2 {
		// As long as multiple confirmed entities are extracted, the query is treated as multi-dimensional.
		// We no longer require a direct 1-hop relationship in the kb_relationships table.
		return &Boundary{
			Type:      BoundaryTypeAnchored,
			EntityIDs: entityIDs,
		}, nil
	}

	// 3. Fallback to a single or dispersed entity focus
	return &Boundary{
		Type:      BoundaryTypeSingle,
		EntityIDs: entityIDs,
	}, nil
}
