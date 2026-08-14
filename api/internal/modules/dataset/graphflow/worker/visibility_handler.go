package worker

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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
	if payloadRevision, ok := visibilityEventRevision(event.Payload); ok {
		if payloadRevision < revision {
			// A newer visibility event owns the projection. Acknowledging the
			// superseded event avoids repeating a KB-wide projection with stale
			// source counts.
			return nil
		}
		if payloadRevision != revision {
			return graphflow.ErrStaleVisibilityRevision
		}
	}
	if dataset.GraphProjectedVisibilityRevision >= revision {
		return nil
	}
	if err := h.visibility.Recalculate(ctx, event.DatasetID, revision); err != nil {
		return err
	}
	if sourceRefID, ok := visibilityEventSourceRefID(event.Payload); ok {
		return h.service.ProjectVisibilityProjectionForSourceRef(ctx, event.DatasetID, revision, sourceRefID)
	}
	return h.service.ProjectVisibilityProjection(ctx, event.DatasetID, revision)
}

func visibilityEventRevision(payload map[string]any) (int64, bool) {
	value, ok := payload["revision"]
	if !ok {
		return 0, false
	}
	switch revision := value.(type) {
	case int64:
		return revision, true
	case int:
		return int64(revision), true
	case float64:
		return int64(revision), true
	default:
		return 0, false
	}
}

func visibilityEventSourceRefID(payload map[string]any) (uuid.UUID, bool) {
	value, ok := payload["source_ref_id"].(string)
	if !ok {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(value)
	return id, err == nil && id != uuid.Nil
}
