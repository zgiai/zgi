package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/app/conversation"
	approvalruntime "github.com/zgiai/ginext/internal/modules/app/workflow/approval"
	workflowpause "github.com/zgiai/ginext/internal/modules/app/workflow/pause"
	"github.com/zgiai/ginext/pkg/database"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildApprovalRequestedEventSanitizesSubmitMethodsAndTokenPolicy(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	webEnabled := true
	emailEnabled := true
	publishedWebPayload := approvalruntime.FormPayload{
		ID:        "form-web",
		Token:     "web-token",
		NodeID:    "approval-1",
		NodeTitle: "Approval",
		Content:   "Please review",
		SubmitMethods: approvalruntime.SubmitMethods{
			WebApp: approvalruntime.WebAppSubmitMethod{Enabled: &webEnabled},
			Email: approvalruntime.EmailSubmitMethod{
				Enabled:    emailEnabled,
				Subject:    "secret subject",
				Body:       "secret {{#url#}}",
				Recipients: []approvalruntime.EmailRecipient{{Type: "external", Email: "approver@example.com"}},
			},
		},
		ExpirationAt: time.Now().Add(time.Hour).Unix(),
	}

	publishedWebEvent := buildApprovalRequestedEvent(ctx, approvalRequestedEventContext{
		WorkflowRunID: "run-web",
		NodeID:        "approval-1",
		NodeTitle:     "Approval",
		IsDraft:       false,
		TriggeredFrom: "web-app",
	}, map[string]interface{}{approvalFormOutputKey: publishedWebPayload})
	if got := publishedWebEvent["token"]; got != "web-token" {
		t.Fatalf("published web token = %#v, want web-token", got)
	}
	assertApprovalSubmitMethods(t, publishedWebEvent, true, true)
	emailMethod := publishedWebEvent["submit_methods"].(map[string]interface{})["email"].(map[string]interface{})
	for _, key := range []string{"subject", "body", "recipients"} {
		if _, exists := emailMethod[key]; exists {
			t.Fatalf("email submit method should not expose %s: %#v", key, emailMethod)
		}
	}

	webDisabled := false
	emailOnlyPayload := approvalruntime.FormPayload{
		ID:        "form-email",
		NodeID:    "approval-2",
		NodeTitle: "Approval",
		Content:   "Please review",
		SubmitMethods: approvalruntime.SubmitMethods{
			WebApp: approvalruntime.WebAppSubmitMethod{Enabled: &webDisabled},
			Email:  approvalruntime.EmailSubmitMethod{Enabled: true},
		},
		ExpirationAt: time.Now().Add(time.Hour).Unix(),
	}
	publishedEmailEvent := buildApprovalRequestedEvent(ctx, approvalRequestedEventContext{
		WorkflowRunID: "run-email",
		NodeID:        "approval-2",
		NodeTitle:     "Approval",
		IsDraft:       false,
		TriggeredFrom: "web-app",
	}, map[string]interface{}{approvalFormOutputKey: emailOnlyPayload})
	if _, exists := publishedEmailEvent["token"]; exists {
		t.Fatalf("published email-only event should not expose token: %#v", publishedEmailEvent)
	}
	assertApprovalSubmitMethods(t, publishedEmailEvent, false, true)

	if err := db.Create(&approvalruntime.Form{
		ID:              "form-email",
		TenantID:        uuid.NewString(),
		AppID:           uuid.NewString(),
		WorkflowRunID:   "run-email",
		NodeID:          "approval-2",
		NodeTitle:       "Approval",
		FormDefinition:  "{}",
		RenderedContent: "Please review",
		Status:          approvalruntime.FormStatusWaiting,
		ExpirationTime:  time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create email-only form: %v", err)
	}
	if err := db.Create(&approvalruntime.Recipient{
		ID:               uuid.NewString(),
		FormID:           "form-email",
		DeliveryID:       uuid.NewString(),
		RecipientType:    approvalruntime.RecipientTypeEmailExternal,
		RecipientPayload: `{"type":"email_external","email":"approver@example.com"}`,
		AccessToken:      "email-recipient-token",
	}).Error; err != nil {
		t.Fatalf("create email recipient: %v", err)
	}

	debugEmailEvent := buildApprovalRequestedEvent(ctx, approvalRequestedEventContext{
		WorkflowRunID: "run-email",
		NodeID:        "approval-2",
		NodeTitle:     "Approval",
		IsDraft:       true,
		TriggeredFrom: "debugging",
	}, map[string]interface{}{approvalFormOutputKey: emailOnlyPayload})
	debugToken, _ := debugEmailEvent["token"].(string)
	if debugToken == "" {
		t.Fatalf("debug email-only token should be present: %#v", debugEmailEvent)
	}
	if len(debugToken) != 8 {
		t.Fatalf("debug email-only token length = %d, want 8", len(debugToken))
	}
	if debugToken == "email-recipient-token" {
		t.Fatal("debug email-only token should not reuse email recipient token")
	}
	var consoleRecipient approvalruntime.Recipient
	if err := db.First(&consoleRecipient, "form_id = ? AND recipient_type = ?", "form-email", approvalruntime.RecipientTypeConsole).Error; err != nil {
		t.Fatalf("load console debug recipient: %v", err)
	}
	if consoleRecipient.AccessToken != debugToken {
		t.Fatalf("console debug token = %q, want %q", consoleRecipient.AccessToken, debugToken)
	}
}

func TestPersistApprovalResumeCompletionAddsConversationMessageEventsBeforeFinish(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	run := &WorkflowRunLog{
		ID:         uuid.NewString(),
		WorkflowID: uuid.NewString(),
		TenantID:   uuid.NewString(),
		AgentID:    uuid.NewString(),
		CreatedBy:  uuid.NewString(),
	}
	conversationID := uuid.NewString()
	outputs := map[string]interface{}{
		"answer":                       "approval accepted",
		workflowInternalElapsedTimeKey: 26.5,
	}
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.query":           "please approve",
		"sys.user_id":         run.CreatedBy,
	}
	pauseService := workflowpause.NewService(db)
	messageService := &workflowApprovalRuntimeMessageService{}

	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}
	handler.persistApprovalResumeCompletion(ctx, pauseService, nil, run, outputs, time.Now(), "CONVERSATION_WORKFLOW", systemInputs, nil, false, false)

	payload, err := pauseService.ListEvents(ctx, run.TenantID, run.ID, 0, 10)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(payload.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(payload.Events))
	}
	if payload.Events[0].Event != "message" {
		t.Fatalf("first event = %s, want message", payload.Events[0].Event)
	}
	if payload.Events[1].Event != "message_end" {
		t.Fatalf("second event = %s, want message_end", payload.Events[1].Event)
	}
	if payload.Events[2].Event != workflowpause.EventWorkflowFinished {
		t.Fatalf("third event = %s, want workflow_finished", payload.Events[2].Event)
	}
	if got := payload.Events[0].Data["answer"]; got != "approval accepted" {
		t.Fatalf("message answer = %#v, want approval accepted", got)
	}
	if got := payload.Events[0].Data["conversation_id"]; got != conversationID {
		t.Fatalf("message conversation_id = %#v, want %s", got, conversationID)
	}
	finishedOutputs, ok := payload.Events[2].Data["outputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("workflow_finished outputs type = %T, want map", payload.Events[2].Data["outputs"])
	}
	if got := finishedOutputs["answer"]; got != "approval accepted" {
		t.Fatalf("workflow_finished answer = %#v, want approval accepted", got)
	}
	if _, exists := finishedOutputs[workflowInternalElapsedTimeKey]; exists {
		t.Fatalf("workflow_finished outputs leaked internal elapsed key")
	}
	if got := payload.Events[2].Data["elapsed_time"]; got != 26.5 {
		t.Fatalf("workflow_finished elapsed_time = %#v, want 26.5", got)
	}

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	if messageService.messages[0].ConversationID.String() != conversationID {
		t.Fatalf("persisted conversation_id = %s, want %s", messageService.messages[0].ConversationID.String(), conversationID)
	}
	if messageService.messages[0].Query != "please approve" {
		t.Fatalf("persisted query = %q, want please approve", messageService.messages[0].Query)
	}
	if messageService.messages[0].Answer != "approval accepted" {
		t.Fatalf("persisted answer = %q, want approval accepted", messageService.messages[0].Answer)
	}
	if messageService.messages[0].Status != conversation.AgentMessageStatusCompleted {
		t.Fatalf("persisted status = %q, want %s", messageService.messages[0].Status, conversation.AgentMessageStatusCompleted)
	}

	handler.persistApprovalResumeCompletion(ctx, pauseService, nil, run, outputs, time.Now(), "CONVERSATION_WORKFLOW", systemInputs, nil, true, false)
	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages after retry = %d, want 1", len(messageService.messages))
	}
}

func TestPersistApprovalResumeCompletionPersistsPauseRequestInputs(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	runInputs := `{"query":"stale question","conversation_params":{"from_source":"account","invoke_from":"workflow"}}`
	run := &WorkflowRunLog{
		ID:         uuid.NewString(),
		WorkflowID: uuid.NewString(),
		TenantID:   uuid.NewString(),
		AgentID:    uuid.NewString(),
		CreatedBy:  uuid.NewString(),
		Inputs:     &runInputs,
	}
	conversationID := uuid.NewString()
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.user_id":         run.CreatedBy,
	}
	resumeInputs := map[string]interface{}{
		"query": "fresh question",
		"conversation_params": map[string]interface{}{
			"from_source": "end_user",
			"invoke_from": "web-app",
		},
		"form_value": "kept",
	}
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}

	handler.persistApprovalResumeCompletion(ctx, workflowpause.NewService(db), nil, run, map[string]interface{}{"answer": "done"}, time.Now(), "CONVERSATION_WORKFLOW", systemInputs, resumeInputs, false, false)

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	message := messageService.messages[0]
	if message.Query != "fresh question" {
		t.Fatalf("message query = %q, want fresh question", message.Query)
	}
	if message.FromSource != "end_user" {
		t.Fatalf("message from_source = %q, want end_user", message.FromSource)
	}
	if message.InvokeFrom == nil || *message.InvokeFrom != "web-app" {
		t.Fatalf("message invoke_from = %v, want web-app", message.InvokeFrom)
	}
	inputs, err := message.GetInputsAsMap()
	if err != nil {
		t.Fatalf("decode message inputs: %v", err)
	}
	if got := inputs["query"]; got != "fresh question" {
		t.Fatalf("message inputs query = %#v, want fresh question", got)
	}
	if got := inputs["form_value"]; got != "kept" {
		t.Fatalf("message inputs form_value = %#v, want kept", got)
	}
	if message.Status != conversation.AgentMessageStatusCompleted {
		t.Fatalf("message status = %q, want %s", message.Status, conversation.AgentMessageStatusCompleted)
	}
}

func TestPersistApprovalPauseConversationMessageCreatesReplayableMessage(t *testing.T) {
	ctx := context.Background()
	runID := uuid.NewString()
	agentID := uuid.NewString()
	accountID := uuid.NewString()
	conversationID := uuid.NewString()
	webAppID := uuid.NewString()
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.query":           "hello",
		"sys.user_id":         accountID,
	}
	requestInputs := map[string]interface{}{
		"conversation_params": map[string]interface{}{
			"from_source": "end_user",
			"invoke_from": "web-app",
		},
		"sys.web_app_id": webAppID,
		"form_value":     "hospital question",
	}
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}

	handler.persistApprovalPauseConversationMessage(ctx, runID, agentID, accountID, systemInputs, requestInputs, string(InvokeFromWebApp), "")
	handler.persistApprovalPauseConversationMessage(ctx, runID, agentID, accountID, systemInputs, requestInputs, string(InvokeFromWebApp), "")

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	message := messageService.messages[0]
	if message.WorkflowRunID == nil || message.WorkflowRunID.String() != runID {
		t.Fatalf("workflow_run_id = %v, want %s", message.WorkflowRunID, runID)
	}
	if message.ConversationID.String() != conversationID {
		t.Fatalf("conversation_id = %s, want %s", message.ConversationID.String(), conversationID)
	}
	if message.Query != "hello" {
		t.Fatalf("query = %q, want hello", message.Query)
	}
	if message.Answer != "" {
		t.Fatalf("answer = %q, want empty while paused", message.Answer)
	}
	if message.Status != conversation.AgentMessageStatusPendingApproval {
		t.Fatalf("status = %q, want %s", message.Status, conversation.AgentMessageStatusPendingApproval)
	}
	if message.FromSource != "end_user" {
		t.Fatalf("from_source = %q, want end_user", message.FromSource)
	}
	if message.InvokeFrom == nil || *message.InvokeFrom != "web-app" {
		t.Fatalf("invoke_from = %v, want web-app", message.InvokeFrom)
	}
	if message.WebAppID == nil || *message.WebAppID != webAppID {
		t.Fatalf("web_app_id = %v, want %s", message.WebAppID, webAppID)
	}
	inputs, err := message.GetInputsAsMap()
	if err != nil {
		t.Fatalf("decode message inputs: %v", err)
	}
	if got := inputs["form_value"]; got != "hospital question" {
		t.Fatalf("message inputs form_value = %#v, want hospital question", got)
	}
}

func TestPersistApprovalPauseConversationMessageStoresDisplayedAnswer(t *testing.T) {
	ctx := context.Background()
	runID := uuid.NewString()
	agentID := uuid.NewString()
	accountID := uuid.NewString()
	conversationID := uuid.NewString()
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.query":           "hello",
		"sys.user_id":         accountID,
	}
	requestInputs := map[string]interface{}{
		"conversation_params": map[string]interface{}{
			"from_source": "end_user",
			"invoke_from": "web-app",
		},
	}
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}

	handler.persistApprovalPauseConversationMessage(ctx, runID, agentID, accountID, systemInputs, requestInputs, string(InvokeFromWebApp), "before approval")

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	message := messageService.messages[0]
	if message.Answer != "before approval" {
		t.Fatalf("answer = %q, want before approval", message.Answer)
	}
	if message.Status != conversation.AgentMessageStatusPendingApproval {
		t.Fatalf("status = %q, want %s", message.Status, conversation.AgentMessageStatusPendingApproval)
	}
}

func TestPersistWorkflowConversationAnswerSnapshotOverwritesSameMessage(t *testing.T) {
	ctx := context.Background()
	runID := uuid.NewString()
	agentID := uuid.NewString()
	accountID := uuid.NewString()
	conversationID := uuid.NewString()
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.query":           "hello",
		"sys.user_id":         accountID,
	}
	requestInputs := map[string]interface{}{
		"conversation_params": map[string]interface{}{
			"from_source": "end_user",
			"invoke_from": "web-app",
		},
	}
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}

	writer := newAnswerSnapshotWriter(handler, runID, agentID, accountID, systemInputs, requestInputs, string(InvokeFromWebApp))
	writer.Persist(ctx, "65", conversation.AgentMessageStatusRunning, false)
	writer.Persist(ctx, "6513", conversation.AgentMessageStatusCompleted, true)

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	message := messageService.messages[0]
	if message.Answer != "6513" {
		t.Fatalf("answer = %q, want 6513", message.Answer)
	}
	if message.Status != conversation.AgentMessageStatusCompleted {
		t.Fatalf("status = %q, want %s", message.Status, conversation.AgentMessageStatusCompleted)
	}
}

func TestPersistApprovalResumeCompletionUpdatesPausedConversationMessage(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	run := &WorkflowRunLog{
		ID:            uuid.NewString(),
		WorkflowID:    uuid.NewString(),
		TenantID:      uuid.NewString(),
		AgentID:       uuid.NewString(),
		CreatedBy:     uuid.NewString(),
		TriggeredFrom: string(InvokeFromWebApp),
	}
	conversationID := uuid.NewString()
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.query":           "hello",
		"sys.user_id":         run.CreatedBy,
	}
	requestInputs := map[string]interface{}{
		"conversation_params": map[string]interface{}{
			"from_source": "end_user",
			"invoke_from": "web-app",
		},
		"form_value": "hospital question",
	}
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}

	handler.persistApprovalPauseConversationMessage(ctx, run.ID, run.AgentID, run.CreatedBy, systemInputs, requestInputs, string(InvokeFromWebApp), "before approval ")
	handler.persistApprovalResumeCompletion(ctx, workflowpause.NewService(db), nil, run, map[string]interface{}{"answer": "approved"}, time.Now(), "CONVERSATION_WORKFLOW", systemInputs, requestInputs, false, false)

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	message := messageService.messages[0]
	if message.Answer != "before approval approved" {
		t.Fatalf("answer = %q, want before approval approved", message.Answer)
	}
	if message.Query != "hello" {
		t.Fatalf("query = %q, want hello", message.Query)
	}
	if message.Status != conversation.AgentMessageStatusCompleted {
		t.Fatalf("status = %q, want %s", message.Status, conversation.AgentMessageStatusCompleted)
	}
	inputs, err := message.GetInputsAsMap()
	if err != nil {
		t.Fatalf("decode message inputs: %v", err)
	}
	if got := inputs["form_value"]; got != "hospital question" {
		t.Fatalf("message inputs form_value = %#v, want hospital question", got)
	}
}

func TestPersistApprovalResumeCompletionSplitsEventDeltaFromStoredAnswer(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	run := &WorkflowRunLog{
		ID:            uuid.NewString(),
		WorkflowID:    uuid.NewString(),
		TenantID:      uuid.NewString(),
		AgentID:       uuid.NewString(),
		CreatedBy:     uuid.NewString(),
		TriggeredFrom: string(InvokeFromWebApp),
	}
	conversationID := uuid.NewString()
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.query":           "hello",
		"sys.user_id":         run.CreatedBy,
	}
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}
	pauseService := workflowpause.NewService(db)

	handler.persistApprovalPauseConversationMessage(ctx, run.ID, run.AgentID, run.CreatedBy, systemInputs, nil, string(InvokeFromWebApp), "65")
	handler.persistApprovalResumeCompletion(ctx, pauseService, nil, run, map[string]interface{}{"answer": "6565"}, time.Now(), "CONVERSATION_WORKFLOW", systemInputs, nil, false, false)

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	if got := messageService.messages[0].Answer; got != "6565" {
		t.Fatalf("stored answer = %q, want 6565", got)
	}

	payload, err := pauseService.ListEvents(ctx, run.TenantID, run.ID, 0, 10)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(payload.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(payload.Events))
	}
	if payload.Events[0].Event != workflowEventMessage {
		t.Fatalf("first event = %s, want message", payload.Events[0].Event)
	}
	if got := payload.Events[0].Data["answer"]; got != "65" {
		t.Fatalf("message event answer = %#v, want only resume delta 65", got)
	}
	finishedOutputs, ok := payload.Events[2].Data["outputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("workflow_finished outputs type = %T, want map", payload.Events[2].Data["outputs"])
	}
	if got := finishedOutputs["answer"]; got != "6565" {
		t.Fatalf("workflow_finished answer = %#v, want 6565", got)
	}
}

func TestApprovalResumeAnswerHelpersAvoidDuplicateFullAnswer(t *testing.T) {
	if got := approvalResumeMessageEventAnswer("6565", "65"); got != "65" {
		t.Fatalf("message event delta = %q, want 65", got)
	}
	if got := mergeApprovalConversationAnswer("65", "6565"); got != "6565" {
		t.Fatalf("merged full answer = %q, want 6565", got)
	}
	if got := mergeApprovalConversationAnswer("before approval ", "approved"); got != "before approval approved" {
		t.Fatalf("merged delta answer = %q, want before approval approved", got)
	}
}

func TestPersistApprovalResumeCompletionMarksExpiredMessage(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	run := &WorkflowRunLog{
		ID:            uuid.NewString(),
		WorkflowID:    uuid.NewString(),
		TenantID:      uuid.NewString(),
		AgentID:       uuid.NewString(),
		CreatedBy:     uuid.NewString(),
		TriggeredFrom: string(InvokeFromWebApp),
	}
	conversationID := uuid.NewString()
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.query":           "hello",
		"sys.user_id":         run.CreatedBy,
	}
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}

	handler.persistApprovalPauseConversationMessage(ctx, run.ID, run.AgentID, run.CreatedBy, systemInputs, nil, string(InvokeFromWebApp), "")
	handler.persistApprovalResumeCompletion(ctx, workflowpause.NewService(db), nil, run, map[string]interface{}{"answer": "expired branch answer"}, time.Now(), "CONVERSATION_WORKFLOW", systemInputs, nil, false, true)

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	message := messageService.messages[0]
	if message.Answer != "expired branch answer" {
		t.Fatalf("answer = %q, want expired branch answer", message.Answer)
	}
	if message.Status != conversation.AgentMessageStatusExpired {
		t.Fatalf("status = %q, want %s", message.Status, conversation.AgentMessageStatusExpired)
	}
}

func TestPersistApprovalResumeErrorMarksMessageError(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	run := &WorkflowRunLog{
		ID:            uuid.NewString(),
		WorkflowID:    uuid.NewString(),
		TenantID:      uuid.NewString(),
		AgentID:       uuid.NewString(),
		CreatedBy:     uuid.NewString(),
		TriggeredFrom: string(InvokeFromWebApp),
	}
	conversationID := uuid.NewString()
	systemInputs := map[string]interface{}{
		"sys.conversation_id": conversationID,
		"sys.query":           "hello",
		"sys.user_id":         run.CreatedBy,
	}
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}

	handler.persistApprovalPauseConversationMessage(ctx, run.ID, run.AgentID, run.CreatedBy, systemInputs, nil, string(InvokeFromWebApp), "")
	handler.persistApprovalResumeError(ctx, workflowpause.NewService(db), nil, run, errors.New("resume failed"), time.Now())

	if len(messageService.messages) != 1 {
		t.Fatalf("persisted messages = %d, want 1", len(messageService.messages))
	}
	message := messageService.messages[0]
	if message.Status != conversation.AgentMessageStatusError {
		t.Fatalf("status = %q, want %s", message.Status, conversation.AgentMessageStatusError)
	}
	if message.Error == nil || *message.Error == "" {
		t.Fatalf("message error should be set")
	}
}

func TestWorkflowStatusToMessageStatus(t *testing.T) {
	cases := []struct {
		name           string
		workflowStatus string
		want           string
	}{
		{name: "succeeded", workflowStatus: "succeeded", want: conversation.AgentMessageStatusCompleted},
		{name: "failed", workflowStatus: "failed", want: conversation.AgentMessageStatusError},
		{name: "stopped", workflowStatus: "stopped", want: conversation.AgentMessageStatusStopped},
		{name: "paused", workflowStatus: "paused", want: conversation.AgentMessageStatusPendingApproval},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := workflowStatusToMessageStatus(tc.workflowStatus); got != tc.want {
				t.Fatalf("status = %q, want %s", got, tc.want)
			}
		})
	}
}

func TestPersistApprovalResumeCompletionUsesRunInputsConversationIDFallback(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	inputs := `{"sys.conversation_id":"conversation-from-run"}`
	run := &WorkflowRunLog{
		ID:         "run-" + uuid.NewString(),
		WorkflowID: uuid.NewString(),
		TenantID:   uuid.NewString(),
		AgentID:    uuid.NewString(),
		CreatedBy:  uuid.NewString(),
		Inputs:     &inputs,
	}
	outputs := map[string]interface{}{"answer": "approval accepted"}
	pauseService := workflowpause.NewService(db)

	handler := &WorkflowHandler{}
	handler.persistApprovalResumeCompletion(ctx, pauseService, nil, run, outputs, time.Now(), "CONVERSATION_WORKFLOW", nil, nil, false, false)

	payload, err := pauseService.ListEvents(ctx, run.TenantID, run.ID, 0, 10)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if got := payload.Events[0].Data["conversation_id"]; got != "conversation-from-run" {
		t.Fatalf("message conversation_id = %#v, want conversation-from-run", got)
	}
}

func TestApprovalResumeStoredConversationIDUsesWorkflowStartedEvent(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	run := &WorkflowRunLog{
		ID:       uuid.NewString(),
		TenantID: uuid.NewString(),
		AgentID:  uuid.NewString(),
	}
	conversationID := uuid.NewString()
	pauseService := workflowpause.NewService(db)
	if err := pauseService.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      run.TenantID,
		AppID:         run.AgentID,
		WorkflowRunID: run.ID,
		EventType:     workflowpause.EventWorkflowStarted,
		EventData: map[string]interface{}{
			"id":              run.ID,
			"conversation_id": conversationID,
			"inputs":          map[string]interface{}{},
		},
	}); err != nil {
		t.Fatalf("append workflow_started event: %v", err)
	}

	if got := approvalResumeStoredConversationID(ctx, pauseService, run); got != conversationID {
		t.Fatalf("stored conversation id = %q, want %s", got, conversationID)
	}
}

func TestPersistApprovalResumeCompletionSkipsConversationMessageForWorkflowRun(t *testing.T) {
	ctx := context.Background()
	db := newWorkflowApprovalRuntimeTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	run := &WorkflowRunLog{
		ID:         uuid.NewString(),
		WorkflowID: uuid.NewString(),
		TenantID:   uuid.NewString(),
		AgentID:    uuid.NewString(),
		CreatedBy:  uuid.NewString(),
	}
	pauseService := workflowpause.NewService(db)
	messageService := &workflowApprovalRuntimeMessageService{}
	handler := &WorkflowHandler{advancedChatHandler: &AdvancedChatWorkflowHandler{messageService: messageService}}

	handler.persistApprovalResumeCompletion(ctx, pauseService, nil, run, map[string]interface{}{"answer": "done"}, time.Now(), "WORKFLOW", nil, nil, false, false)

	if len(messageService.messages) != 0 {
		t.Fatalf("persisted messages = %d, want 0", len(messageService.messages))
	}
	payload, err := pauseService.ListEvents(ctx, run.TenantID, run.ID, 0, 10)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(payload.Events))
	}
	if payload.Events[0].Event != workflowpause.EventWorkflowFinished {
		t.Fatalf("event = %s, want workflow_finished", payload.Events[0].Event)
	}
}

func assertApprovalSubmitMethods(t *testing.T, event map[string]interface{}, wantWebApp, wantEmail bool) {
	t.Helper()

	methods, ok := event["submit_methods"].(map[string]interface{})
	if !ok {
		t.Fatalf("submit_methods type = %T, want map", event["submit_methods"])
	}
	webapp, ok := methods["webapp"].(map[string]interface{})
	if !ok {
		t.Fatalf("webapp submit method type = %T, want map", methods["webapp"])
	}
	email, ok := methods["email"].(map[string]interface{})
	if !ok {
		t.Fatalf("email submit method type = %T, want map", methods["email"])
	}
	if got := webapp["enabled"]; got != wantWebApp {
		t.Fatalf("webapp.enabled = %#v, want %v", got, wantWebApp)
	}
	if got := email["enabled"]; got != wantEmail {
		t.Fatalf("email.enabled = %#v, want %v", got, wantEmail)
	}
}

func newWorkflowApprovalRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "go-sqlite3 requires cgo") || strings.Contains(err.Error(), "CGO_ENABLED=0") {
			t.Skipf("sqlite test db unavailable in current environment: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&approvalruntime.Form{},
		&approvalruntime.Delivery{},
		&approvalruntime.Recipient{},
		&workflowpause.RunPause{},
		&workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{},
	); err != nil {
		t.Fatalf("auto migrate workflow approval runtime tables: %v", err)
	}
	return db
}

type workflowApprovalRuntimeMessageService struct {
	messages []*conversation.AgentMessage
}

func (s *workflowApprovalRuntimeMessageService) CreateMessage(context.Context, *conversation.CreateMessageRequest) (*conversation.AgentMessage, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *workflowApprovalRuntimeMessageService) GetMessage(context.Context, uuid.UUID) (*conversation.AgentMessage, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *workflowApprovalRuntimeMessageService) GetMessagesByConversation(context.Context, uuid.UUID, int, int) ([]*conversation.AgentMessage, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *workflowApprovalRuntimeMessageService) UpdateMessage(_ context.Context, updated *conversation.AgentMessage) error {
	for i, message := range s.messages {
		if message != nil && updated != nil && message.ID == updated.ID {
			s.messages[i] = updated
			return nil
		}
	}
	return fmt.Errorf("message not found")
}

func (s *workflowApprovalRuntimeMessageService) DeleteMessage(context.Context, uuid.UUID, uuid.UUID) error {
	return fmt.Errorf("not implemented")
}

func (s *workflowApprovalRuntimeMessageService) GetMessagesByUser(context.Context, uuid.UUID, string, uuid.UUID, int, int) ([]*conversation.AgentMessage, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

func (s *workflowApprovalRuntimeMessageService) GetLatestMessageByConversation(context.Context, uuid.UUID) (*conversation.AgentMessage, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *workflowApprovalRuntimeMessageService) GetMessagesByWorkflowRun(ctx context.Context, workflowRunID uuid.UUID) ([]*conversation.AgentMessage, error) {
	return s.messagesByWorkflowRun(workflowRunID), nil
}

func (s *workflowApprovalRuntimeMessageService) GetFirstMessagesByWorkflowRunIDs(ctx context.Context, workflowRunIDs []string) (map[string]*conversation.AgentMessage, error) {
	result := make(map[string]*conversation.AgentMessage)
	for _, runID := range workflowRunIDs {
		parsed, err := uuid.Parse(runID)
		if err != nil {
			continue
		}
		messages := s.messagesByWorkflowRun(parsed)
		if len(messages) > 0 {
			result[runID] = messages[0]
		}
	}
	return result, nil
}

func (s *workflowApprovalRuntimeMessageService) UpdateMessageStatus(_ context.Context, id uuid.UUID, status string, messageError *string) error {
	for _, message := range s.messages {
		if message != nil && message.ID == id {
			message.Status = status
			message.Error = messageError
			return nil
		}
	}
	return fmt.Errorf("message not found")
}

func (s *workflowApprovalRuntimeMessageService) CreateWorkflowMessage(ctx context.Context, req *conversation.CreateWorkflowMessageRequest) (*conversation.AgentMessage, error) {
	inputsJSON, err := json.Marshal(req.Inputs)
	if err != nil {
		return nil, err
	}
	invokeFrom := req.InvokeFrom
	status := req.Status
	if status == "" {
		status = conversation.AgentMessageStatusCompleted
	}
	message := &conversation.AgentMessage{
		ID:             uuid.New(),
		AgentID:        req.AgentID,
		ConversationID: req.ConversationID,
		Inputs:         string(inputsJSON),
		Query:          req.Query,
		Answer:         req.Answer,
		Status:         status,
		WorkflowRunID:  &req.WorkflowRunID,
		FromSource:     req.FromSource,
		InvokeFrom:     &invokeFrom,
		CreatedBy:      req.CreatedBy,
		WebAppID:       req.WebAppID,
	}
	s.messages = append(s.messages, message)
	return message, nil
}

func (s *workflowApprovalRuntimeMessageService) GetConversationMessages(context.Context, uuid.UUID) ([]*conversation.AgentMessage, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *workflowApprovalRuntimeMessageService) messagesByWorkflowRun(workflowRunID uuid.UUID) []*conversation.AgentMessage {
	result := make([]*conversation.AgentMessage, 0)
	for _, message := range s.messages {
		if message == nil || message.WorkflowRunID == nil || *message.WorkflowRunID != workflowRunID {
			continue
		}
		result = append(result, message)
	}
	return result
}

func swapWorkflowRuntimeTestDB(db *gorm.DB) func() {
	old := database.GetDB()
	database.SetDB(db)
	return func() {
		database.SetDB(old)
	}
}
