package worker

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

type VisibilityHandler struct {
	service    *graphflow.Service
	visibility *graphflow.VisibilityService
}

func NewVisibilityHandler(service *graphflow.Service) *VisibilityHandler {
	if service == nil {
		return &VisibilityHandler{}
	}
	return &VisibilityHandler{
		service:    service,
		visibility: graphflow.NewVisibilityService(service.DB),
	}
}

func (h *VisibilityHandler) Process(ctx context.Context, event *model.GraphOutboxEvent) error {
	if h == nil || h.service == nil || h.visibility == nil || event == nil {
		return fmt.Errorf("visibility handler is not configured")
	}
	if event.EventType != model.GraphOutboxEventVisibility {
		return fmt.Errorf("unsupported visibility event type")
	}
	var dataset datasetmodel.Dataset
	if err := h.service.DB.WithContext(ctx).First(&dataset, "id = ?", event.DatasetID).Error; err != nil {
		return err
	}
	if dataset.OrganizationID != event.OrganizationID.String() ||
		(event.WorkspaceID != nil && dataset.WorkspaceID != event.WorkspaceID.String()) {
		return graphflow.ErrGraphFlowTenantScopeMismatch
	}
	revision := dataset.GraphVisibilityRevision
	if dataset.GraphProjectedVisibilityRevision >= revision {
		return nil
	}
	if err := h.visibility.Recalculate(ctx, event.DatasetID, revision); err != nil {
		return err
	}
	entities, err := h.service.EntityRepo.FindByKBID(ctx, event.DatasetID)
	if err != nil {
		return err
	}
	relationships, err := h.service.RelationshipRepo.FindByKBID(ctx, event.DatasetID)
	if err != nil {
		return err
	}
	entityUpdates := make([]map[string]interface{}, 0, len(entities))
	for _, entity := range entities {
		entityUpdates = append(entityUpdates, map[string]interface{}{
			"id":                  entity.ID.String(),
			"source_count":        entity.SourceCount,
			"active_source_count": entity.ActiveSourceCount,
			"content_revision":    entity.ContentRevision,
			"visibility_revision": revision,
		})
	}
	relationshipUpdates := make([]map[string]interface{}, 0, len(relationships))
	for _, relationship := range relationships {
		if relationship.IsDeleted {
			continue
		}
		relationshipUpdates = append(relationshipUpdates, map[string]interface{}{
			"id":                  relationship.ID.String(),
			"weight":              relationship.Weight,
			"active_weight":       relationship.ActiveWeight,
			"content_revision":    relationship.ContentRevision,
			"visibility_revision": revision,
		})
	}
	if h.service.Neo4jClient == nil {
		return fmt.Errorf("neo4j client not configured")
	}
	if err := h.service.Neo4jClient.UpdateVisibilityProjection(ctx, event.DatasetID.String(), entityUpdates, relationshipUpdates); err != nil {
		return err
	}
	if h.service.WeaviateClient != nil {
		className := fmt.Sprintf("Entity_%s", event.DatasetID)
		for _, entity := range entities {
			if entity.EmbeddingID == "" {
				continue
			}
			if err := h.service.WeaviateClient.UpdateObjectProperties(ctx, entity.ID.String(), className, map[string]interface{}{
				"source_count":        entity.SourceCount,
				"active_source_count": entity.ActiveSourceCount,
				"content_revision":    entity.ContentRevision,
				"visibility_revision": revision,
			}); err != nil {
				return err
			}
		}
	}
	return h.visibility.ConfirmProjection(ctx, event.DatasetID, revision)
}
