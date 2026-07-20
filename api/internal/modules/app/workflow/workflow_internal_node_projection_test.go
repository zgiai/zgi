package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type internalProjectionTestRun struct {
	ID            string `gorm:"primaryKey"`
	TenantID      string
	AgentID       string
	WorkflowID    string
	TriggeredFrom string
	CreatedBy     string
	DeletedAt     *time.Time
}

func (internalProjectionTestRun) TableName() string { return "workflow_run_logs" }

func TestProjectInternalNodeExecutionMergesOutOfOrderStart(t *testing.T) {
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&internalProjectionTestRun{}, &WorkflowNodeRuntimeLog{}); err != nil {
		t.Fatal(err)
	}
	run := internalProjectionTestRun{ID: "run-1", TenantID: "tenant-1", AgentID: "agent-1", WorkflowID: "workflow-1", TriggeredFrom: "debugging", CreatedBy: "account-1"}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	finished := &workflowpause.RunEventPayload{Sequence: 2}
	data := map[string]interface{}{
		"node_execution_id": "node-exec-1", "node_id": "child", "node_type": "llm", "title": "Child",
		"container_id": "loop-1", "container_type": "loop", "round_index": 2,
		"status": "succeeded", "outputs": map[string]interface{}{"text": "done"},
	}
	if err := projectInternalNodeExecution(context.Background(), db, run.ID, workflowpause.EventNodeFinished, data, finished); err != nil {
		t.Fatal(err)
	}
	started := &workflowpause.RunEventPayload{Sequence: 1}
	data["inputs"] = map[string]interface{}{"prompt": "hello"}
	if err := projectInternalNodeExecution(context.Background(), db, run.ID, workflowpause.EventNodeStarted, data, started); err != nil {
		t.Fatal(err)
	}

	var log WorkflowNodeRuntimeLog
	if err := db.Where("workflow_run_id = ? AND node_execution_id = ?", run.ID, "node-exec-1").First(&log).Error; err != nil {
		t.Fatal(err)
	}
	if log.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", log.Status)
	}
	if log.StartedEventSequence == nil || *log.StartedEventSequence != 1 || log.FinishedEventSequence == nil || *log.FinishedEventSequence != 2 {
		t.Fatalf("event sequences = start %#v finish %#v", log.StartedEventSequence, log.FinishedEventSequence)
	}
	inputs, err := log.GetInputsDict()
	if err != nil || inputs["prompt"] != "hello" {
		t.Fatalf("inputs = %#v err = %v", inputs, err)
	}
}
