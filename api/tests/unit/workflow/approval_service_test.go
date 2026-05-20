package workflow_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	approvalruntime "github.com/zgiai/ginext/internal/modules/app/workflow/approval"
	workflowpause "github.com/zgiai/ginext/internal/modules/app/workflow/pause"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestApprovalServiceCreateSubmitAndRejectDuplicate(t *testing.T) {
	ctx := context.Background()
	db := newApprovalServiceTestDB(t)
	service := approvalruntime.NewService(db)

	form := createApprovalForm(t, ctx, service, "node-approval", "run-"+uuid.NewString())

	if form.Payload.Token == "" {
		t.Fatal("expected webapp token")
	}
	if len(form.Payload.Token) != 8 {
		t.Fatalf("webapp token length = %d, want 8", len(form.Payload.Token))
	}
	if form.Payload.Content != "Please review value" {
		t.Fatalf("payload content = %q, want rendered content", form.Payload.Content)
	}
	if form.Payload.SubmitMethods.WebApp.Enabled == nil || !*form.Payload.SubmitMethods.WebApp.Enabled {
		t.Fatalf("webapp submit method should be enabled in form payload")
	}

	payload, err := service.GetFormByToken(ctx, form.Payload.Token)
	if err != nil {
		t.Fatalf("GetFormByToken returned error: %v", err)
	}
	if payload.ID != form.Form.ID {
		t.Fatalf("payload id = %s, want %s", payload.ID, form.Form.ID)
	}

	submitted, err := service.SubmitByToken(ctx, form.Payload.Token, approvalruntime.SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{
			"comment": "Looks good",
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("SubmitByToken returned error: %v", err)
	}
	if submitted.Status != approvalruntime.FormStatusSubmitted {
		t.Fatalf("submitted status = %s, want submitted", submitted.Status)
	}
	if submitted.SelectedActionID == nil || *submitted.SelectedActionID != "approve" {
		t.Fatalf("submitted action = %v, want approve", submitted.SelectedActionID)
	}

	_, err = service.SubmitByToken(ctx, form.Payload.Token, approvalruntime.SubmitRequest{
		Action: "reject",
		Inputs: map[string]interface{}{
			"comment": "No",
		},
	}, nil, nil)
	if !errors.Is(err, approvalruntime.ErrFormAlreadySubmitted) {
		t.Fatalf("duplicate submit error = %v, want ErrFormAlreadySubmitted", err)
	}
}

func TestApprovalServiceRejectsExpiredActionID(t *testing.T) {
	err := approvalruntime.ValidateConfig(approvalruntime.NodeConfig{
		Content: "review",
		Actions: []approvalruntime.Action{
			{ID: approvalruntime.ActionExpired, Label: "Expired"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("ValidateConfig error = %v, want reserved action id error", err)
	}
}

func TestApprovalServiceTimeoutExpiredForms(t *testing.T) {
	ctx := context.Background()
	db := newApprovalServiceTestDB(t)
	service := approvalruntime.NewService(db)

	form := createApprovalForm(t, ctx, service, "node-timeout", "run-"+uuid.NewString())
	if err := db.Model(&approvalruntime.Form{}).
		Where("id = ?", form.Form.ID).
		Updates(map[string]interface{}{
			"expiration_time": time.Now().Add(-time.Hour),
			"status":          approvalruntime.FormStatusWaiting,
		}).Error; err != nil {
		t.Fatalf("expire form: %v", err)
	}

	forms, err := service.TimeoutExpiredForms(ctx, 10)
	if err != nil {
		t.Fatalf("TimeoutExpiredForms returned error: %v", err)
	}
	if len(forms) != 1 {
		t.Fatalf("timed out forms = %d, want 1", len(forms))
	}
	if forms[0].Status != approvalruntime.FormStatusTimeout {
		t.Fatalf("timed out status = %s, want timeout", forms[0].Status)
	}
}

func TestApprovalServiceEmailDeliveryRecordsSentAt(t *testing.T) {
	ctx := context.Background()
	db := newApprovalServiceTestDB(t)
	sender := &fakeApprovalEmailSender{}
	service := approvalruntime.NewServiceWithEmailSender(db, sender)

	form := createApprovalEmailForm(t, ctx, service, "node-email", "run-"+uuid.NewString())

	if len(sender.messages) != 1 {
		t.Fatalf("sent emails = %d, want 1", len(sender.messages))
	}
	message := sender.messages[0]
	if len(message.to) != 1 || message.to[0] != "approver@example.com" {
		t.Fatalf("email recipients = %#v, want approver@example.com", message.to)
	}
	if message.subject != "Approval Request" {
		t.Fatalf("email subject = %q, want sanitized subject", message.subject)
	}
	if !strings.Contains(message.body, "/a/") ||
		strings.Contains(message.body, "/approval/forms/") ||
		strings.Contains(message.body, "{{#url#}}") {
		t.Fatalf("email body should contain rendered short approval link, got %q", message.body)
	}

	var delivery approvalruntime.Delivery
	if err := db.First(&delivery, "form_id = ? AND delivery_method_type = ?", form.Form.ID, approvalruntime.DeliveryTypeEmail).Error; err != nil {
		t.Fatalf("load email delivery: %v", err)
	}
	if delivery.SentAt == nil {
		t.Fatal("email delivery sent_at should be set")
	}
	if delivery.LastError != nil {
		t.Fatalf("email delivery last_error = %q, want empty", *delivery.LastError)
	}

	var recipient approvalruntime.Recipient
	if err := db.First(&recipient, "form_id = ? AND recipient_type = ?", form.Form.ID, approvalruntime.RecipientTypeEmailExternal).Error; err != nil {
		t.Fatalf("load email recipient: %v", err)
	}
	if recipient.AccessToken == "" || recipient.AccessToken == form.Payload.Token {
		t.Fatalf("email recipient token should be present and independent from web token")
	}
	if len(recipient.AccessToken) != 8 {
		t.Fatalf("email recipient token length = %d, want 8", len(recipient.AccessToken))
	}
	payload, err := service.GetFormByToken(ctx, recipient.AccessToken)
	if err != nil {
		t.Fatalf("GetFormByToken with email token returned error: %v", err)
	}
	if payload.ID != form.Form.ID || payload.Token != recipient.AccessToken {
		t.Fatalf("email token payload = %#v, want form %s and email token", payload, form.Form.ID)
	}
}

func TestApprovalServiceEmailDeliveryRecordsFailureWithoutFailingFormCreation(t *testing.T) {
	ctx := context.Background()
	db := newApprovalServiceTestDB(t)
	sender := &fakeApprovalEmailSender{err: errors.New("recipient domain not verified")}
	service := approvalruntime.NewServiceWithEmailSender(db, sender)

	form := createApprovalEmailForm(t, ctx, service, "node-email-failure", "run-"+uuid.NewString())

	if len(sender.messages) != 1 {
		t.Fatalf("sent emails = %d, want 1 attempted email", len(sender.messages))
	}
	var stored approvalruntime.Form
	if err := db.First(&stored, "id = ?", form.Form.ID).Error; err != nil {
		t.Fatalf("load form: %v", err)
	}
	if stored.Status != approvalruntime.FormStatusWaiting {
		t.Fatalf("form status = %s, want waiting after email failure", stored.Status)
	}

	var delivery approvalruntime.Delivery
	if err := db.First(&delivery, "form_id = ? AND delivery_method_type = ?", form.Form.ID, approvalruntime.DeliveryTypeEmail).Error; err != nil {
		t.Fatalf("load email delivery: %v", err)
	}
	if delivery.SentAt != nil {
		t.Fatal("email delivery sent_at should not be set after failure")
	}
	if delivery.LastError == nil || *delivery.LastError != "recipient domain not verified" {
		t.Fatalf("email delivery last_error = %v, want sender error", delivery.LastError)
	}
}

func TestApprovalServiceListRunEventsByTokenReturnsEventsAfterPause(t *testing.T) {
	ctx := context.Background()
	db := newApprovalServiceTestDB(t)
	service := approvalruntime.NewService(db)

	runID := "run-" + uuid.NewString()
	form := createApprovalForm(t, ctx, service, "node-events", runID)
	pauseService := workflowpause.NewService(db)

	if err := pauseService.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      form.Form.TenantID,
		AppID:         form.Form.AppID,
		WorkflowRunID: runID,
		EventType:     "node_started",
		EventData:     map[string]interface{}{"node_id": "start"},
	}); err != nil {
		t.Fatalf("append pre-pause event: %v", err)
	}
	if err := pauseService.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      form.Form.TenantID,
		AppID:         form.Form.AppID,
		WorkflowRunID: runID,
		EventType:     "workflow_paused",
		EventData:     map[string]interface{}{"node_id": "node-events"},
	}); err != nil {
		t.Fatalf("append pause event: %v", err)
	}
	if err := pauseService.AppendEvent(ctx, workflowpause.AppendEventParams{
		TenantID:      form.Form.TenantID,
		AppID:         form.Form.AppID,
		WorkflowRunID: runID,
		EventType:     "node_finished",
		EventData: map[string]interface{}{
			"node_id": "answer",
			"status":  "succeeded",
			"inputs": map[string]interface{}{
				"comment":                      "ok",
				"sys.workflow_resume_state":    map[string]interface{}{"workflow_run_id": runID},
				"sys.workflow_resume_pause_id": "pause-1",
				"__approval_form":              map[string]interface{}{"token": "secret"},
				"__approval_form_id":           "form-1",
				"__approval_token":             "secret",
			},
		},
	}); err != nil {
		t.Fatalf("append resume event: %v", err)
	}

	payload, err := service.ListRunEventsByToken(ctx, form.Payload.Token, 0, 10)
	if err != nil {
		t.Fatalf("ListRunEventsByToken returned error: %v", err)
	}
	if payload.WorkflowRunID != runID {
		t.Fatalf("workflow run id = %s, want %s", payload.WorkflowRunID, runID)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(payload.Events))
	}
	if payload.Events[0].Event != "node_finished" {
		t.Fatalf("event = %s, want node_finished", payload.Events[0].Event)
	}
	if payload.Events[0].Data["node_id"] != "answer" {
		t.Fatalf("node_id = %v, want answer", payload.Events[0].Data["node_id"])
	}
	inputs, ok := payload.Events[0].Data["inputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("inputs type = %T, want map", payload.Events[0].Data["inputs"])
	}
	for _, key := range []string{"sys.workflow_resume_state", "sys.workflow_resume_pause_id", "__approval_form", "__approval_form_id", "__approval_token"} {
		if _, exists := inputs[key]; exists {
			t.Fatalf("sensitive key %s should be removed from event inputs: %#v", key, inputs)
		}
	}
	if got := inputs["comment"]; got != "ok" {
		t.Fatalf("inputs[comment] = %#v, want ok", got)
	}

	empty, err := service.ListRunEventsByToken(ctx, form.Payload.Token, payload.Events[0].Sequence, 10)
	if err != nil {
		t.Fatalf("ListRunEventsByToken after last sequence returned error: %v", err)
	}
	if len(empty.Events) != 0 {
		t.Fatalf("events after last sequence = %d, want 0", len(empty.Events))
	}
}

func TestApprovalServiceActivePauseRequiresAllApprovalFormsSubmitted(t *testing.T) {
	ctx := context.Background()
	db := newApprovalServiceTestDB(t)
	service := approvalruntime.NewService(db)
	pauseService := workflowpause.NewService(db)

	runID := "run-" + uuid.NewString()
	tenantID := uuid.NewString()
	appID := uuid.NewString()
	formA := createApprovalFormForRun(t, ctx, service, tenantID, appID, "approval-a", runID)
	formB := createApprovalFormForRun(t, ctx, service, tenantID, appID, "approval-b", runID)

	state := workflowpause.State{
		Version:       workflowpause.StateVersion,
		WorkflowRunID: runID,
		AppID:         appID,
		TenantID:      tenantID,
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID:  "approval-a",
			PausedNodeIDs: []string{"approval-a", "approval-b"},
		},
	}
	if _, err := pauseService.Save(ctx, workflowpause.SaveParams{
		TenantID:      tenantID,
		AppID:         appID,
		WorkflowRunID: runID,
		NodeID:        "approval-a",
		Reason:        workflowpause.ReasonTypeApprovalRequired,
		State:         state,
		Reasons: []workflowpause.Reason{
			{Type: workflowpause.ReasonTypeApprovalRequired, NodeID: "approval-a", FormID: formA.Form.ID},
			{Type: workflowpause.ReasonTypeApprovalRequired, NodeID: "approval-b", FormID: formB.Form.ID},
		},
	}); err != nil {
		t.Fatalf("Save pause returned error: %v", err)
	}

	ready, err := service.ActivePauseApprovalFormsSubmitted(ctx, runID)
	if err != nil {
		t.Fatalf("ActivePauseApprovalFormsSubmitted returned error: %v", err)
	}
	if ready {
		t.Fatal("ready = true, want false before approval forms are submitted")
	}

	submittedA, err := service.SubmitByToken(ctx, formA.Payload.Token, approvalruntime.SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{"comment": "A ok"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("SubmitByToken(A) returned error: %v", err)
	}
	ready, err = service.ActivePauseApprovalFormsSubmitted(ctx, runID)
	if err != nil {
		t.Fatalf("ActivePauseApprovalFormsSubmitted after A returned error: %v", err)
	}
	if ready {
		t.Fatal("ready = true, want false while approval-b is still waiting")
	}
	if err := service.AppendApprovalResultFilledEvent(ctx, submittedA); err != nil {
		t.Fatalf("AppendApprovalResultFilledEvent returned error: %v", err)
	}
	events, err := pauseService.ListEvents(ctx, tenantID, runID, 0, 10)
	if err != nil {
		t.Fatalf("ListEvents returned error: %v", err)
	}
	if len(events.Events) != 1 || events.Events[0].Event != workflowpause.EventApprovalResultFilled {
		t.Fatalf("events = %#v, want one approval_result_filled", events.Events)
	}
	if events.Events[0].Data["node_id"] != "approval-a" {
		t.Fatalf("approval event node_id = %#v, want approval-a", events.Events[0].Data["node_id"])
	}

	if _, err := service.SubmitByToken(ctx, formB.Payload.Token, approvalruntime.SubmitRequest{
		Action: "approve",
		Inputs: map[string]interface{}{"comment": "B ok"},
	}, nil, nil); err != nil {
		t.Fatalf("SubmitByToken(B) returned error: %v", err)
	}
	ready, err = service.ActivePauseApprovalFormsSubmitted(ctx, runID)
	if err != nil {
		t.Fatalf("ActivePauseApprovalFormsSubmitted after B returned error: %v", err)
	}
	if !ready {
		t.Fatal("ready = false, want true after all approval forms are submitted")
	}
}

func TestApprovalHandlerSubmitRequiresTaskManager(t *testing.T) {
	ctx := context.Background()
	db := newApprovalServiceTestDB(t)
	service := approvalruntime.NewService(db)
	form := createApprovalForm(t, ctx, service, "node-task-manager", "run-"+uuid.NewString())

	gin.SetMode(gin.TestMode)
	handler := approvalruntime.NewHandler(service, nil)
	router := gin.New()
	router.POST("/approval/forms/:token/submit", handler.SubmitForm)

	req := httptest.NewRequest(http.MethodPost, "/approval/forms/"+form.Payload.Token+"/submit", bytes.NewBufferString(`{"action":"approve","inputs":{"comment":"ok"}}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusOK {
		t.Fatalf("status code = %d, want non-200 when task manager is missing", recorder.Code)
	}
	var stored approvalruntime.Form
	if err := db.First(&stored, "id = ?", form.Form.ID).Error; err != nil {
		t.Fatalf("load form: %v", err)
	}
	if stored.Status != approvalruntime.FormStatusWaiting {
		t.Fatalf("form status = %s, want waiting", stored.Status)
	}
	if stored.SubmittedData != nil {
		t.Fatal("submitted data should remain empty when task manager is missing")
	}
}

func newApprovalServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&approvalruntime.Form{},
		&approvalruntime.Delivery{},
		&approvalruntime.Recipient{},
		&approvalruntime.RunEvent{},
		&workflowpause.RunPause{},
		&workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{},
	); err != nil {
		t.Fatalf("auto migrate approval tables: %v", err)
	}
	return db
}

func createApprovalForm(t *testing.T, ctx context.Context, service *approvalruntime.Service, nodeID, runID string) *approvalruntime.RuntimeForm {
	t.Helper()
	return createApprovalFormForRun(t, ctx, service, uuid.NewString(), uuid.NewString(), nodeID, runID)
}

func createApprovalFormForRun(t *testing.T, ctx context.Context, service *approvalruntime.Service, tenantID, appID, nodeID, runID string) *approvalruntime.RuntimeForm {
	t.Helper()

	webEnabled := true
	form, err := service.CreateOrGetRuntimeForm(ctx, approvalruntime.CreateRuntimeFormParams{
		TenantID:      tenantID,
		AppID:         appID,
		WorkflowRunID: runID,
		NodeID:        nodeID,
		NodeTitle:     "Approval",
		Rendered:      "Please review value",
		DefaultValues: map[string]interface{}{"comment": "draft"},
		Config: approvalruntime.NodeConfig{
			Content: "Please review {{#start.value#}}",
			Fields: []approvalruntime.FieldConfig{
				{
					Key:      "comment",
					Label:    "Comment",
					Type:     "textarea",
					Required: true,
				},
			},
			Actions: []approvalruntime.Action{
				{ID: "approve", Label: "Approve"},
				{ID: "reject", Label: "Reject"},
			},
			SubmitMethods: approvalruntime.SubmitMethods{
				WebApp: approvalruntime.WebAppSubmitMethod{Enabled: &webEnabled},
			},
			Timeout: approvalruntime.TimeoutConfig{
				Duration: 1,
				Unit:     "hour",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
	return form
}

func createApprovalEmailForm(t *testing.T, ctx context.Context, service *approvalruntime.Service, nodeID, runID string) *approvalruntime.RuntimeForm {
	t.Helper()

	webEnabled := true
	form, err := service.CreateOrGetRuntimeForm(ctx, approvalruntime.CreateRuntimeFormParams{
		TenantID:      uuid.NewString(),
		AppID:         uuid.NewString(),
		WorkflowRunID: runID,
		NodeID:        nodeID,
		NodeTitle:     "Approval",
		Rendered:      "Please review email value",
		Config: approvalruntime.NodeConfig{
			Content: "Please review {{#start.value#}}",
			Fields: []approvalruntime.FieldConfig{
				{
					Key:      "comment",
					Label:    "Comment",
					Type:     "textarea",
					Required: true,
				},
			},
			Actions: []approvalruntime.Action{
				{ID: "approve", Label: "Approve"},
				{ID: "reject", Label: "Reject"},
			},
			SubmitMethods: approvalruntime.SubmitMethods{
				WebApp: approvalruntime.WebAppSubmitMethod{Enabled: &webEnabled},
				Email: approvalruntime.EmailSubmitMethod{
					Enabled: true,
					Subject: "Approval\nRequest",
					Body:    "Open {{#url#}}",
					Recipients: []approvalruntime.EmailRecipient{
						{Type: "external", Email: "approver@example.com"},
					},
				},
			},
			Timeout: approvalruntime.TimeoutConfig{
				Duration: 1,
				Unit:     "hour",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateOrGetRuntimeForm returned error: %v", err)
	}
	return form
}

type fakeApprovalEmailSender struct {
	err      error
	messages []approvalEmailMessage
}

type approvalEmailMessage struct {
	to      []string
	subject string
	body    string
}

func (s *fakeApprovalEmailSender) SendEmail(to []string, subject, body string) error {
	s.messages = append(s.messages, approvalEmailMessage{
		to:      append([]string(nil), to...),
		subject: subject,
		body:    body,
	})
	return s.err
}
