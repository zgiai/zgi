package digitalhuman

import (
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkflowApprovalRunLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Run{}, &runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE connector_confirmation_continuations (origin_version integer, origin_surface text, run_id text, digital_human_id text, state text, route_receipt text)`).Error; err != nil {
		t.Fatal(err)
	}
	run, message := platformWaitingRunFixture(t)
	run.IdempotencyKey = run.ID.String()
	run.InvocationEvent, run.RunSpec, run.ExecutionSnapshot, run.ExecutionTrace = datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`)
	// A generic workflow approval, not a platform-governance one.
	message.Metadata = map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "workflow-run-1"}}
	if err := db.Select("id", "organization_id", "workspace_id", "digital_human_id", "digital_human_version_id", "user_subject_kind", "user_subject_id", "surface", "conversation_id", "message_id", "status", "idempotency_key", "invocation_event", "run_spec", "run_spec_revision", "execution_snapshot", "execution_snapshot_revision", "execution_trace", "query", "result_summary", "error_code", "error_message", "created_at", "updated_at").Create(run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Select("id", "conversation_id", "status", "metadata", "query", "model_name", "updated_at").Create(message).Error; err != nil {
		t.Fatal(err)
	}
	repo, now := NewRepository(db), time.Now().UTC()

	if err := repo.MarkRunWaiting(t.Context(), run.ID, runtimemodel.MessageStatusWaitingApproval, now); err != nil {
		t.Fatalf("workflow approval did not hold the run: %v", err)
	}
	var waiting Run
	if err := db.First(&waiting, "id = ?", run.ID).Error; err != nil || waiting.Status != RunStatusWaiting || waiting.WaitingApprovalOrigin != workflowApprovalOrigin || waiting.WaitingReceipt != nil {
		t.Fatalf("waiting run = %#v, err %v", waiting, err)
	}
	if err := repo.ResumeWaitingRun(t.Context(), run.ID, now); err != nil {
		t.Fatalf("resume waiting run: %v", err)
	}
	var resumed Run
	if err := db.First(&resumed, "id = ?", run.ID).Error; err != nil || resumed.Status != RunStatusRunning || resumed.WaitingReason != "" || resumed.WaitingApprovalOrigin != "" || resumed.WaitingReceipt != nil {
		t.Fatalf("resumed run = %#v, err %v; want running with every waiting marker cleared", resumed, err)
	}
	if err := repo.CompleteRun(t.Context(), run.ID, "approved result", datatypes.JSON(`{}`), now); err != nil {
		t.Fatalf("complete resumed run: %v", err)
	}
	var stored Run
	if err := db.First(&stored, "id = ?", run.ID).Error; err != nil || stored.Status != RunStatusSucceeded || stored.WaitingReason != "" || stored.WaitingApprovalOrigin != "" {
		t.Fatalf("stored run = %#v, err %v; want succeeded without waiting state", stored, err)
	}
}

func TestWorkflowApprovalDoesNotHoldRunForTaskApprovalOrMissingState(t *testing.T) {
	valid := map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "workflow-run-1"}}
	for name, test := range map[string]struct {
		metadata map[string]interface{}
		surface  string
		want     bool
	}{
		"workflow approval":  {valid, SurfaceConsole, true},
		"webapp surface":     {valid, SurfaceWebApp, true},
		"api surface":        {valid, SurfaceAPI, true},
		"channel surface":    {valid, SurfaceChannel, false},
		"background surface": {valid, SurfaceTask, false},
		"task approval":      {map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"kind": "digital_human_task", "workflow_run_id": "workflow-run-1"}}, SurfaceConsole, false},
		"no workflow run":    {map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"kind": "workflow"}}, SurfaceConsole, false},
		"no continuation":    {map[string]interface{}{}, SurfaceConsole, false},
	} {
		t.Run(name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&runtimemodel.Message{}); err != nil {
				t.Fatal(err)
			}
			conversationID, messageID := uuid.New(), uuid.New()
			message := &runtimemodel.Message{ID: messageID, ConversationID: conversationID, Status: runtimemodel.MessageStatusWaitingApproval, Metadata: test.metadata}
			if err := db.Select("id", "conversation_id", "status", "metadata", "query", "model_name", "updated_at").Create(message).Error; err != nil {
				t.Fatal(err)
			}
			run := &Run{ConversationID: &conversationID, MessageID: &messageID, Surface: test.surface}
			matched, err := runHasWorkflowContinuationMessage(db, run, runtimemodel.MessageStatusWaitingApproval)
			if err != nil || matched != test.want {
				t.Fatalf("matched = %v, err = %v; want %v", matched, err, test.want)
			}
		})
	}
}
