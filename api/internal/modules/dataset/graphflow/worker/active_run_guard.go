package worker

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
)

// validateActiveRunTask prevents an Asynq delivery left behind by a
// superseded run from mutating the current graph version. Legacy tasks without
// a run remain supported for migration compatibility.
func validateActiveRunTask(ctx context.Context, svc *graphflow.Service, task *model.GraphFlowTask) error {
	if svc == nil || task == nil || task.RunID == nil {
		return nil
	}
	run, err := svc.RunRepo.FindByID(ctx, *task.RunID)
	if err != nil {
		return fmt.Errorf("load graph run: %w", err)
	}
	if run.Status != model.GraphFlowRunStatusProcessing {
		return fmt.Errorf("%w: run %s is %s", errStaleGraphFlowRun, run.ID, run.Status)
	}
	dataset, err := svc.DatasetRepo.GetByID(ctx, task.KBID.String())
	if err != nil {
		return fmt.Errorf("load graph dataset: %w", err)
	}
	if dataset == nil || dataset.GraphCurrentRunID == nil || *dataset.GraphCurrentRunID != run.ID.String() {
		return fmt.Errorf("%w: run %s is not the dataset current run", errStaleGraphFlowRun, run.ID)
	}
	return nil
}
