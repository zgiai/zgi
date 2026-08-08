package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openWorkflowConversationRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&conversation.AgentConversation{}, &conversation.AgentMessage{}, &WorkflowRunLog{}); err != nil {
		t.Fatalf("migrate workflow conversation runtime: %v", err)
	}
	return db
}

func TestConversationWorkflowAllowsOnlyOneActiveRun(t *testing.T) {
	db := openWorkflowConversationRuntimeTestDB(t)
	previousDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previousDB) })

	agentID := uuid.New()
	conversationID := uuid.New()
	conv := conversation.AgentConversation{
		ID: conversationID, AgentID: agentID, Mode: "chat", Name: "conversation",
		Inputs: "{}", Status: "normal", FromSource: "account",
		RuntimeStatus: conversation.ConversationRuntimeIdle,
	}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	service := &WorkflowService{workflowRunLogRepo: NewWorkflowRunLogRepository(db)}
	newRun := func() *WorkflowRunLog {
		inputs, _ := json.Marshal(map[string]interface{}{
			"sys.workflow_type":   "chat",
			"sys.conversation_id": conversationID.String(),
		})
		inputString := string(inputs)
		return &WorkflowRunLog{
			TenantID: uuid.NewString(), AgentID: agentID.String(), WorkflowID: uuid.NewString(),
			Type: dto.WorkflowTypeChat, TriggeredFrom: "debugging", Version: "draft",
			Inputs: &inputString, Status: dto.WorkflowRunStatusRunning, CreatedByRole: CreatedByRoleAccount,
			CreatedBy: uuid.NewString(),
		}
	}

	first := newRun()
	if err := service.createWorkflowRunLogWithConversationClaim(context.Background(), first); err != nil {
		t.Fatalf("claim first run: %v", err)
	}
	var claimed conversation.AgentConversation
	if err := db.First(&claimed, "id = ?", conversationID).Error; err != nil {
		t.Fatalf("load claimed conversation: %v", err)
	}
	if claimed.RuntimeStatus != conversation.ConversationRuntimeRunning || claimed.ActiveWorkflowRunID == nil || claimed.ActiveWorkflowRunID.String() != first.ID {
		t.Fatalf("conversation claim = status %q run %v", claimed.RuntimeStatus, claimed.ActiveWorkflowRunID)
	}

	second := newRun()
	err := service.createWorkflowRunLogWithConversationClaim(context.Background(), second)
	var busy *WorkflowConversationBusyError
	if !errors.As(err, &busy) {
		t.Fatalf("second run error = %v, want WorkflowConversationBusyError", err)
	}
	if busy.WorkflowRunID != first.ID || busy.RuntimeStatus != conversation.ConversationRuntimeRunning {
		t.Fatalf("busy state = %#v", busy)
	}
	var runCount int64
	if err := db.Model(&WorkflowRunLog{}).Where("conversation_id = ?", conversationID.String()).Count(&runCount).Error; err != nil {
		t.Fatalf("count claimed runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("claimed run count = %d, want 1", runCount)
	}

	otherConversationID := uuid.New()
	otherConversation := conversation.AgentConversation{
		ID: otherConversationID, AgentID: agentID, Mode: "chat", Name: "other conversation",
		Inputs: "{}", Status: "normal", FromSource: "account",
		RuntimeStatus: conversation.ConversationRuntimeIdle,
	}
	if err := db.Create(&otherConversation).Error; err != nil {
		t.Fatalf("create other conversation: %v", err)
	}
	otherInputs, _ := json.Marshal(map[string]interface{}{
		"sys.workflow_type":   "chat",
		"sys.conversation_id": otherConversationID.String(),
	})
	otherInputString := string(otherInputs)
	otherRun := newRun()
	otherRun.Inputs = &otherInputString
	if err := service.createWorkflowRunLogWithConversationClaim(context.Background(), otherRun); err != nil {
		t.Fatalf("claim different conversation while first is active: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error { return releaseWorkflowConversationTx(tx, first) }); err != nil {
		t.Fatalf("release first run: %v", err)
	}
	if err := db.Model(&WorkflowRunLog{}).Where("id = ?", first.ID).Update("status", dto.WorkflowRunStatusSucceeded).Error; err != nil {
		t.Fatalf("finish first run: %v", err)
	}
	if err := service.createWorkflowRunLogWithConversationClaim(context.Background(), second); err != nil {
		t.Fatalf("claim second run after release: %v", err)
	}
}

func TestStoppedWorkflowConversationAnswerPersistsOnlyRevokedGenerationTail(t *testing.T) {
	db := openWorkflowConversationRuntimeTestDB(t)
	previousDB := database.GetDB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previousDB) })

	agentID := uuid.New()
	accountID := uuid.New()
	conversationID := uuid.New()
	runID := uuid.New()
	executionID := uuid.NewString()
	finishedAt := time.Now()
	if err := db.Create(&conversation.AgentConversation{
		ID: conversationID, AgentID: agentID, Mode: "chat", Name: "stopped tail",
		Inputs: "{}", Status: "normal", FromSource: "account",
		RuntimeStatus: conversation.ConversationRuntimeIdle,
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
		"sys.conversation_id": conversationID.String(),
		"sys.user_id":         accountID.String(),
		"sys.query":           "continue",
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
	staleOwner.Generation = 3
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
		t.Fatalf("stale owner overwrote answer: %q", message.Answer)
	}
}
