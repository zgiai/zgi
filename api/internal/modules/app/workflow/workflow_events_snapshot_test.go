package workflow

import (
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkflowSnapshotIsTerminalUsesSnapshotState(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{status: string(dto.WorkflowRunStatusRunning), want: false},
		{status: string(dto.WorkflowRunStatusPaused), want: false},
		{status: string(dto.WorkflowRunStatusSucceeded), want: true},
		{status: string(dto.WorkflowRunStatusFailed), want: true},
		{status: string(dto.WorkflowRunStatusStopped), want: true},
	}
	for _, tt := range tests {
		snapshot := workflowpause.RunEventPayload{Data: map[string]interface{}{
			"workflow_run": map[string]interface{}{"status": tt.status},
		}}
		if got := workflowSnapshotIsTerminal(snapshot); got != tt.want {
			t.Fatalf("workflowSnapshotIsTerminal(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestWorkflowSnapshotPauseReasonsRestoresDurableInteractionPayload(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:workflow_snapshot_reasons?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&workflowpause.RunEvent{}); err != nil {
		t.Skipf("sqlite migration unavailable: %v", err)
	}
	pauseID := "a81755aa-2dcc-4677-a4af-c83f31092272"
	pauseGeneration := int64(2)
	event := workflowpause.RunEvent{
		ID:              "dd2366d6-37a1-4fb1-bca0-3c9e2a120119",
		TenantID:        "b34e3838-335d-47ca-81f7-7cf3a8212375",
		AppID:           "65d7e24f-9af0-4ddd-adb0-eac6dcbb0f72",
		WorkflowRunID:   "run-1",
		Sequence:        3,
		EventType:       workflowpause.EventQuestionAnswerRequested,
		EventData:       `{"node_id":"question-1","question":"请选择处理方式","options":[{"id":"keep","label":"保留"}]}`,
		SchemaVersion:   2,
		Category:        workflowpause.EventCategoryInteraction,
		PauseID:         &pauseID,
		PauseGeneration: &pauseGeneration,
		OccurredAt:      time.Now(),
	}
	if err := db.Create(&event).Error; err != nil {
		t.Fatalf("create requested event: %v", err)
	}
	pauseRecord := &workflowpause.RunPause{ID: pauseID, WorkflowRunID: "run-1", Generation: pauseGeneration}
	reasons := []workflowpause.RunPauseReason{{
		ID: "reason-1", PauseID: pauseID, Type: workflowpause.ReasonTypeQuestionAnswerRequired,
		NodeID: "question-1", Status: workflowpause.RunPauseReasonStatusPending,
	}}

	got, err := workflowSnapshotPauseReasons(db, pauseRecord, reasons)
	if err != nil {
		t.Fatalf("workflowSnapshotPauseReasons: %v", err)
	}
	if len(got) != 1 || got[0]["question"] != "请选择处理方式" {
		t.Fatalf("snapshot reasons = %#v", got)
	}
	if options, ok := got[0]["options"].([]interface{}); !ok || len(options) != 1 {
		t.Fatalf("snapshot options = %#v", got[0]["options"])
	}
	if _, exposed := got[0]["state_json"]; exposed {
		t.Fatalf("snapshot must not expose raw pause state: %#v", got[0])
	}
}
