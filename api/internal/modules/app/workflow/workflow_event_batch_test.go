package workflow

import (
	"context"
	"fmt"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
)

func TestWorkflowEventBatchProjectsParallelInternalNodes(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := db.AutoMigrate(&WorkflowRunLog{}, &WorkflowNodeRuntimeLog{}, &workflowpause.RunEvent{}); err != nil {
		t.Fatal(err)
	}
	previous := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previous) })

	executionID := "00000000-0000-0000-0000-000000000801"
	run := WorkflowRunLog{
		ID: "00000000-0000-0000-0000-000000000811", TenantID: "00000000-0000-0000-0000-000000000821",
		AgentID: "00000000-0000-0000-0000-000000000831", WorkflowID: "00000000-0000-0000-0000-000000000841",
		Type: dto.WorkflowTypeChat, TriggeredFrom: "debugging", Version: "draft", Status: dto.WorkflowRunStatusRunning,
		CreatedByRole: CreatedByRoleAccount, CreatedBy: "00000000-0000-0000-0000-000000000851",
		RuntimeProtocolVersion: 2, ExecutionGeneration: 1, ActiveExecutionID: &executionID,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	ctx := withWorkflowExecutionOwner(context.Background(), workflowExecutionOwner{
		WorkflowRunID: run.ID, ExecutionID: executionID, Generation: 1,
	})
	records := make([]workflowRunEventRecord, 0, 32)
	for index := 0; index < 16; index++ {
		nodeExecutionID := fmt.Sprintf("node-execution-%d", index)
		base := map[string]interface{}{
			"id": nodeExecutionID, "node_execution_id": nodeExecutionID, "node_id": "child-node",
			"node_type": "llm", "title": "Child", "loop_id": "loop-node", "loop_index": index,
		}
		started := make(map[string]interface{}, len(base)+1)
		finished := make(map[string]interface{}, len(base)+2)
		for key, value := range base {
			started[key] = value
			finished[key] = value
		}
		started["status"] = "running"
		finished["status"] = "succeeded"
		finished["outputs"] = map[string]interface{}{"text": fmt.Sprintf("result-%d", index)}
		records = append(records,
			workflowRunEventRecord{eventType: workflowpause.EventNodeStarted, data: started},
			workflowRunEventRecord{eventType: workflowpause.EventNodeFinished, data: finished},
		)
	}
	stored, err := appendWorkflowRunEventBatchPayloadResult(ctx, run.TenantID, run.AgentID, run.ID, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 32 || stored[31].Sequence != 32 {
		t.Fatalf("stored events = %d last sequence = %d", len(stored), stored[len(stored)-1].Sequence)
	}
	var projections []WorkflowNodeRuntimeLog
	if err := db.Where("workflow_run_id = ?", run.ID).Order("round_index ASC").Find(&projections).Error; err != nil {
		t.Fatal(err)
	}
	if len(projections) != 16 {
		t.Fatalf("node projections = %d, want 16", len(projections))
	}
	for index, projection := range projections {
		if projection.Status != "succeeded" || projection.StartedEventSequence == nil || projection.FinishedEventSequence == nil {
			t.Fatalf("projection %d = %#v", index, projection)
		}
	}
}
