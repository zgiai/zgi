package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	graphmodel "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/pkg/queue"
)

func isFullDatasetRebuild(run *graphmodel.GraphFlowRun) bool {
	return run != nil && run.Mode == graphmodel.GraphFlowRunModeRebuild && run.DocumentID == nil
}

func usesBatchPipeline(run *graphmodel.GraphFlowRun) bool {
	return run != nil && (run.SyncBatchID != nil || isFullDatasetRebuild(run))
}

func advanceBatchPipelineAfterItemTask(
	ctx context.Context,
	svc *graphflow.Service,
	taskManager *queue.TaskManager,
	currentTask *graphmodel.GraphFlowTask,
) error {
	if svc == nil || currentTask == nil || currentTask.RunID == nil {
		return nil
	}
	run, err := svc.RunRepo.FindByID(ctx, *currentTask.RunID)
	if err != nil {
		return err
	}
	if !usesBatchPipeline(run) {
		return nil
	}

	var items []graphmodel.GraphFlowRunItem
	if err := svc.DB.WithContext(ctx).Where("run_id = ?", run.ID).Order("created_at ASC").Find(&items).Error; err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("batched graph run %s has no run items", run.ID)
	}

	addItems := make([]graphmodel.GraphFlowRunItem, 0, len(items))
	cleanupItems := make([]graphmodel.GraphFlowRunItem, 0, len(items))
	for _, item := range items {
		switch item.Operation {
		case graphmodel.GraphFlowRunItemOperationAdd:
			addItems = append(addItems, item)
		case graphmodel.GraphFlowRunItemOperationCleanup:
			cleanupItems = append(cleanupItems, item)
		}
	}
	if len(addItems) == 0 {
		return fmt.Errorf("batched graph run %s has no add items", run.ID)
	}
	allExtracted, err := allRunItemTasksCompleted(ctx, svc, run.ID, "extraction", len(addItems))
	if err != nil || !allExtracted {
		return err
	}

	for i := range cleanupItems {
		item := &cleanupItems[i]
		task := &graphmodel.GraphFlowTask{
			ID:          uuid.New(),
			TenantID:    run.OrganizationID,
			KBID:        run.DatasetID,
			DocumentID:  item.DocumentID,
			RunID:       &run.ID,
			RunItemID:   &item.ID,
			SourceRefID: item.SourceRefID,
			TaskType:    "cleanup",
			Status:      "pending",
			Metadata: map[string]interface{}{
				"graph_revision": run.GraphRevision,
			},
		}
		taskID, err := svc.TaskRepo.CreateTaskAndReturnID(ctx, task)
		if err != nil {
			return err
		}
		queued, err := CreateGraphFlowCleanupTask(taskID.String(), item.DocumentID.String(), run.DatasetID.String(), taskManager)
		if err != nil {
			return err
		}
		if _, err := taskManager.EnqueueTask(queued, asynq.Queue("graphflow")); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
			return err
		}
	}
	if len(cleanupItems) > 0 {
		allCleaned, err := allRunItemTasksCompleted(ctx, svc, run.ID, "cleanup", len(cleanupItems))
		if err != nil || !allCleaned {
			return err
		}
	}

	coordinator := addItems[0]
	alignmentTask := &graphmodel.GraphFlowTask{
		ID:                 uuid.New(),
		TenantID:           run.OrganizationID,
		KBID:               run.DatasetID,
		DocumentID:         coordinator.DocumentID,
		RunID:              &run.ID,
		RunItemID:          &coordinator.ID,
		SourceRefID:        coordinator.SourceRefID,
		TaskType:           "alignment",
		ExtractionStrategy: currentTask.ExtractionStrategy,
		Status:             "pending",
		Metadata: map[string]interface{}{
			"graph_revision": run.GraphRevision,
		},
	}
	taskID, err := svc.TaskRepo.CreateTaskAndReturnID(ctx, alignmentTask)
	if err != nil {
		return err
	}
	queued, err := NewGraphFlowTask(TypeGraphFlowAlignment, taskID.String(), taskManager)
	if err != nil {
		return err
	}
	if _, err := taskManager.EnqueueTask(queued, asynq.Queue("graphflow")); err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		return err
	}
	return nil
}

func allRunItemTasksCompleted(ctx context.Context, svc *graphflow.Service, runID uuid.UUID, taskType string, expected int) (bool, error) {
	var total int64
	if err := svc.DB.WithContext(ctx).Model(&graphmodel.GraphFlowTask{}).
		Where("run_id = ? AND task_type = ?", runID, taskType).Count(&total).Error; err != nil {
		return false, err
	}
	if total != int64(expected) {
		return false, nil
	}
	var incomplete int64
	if err := svc.DB.WithContext(ctx).Model(&graphmodel.GraphFlowTask{}).
		Where("run_id = ? AND task_type = ? AND status <> ?", runID, taskType, "completed").Count(&incomplete).Error; err != nil {
		return false, err
	}
	return incomplete == 0, nil
}
