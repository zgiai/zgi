package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/pkg/queue"
	"gorm.io/gorm/clause"
)

type RunOutboxHandler struct {
	service     *graphflow.Service
	taskManager *queue.TaskManager
}

func NewRunOutboxHandler(service *graphflow.Service, taskManager *queue.TaskManager) *RunOutboxHandler {
	return &RunOutboxHandler{service: service, taskManager: taskManager}
}

func (h *RunOutboxHandler) Process(ctx context.Context, event *graphmodel.GraphOutboxEvent) error {
	if h == nil || h.service == nil || h.taskManager == nil || event == nil || event.RunID == nil {
		return fmt.Errorf("graph run outbox handler is not configured")
	}
	run, err := h.service.RunRepo.FindByID(ctx, *event.RunID)
	if err != nil {
		return err
	}
	if run.Status == graphmodel.GraphFlowRunStatusReady || run.Status == graphmodel.GraphFlowRunStatusSuperseded || run.Status == graphmodel.GraphFlowRunStatusCancelled {
		return nil
	}
	if run.Status == graphmodel.GraphFlowRunStatusPending {
		run, err = h.service.RunRepo.Claim(ctx, run.ID, 10*time.Minute)
		if err != nil {
			return err
		}
	}
	documents, err := h.runDocuments(ctx, run)
	if err != nil {
		return err
	}
	for _, documentID := range documents {
		if err := h.enqueueDocumentTask(ctx, run, documentID); err != nil {
			return err
		}
	}
	return nil
}

func (h *RunOutboxHandler) runDocuments(ctx context.Context, run *graphmodel.GraphFlowRun) ([]uuid.UUID, error) {
	if run.DocumentID != nil {
		return []uuid.UUID{*run.DocumentID}, nil
	}
	var refs []model.KnowledgeBaseAssetRef
	query := h.service.DB.WithContext(ctx).
		Where("organization_id = ? AND dataset_id = ? AND dataset_document_id IS NOT NULL AND deleted_at IS NULL", run.OrganizationID, run.DatasetID)
	if run.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", run.WorkspaceID.String())
	}
	if err := query.Find(&refs).Error; err != nil {
		return nil, err
	}
	documents := make([]uuid.UUID, 0, len(refs))
	for _, ref := range refs {
		if ref.DatasetDocumentID != nil {
			documents = append(documents, *ref.DatasetDocumentID)
		}
	}
	return documents, nil
}

func (h *RunOutboxHandler) enqueueDocumentTask(ctx context.Context, run *graphmodel.GraphFlowRun, documentID uuid.UUID) error {
	taskType := "extraction"
	if run.Mode == graphmodel.GraphFlowRunModeCleanup {
		taskType = "cleanup"
	}
	task := &graphmodel.GraphFlowTask{
		ID:         uuid.New(),
		TenantID:   run.OrganizationID,
		KBID:       run.DatasetID,
		DocumentID: documentID,
		RunID:      &run.ID,
		TaskType:   taskType,
		Status:     "pending",
		Metadata: map[string]interface{}{
			"graph_revision": run.GraphRevision,
		},
	}
	result := h.service.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(task)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if err := h.service.DB.WithContext(ctx).
			Where("run_id = ? AND document_id = ? AND task_type = ?", run.ID, documentID, taskType).
			First(task).Error; err != nil {
			return err
		}
	}
	if run.Mode != graphmodel.GraphFlowRunModeCleanup {
		if err := h.service.DB.WithContext(ctx).
			Model(&model.KnowledgeBaseAssetRef{}).
			Where("organization_id = ? AND dataset_id = ? AND dataset_document_id = ? AND deleted_at IS NULL", run.OrganizationID, run.DatasetID, documentID).
			Updates(map[string]any{
				"graph_run_id":      run.ID,
				"graph_sync_status": "queued",
				"updated_at":        time.Now().UTC(),
			}).Error; err != nil {
			return err
		}
	}
	if taskType == "cleanup" {
		queued, err := CreateGraphFlowCleanupTask(task.ID.String(), documentID.String(), run.DatasetID.String(), h.taskManager)
		if err != nil {
			return err
		}
		_, err = h.taskManager.EnqueueTask(queued, asynq.Queue("graphflow"))
		return err
	}
	queued, err := CreateGraphFlowExtractionTask(task.ID.String(), h.taskManager)
	if err != nil {
		return err
	}
	_, err = h.taskManager.EnqueueTask(queued, asynq.Queue("graphflow"))
	return err
}
