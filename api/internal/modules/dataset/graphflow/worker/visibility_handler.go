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
	return h.service.ProjectVisibilityProjection(ctx, event.DatasetID)
}
