package workflow

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkflowRunNeedsRuntimeCancellation(t *testing.T) {
	executionID := "execution-1"
	for _, tt := range []struct {
		name string
		run  *WorkflowRunLog
		want bool
	}{
		{name: "nil", run: nil, want: false},
		{name: "legacy paused may still have local runtime", run: &WorkflowRunLog{Status: dto.WorkflowRunStatusPaused, RuntimeProtocolVersion: 1}, want: true},
		{name: "v2 paused released owner", run: &WorkflowRunLog{Status: dto.WorkflowRunStatusPaused, RuntimeProtocolVersion: 2}, want: false},
		{name: "v2 running owner", run: &WorkflowRunLog{Status: dto.WorkflowRunStatusRunning, RuntimeProtocolVersion: 2, ActiveExecutionID: &executionID}, want: true},
		{name: "v2 running without owner", run: &WorkflowRunLog{Status: dto.WorkflowRunStatusRunning, RuntimeProtocolVersion: 2}, want: false},
		{name: "terminal owner is stale", run: &WorkflowRunLog{Status: dto.WorkflowRunStatusSucceeded, RuntimeProtocolVersion: 2, ActiveExecutionID: &executionID}, want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := workflowRunNeedsRuntimeCancellation(tt.run); got != tt.want {
				t.Fatalf("workflowRunNeedsRuntimeCancellation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateWorkflowRunLogStatusUsesTransactionalV2Finalizer(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := migrateWorkflowStopV2TestTables(db); err != nil {
		t.Fatalf("migrate runtime tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	executionID := "00000000-0000-0000-0000-000000000019"
	run := workflowStopTestRun{
		ID: "00000000-0000-0000-0000-000000000109", TenantID: "00000000-0000-0000-0000-000000000209",
		AgentID: "00000000-0000-0000-0000-000000000309", WorkflowID: "00000000-0000-0000-0000-000000000409",
		Status: "running", RuntimeProtocolVersion: 2, ExecutionGeneration: 1,
		ActiveExecutionID: &executionID, CreatedAt: time.Now(),
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	service := &WorkflowService{workflowRunLogRepo: NewWorkflowRunLogRepository(db)}
	ownerCtx := withWorkflowExecutionOwner(context.Background(), workflowExecutionOwner{
		WorkflowRunID: run.ID, ExecutionID: executionID, Generation: 1,
	})
	outputs := map[string]interface{}{"answer": "done"}
	if err := service.UpdateWorkflowRunLogStatus(ownerCtx, run.ID, string(dto.WorkflowRunStatusSucceeded), outputs, 12, 34, 2, ""); err != nil {
		t.Fatalf("finalize workflow run: %v", err)
	}

	var persisted workflowStopTestRun
	if err := db.First(&persisted, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load workflow run: %v", err)
	}
	if persisted.Status != string(dto.WorkflowRunStatusSucceeded) || persisted.ActiveExecutionID != nil {
		t.Fatalf("run status=%q active_execution_id=%v", persisted.Status, persisted.ActiveExecutionID)
	}
	var events []workflowpause.RunEvent
	if err := db.Where("workflow_run_id = ?", run.ID).Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatalf("load terminal events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != workflowpause.EventWorkflowFinished {
		t.Fatalf("terminal events = %#v", events)
	}
	if err := service.UpdateWorkflowRunLogStatus(ownerCtx, run.ID, string(dto.WorkflowRunStatusSucceeded), outputs, 12, 34, 2, ""); !errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
		t.Fatalf("stale finalization error = %v, want ownership lost", err)
	}
}

func TestFinalizeWorkflowRunCommitsAnswerAndTerminalEventsTogether(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := migrateWorkflowStopV2TestTables(db); err != nil {
		t.Fatalf("migrate runtime tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	executionID := "00000000-0000-0000-0000-000000000029"
	run := workflowStopTestRun{
		ID: "00000000-0000-0000-0000-000000000119", TenantID: "00000000-0000-0000-0000-000000000219",
		AgentID: "00000000-0000-0000-0000-000000000319", WorkflowID: "00000000-0000-0000-0000-000000000419",
		Status: "running", RuntimeProtocolVersion: 2, ExecutionGeneration: 4,
		ActiveExecutionID: &executionID, CreatedAt: time.Now(),
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	message := workflowStopTestMessage{
		ID: "00000000-0000-0000-0000-000000000519", WorkflowRunID: run.ID,
		ConversationID: "00000000-0000-0000-0000-000000000619",
		Answer:         "partial", Status: "running", ExecutionGeneration: 4,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create workflow message: %v", err)
	}
	ctx := withWorkflowExecutionOwner(context.Background(), workflowExecutionOwner{
		WorkflowRunID: run.ID, ExecutionID: executionID, Generation: 4,
	})
	messageEnd := map[string]interface{}{"message_id": message.ID, "conversation_id": message.ConversationID, "status": "completed"}
	finished := map[string]interface{}{"id": run.ID, "status": "succeeded"}
	if err := finalizeWorkflowRun(ctx, finalizeWorkflowRunParams{
		WorkflowRunID: run.ID, Status: "succeeded", Outputs: map[string]interface{}{"answer": "complete"},
		FinalAnswer: "complete", MessageStatus: "completed", MessageEnd: messageEnd, WorkflowFinished: finished,
	}); err != nil {
		t.Fatalf("finalize workflow: %v", err)
	}
	var persistedMessage workflowStopTestMessage
	if err := db.First(&persistedMessage, "id = ?", message.ID).Error; err != nil {
		t.Fatalf("load terminal message: %v", err)
	}
	if persistedMessage.Answer != "complete" || persistedMessage.Status != "completed" || persistedMessage.ProjectionRevision != 1 {
		t.Fatalf("terminal message projection = %+v", persistedMessage)
	}
	var events []workflowpause.RunEvent
	if err := db.Where("workflow_run_id = ?", run.ID).Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatalf("load terminal event sequence: %v", err)
	}
	want := []string{workflowEventMessage, workflowEventMessageEnd, workflowpause.EventWorkflowFinished}
	if len(events) != len(want) {
		t.Fatalf("terminal event count = %d, want %d", len(events), len(want))
	}
	for index := range want {
		if events[index].EventType != want[index] {
			t.Fatalf("terminal event %d = %q, want %q", index, events[index].EventType, want[index])
		}
	}
}

func TestPersistApprovalResumeCompletionSkipsHostOwnedMessageProjectionV2(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := migrateWorkflowStopV2TestTables(db); err != nil {
		t.Fatalf("migrate runtime tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	executionID := "00000000-0000-0000-0000-00000000002a"
	runRecord := workflowStopTestRun{
		ID: "00000000-0000-0000-0000-00000000011a", TenantID: "00000000-0000-0000-0000-00000000021a",
		AgentID: "00000000-0000-0000-0000-00000000031a", WorkflowID: "00000000-0000-0000-0000-00000000041a",
		Status: "running", RuntimeProtocolVersion: 2, ExecutionGeneration: 3,
		ActiveExecutionID: &executionID, CreatedAt: time.Now(),
	}
	if err := db.Create(&runRecord).Error; err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	run := &WorkflowRunLog{
		ID: runRecord.ID, TenantID: runRecord.TenantID, AgentID: runRecord.AgentID, WorkflowID: runRecord.WorkflowID,
		Type: dto.WorkflowTypeChat, Status: dto.WorkflowRunStatusRunning, RuntimeProtocolVersion: 2,
		ExecutionGeneration: 3, ActiveExecutionID: &executionID, CreatedAt: runRecord.CreatedAt,
	}
	ctx := withWorkflowExecutionOwner(context.Background(), workflowExecutionOwner{
		WorkflowRunID: run.ID, ExecutionID: executionID, Generation: 3,
	})
	handler := &WorkflowHandler{}
	if err := handler.persistApprovalResumeCompletion(
		ctx, workflowpause.NewService(db), nil, run,
		map[string]interface{}{"answer": "complete"}, time.Now(), "CONVERSATION_WORKFLOW",
		map[string]interface{}{"sys.conversation_id": "00000000-0000-0000-0000-00000000061a"}, map[string]interface{}{}, false, false, nil, "complete",
	); err != nil {
		t.Fatalf("finalize agent-hosted conversational workflow: %v", err)
	}

	var persisted workflowStopTestRun
	if err := db.First(&persisted, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load finalized workflow run: %v", err)
	}
	if persisted.Status != "succeeded" || persisted.ActiveExecutionID != nil {
		t.Fatalf("run status=%q active_execution_id=%v", persisted.Status, persisted.ActiveExecutionID)
	}
	var messageCount int64
	if err := db.Model(&workflowStopTestMessage{}).Where("workflow_run_id = ?", run.ID).Count(&messageCount).Error; err != nil {
		t.Fatalf("count workflow-owned messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("workflow-owned message count = %d, want 0", messageCount)
	}
	var events []workflowpause.RunEvent
	if err := db.Where("workflow_run_id = ?", run.ID).Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatalf("load terminal events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != workflowpause.EventWorkflowFinished {
		t.Fatalf("terminal events = %#v", events)
	}
}

func TestResolveWorkflowConversationProjectionKeepsWorkflowOwnedMessage(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := db.AutoMigrate(&workflowStopTestMessage{}); err != nil {
		t.Fatalf("migrate workflow message: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	message := workflowStopTestMessage{
		ID: "00000000-0000-0000-0000-00000000052b", WorkflowRunID: "00000000-0000-0000-0000-00000000012b",
		ConversationID: "00000000-0000-0000-0000-00000000062b", Status: "running",
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create workflow message: %v", err)
	}
	messageEnd := map[string]interface{}{"message_id": message.ID, "conversation_id": message.ConversationID}
	status, resolvedEnd, err := resolveWorkflowConversationProjection(
		context.Background(), message.WorkflowRunID, message.ConversationID, "completed", messageEnd,
	)
	if err != nil {
		t.Fatalf("resolve workflow-owned projection: %v", err)
	}
	if status != "completed" || resolvedEnd == nil {
		t.Fatalf("resolved projection status=%q message_end=%#v", status, resolvedEnd)
	}
}

func TestFinalizeWorkflowRunRollsBackProjectionWhenTerminalEventFails(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := migrateWorkflowStopV2TestTables(db); err != nil {
		t.Fatalf("migrate runtime tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	executionID := "00000000-0000-0000-0000-000000000039"
	run := workflowStopTestRun{
		ID: "00000000-0000-0000-0000-000000000129", TenantID: "00000000-0000-0000-0000-000000000229",
		AgentID: "00000000-0000-0000-0000-000000000329", WorkflowID: "00000000-0000-0000-0000-000000000429",
		Status: "running", RuntimeProtocolVersion: 2, ExecutionGeneration: 5,
		ActiveExecutionID: &executionID, CreatedAt: time.Now(),
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	message := workflowStopTestMessage{
		ID: "00000000-0000-0000-0000-000000000529", WorkflowRunID: run.ID,
		ConversationID: "00000000-0000-0000-0000-000000000629",
		Answer:         "partial", Status: "running", ExecutionGeneration: 5,
	}
	if err := db.Create(&message).Error; err != nil {
		t.Fatalf("create workflow message: %v", err)
	}
	ctx := withWorkflowExecutionOwner(context.Background(), workflowExecutionOwner{
		WorkflowRunID: run.ID, ExecutionID: executionID, Generation: 5,
	})
	err := finalizeWorkflowRun(ctx, finalizeWorkflowRunParams{
		WorkflowRunID: run.ID, Status: "succeeded", Outputs: map[string]interface{}{"answer": "complete"},
		FinalAnswer: "complete", MessageStatus: "completed",
		WorkflowFinished: map[string]interface{}{"id": run.ID, "not_json": func() {}},
	})
	if err == nil {
		t.Fatal("finalize workflow succeeded despite invalid terminal event payload")
	}
	var persistedRun workflowStopTestRun
	if err := db.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("reload workflow run: %v", err)
	}
	if persistedRun.Status != "running" || persistedRun.ActiveExecutionID == nil || *persistedRun.ActiveExecutionID != executionID {
		t.Fatalf("run changed after rolled back finalization: %+v", persistedRun)
	}
	var persistedMessage workflowStopTestMessage
	if err := db.First(&persistedMessage, "id = ?", message.ID).Error; err != nil {
		t.Fatalf("reload workflow message: %v", err)
	}
	if persistedMessage.Answer != "partial" || persistedMessage.Status != "running" || persistedMessage.ProjectionRevision != 0 {
		t.Fatalf("message changed after rolled back finalization: %+v", persistedMessage)
	}
	var eventCount int64
	if err := db.Model(&workflowpause.RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count terminal events: %v", err)
	}
	if eventCount != 0 {
		t.Fatalf("terminal event count after rollback = %d, want 0", eventCount)
	}
}

func TestFinalizeStoppedWorkflowRunV2RevokesExecutionOwner(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := migrateWorkflowStopV2TestTables(db); err != nil {
		t.Fatalf("migrate runtime tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	executionID := "00000000-0000-0000-0000-000000000011"
	outputs := `{"partial":true}`
	run := WorkflowRunLog{
		ID: "00000000-0000-0000-0000-000000000101", TenantID: "00000000-0000-0000-0000-000000000201",
		AgentID: "00000000-0000-0000-0000-000000000301", WorkflowID: "00000000-0000-0000-0000-000000000401",
		Type: "workflow", TriggeredFrom: "debugging", Version: "draft", Status: "running",
		Outputs: &outputs, CreatedByRole: CreatedByRoleAccount,
		CreatedBy: "00000000-0000-0000-0000-000000000501", RuntimeProtocolVersion: 2,
		ExecutionGeneration: 1, ActiveExecutionID: &executionID,
	}
	if err := db.Create(&workflowStopTestRun{
		ID: run.ID, TenantID: run.TenantID, AgentID: run.AgentID, WorkflowID: run.WorkflowID,
		Status: string(run.Status), Outputs: run.Outputs, CreatedAt: run.CreatedAt,
		RuntimeProtocolVersion: 2, ExecutionGeneration: 1, ActiveExecutionID: &executionID,
	}).Error; err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	service := &WorkflowService{workflowRunLogRepo: NewWorkflowRunLogRepository(db)}
	if err := service.finalizeStoppedWorkflowRunV2(context.Background(), &run, time.Now()); err != nil {
		t.Fatalf("finalize stopped run: %v", err)
	}

	var persisted WorkflowRunLog
	if err := db.First(&persisted, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load workflow run: %v", err)
	}
	if persisted.Status != "stopped" || persisted.ActiveExecutionID != nil {
		t.Fatalf("run status=%q active_execution_id=%v", persisted.Status, persisted.ActiveExecutionID)
	}
	var events []workflowpause.RunEvent
	if err := db.Where("workflow_run_id = ?", run.ID).Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatalf("load terminal events: %v", err)
	}
	if len(events) != 1 || events[0].EventType != workflowpause.EventWorkflowFinished {
		t.Fatalf("terminal events = %#v", events)
	}
}

func TestFinalizeStoppedWorkflowRunV2ClosesPausedRunWithoutOwner(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := migrateWorkflowStopV2TestTables(db); err != nil {
		t.Fatalf("migrate runtime tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	runID := "00000000-0000-0000-0000-000000000102"
	run := workflowStopTestRun{
		ID: runID, TenantID: "00000000-0000-0000-0000-000000000202",
		AgentID: "00000000-0000-0000-0000-000000000302", WorkflowID: "00000000-0000-0000-0000-000000000402",
		Status: "paused", RuntimeProtocolVersion: 2, ExecutionGeneration: 2, CreatedAt: time.Now(),
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create paused run: %v", err)
	}
	pauseRecord := workflowpause.RunPause{
		ID: "00000000-0000-0000-0000-000000000602", TenantID: run.TenantID, AppID: run.AgentID,
		WorkflowRunID: run.ID, NodeID: "approval", Reason: workflowpause.ReasonTypeApprovalRequired,
		StateJSON: `{"version":"2"}`, Generation: 1, Status: workflowpause.RunPauseStatusPaused,
	}
	if err := db.Create(&pauseRecord).Error; err != nil {
		t.Fatalf("create pause: %v", err)
	}
	service := &WorkflowService{workflowRunLogRepo: NewWorkflowRunLogRepository(db)}
	if err := service.finalizeUnownedStoppedWorkflowRunV2(context.Background(), run.ID, time.Now()); err != nil {
		t.Fatalf("finalize paused run: %v", err)
	}
	var persisted workflowStopTestRun
	if err := db.First(&persisted, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load stopped run: %v", err)
	}
	if persisted.Status != "stopped" {
		t.Fatalf("run status = %q", persisted.Status)
	}
	var closed workflowpause.RunPause
	if err := db.First(&closed, "id = ?", pauseRecord.ID).Error; err != nil {
		t.Fatalf("load pause: %v", err)
	}
	if closed.Status != workflowpause.RunPauseStatusClosed {
		t.Fatalf("pause status = %q", closed.Status)
	}
}

func TestFinalizeStoppedWorkflowRunV2FencesResumeAndClosesRuntimeState(t *testing.T) {
	db := openWorkflowStopV2TestDB(t)
	if err := migrateWorkflowStopV2TestTables(db); err != nil {
		t.Fatalf("migrate runtime tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	executionID := "00000000-0000-0000-0000-000000000071"
	conversationID := "00000000-0000-0000-0000-000000000871"
	run := workflowStopTestRun{
		ID: "00000000-0000-0000-0000-000000000171", TenantID: "00000000-0000-0000-0000-000000000271",
		AgentID: "00000000-0000-0000-0000-000000000371", WorkflowID: "00000000-0000-0000-0000-000000000471",
		Status: "running", RuntimeProtocolVersion: 2, ExecutionGeneration: 3,
		ActiveExecutionID: &executionID, ConversationID: &conversationID, CreatedAt: time.Now(),
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create workflow run: %v", err)
	}
	activeRunID := uuid.MustParse(run.ID)
	conversationRecord := workflowStopTestConversation{
		ID: uuid.MustParse(conversationID), RuntimeStatus: "running", ActiveWorkflowRunID: &activeRunID,
	}
	if err := db.Create(&conversationRecord).Error; err != nil {
		t.Fatalf("create workflow conversation: %v", err)
	}
	leaseExpiresAt := time.Now().Add(time.Minute)
	pauseRecord := workflowpause.RunPause{
		ID: "00000000-0000-0000-0000-000000000571", TenantID: run.TenantID, AppID: run.AgentID,
		WorkflowRunID: run.ID, NodeID: "loop-node", Reason: workflowpause.ReasonTypeApprovalRequired,
		StateJSON: `{"version":"2"}`, Generation: 1, Status: workflowpause.RunPauseStatusResuming,
		Revision: 2, ResumeExecutionID: &executionID, LeaseExpiresAt: &leaseExpiresAt,
	}
	if err := db.Create(&pauseRecord).Error; err != nil {
		t.Fatalf("create resuming pause: %v", err)
	}
	outbox := workflowpause.RuntimeOutbox{
		ID: "00000000-0000-0000-0000-000000000671", TenantID: run.TenantID, WorkflowRunID: run.ID,
		PauseID: &pauseRecord.ID, Kind: workflowpause.RuntimeOutboxKindResume,
		IdempotencyKey: "workflow-resume:" + pauseRecord.ID + ":1", PayloadJSON: `{}`,
		Status: workflowpause.RuntimeOutboxPublished, NextAttemptAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&outbox).Error; err != nil {
		t.Fatalf("create resume outbox: %v", err)
	}
	nodeExecutionID := "loop-execution-1"
	node := workflowStopTestNode{
		ID: "00000000-0000-0000-0000-000000000771", WorkflowRunID: &run.ID,
		NodeExecutionID: &nodeExecutionID, NodeID: "loop-node", NodeType: "loop", Title: "Loop",
		Status: "running", CreatedAt: time.Now(),
	}
	if err := db.Create(&node).Error; err != nil {
		t.Fatalf("create running node: %v", err)
	}

	service := &WorkflowService{workflowRunLogRepo: NewWorkflowRunLogRepository(db)}
	if err := service.finalizeStoppedWorkflowRunV2(t.Context(), &WorkflowRunLog{ID: run.ID}, time.Now()); err != nil {
		t.Fatalf("finalize stopped workflow: %v", err)
	}

	var persistedRun workflowStopTestRun
	if err := db.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load stopped run: %v", err)
	}
	if persistedRun.Status != "stopped" || persistedRun.ActiveExecutionID != nil || persistedRun.ExecutionGeneration != 4 {
		t.Fatalf("stopped run = %+v, want status stopped, no owner, generation 4", persistedRun)
	}
	var persistedPause workflowpause.RunPause
	if err := db.First(&persistedPause, "id = ?", pauseRecord.ID).Error; err != nil {
		t.Fatalf("load closed pause: %v", err)
	}
	if persistedPause.Status != workflowpause.RunPauseStatusClosed || persistedPause.ResumeExecutionID != nil || persistedPause.LeaseExpiresAt != nil {
		t.Fatalf("closed pause = %+v", persistedPause)
	}
	var persistedOutbox workflowpause.RuntimeOutbox
	if err := db.First(&persistedOutbox, "id = ?", outbox.ID).Error; err != nil {
		t.Fatalf("load obsolete outbox: %v", err)
	}
	if persistedOutbox.Status != workflowpause.RuntimeOutboxObsolete {
		t.Fatalf("outbox status = %q, want %q", persistedOutbox.Status, workflowpause.RuntimeOutboxObsolete)
	}
	var persistedNode workflowStopTestNode
	if err := db.First(&persistedNode, "id = ?", node.ID).Error; err != nil {
		t.Fatalf("load stopped node: %v", err)
	}
	if persistedNode.Status != "stopped" || persistedNode.FinishedAt == nil {
		t.Fatalf("stopped node = %+v", persistedNode)
	}
	var events []workflowpause.RunEvent
	if err := db.Where("workflow_run_id = ?", run.ID).Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatalf("load stop events: %v", err)
	}
	if len(events) != 2 || events[0].EventType != workflowpause.EventNodeFinished || events[1].EventType != workflowpause.EventWorkflowFinished {
		t.Fatalf("stop event order = %#v", events)
	}
	if _, err := workflowpause.NewService(db).ClaimResume(t.Context(), run.ID, pauseRecord.ID, time.Minute); !errors.Is(err, workflowpause.ErrPauseNotResumeReady) {
		t.Fatalf("ClaimResume() error = %v, want pause not resume ready", err)
	}
	if err := service.finalizeStoppedWorkflowRunV2(t.Context(), &WorkflowRunLog{ID: run.ID}, time.Now()); err != nil {
		t.Fatalf("repeat stop: %v", err)
	}
	var eventCount int64
	if err := db.Model(&workflowpause.RunEvent{}).Where("workflow_run_id = ?", run.ID).Count(&eventCount).Error; err != nil {
		t.Fatalf("count stop events: %v", err)
	}
	if eventCount != 2 {
		t.Fatalf("stop event count after repeat = %d, want 2", eventCount)
	}
	var persistedConversation workflowStopTestConversation
	if err := db.First(&persistedConversation, "id = ?", conversationRecord.ID).Error; err != nil {
		t.Fatalf("load released workflow conversation: %v", err)
	}
	if persistedConversation.RuntimeStatus != "idle" || persistedConversation.ActiveWorkflowRunID != nil || persistedConversation.RuntimeRevision != 1 {
		t.Fatalf("released workflow conversation = %+v", persistedConversation)
	}
}

func TestPostgresStopBarrierWinsAgainstConcurrentResumeClaim(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN")) == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	db := openWorkflowStopV2TestDB(t)
	if err := migrateWorkflowStopV2TestTables(db); err != nil {
		t.Fatalf("migrate runtime tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	run := workflowStopTestRun{
		ID: "00000000-0000-0000-0000-000000000181", TenantID: "00000000-0000-0000-0000-000000000281",
		AgentID: "00000000-0000-0000-0000-000000000381", WorkflowID: "00000000-0000-0000-0000-000000000481",
		Status: "paused", RuntimeProtocolVersion: 2, ExecutionGeneration: 7, CreatedAt: time.Now(),
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create paused workflow run: %v", err)
	}
	pauseRecord := workflowpause.RunPause{
		ID: "00000000-0000-0000-0000-000000000581", TenantID: run.TenantID, AppID: run.AgentID,
		WorkflowRunID: run.ID, NodeID: "approval", Reason: workflowpause.ReasonTypeApprovalRequired,
		StateJSON: `{"version":"2"}`, Generation: 1, Status: workflowpause.RunPauseStatusResumeReady,
	}
	if err := db.Create(&pauseRecord).Error; err != nil {
		t.Fatalf("create resume-ready pause: %v", err)
	}
	outbox := workflowpause.RuntimeOutbox{
		ID: "00000000-0000-0000-0000-000000000681", TenantID: run.TenantID, WorkflowRunID: run.ID,
		PauseID: &pauseRecord.ID, Kind: workflowpause.RuntimeOutboxKindResume,
		IdempotencyKey: "workflow-resume:" + pauseRecord.ID + ":1", PayloadJSON: `{}`,
		Status: workflowpause.RuntimeOutboxPending, NextAttemptAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := db.Create(&outbox).Error; err != nil {
		t.Fatalf("create pending resume outbox: %v", err)
	}

	start := make(chan struct{})
	result := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		result <- (&WorkflowService{}).finalizeStoppedWorkflowRunV2(t.Context(), &WorkflowRunLog{ID: run.ID}, time.Now())
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := workflowpause.NewService(db).ClaimResume(t.Context(), run.ID, pauseRecord.ID, time.Minute)
		if err != nil && !errors.Is(err, workflowpause.ErrPauseNotFound) && !errors.Is(err, workflowpause.ErrPauseNotResumeReady) {
			result <- err
			return
		}
		result <- nil
	}()
	close(start)
	wg.Wait()
	close(result)
	for err := range result {
		if err != nil {
			t.Fatalf("concurrent stop/resume: %v", err)
		}
	}

	var persistedRun workflowStopTestRun
	if err := db.First(&persistedRun, "id = ?", run.ID).Error; err != nil {
		t.Fatalf("load stopped run: %v", err)
	}
	if persistedRun.Status != "stopped" || persistedRun.FinishedAt == nil || persistedRun.ActiveExecutionID != nil {
		t.Fatalf("run after concurrent stop/resume = %+v", persistedRun)
	}
	var persistedPause workflowpause.RunPause
	if err := db.First(&persistedPause, "id = ?", pauseRecord.ID).Error; err != nil {
		t.Fatalf("load closed pause: %v", err)
	}
	if persistedPause.Status != workflowpause.RunPauseStatusClosed || persistedPause.ResumeExecutionID != nil || persistedPause.LeaseExpiresAt != nil {
		t.Fatalf("pause after concurrent stop/resume = %+v", persistedPause)
	}
	var persistedOutbox workflowpause.RuntimeOutbox
	if err := db.First(&persistedOutbox, "id = ?", outbox.ID).Error; err != nil {
		t.Fatalf("load obsolete outbox: %v", err)
	}
	if persistedOutbox.Status != workflowpause.RuntimeOutboxObsolete {
		t.Fatalf("outbox status = %q, want obsolete", persistedOutbox.Status)
	}
	var events []workflowpause.RunEvent
	if err := db.Where("workflow_run_id = ?", run.ID).Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatalf("load workflow events: %v", err)
	}
	finishedSequence := 0
	for _, event := range events {
		if event.EventType == workflowpause.EventWorkflowFinished {
			finishedSequence = event.Sequence
		}
		if finishedSequence > 0 && event.EventType == workflowpause.EventWorkflowResumed {
			t.Fatalf("workflow_resumed appeared after terminal sequence: %#v", events)
		}
	}
	if finishedSequence == 0 {
		t.Fatalf("workflow_finished event missing: %#v", events)
	}
	if _, err := workflowpause.NewService(db).ClaimResume(t.Context(), run.ID, pauseRecord.ID, time.Minute); !errors.Is(err, workflowpause.ErrPauseNotResumeReady) {
		t.Fatalf("post-stop ClaimResume() error = %v, want pause not resume ready", err)
	}
}

func TestPostgresStoppedAnswerProjectionAcceptsOnlyImmediatelyRevokedGeneration(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN")) == "" {
		t.Skip("TEST_POSTGRES_DSN is not configured")
	}
	db := openWorkflowStopV2TestDB(t)
	if err := db.AutoMigrate(&conversation.AgentConversation{}, &conversation.AgentMessage{}, &WorkflowRunLog{}); err != nil {
		t.Fatalf("migrate stopped projection tables: %v", err)
	}
	oldDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(oldDB) })

	agentID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	runID := uuid.New()
	executionID := uuid.NewString()
	finishedAt := time.Now()
	if err := db.Create(&conversation.AgentConversation{
		ID: conversationID, AgentID: agentID, Mode: "chat", Name: "stopped projection",
		Inputs: "{}", Status: "normal", FromSource: "account", RuntimeStatus: conversation.ConversationRuntimeIdle,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&WorkflowRunLog{
		ID: runID.String(), TenantID: uuid.NewString(), AgentID: agentID.String(), WorkflowID: uuid.NewString(),
		Type: dto.WorkflowTypeChat, TriggeredFrom: "debugging", Version: "draft",
		Status: dto.WorkflowRunStatusRunning, CreatedByRole: CreatedByRoleAccount, CreatedBy: accountID.String(),
		RuntimeProtocolVersion: workflowRuntimeProtocolVersionV2, ExecutionGeneration: 4, ActiveExecutionID: &executionID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&WorkflowRunLog{}).Where("id = ?", runID).Updates(map[string]interface{}{
		"status": dto.WorkflowRunStatusStopped, "finished_at": finishedAt,
		"active_execution_id": nil, "execution_lease_expires_at": nil, "execution_generation": 5,
	}).Error; err != nil {
		t.Fatal(err)
	}

	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{}}
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID.String(), "sys.user_id": accountID.String(), "sys.query": "continue",
	}
	owner := workflowExecutionOwner{WorkflowRunID: runID.String(), ExecutionID: executionID, Generation: 4}
	answer := "七零八落\n一心一意\n"
	if err := handler.persistStoppedWorkflowConversationAnswer(
		t.Context(), owner, runID.String(), agentID.String(), accountID.String(), systemInputs, nil, "debugging", answer,
	); err != nil {
		t.Fatal(err)
	}
	var message conversation.AgentMessage
	if err := db.Where("workflow_run_id = ?", runID).First(&message).Error; err != nil {
		t.Fatal(err)
	}
	if message.Answer != answer || message.Status != conversation.AgentMessageStatusStopped || message.ExecutionGeneration != owner.Generation {
		t.Fatalf("stopped projection answer=%q status=%q generation=%d", message.Answer, message.Status, message.ExecutionGeneration)
	}

	staleOwner := owner
	staleOwner.Generation--
	err := handler.persistStoppedWorkflowConversationAnswer(
		t.Context(), staleOwner, runID.String(), agentID.String(), accountID.String(), systemInputs, nil, "debugging", "stale",
	)
	if !errors.Is(err, workflowpause.ErrExecutionOwnershipLost) {
		t.Fatalf("stale owner error = %v, want ownership lost", err)
	}
	if err := db.First(&message, "id = ?", message.ID).Error; err != nil {
		t.Fatal(err)
	}
	if message.Answer != answer {
		t.Fatalf("stale owner overwrote stopped answer: %q", message.Answer)
	}
}

func migrateWorkflowStopV2TestTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&workflowStopTestRun{},
		&workflowStopTestMessage{},
		&workflowStopTestNode{},
		&workflowStopTestConversation{},
		&workflowpause.RunPause{},
		&workflowpause.RunEvent{},
		&workflowpause.RuntimeOutbox{},
	)
}

type workflowStopTestRun struct {
	ID                      string `gorm:"type:uuid;primaryKey"`
	TenantID                string `gorm:"type:uuid"`
	AgentID                 string `gorm:"type:uuid"`
	WorkflowID              string `gorm:"type:uuid"`
	Status                  string
	Outputs                 *string
	Error                   *string
	ElapsedTime             float64
	TotalTokens             int64
	TotalSteps              int
	ExceptionsCount         int
	CreatedAt               time.Time
	FinishedAt              *time.Time
	DeletedAt               *time.Time
	RuntimeProtocolVersion  int
	NextEventSequence       int64
	ExecutionGeneration     int64
	ActiveExecutionID       *string
	ExecutionLeaseExpiresAt *time.Time
	StateRevision           int64
	ConversationID          *string
}

func (workflowStopTestRun) TableName() string { return "workflow_run_logs" }

type workflowStopTestMessage struct {
	ID                  string `gorm:"type:uuid;primaryKey"`
	WorkflowRunID       string `gorm:"type:uuid"`
	ConversationID      string `gorm:"type:uuid"`
	Answer              string
	Status              string
	Error               *string
	ExecutionGeneration int64
	ProjectionRevision  int64
	UpdatedAt           time.Time
	DeletedAt           *time.Time
}

func (workflowStopTestMessage) TableName() string { return "agents_messages" }

type workflowStopTestNode struct {
	ID              string  `gorm:"type:uuid;primaryKey"`
	WorkflowRunID   *string `gorm:"type:uuid"`
	NodeExecutionID *string
	NodeID          string
	NodeType        string
	Title           string
	Index           int
	Status          string
	ElapsedTime     float64
	CreatedAt       time.Time
	FinishedAt      *time.Time
	DeletedAt       *time.Time
}

func (workflowStopTestNode) TableName() string { return "workflow_node_runtime_logs" }

type workflowStopTestConversation struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey"`
	RuntimeStatus       string
	ActiveWorkflowRunID *uuid.UUID `gorm:"type:uuid"`
	RuntimeRevision     int64
	UpdatedAt           time.Time
}

func (workflowStopTestConversation) TableName() string { return "agents_conversations" }

func openWorkflowStopV2TestDB(t *testing.T) *gorm.DB {
	t.Helper()
	config := &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true}
	if dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN")); dsn != "" {
		admin, err := gorm.Open(postgres.Open(dsn), config)
		if err != nil {
			t.Fatalf("open postgres: %v", err)
		}
		schema := "workflow_stop_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatalf("create test schema: %v", err)
		}
		t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error })
		parsed, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("parse postgres DSN: %v", err)
		}
		query := parsed.Query()
		query.Set("search_path", schema+",public")
		parsed.RawQuery = query.Encode()
		db, err := gorm.Open(postgres.Open(parsed.String()), config)
		if err != nil {
			t.Fatalf("open scoped postgres: %v", err)
		}
		return db
	}
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), config)
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}
