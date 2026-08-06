package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/extractor"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/pkg/queue"
	"gorm.io/gorm"
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
	if run.Status == graphmodel.GraphFlowRunStatusReady ||
		run.Status == graphmodel.GraphFlowRunStatusFailed ||
		run.Status == graphmodel.GraphFlowRunStatusSuperseded ||
		run.Status == graphmodel.GraphFlowRunStatusCancelled {
		return nil
	}
	if run.Status == graphmodel.GraphFlowRunStatusPending {
		// Only the lifecycle promoter may claim a queued run. A stale outbox
		// event must be retried until promotion; acknowledging it could race
		// with promotion and leave the processing run without a dispatch event.
		return fmt.Errorf("graph run %s is still waiting for serial promotion", run.ID)
	}
	if run.Status != graphmodel.GraphFlowRunStatusProcessing {
		return fmt.Errorf("graph run %s is not dispatchable in status %s", run.ID, run.Status)
	}
	if isFullDatasetRebuild(run) {
		if err := h.snapshotFullRebuildItems(ctx, run); err != nil {
			return err
		}
	}
	if usesBatchPipeline(run) {
		return h.enqueueBatchExtractionTasks(ctx, run)
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

func (h *RunOutboxHandler) snapshotFullRebuildItems(ctx context.Context, run *graphmodel.GraphFlowRun) error {
	if !isFullDatasetRebuild(run) {
		return nil
	}
	return h.service.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existingItems int64
		if err := tx.WithContext(ctx).Model(&graphmodel.GraphFlowRunItem{}).
			Where("run_id = ?", run.ID).Count(&existingItems).Error; err != nil {
			return err
		}
		// Once the snapshot exists, retries must reuse it instead of pulling in
		// documents that belong to a later graph revision.
		if existingItems > 0 {
			return nil
		}

		var refs []model.KnowledgeBaseAssetRef
		query := tx.WithContext(ctx).
			Where("organization_id = ? AND dataset_id = ? AND dataset_document_id IS NOT NULL AND deleted_at IS NULL", run.OrganizationID, run.DatasetID).
			Order("created_at ASC")
		if run.WorkspaceID != nil {
			query = query.Where("workspace_id = ?", run.WorkspaceID.String())
		}
		if err := query.Find(&refs).Error; err != nil {
			return err
		}
		if len(refs) == 0 {
			return fmt.Errorf("full graph rebuild %s has no current documents", run.ID)
		}

		refIDs := make([]uuid.UUID, 0, len(refs))
		for i := range refs {
			ref := &refs[i]
			item := &graphmodel.GraphFlowRunItem{
				RunID:             run.ID,
				OrganizationID:    run.OrganizationID,
				DatasetID:         run.DatasetID,
				SourceRefID:       &ref.ID,
				SyncRunID:         ref.SyncRunID,
				SyncBatchID:       run.ID,
				Operation:         graphmodel.GraphFlowRunItemOperationAdd,
				DocumentID:        *ref.DatasetDocumentID,
				AssetGenerationNo: ref.SyncedGenerationNo,
			}
			if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(item).Error; err != nil {
				return err
			}
			refIDs = append(refIDs, ref.ID)
		}
		return tx.WithContext(ctx).Model(&model.KnowledgeBaseAssetRef{}).
			Where("id IN ?", refIDs).
			Updates(map[string]any{
				"graph_run_id":      run.ID,
				"graph_sync_status": "queued",
				"updated_at":        time.Now().UTC(),
			}).Error
	})
}

func (h *RunOutboxHandler) enqueueBatchExtractionTasks(ctx context.Context, run *graphmodel.GraphFlowRun) error {
	var items []graphmodel.GraphFlowRunItem
	if err := h.service.DB.WithContext(ctx).
		Where("run_id = ? AND operation = ?", run.ID, graphmodel.GraphFlowRunItemOperationAdd).
		Order("created_at ASC").Find(&items).Error; err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("batched graph run %s has no add items", run.ID)
	}
	for i := range items {
		item := &items[i]
		task, err := h.createRunItemTask(ctx, run, item, "extraction")
		if err != nil {
			return err
		}
		queued, err := CreateGraphFlowExtractionTask(task.ID.String(), h.taskManager)
		if err != nil {
			return err
		}
		if _, err = h.taskManager.EnqueueTask(queued, asynq.Queue("graphflow")); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
			return err
		}
	}
	return nil
}

func (h *RunOutboxHandler) createRunItemTask(
	ctx context.Context,
	run *graphmodel.GraphFlowRun,
	item *graphmodel.GraphFlowRunItem,
	taskType string,
) (*graphmodel.GraphFlowTask, error) {
	if item == nil {
		return nil, fmt.Errorf("graph run item is required")
	}
	task := &graphmodel.GraphFlowTask{
		ID:                 uuid.New(),
		TenantID:           run.OrganizationID,
		KBID:               run.DatasetID,
		DocumentID:         item.DocumentID,
		RunID:              &run.ID,
		RunItemID:          &item.ID,
		SourceRefID:        item.SourceRefID,
		TaskType:           taskType,
		ExtractionStrategy: extractor.StrategyLLM,
		Status:             "pending",
		Metadata: map[string]interface{}{
			"graph_revision": run.GraphRevision,
		},
	}
	result := h.service.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(task)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		if err := h.service.DB.WithContext(ctx).
			Where("run_id = ? AND document_id = ? AND task_type = ?", run.ID, item.DocumentID, taskType).
			First(task).Error; err != nil {
			return nil, err
		}
	}
	return task, nil
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
	resumed, err := h.enqueuePendingDocumentTasks(ctx, run, documentID)
	if err != nil {
		return err
	}
	if resumed {
		return nil
	}

	taskType := "extraction"
	if run.Mode == graphmodel.GraphFlowRunModeCleanup {
		taskType = "cleanup"
	}
	task, err := h.createDocumentTask(ctx, run, documentID, taskType)
	if err != nil {
		return err
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

func (h *RunOutboxHandler) enqueuePendingDocumentTasks(ctx context.Context, run *graphmodel.GraphFlowRun, documentID uuid.UUID) (bool, error) {
	var tasks []graphmodel.GraphFlowTask
	if err := h.service.DB.WithContext(ctx).
		Where("run_id = ? AND document_id = ?", run.ID, documentID).
		Find(&tasks).Error; err != nil {
		return false, err
	}
	if len(tasks) == 0 {
		return false, nil
	}

	enqueue := func(task graphmodel.GraphFlowTask, queueType string) error {
		queued, err := NewGraphFlowTask(queueType, task.ID.String(), h.taskManager)
		if err != nil {
			return err
		}
		_, err = h.taskManager.EnqueueTask(queued, asynq.Queue("graphflow"))
		return err
	}

	if task, queueType, ok := nextPendingDocumentTask(tasks); ok {
		return true, enqueue(task, queueType)
	}
	return false, nil
}

func nextPendingDocumentTask(tasks []graphmodel.GraphFlowTask) (graphmodel.GraphFlowTask, string, bool) {
	byType := make(map[string]graphmodel.GraphFlowTask, len(tasks))
	for _, task := range tasks {
		byType[task.TaskType] = task
	}
	for _, stage := range []struct {
		taskType  string
		queueType string
	}{
		{taskType: "extraction", queueType: TypeGraphFlowExtraction},
		{taskType: "alignment", queueType: TypeGraphFlowAlignment},
		{taskType: "graph_sync", queueType: TypeGraphFlowSync},
		{taskType: "vector_sync", queueType: TypeGraphFlowVectorSync},
	} {
		if task, ok := byType[stage.taskType]; ok && task.Status != "completed" {
			return task, stage.queueType, true
		}
	}
	return graphmodel.GraphFlowTask{}, "", false
}

func (h *RunOutboxHandler) createDocumentTask(
	ctx context.Context,
	run *graphmodel.GraphFlowRun,
	documentID uuid.UUID,
	taskType string,
) (*graphmodel.GraphFlowTask, error) {
	if run.Mode != graphmodel.GraphFlowRunModeCleanup {
		var count int64
		if err := h.service.DB.WithContext(ctx).
			Table("documents").
			Where("id = ? AND dataset_id = ? AND organization_id = ?", documentID, run.DatasetID, run.OrganizationID).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, fmt.Errorf("%w: document %s", graphflow.ErrStaleDocumentSnapshot, documentID)
		}
	}
	task := &graphmodel.GraphFlowTask{
		ID:                 uuid.New(),
		TenantID:           run.OrganizationID,
		KBID:               run.DatasetID,
		DocumentID:         documentID,
		RunID:              &run.ID,
		TaskType:           taskType,
		ExtractionStrategy: extractor.StrategyLLM,
		Status:             "pending",
		Metadata: map[string]interface{}{
			"graph_revision": run.GraphRevision,
		},
	}
	result := h.service.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(task)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		if err := h.service.DB.WithContext(ctx).
			Where("run_id = ? AND document_id = ? AND task_type = ?", run.ID, documentID, taskType).
			First(task).Error; err != nil {
			return nil, err
		}
	}
	return task, nil
}

func (h *RunOutboxHandler) HandleTerminalFailure(ctx context.Context, event *graphmodel.GraphOutboxEvent, _ error) error {
	if h == nil || h.service == nil || h.service.Lifecycle == nil || event == nil || event.RunID == nil {
		return nil
	}
	return h.service.Lifecycle.FailRun(
		ctx,
		*event.RunID,
		"graph_outbox_failed",
		"Graph task could not be scheduled after repeated attempts.",
	)
}
