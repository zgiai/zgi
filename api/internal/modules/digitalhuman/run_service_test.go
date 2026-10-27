package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/datatypes"
)

func TestScheduledAPIEndUserTaskDoesNotBorrowPersonalConnectors(t *testing.T) {
	event := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginSchedule}}
	for _, testCase := range []struct {
		name string
		run  *Run
		want bool
	}{
		{"API end user", &Run{Surface: SurfaceTask, UserSubjectKind: string(usersubject.APIEndUser)}, true},
		{"account owner", &Run{Surface: SurfaceTask, UserSubjectKind: string(usersubject.Account)}, false},
		{"shared task", &Run{Surface: SurfaceTask}, false},
		{"manual API call", &Run{Surface: SurfaceAPI, UserSubjectKind: string(usersubject.APIEndUser)}, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config := chatruntime.RunConfig{}
			applyScheduledConnectorPolicy(&config, testCase.run, event)
			if config.ConnectorToolsDisabled != testCase.want {
				t.Fatalf("ConnectorToolsDisabled = %v, want %v", config.ConnectorToolsDisabled, testCase.want)
			}
		})
	}
}

func TestScheduledAPIEndUserTaskKeepsOnlyItsOwnConnectors(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_PER_USER_CONNECTORS_ENABLED", "true")
	event := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginSchedule}}
	config := chatruntime.RunConfig{}
	applyScheduledConnectorPolicy(&config, &Run{Surface: SurfaceTask, UserSubjectKind: string(usersubject.APIEndUser)}, event)
	if config.ConnectorToolsDisabled || !config.ConnectorDelegationDisabled {
		t.Fatalf("scheduled end-user task = tools disabled %v, delegation disabled %v", config.ConnectorToolsDisabled, config.ConnectorDelegationDisabled)
	}
	subject := uuid.New()
	for name, run := range map[string]*Run{
		"end user task": {Surface: SurfaceTask, UserSubjectKind: string(usersubject.APIEndUser), UserSubjectID: &subject},
		"member task":   {Surface: SurfaceTask, UserSubjectKind: string(usersubject.Account), UserSubjectID: &subject},
	} {
		if requester, _, ok := perUserRequester(run); !ok || requester != subject {
			t.Fatalf("%s acts as %v, %v", name, requester, ok)
		}
	}
	if _, _, ok := perUserRequester(&Run{Surface: SurfaceTask}); ok {
		t.Fatal("a task without a requester resolved a per-user slot")
	}
}

func TestScheduledAPIEndUserTaskPreparesChatWithoutPersonalConnectors(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID, subjectID := testRunScope(), uuid.New(), uuid.New()
	scope.ActorID, scope.PrincipalID = subjectID, subjectID
	scope.PrincipalType, scope.Surface = string(invocation.PrincipalSystem), SurfaceTask
	scope.UserSubjectKind, scope.UserSubjectID = string(usersubject.APIEndUser), &subjectID
	event := invocation.Event{
		ID: uuid.NewString(), Type: invocation.EventTypeSchedule,
		Origin: invocation.Origin{Kind: invocation.OriginSchedule},
		Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "digital-human-task-scheduler"}, OccurredAt: service.now(),
	}
	run, created, err := service.SubmitInvocation(t.Context(), scope, event, "Reply without using a connector", nil)
	if err != nil || !created {
		t.Fatalf("SubmitInvocation() = created %v, err %v", created, err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if chat.prepareCalls != 1 || !chat.config.ConnectorToolsDisabled || chat.request.Query == "" {
		t.Fatal("API end-user task reached ChatRuntime without the account-safe Connector policy")
	}
}

func TestRuntimeServiceSubmitIsDurableAndIdempotent(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	request := ManualInvocationRequest{RequestID: uuid.New(), Query: "research the issue"}

	first, created, err := service.SubmitManual(t.Context(), scope, digitalHumanID, request)
	if err != nil || !created {
		t.Fatalf("SubmitManual(first) = created %v, err %v", created, err)
	}
	second, created, err := service.SubmitManual(t.Context(), scope, digitalHumanID, request)
	if err != nil || created {
		t.Fatalf("SubmitManual(duplicate) = created %v, err %v", created, err)
	}
	if second.ID != first.ID || len(store.runs) != 1 || len(store.outbox) != 1 {
		t.Fatalf("duplicate created extra durable state: runs=%d outbox=%d", len(store.runs), len(store.outbox))
	}
	if len(dispatcher.enqueued) != 1 || dispatcher.enqueued[0] != first.ID {
		t.Fatalf("queue delivery = %v, want [%s]", dispatcher.enqueued, first.ID)
	}
	if got := store.outbox[0].Status; got != RunOutboxConfirmed {
		t.Fatalf("outbox status = %s, want confirmed", got)
	}
}

func TestRuntimeServiceSubmitAPIStoresTaskTokenBudgetInInvocationContext(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope := testRunScope()
	scope.Surface = SurfaceAPI
	scope.PrincipalType = string(invocation.PrincipalService)
	scope.PrincipalID = scope.ActorID
	subjectID := uuid.New()
	scope.UserSubjectID = &subjectID
	scope.UserSubjectKind = string(usersubject.APIEndUser)
	digitalHumanID := uuid.New()

	run, created, err := service.SubmitAPI(t.Context(), scope, digitalHumanID, ManualInvocationRequest{
		RequestID: uuid.New(), Query: "inspect the deployment", MaxTaskTokens: 42000,
	})
	if err != nil || !created || run == nil {
		t.Fatalf("SubmitAPI() = %#v, created %v, error %v", run, created, err)
	}
	_, event, err := decodeAndVerifyRun(run)
	if err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if got := intContextValue(event.Context, "max_task_tokens"); got != 42000 {
		t.Fatalf("max_task_tokens = %d, want 42000", got)
	}

	for _, invalid := range []int{-1, 100000001} {
		failed, failedCreated, err := service.SubmitAPI(t.Context(), scope, digitalHumanID, ManualInvocationRequest{
			RequestID: uuid.New(), Query: "reject invalid budget", MaxTaskTokens: invalid,
		})
		if failed != nil || failedCreated || !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("SubmitAPI(max_task_tokens=%d) = %#v, created %v, error %v", invalid, failed, failedCreated, err)
		}
	}
	webScope := scope
	webScope.Surface = SurfaceWebApp
	failed, failedCreated, err := service.SubmitWebApp(t.Context(), webScope, digitalHumanID, ManualInvocationRequest{
		RequestID: uuid.New(), Query: "reject webapp budget", MaxTaskTokens: 42000,
	})
	if failed != nil || failedCreated || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("SubmitWebApp(max_task_tokens) = %#v, created %v, error %v", failed, failedCreated, err)
	}
}

func TestRuntimeServiceSubmitRegenerationUsesSourceMessageAndDurableRun(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	conversationID, messageID := uuid.New(), uuid.New()
	chat.recoveryMessage = &runtimemodel.Message{ID: messageID, ConversationID: conversationID, Query: "original question"}

	query := "edited question"
	run, created, err := service.SubmitRegeneration(t.Context(), scope, digitalHumanID, RegenerateInvocationRequest{
		MessageID: messageID, Query: &query,
	})
	if err != nil || !created {
		t.Fatalf("SubmitRegeneration() = %#v, created %v, error %v", run, created, err)
	}
	if run.ConversationID == nil || *run.ConversationID != conversationID || run.Query != query {
		t.Fatalf("regeneration run = %#v", run)
	}
	_, event, err := decodeAndVerifyRun(run)
	if err != nil {
		t.Fatalf("decode regeneration run: %v", err)
	}
	if got, ok := stringContextValue(event.Context, "regenerate_message_id"); !ok || got != messageID.String() {
		t.Fatalf("regeneration source = %q, %v", got, ok)
	}

	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute(regeneration) error = %v", err)
	}
	if chat.regenerationMessageID != messageID || chat.regenerationRequest.Query == nil || *chat.regenerationRequest.Query != query {
		t.Fatalf("root regeneration request = %#v, message=%s", chat.regenerationRequest, chat.regenerationMessageID)
	}
}

func TestRuntimeServiceValidatesConsoleFilesBeforeCreatingRun(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	fileID := uuid.NewString()
	validator := &manualInvocationFileValidatorStub{normalized: []string{fileID}}
	service.manualFiles = validator
	scope, digitalHumanID := testRunScope(), uuid.New()
	parentMessageID := uuid.New()

	run, created, err := service.SubmitManual(t.Context(), scope, digitalHumanID, ManualInvocationRequest{
		RequestID: uuid.New(), Query: "review the attachment", ParentMessageID: &parentMessageID,
		FileIDs: []string{"  " + fileID + "  "},
	})
	if err != nil || !created {
		t.Fatalf("SubmitManual() = %#v, created %v, error %v", run, created, err)
	}
	if validator.scope.Surface != SurfaceConsole || validator.scope.ActorID != scope.ActorID {
		t.Fatalf("validator scope = %#v", validator.scope)
	}
	_, event, err := decodeAndVerifyRun(run)
	if err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if got := stringSliceContextValue(event.Context, "file_ids"); len(got) != 1 || got[0] != fileID {
		t.Fatalf("immutable file IDs = %v, want [%s]", got, fileID)
	}
	if got, ok := stringContextValue(event.Context, "parent_message_id"); !ok || got != parentMessageID.String() {
		t.Fatalf("immutable parent message ID = %q, %v, want %s", got, ok, parentMessageID)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if chat.request.ParentID != parentMessageID.String() || len(chat.request.FileIDs) != 1 || chat.request.FileIDs[0] != fileID {
		t.Fatalf("ChatRuntime request = %#v", chat.request)
	}

	validator.err = ErrConsoleFileNotAccessible
	failed, failedCreated, err := service.SubmitManual(t.Context(), scope, digitalHumanID, ManualInvocationRequest{
		RequestID: uuid.New(), Query: "use another attachment", FileIDs: []string{uuid.NewString()},
	})
	if failed != nil || failedCreated || !errors.Is(err, ErrConsoleFileNotAccessible) {
		t.Fatalf("SubmitManual(inaccessible) = %#v, created %v, error %v", failed, failedCreated, err)
	}
	if len(store.runs) != 1 || len(store.outbox) != 1 {
		t.Fatalf("inaccessible file created durable state: runs=%d outbox=%d", len(store.runs), len(store.outbox))
	}
}

func TestRuntimeServiceSubmitsRuntimeInputFollowUpAsDurableDigitalHumanRun(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	original, created, err := service.SubmitManual(t.Context(), scope, digitalHumanID, ManualInvocationRequest{
		RequestID: uuid.New(), Query: "prepare the first answer",
	})
	if err != nil || !created {
		t.Fatalf("SubmitManual() = %#v, %v, want created run", original, err)
	}
	conversationID, messageID, batchID := uuid.New(), uuid.New(), uuid.New()
	original.ConversationID = &conversationID
	original.MessageID = &messageID
	chat.followUpContext = &chatruntime.RuntimeInputFollowUpContext{
		BatchID: batchID, OrganizationID: scope.OrganizationID, WorkspaceID: &scope.WorkspaceID,
		AccountID: scope.ActorID, CallerType: runtimemodel.ConversationCallerDigitalHuman, CallerID: digitalHumanID,
		ConversationID: conversationID, TargetMessageID: messageID,
		ClientInputIDs: []uuid.UUID{uuid.New(), uuid.New()}, Contents: []string{"first supplement", "second supplement"},
	}

	followUp, followUpCreated, err := service.SubmitRuntimeInputFollowUp(t.Context(), batchID)
	if err != nil || !followUpCreated {
		t.Fatalf("SubmitRuntimeInputFollowUp() = %#v, %v, want created run", followUp, err)
	}
	if followUp == nil || followUp.ID == original.ID || followUp.DigitalHumanID != digitalHumanID ||
		followUp.ConversationID == nil || *followUp.ConversationID != conversationID {
		t.Fatalf("follow-up run = %#v", followUp)
	}
	if followUp.Query != "first supplement\n\nsecond supplement" {
		t.Fatalf("follow-up query = %q", followUp.Query)
	}
	_, event, err := decodeAndVerifyRun(followUp)
	if err != nil {
		t.Fatalf("decode follow-up run: %v", err)
	}
	if got, ok := runtimeInputFollowUpBatchID(event); !ok || got != batchID {
		t.Fatalf("follow-up batch = %s, %v, want %s", got, ok, batchID)
	}
}

func TestRuntimeServiceReconcilesFailedQueueDelivery(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{fail: true}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	service.now = advancingClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))

	run, created, err := service.SubmitManual(t.Context(), testRunScope(), uuid.New(), ManualInvocationRequest{RequestID: uuid.New(), Query: "retry me"})
	if err != nil || !created {
		t.Fatalf("SubmitManual() = created %v, err %v", created, err)
	}
	if store.outbox[0].Status != RunOutboxPending {
		t.Fatalf("outbox status after failure = %s", store.outbox[0].Status)
	}
	dispatcher.fail = false
	store.outbox[0].AvailableAt = service.now().Add(-time.Second)
	if err := service.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if store.outbox[0].Status != RunOutboxConfirmed || dispatcher.enqueued[len(dispatcher.enqueued)-1] != run.ID {
		t.Fatalf("reconciled outbox = %#v, enqueued=%v", store.outbox[0], dispatcher.enqueued)
	}
}

func TestRuntimeServiceReconcilesCompletedChatMessageWithoutRerunningModel(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	chat := &fakeDigitalHumanChat{recoveryMessage: &runtimemodel.Message{
		Status: runtimemodel.MessageStatusCompleted,
		Answer: "stored completed answer",
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{map[string]interface{}{
				"skill_id": "calculator", "status": "succeeded", "arguments": "must-not-persist",
			}},
		},
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	now := time.Date(2026, 8, 18, 6, 1, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	run, _, err := service.SubmitManual(t.Context(), testRunScope(), uuid.New(), ManualInvocationRequest{RequestID: uuid.New(), Query: "recover me"})
	if err != nil {
		t.Fatal(err)
	}
	messageID, conversationID := uuid.New(), uuid.New()
	run.Status, run.MessageID, run.ConversationID = RunStatusRunning, &messageID, &conversationID
	run.UpdatedAt = now.Add(-completedRunRecoveryWindow - time.Second)

	if err := service.Reconcile(t.Context()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if run.Status != RunStatusSucceeded || run.ResultSummary != "stored completed answer" {
		t.Fatalf("recovered run = status %s, summary %q", run.Status, run.ResultSummary)
	}
	if chat.prepareCalls != 0 || chat.runCalls != 0 || chat.getMessageCalls != 1 {
		t.Fatalf("recovery unexpectedly reran model: prepares=%d runs=%d gets=%d", chat.prepareCalls, chat.runCalls, chat.getMessageCalls)
	}
	if bytes.Contains(run.ExecutionTrace, []byte("must-not-persist")) || !bytes.Contains(run.ExecutionTrace, []byte(`"skill_id":"calculator"`)) {
		t.Fatalf("recovered trace was not safely projected: %s", run.ExecutionTrace)
	}
}

func TestRuntimeServiceGetRunRuntimeLogProjectsModelAndToolSteps(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	messageID, conversationID := uuid.New(), uuid.New()
	chat := &fakeDigitalHumanChat{recoveryMessage: &runtimemodel.Message{
		ID:             messageID,
		ConversationID: conversationID,
		Query:          "calculate",
		Answer:         "4",
		Status:         runtimemodel.MessageStatusCompleted,
		ModelName:      "test-model",
		Metadata: map[string]interface{}{
			"model_invocations": []interface{}{map[string]interface{}{
				"kind": "model_call", "phase": "skill_planning", "status": "success",
				"request":    map[string]interface{}{"model": "test-model", "messages": []interface{}{map[string]interface{}{"role": "user", "content": "calculate"}}},
				"response":   map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "calling calculator"}},
				"runtime_id": "model_call:skill_planning:0:1",
			}},
			"skill_invocations": []interface{}{map[string]interface{}{
				"kind": "tool_call", "skill_id": "calculator", "tool_name": "calculate", "status": "success",
				"arguments": map[string]interface{}{"expression": "2+2", "api_key": "must-not-leak"},
				"result":    map[string]interface{}{"value": 4}, "runtime_id": "tool_call:calculator:calculate#1",
			}},
		},
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	runID := uuid.New()
	store.runs[runID] = &Run{
		ID: runID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		DigitalHumanID: digitalHumanID, MessageID: &messageID, ConversationID: &conversationID,
	}

	log, err := service.GetRunRuntimeLog(t.Context(), scope, digitalHumanID, runID)
	if err != nil {
		t.Fatalf("GetRunRuntimeLog() error = %v", err)
	}
	if log.Detail == nil || log.Detail.ModelName != "test-model" || len(log.Steps) != 3 {
		t.Fatalf("runtime log = %#v, want detail and model/tool/answer steps", log)
	}
	if log.Steps[0].Type != "model_call" || log.Steps[1].Type != "tool_call" || log.Steps[2].Type != "model_answer" {
		t.Fatalf("runtime step order = %#v", log.Steps)
	}
	encoded, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("must-not-leak")) {
		t.Fatalf("runtime log leaked a credential: %s", encoded)
	}
	if chat.caller.Type != runtimemodel.ConversationCallerDigitalHuman || chat.caller.ID == nil || *chat.caller.ID != digitalHumanID {
		t.Fatalf("runtime log caller = %#v", chat.caller)
	}
}

func TestRuntimeServiceRunRuntimeLogUsesDurableStateAndProjectsWorkflowApproval(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	messageID, conversationID := uuid.New(), uuid.New()
	chat := &fakeDigitalHumanChat{recoveryMessage: &runtimemodel.Message{
		ID: messageID, ConversationID: conversationID, Query: "run workflow", Status: runtimemodel.MessageStatusWaitingApproval,
		Metadata: map[string]interface{}{"workflow_runs": []interface{}{map[string]interface{}{
			"status": "pending_approval",
			"approval": map[string]interface{}{
				"approval_token":      "approval-token",
				"ui_approval_allowed": true,
				"approval_form": map[string]interface{}{
					"id": "form-1", "node_id": "approval-node", "node_title": "Approve",
					"content": "Please approve", "fields": []interface{}{},
					"actions": []interface{}{map[string]interface{}{"id": "approve", "label": "Approve"}},
				},
			},
		}}},
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	startedAt := time.Date(2026, 8, 18, 6, 0, 0, 0, time.UTC)
	createdAt := startedAt.Add(-time.Minute)
	runID := uuid.New()
	store.runs[runID] = &Run{
		ID: runID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		DigitalHumanID: digitalHumanID, MessageID: &messageID, ConversationID: &conversationID,
		Query: "run workflow", Status: RunStatusWaiting, WaitingReason: runtimemodel.MessageStatusWaitingApproval,
		CreatedAt: createdAt, StartedAt: &startedAt, UpdatedAt: startedAt.Add(5 * time.Second),
	}

	log, err := service.GetRunRuntimeLog(t.Context(), scope, digitalHumanID, runID)
	if err != nil {
		t.Fatalf("GetRunRuntimeLog() error = %v", err)
	}
	if log.Detail == nil || log.Detail.ID != runID.String() || log.Detail.Status != string(RunStatusWaiting) || log.Detail.Query != "run workflow" {
		t.Fatalf("durable detail = %#v", log.Detail)
	}
	if log.PendingWorkflowApproval == nil || log.PendingWorkflowApproval.ApprovalToken != "approval-token" || log.PendingWorkflowApproval.ApprovalForm["id"] != "form-1" {
		t.Fatalf("pending workflow approval = %#v", log.PendingWorkflowApproval)
	}
}

func TestChannelContextRuntimeStepTreatsUnsupportedProviderAsCapabilityBoundary(t *testing.T) {
	run := &Run{
		ID:             uuid.New(),
		CreatedAt:      time.Date(2026, 8, 24, 10, 0, 0, 0, time.UTC),
		ExecutionTrace: datatypes.JSON(`{"channel_context":[{"status":"unavailable","error_code":"channel_context_not_supported"}]}`),
	}

	steps := channelContextRuntimeSteps(run, nil)
	if len(steps) != 1 || steps[0].Status != "skipped" {
		t.Fatalf("channel context steps = %#v, want one skipped capability-boundary step", steps)
	}

	run.ExecutionTrace = datatypes.JSON(`{"channel_context":[{"status":"unavailable","error_code":"channel_context_unavailable"}]}`)
	steps = channelContextRuntimeSteps(run, nil)
	if len(steps) != 1 || steps[0].Status != "failed" {
		t.Fatalf("channel context steps = %#v, want one failed runtime-read step", steps)
	}
}

func TestRuntimeServiceUsesCopyPasteGuidanceWhenChannelContextReadFails(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	now := time.Date(2026, 8, 24, 12, 45, 0, 0, time.UTC)
	service := NewRuntimeService(
		store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, dispatcher,
		WithChannelContextRuntime(&fakeChannelContextRuntimeProvider{err: errors.New("provider timeout")}),
	)
	service.now = func() time.Time { return now }
	scope, digitalHumanID, conversationID := testRunScope(), uuid.New(), uuid.New()
	installationID := uuid.NewString()
	event := invocation.Event{
		ID: "dingtalk-context-failure", Type: invocation.EventTypeChannelMessage,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "dingtalk", InstallationID: installationID},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "dingtalk-user"},
		OccurredAt: now,
		ReplyTo:    &invocation.ReplyAddress{Provider: "dingtalk", InstallationID: installationID, ConversationID: "cid-group"},
	}
	run, created, err := service.SubmitInvocation(t.Context(), scope, event, "回答一下这个问题", &conversationID)
	if err != nil || !created {
		t.Fatalf("SubmitInvocation() = created %v, err %v", created, err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, expected := range []string{
		"copy the missing source text into the same message as the mention",
		"ask one brief clarification",
		"only recovery action",
	} {
		if !strings.Contains(chat.config.SystemPrompt, expected) {
			t.Fatalf("fallback prompt missing %q: %s", expected, chat.config.SystemPrompt)
		}
	}
	for _, unwanted := range []string{"ask the user to quote", "reply directly to it"} {
		if strings.Contains(chat.config.SystemPrompt, unwanted) {
			t.Fatalf("fallback prompt contains unsupported guidance %q: %s", unwanted, chat.config.SystemPrompt)
		}
	}
}

func TestRuntimeServiceExecuteUsesDigitalHumanCallerAndNoLegacyMemory(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	run, _, err := service.SubmitManual(t.Context(), scope, digitalHumanID, ManualInvocationRequest{RequestID: uuid.New(), Query: "invoke bound skill"})
	if err != nil {
		t.Fatalf("SubmitManual() error = %v", err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if chat.caller.Type != runtimemodel.ConversationCallerDigitalHuman || chat.caller.ID == nil || *chat.caller.ID != digitalHumanID {
		t.Fatalf("caller = %#v", chat.caller)
	}
	if chat.request.UseMemory || chat.config.UseMemory || chat.config.AgentMemoryEnabled || chat.config.AgentMemoryAutoExtractionEnabled {
		t.Fatal("DigitalHuman execution enabled a legacy memory path")
	}
	if chat.request.SourceMessageID == nil || *chat.request.SourceMessageID != run.ID {
		t.Fatalf("source message id = %v, want run id %s", chat.request.SourceMessageID, run.ID)
	}
	stored := store.runs[run.ID]
	if stored.Status != RunStatusSucceeded || stored.ResultSummary != "completed answer" || stored.MessageID == nil {
		t.Fatalf("completed run = %#v", stored)
	}
	if !bytes.Contains(stored.ExecutionTrace, []byte(`"skill_id":"calculator"`)) ||
		!bytes.Contains(stored.ExecutionTrace, []byte(`"tool_file_id":"tool-file-1"`)) ||
		bytes.Contains(stored.ExecutionTrace, []byte("must-not-persist")) ||
		bytes.Contains(stored.ExecutionTrace, []byte("stale.example")) ||
		bytes.Contains(stored.ExecutionTrace, []byte("arguments")) {
		t.Fatalf("execution trace was not safely projected: %s", stored.ExecutionTrace)
	}
}

func TestRuntimeServiceEnsuresReservedConversationForChannelInvocation(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID, conversationID := testRunScope(), uuid.New(), uuid.New()
	installationID := uuid.NewString()
	event := invocation.Event{
		ID: "feishu-channel-event", Type: invocation.EventTypeChannelMessage,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installationID},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_private_sender"},
		OccurredAt: time.Now().UTC(),
		ReplyTo: &invocation.ReplyAddress{
			Provider: "feishu", InstallationID: installationID, ConversationID: "oc_private",
		},
	}
	run, created, err := service.SubmitInvocation(t.Context(), scope, event, "continue the private chat", &conversationID)
	if err != nil || !created {
		t.Fatalf("SubmitInvocation() = created %v, err %v", created, err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if chat.ensureCalls != 1 || chat.ensuredConversationID != conversationID {
		t.Fatalf("ensured conversation = %s after %d calls, want %s", chat.ensuredConversationID, chat.ensureCalls, conversationID)
	}
	if chat.caller.Source != digitalHumanChannelConversationSource {
		t.Fatalf("channel conversation source = %q, want %q", chat.caller.Source, digitalHumanChannelConversationSource)
	}
	if chat.request.ConversationID != conversationID.String() {
		t.Fatalf("prepared conversation = %q, want %s", chat.request.ConversationID, conversationID)
	}
	if chat.request.HistoryPolicy == nil || chat.request.HistoryPolicy.Mode != runtimedto.ConversationHistoryModeSupplemental ||
		chat.request.HistoryPolicy.MaxTurns != channelFormalHistoryTurns ||
		len(chat.request.HistoryPolicy.ExcludedTexts) != 1 || chat.request.HistoryPolicy.ExcludedTexts[0] != "continue the private chat" {
		t.Fatalf("channel history policy = %#v", chat.request.HistoryPolicy)
	}
}

func TestRuntimeServiceAbsorbsPrelinkedChannelRunIntoVisibleActiveTurn(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	conversationID, activeMessageID := uuid.New(), uuid.New()
	chat := &fakeDigitalHumanChat{activeConversation: &runtimemodel.Conversation{
		ID: conversationID, RuntimeStatus: runtimemodel.ConversationRuntimeStatusStreaming,
		ActiveMessageID: &activeMessageID,
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID, inboundID := testRunScope(), uuid.New(), uuid.New()
	installationID := uuid.NewString()
	event := invocation.Event{
		ID: "feishu-prelinked-supplement", Type: invocation.EventTypeChannelMention,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installationID},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_supplement"},
		Context: map[string]any{
			"channel_inbound_event_id": inboundID.String(), "conversation_type": string(channelruntime.ConversationTypeGroup),
			"sender_display_name": "小王", "external_conversation_id": "oc_active",
		},
		OccurredAt: service.now(),
		ReplyTo: &invocation.ReplyAddress{
			Provider: "feishu", InstallationID: installationID, ConversationID: "oc_active", RecipientID: "ou_supplement",
		},
	}
	run, created, err := service.SubmitInvocation(t.Context(), scope, event, "补充这项要求", &conversationID)
	if err != nil || !created {
		t.Fatalf("SubmitInvocation() = created %v, err %v", created, err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if chat.prepareCalls != 0 || chat.runCalls != 0 || chat.submittedRuntimeInput == nil {
		t.Fatalf("prelinked run was not absorbed: prepares=%d runs=%d input=%#v", chat.prepareCalls, chat.runCalls, chat.submittedRuntimeInput)
	}
	if chat.submittedRuntimeInput.ClientInputID != inboundID || chat.submittedRuntimeInput.TargetMessageID != activeMessageID ||
		chat.submittedRuntimeInput.Content != "Channel message from 小王:\n补充这项要求" {
		t.Fatalf("submitted runtime input = %#v", chat.submittedRuntimeInput)
	}
	if run.Status != RunStatusSucceeded || !bytes.Contains(run.ExecutionTrace, []byte("absorbed_into_active_turn")) {
		t.Fatalf("absorbed run = status %s, trace %s", run.Status, run.ExecutionTrace)
	}
}

func TestRuntimeServiceChannelRuntimeInputAdmissionUsesConversationAndSender(t *testing.T) {
	tests := []struct {
		name                 string
		activeConversation   channelruntime.ConversationType
		activeSender         string
		incomingConversation channelruntime.ConversationType
		incomingSender       string
		wantAdmission        chatruntime.RuntimeInputAdmissionMode
	}{
		{
			name:                 "same group sender may join current turn",
			activeConversation:   channelruntime.ConversationTypeGroup,
			activeSender:         "user-a",
			incomingConversation: channelruntime.ConversationTypeGroup,
			incomingSender:       "user-a",
			wantAdmission:        chatruntime.RuntimeInputAdmissionCurrentOrFollowUp,
		},
		{
			name:                 "different group sender waits for next turn",
			activeConversation:   channelruntime.ConversationTypeGroup,
			activeSender:         "user-a",
			incomingConversation: channelruntime.ConversationTypeGroup,
			incomingSender:       "user-b",
			wantAdmission:        chatruntime.RuntimeInputAdmissionFollowUpOnly,
		},
		{
			name:                 "private message waits for next turn",
			activeConversation:   channelruntime.ConversationTypeDirect,
			activeSender:         "user-a",
			incomingConversation: channelruntime.ConversationTypeDirect,
			incomingSender:       "user-a",
			wantAdmission:        chatruntime.RuntimeInputAdmissionFollowUpOnly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
			conversationID, activeMessageID := uuid.New(), uuid.New()
			chat := &fakeDigitalHumanChat{activeConversation: &runtimemodel.Conversation{
				ID: conversationID, RuntimeStatus: runtimemodel.ConversationRuntimeStatusStreaming,
				ActiveMessageID: &activeMessageID,
			}}
			service := newTestRuntimeService(t, store, dispatcher, chat)
			scope, digitalHumanID := testRunScope(), uuid.New()
			installationID := uuid.NewString()
			activeEvent := invocation.Event{
				ID: "active-channel-run-" + uuid.NewString(), Type: invocation.EventTypeChannelMessage,
				Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "test", InstallationID: installationID},
				Target: runtimeidentity.SubjectRef{
					Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
					OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
				},
				Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: tt.activeSender},
				Context: map[string]any{
					"conversation_type": string(tt.activeConversation), "external_conversation_id": "external-conversation",
				},
				OccurredAt: service.now(),
				ReplyTo: &invocation.ReplyAddress{
					Provider: "test", InstallationID: installationID, ConversationID: "external-conversation",
				},
			}
			activeRun, created, err := service.SubmitInvocation(t.Context(), scope, activeEvent, "initial request", &conversationID)
			if err != nil || !created {
				t.Fatalf("submit active invocation = created %v, err %v", created, err)
			}
			activeRun.MessageID = &activeMessageID

			accepted, err := service.SubmitChannelRuntimeInput(t.Context(), scope, digitalHumanID, conversationID, ChannelRuntimeInputRequest{
				ClientInputID: uuid.New(), Content: "next message", ConversationType: tt.incomingConversation,
				SenderExternalID: tt.incomingSender,
			})
			if err != nil || !accepted {
				t.Fatalf("SubmitChannelRuntimeInput() = %v, %v", accepted, err)
			}
			if chat.submittedRuntimeInput == nil || chat.submittedRuntimeInput.AdmissionMode != tt.wantAdmission {
				t.Fatalf("admission mode = %#v, want %q", chat.submittedRuntimeInput, tt.wantAdmission)
			}
		})
	}
}

func TestRuntimeServicePublishesSafeChannelProgressBeforeFinalAnswer(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	chat := &fakeDigitalHumanChat{events: []chatruntime.StreamEvent{
		{EventType: channelEventMessageRetract, CreatedAtMS: 1_000, Sequence: 1, Payload: map[string]any{
			"content": "我先整理数据，再生成文件。", "presentation_disposition": "process", "segment_id": "segment-1",
		}},
		{EventType: channelEventMessageRetract, CreatedAtMS: 1_001, Sequence: 1, Payload: map[string]any{
			"content": "这段候选内容不应发送。", "presentation_disposition": "discard", "segment_id": "segment-discarded",
		}},
		{EventType: channelEventIntermediateAnswer, CreatedAtMS: 1_002, Sequence: 1, Payload: map[string]any{
			"answer_id": "draft-1", "content": "阶段结果", "delta": true, "done": false,
		}},
		{EventType: channelEventIntermediateAnswer, CreatedAtMS: 1_003, Sequence: 1, Payload: map[string]any{
			"answer_id": "draft-1", "content": " 已经准备好。", "delta": true, "done": true,
		}},
		{EventType: channelEventMessageRetract, CreatedAtMS: 1_004, Sequence: 1, Payload: map[string]any{
			"content": "继续检查第三个阶段。", "presentation_disposition": "process", "segment_id": "segment-3",
		}},
		{EventType: channelEventMessageRetract, CreatedAtMS: 1_005, Sequence: 1, Payload: map[string]any{
			"content": "继续检查第四个阶段。", "presentation_disposition": "process", "segment_id": "segment-4",
		}},
		{EventType: channelEventMessageRetract, CreatedAtMS: 1_006, Sequence: 1, Payload: map[string]any{
			"content": "继续检查第五个阶段。", "presentation_disposition": "process", "segment_id": "segment-5",
		}},
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID, conversationID := testRunScope(), uuid.New(), uuid.New()
	installationID := uuid.NewString()
	event := invocation.Event{
		ID: "feishu-progress-event", Type: invocation.EventTypeChannelMessage,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: installationID},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_progress_sender"},
		OccurredAt: time.Now().UTC(),
		ReplyTo: &invocation.ReplyAddress{
			Provider: "feishu", InstallationID: installationID, ConversationID: "oc_progress",
		},
	}
	run, created, err := service.SubmitInvocation(t.Context(), scope, event, "生成一份文件", &conversationID)
	if err != nil || !created {
		t.Fatalf("SubmitInvocation() = created %v, err %v", created, err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	progress := service.transient.(*channelTransientDispatcherStub).requests
	if len(progress) != 5 {
		t.Fatalf("progress deliveries = %#v, want 5", progress)
	}
	if progress[0].Text != "我先整理数据，再生成文件。" || progress[1].Text != "阶段结果 已经准备好。" {
		t.Fatalf("progress texts = %#v", progress)
	}
	for _, item := range progress {
		if item.Kind != channelruntime.DeliveryKindProgress || strings.Contains(item.Text, "候选内容") {
			t.Fatalf("unsafe progress delivery = %#v", item)
		}
	}
	if len(store.progress) != 0 {
		t.Fatalf("durable progress deliveries = %#v, want none", store.progress)
	}
}

func TestRuntimeServiceAddsSubjectMemoryWithoutEnablingLegacyMemory(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	now := time.Date(2026, 8, 18, 6, 0, 0, 0, time.UTC)
	memory := &fakeMemoryRuntimeProvider{preparation: &MemoryRuntimePreparation{
		SystemPromptAddendum: "Authorized active long-term memory:\n- launch is September 8",
		Tools:                []skillloop.RuntimeTool{{Definition: adapter.Tool{Type: "function", Function: adapter.Function{Name: memoryEditToolName}}}},
	}}
	service := NewRuntimeService(store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, dispatcher, WithMemoryRuntime(memory))
	service.now = func() time.Time { return now }
	run, _, err := service.SubmitManual(t.Context(), testRunScope(), uuid.New(), ManualInvocationRequest{RequestID: uuid.New(), Query: "use our launch plan"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil {
		t.Fatal(err)
	}
	if memory.calls != 1 || memory.finalizeCalls != 1 || !strings.Contains(chat.config.SystemPrompt, "launch is September 8") || len(chat.config.RuntimeTools) != 1 {
		t.Fatalf("memory preparation calls=%d finalization calls=%d config=%#v", memory.calls, memory.finalizeCalls, chat.config)
	}
	if chat.config.UseMemory || chat.config.AgentMemoryEnabled || chat.config.AgentMemoryAutoExtractionEnabled {
		t.Fatal("subject memory enabled a legacy memory path")
	}
}

func TestRuntimeServiceCancelUsesDatabaseStateAsAuthority(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	run, _, err := service.SubmitManual(t.Context(), scope, digitalHumanID, ManualInvocationRequest{RequestID: uuid.New(), Query: "cancel me"})
	if err != nil {
		t.Fatalf("SubmitManual() error = %v", err)
	}
	canceled, err := service.CancelRun(t.Context(), scope, digitalHumanID, run.ID)
	if err != nil || canceled.Status != RunStatusCanceled {
		t.Fatalf("CancelRun(queued) = status %s, err %v", canceled.Status, err)
	}
	if err := service.Execute(t.Context(), run.ID); err != nil || chat.prepareCalls != 0 {
		t.Fatalf("canceled run executed: err=%v prepares=%d", err, chat.prepareCalls)
	}
	if len(dispatcher.canceled) != 1 || dispatcher.canceled[0] != run.ID {
		t.Fatalf("queue cancellation = %v", dispatcher.canceled)
	}
}

func TestRuntimeServiceCancelFinalizesStoppedMessage(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	chat := &fakeDigitalHumanChat{stopMessage: &runtimemodel.Message{
		Status: runtimemodel.MessageStatusStopped, Answer: "partial answer",
		Metadata: map[string]interface{}{"runtime_timeline": []interface{}{map[string]interface{}{"kind": "model_call"}}},
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	scope, digitalHumanID := testRunScope(), uuid.New()
	run, _, err := service.SubmitManual(t.Context(), scope, digitalHumanID, ManualInvocationRequest{RequestID: uuid.New(), Query: "cancel running"})
	if err != nil {
		t.Fatalf("SubmitManual() error = %v", err)
	}
	run.Status = RunStatusRunning
	messageID := uuid.New()
	run.MessageID = &messageID
	store.runs[run.ID] = run

	canceled, err := service.CancelRun(t.Context(), scope, digitalHumanID, run.ID)
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if canceled.Status != RunStatusCanceled || canceled.ResultSummary != "partial answer" || canceled.CompletedAt == nil {
		t.Fatalf("canceled run = %#v, want terminal canceled state", canceled)
	}
}

func TestRuntimeServiceRejectsTamperedSnapshotBeforeChatRuntime(t *testing.T) {
	store, dispatcher, chat := newFakeRunStore(), &fakeRunDispatcher{}, &fakeDigitalHumanChat{}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	run, _, err := service.SubmitManual(t.Context(), testRunScope(), uuid.New(), ManualInvocationRequest{RequestID: uuid.New(), Query: "do not run tampered state"})
	if err != nil {
		t.Fatalf("SubmitManual() error = %v", err)
	}
	store.runs[run.ID].ExecutionSnapshotRevision = "sha256:tampered"
	if err := service.Execute(t.Context(), run.ID); !errors.Is(err, ErrRunConflict) {
		t.Fatalf("Execute() error = %v, want run conflict", err)
	}
	if chat.prepareCalls != 0 || store.runs[run.ID].Status != RunStatusFailed {
		t.Fatalf("tampered run reached runtime or was not failed: prepares=%d status=%s", chat.prepareCalls, store.runs[run.ID].Status)
	}
}

func TestRuntimeServiceListsOnlySafeDigitalHumanSkills(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	chat := &fakeDigitalHumanChat{skills: []skills.SkillDiscoveryMetadata{
		{
			ID: skills.SkillTime, Name: "Current time", Description: "Read the current time",
			Display: skills.SkillDisplayMetadata{
				Icon: "clock", Label: map[string]string{"zh_Hans": "时间"},
				Description: map[string]string{"zh_Hans": "查询当前时间。"},
			},
			Enabled: true, Status: skills.SkillStatusActive,
			SupportedCallers: []string{skills.SkillCallerDigitalHuman},
		},
		{
			ID: "agent-only", Name: "Agent only", Enabled: true, Status: skills.SkillStatusActive,
			SupportedCallers: []string{skills.SkillCallerAgent},
		},
		{ID: skills.SkillAgentMemory, Name: "Agent memory", Enabled: true, Status: skills.SkillStatusActive},
		{ID: "broken", Name: "Broken", Enabled: true, Status: skills.SkillStatusInvalid},
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	result, err := service.ListResourceCandidates(t.Context(), testRunScope(), uuid.New(), ResourceCandidateQuery{
		Type: resourcebindings.BindingTypeSkill, Search: "time",
	})
	if err != nil {
		t.Fatalf("ListResourceCandidates() error = %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ResourceID != skills.SkillTime ||
		result.Items[0].ResourceRevision == "" || !result.Items[0].Available {
		t.Fatalf("skill candidates = %#v, want one safe time skill", result)
	}
	display, ok := result.Items[0].Metadata["display"].(skills.SkillDisplayMetadata)
	if !ok || display.Icon != "clock" || display.Label["zh_Hans"] != "时间" {
		t.Fatalf("skill candidate display = %#v, want localized display metadata", result.Items[0].Metadata["display"])
	}
}

func TestRuntimeServiceListsSkillCandidatesBeforeCreation(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	chat := &fakeDigitalHumanChat{skills: []skills.SkillDiscoveryMetadata{
		{
			ID: skills.SkillFileGenerator, Name: "File generation", Enabled: true,
			Status:           skills.SkillStatusActive,
			SupportedCallers: []string{skills.SkillCallerAIChat, skills.SkillCallerAgent, skills.SkillCallerDigitalHuman},
		},
		{
			ID: "aichat-only", Name: "AIChat only", Enabled: true, Status: skills.SkillStatusActive,
			SupportedCallers: []string{skills.SkillCallerAIChat},
		},
		{ID: "disabled", Name: "Disabled", Enabled: false, Status: skills.SkillStatusActive},
	}}
	service := newTestRuntimeService(t, store, dispatcher, chat)
	result, err := service.ListSkillCandidates(t.Context(), testRunScope())
	if err != nil {
		t.Fatalf("ListSkillCandidates() error = %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].ResourceID != skills.SkillFileGenerator {
		t.Fatalf("pre-creation skill candidates = %#v, want only the digital-human skill", result)
	}
}

func TestRuntimeServiceListsConnectorCandidatesBeforeCreation(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	store.connectorCandidates = connectorCandidateFixtures(defaultResourceCandidateLimit + 1)
	service := newTestRuntimeService(t, store, dispatcher, &fakeDigitalHumanChat{})
	result, err := service.ListConnectorCandidates(t.Context(), testRunScope(), ResourceCandidateQuery{})
	if err != nil {
		t.Fatalf("ListConnectorCandidates() error = %v", err)
	}
	if result.Total != defaultResourceCandidateLimit+1 || len(result.Items) != defaultResourceCandidateLimit {
		t.Fatalf("connector candidates = total %d, items %d, want bounded %d", result.Total, len(result.Items), defaultResourceCandidateLimit)
	}
	if result.Items[0].DisplayName != "Connection 00" || result.Items[len(result.Items)-1].DisplayName != fmt.Sprintf("Connection %02d", defaultResourceCandidateLimit-1) {
		t.Fatalf("connector candidate ordering = %q .. %q", result.Items[0].DisplayName, result.Items[len(result.Items)-1].DisplayName)
	}
}

// TestRuntimeServiceSearchesConnectorCandidatesBeyondDefaultPage proves a
// candidate after the first default page is reachable through search, which a
// projection hard-capped before filtering could never return.
func TestRuntimeServiceSearchesConnectorCandidatesBeyondDefaultPage(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	store.connectorCandidates = connectorCandidateFixtures(defaultResourceCandidateLimit + 10)
	service := newTestRuntimeService(t, store, dispatcher, &fakeDigitalHumanChat{})

	beyond := fmt.Sprintf("Connection %02d", defaultResourceCandidateLimit+5)
	result, err := service.ListConnectorCandidates(t.Context(), testRunScope(), ResourceCandidateQuery{Search: beyond})
	if err != nil {
		t.Fatalf("ListConnectorCandidates(search) error = %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].DisplayName != beyond {
		t.Fatalf("searched connector candidates = %#v, want only %q", result, beyond)
	}
}

// TestRuntimeServiceHonorsConnectorCandidateLimitUpToConnectorMaximum proves
// the creation surface can page past the old default and is clamped by the
// same connector maximum the binding editor enforces.
func TestRuntimeServiceHonorsConnectorCandidateLimitUpToConnectorMaximum(t *testing.T) {
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	total := defaultResourceCandidateLimit + 10
	store.connectorCandidates = connectorCandidateFixtures(total)
	service := newTestRuntimeService(t, store, dispatcher, &fakeDigitalHumanChat{})

	result, err := service.ListConnectorCandidates(t.Context(), testRunScope(), ResourceCandidateQuery{Limit: maxResourceBindings})
	if err != nil {
		t.Fatalf("ListConnectorCandidates(limit) error = %v", err)
	}
	if result.Total != total || len(result.Items) != total {
		t.Fatalf("connector candidates = total %d, items %d, want %d", result.Total, len(result.Items), total)
	}

	// The Connector type is forced server-side, so an unsupported caller type
	// cannot switch the projection or make the request fail.
	forced, err := service.ListConnectorCandidates(t.Context(), testRunScope(), ResourceCandidateQuery{Type: resourcebindings.BindingType("bogus"), Limit: maxResourceBindings})
	if err != nil || forced.Total != total || len(forced.Items) != total {
		t.Fatalf("connector candidates with overridden type = %#v, %v", forced, err)
	}
}

func connectorCandidateFixtures(count int) []ResourceCandidate {
	candidates := make([]ResourceCandidate, 0, count)
	for index := 0; index < count; index++ {
		candidates = append(candidates, ResourceCandidate{
			Type: resourcebindings.BindingTypeConnector, ResourceID: uuid.NewString(),
			ParentResourceID: "feishu", DisplayName: fmt.Sprintf("Connection %02d", index),
			ResourceRevision: "connector:1", Metadata: map[string]any{"allowed_operation_ids": []string{"search"}},
			Status: "ready", Available: true,
		})
	}
	return candidates
}

type fakeRunStore struct {
	mu                  sync.Mutex
	runs                map[uuid.UUID]*Run
	outbox              []*RunOutbox
	candidates          []ResourceCandidate
	connectorCandidates []ResourceCandidate
	progress            []fakeChannelProgress
	inbound             map[uuid.UUID]*channelruntime.InboundEvent
}

type fakeChannelProgress struct {
	RunID       uuid.UUID
	Kind        channelruntime.DeliveryKind
	DeliveryKey string
	Sequence    int64
	Text        string
}

func newFakeRunStore() *fakeRunStore {
	return &fakeRunStore{runs: make(map[uuid.UUID]*Run), inbound: make(map[uuid.UUID]*channelruntime.InboundEvent)}
}

func (s *fakeRunStore) RunByMessageID(_ context.Context, organizationID, workspaceID, digitalHumanID, messageID uuid.UUID) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.runs {
		if run.OrganizationID == organizationID && run.WorkspaceID == workspaceID && run.DigitalHumanID == digitalHumanID &&
			run.MessageID != nil && *run.MessageID == messageID {
			return run, nil
		}
	}
	return nil, ErrRunNotFound
}

func (s *fakeRunStore) ChannelInboundEventByID(_ context.Context, organizationID, workspaceID, installationID, eventID uuid.UUID) (*channelruntime.InboundEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inbound := s.inbound[eventID]
	if inbound == nil || inbound.OrganizationID != organizationID || inbound.WorkspaceID != workspaceID || inbound.InstallationID != installationID {
		return nil, ErrNotFound
	}
	return inbound, nil
}

func (s *fakeRunStore) CreateRunWithOutbox(_ context.Context, run *Run, outbox *RunOutbox) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.runs {
		if existing.OrganizationID == run.OrganizationID && existing.DigitalHumanID == run.DigitalHumanID && existing.IdempotencyKey == run.IdempotencyKey {
			return ErrRunConflict
		}
	}
	s.runs[run.ID], s.outbox = run, append(s.outbox, outbox)
	return nil
}

func (s *fakeRunStore) RunByID(_ context.Context, scope Scope, digitalHumanID, runID uuid.UUID) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[runID]
	if run == nil || run.OrganizationID != scope.OrganizationID || run.WorkspaceID != scope.WorkspaceID || run.DigitalHumanID != digitalHumanID {
		return nil, ErrRunNotFound
	}
	return run, nil
}

func (s *fakeRunStore) RunByIdempotency(_ context.Context, organizationID, digitalHumanID uuid.UUID, key string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, run := range s.runs {
		if run.OrganizationID == organizationID && run.DigitalHumanID == digitalHumanID && run.IdempotencyKey == key {
			return run, nil
		}
	}
	return nil, ErrRunNotFound
}

func (s *fakeRunStore) ListRuns(_ context.Context, scope Scope, digitalHumanID uuid.UUID, _ ListQuery) ([]*Run, int64, error) {
	items := make([]*Run, 0)
	for _, run := range s.runs {
		if run.OrganizationID == scope.OrganizationID && run.WorkspaceID == scope.WorkspaceID && run.DigitalHumanID == digitalHumanID {
			items = append(items, run)
		}
	}
	return items, int64(len(items)), nil
}

func (s *fakeRunStore) ListResourceCandidates(context.Context, Scope, uuid.UUID, ResourceCandidateQuery) ([]ResourceCandidate, error) {
	return append([]ResourceCandidate(nil), s.candidates...), nil
}

func (s *fakeRunStore) ListConnectorCandidates(context.Context, Scope) ([]ResourceCandidate, error) {
	return append([]ResourceCandidate(nil), s.connectorCandidates...), nil
}

func (s *fakeRunStore) ClaimRun(_ context.Context, runID uuid.UUID, now time.Time) (*Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.runs[runID]
	if run == nil {
		return nil, false, ErrRunNotFound
	}
	if run.Status != RunStatusQueued {
		return run, false, nil
	}
	run.Status, run.StartedAt, run.UpdatedAt = RunStatusRunning, &now, now
	return run, true, nil
}

func (s *fakeRunStore) LinkRunMessage(_ context.Context, runID, conversationID, messageID uuid.UUID, now time.Time) error {
	run := s.runs[runID]
	if run == nil || (run.Status != RunStatusRunning && run.Status != RunStatusCanceling) {
		return ErrRunConflict
	}
	run.ConversationID, run.MessageID, run.UpdatedAt = &conversationID, &messageID, now
	return nil
}

func (s *fakeRunStore) ListRecoverableRuns(_ context.Context, updatedBefore time.Time, limit int) ([]*Run, error) {
	items := make([]*Run, 0, limit)
	for _, run := range s.runs {
		if len(items) == limit {
			break
		}
		if (run.Status == RunStatusRunning || run.Status == RunStatusCanceling) && run.MessageID != nil && run.ActorID != nil && run.UpdatedAt.Before(updatedBefore) {
			items = append(items, run)
		}
	}
	return items, nil
}

func (s *fakeRunStore) CompleteRun(_ context.Context, runID uuid.UUID, summary string, trace datatypes.JSON, now time.Time) error {
	run := s.runs[runID]
	if run == nil {
		return ErrRunNotFound
	}
	if run.Status == RunStatusCanceling || run.CancelRequestedAt != nil {
		run.Status = RunStatusCanceled
	} else if run.Status == RunStatusRunning {
		run.Status = RunStatusSucceeded
	} else {
		return ErrRunConflict
	}
	run.ResultSummary, run.ExecutionTrace, run.CompletedAt, run.UpdatedAt = summary, trace, &now, now
	return nil
}

func (s *fakeRunStore) CompleteRunWithDelivery(ctx context.Context, runID uuid.UUID, summary string, trace datatypes.JSON, now time.Time) error {
	return s.CompleteRun(ctx, runID, summary, trace, now)
}

func (s *fakeRunStore) EnqueueRunChannelDelivery(_ context.Context, runID uuid.UUID, kind channelruntime.DeliveryKind, deliveryKey string, sequence int64, text string, _ time.Time) error {
	s.progress = append(s.progress, fakeChannelProgress{
		RunID: runID, Kind: kind, DeliveryKey: deliveryKey, Sequence: sequence, Text: text,
	})
	return nil
}

func (s *fakeRunStore) FailRun(_ context.Context, runID uuid.UUID, code, message string, now time.Time) error {
	run := s.runs[runID]
	if run == nil {
		return ErrRunNotFound
	}
	if run.CancelRequestedAt != nil {
		run.Status = RunStatusCanceled
	} else {
		run.Status = RunStatusFailed
	}
	run.ErrorCode, run.ErrorMessage, run.CompletedAt, run.UpdatedAt = code, message, &now, now
	return nil
}

func (s *fakeRunStore) RequestRunCancel(_ context.Context, scope Scope, digitalHumanID, runID uuid.UUID, now time.Time) (*Run, error) {
	run, err := s.RunByID(context.Background(), scope, digitalHumanID, runID)
	if err != nil {
		return nil, err
	}
	switch run.Status {
	case RunStatusQueued:
		run.Status, run.CompletedAt = RunStatusCanceled, &now
	case RunStatusRunning:
		run.Status = RunStatusCanceling
	case RunStatusCanceling, RunStatusCanceled:
	default:
		return nil, ErrRunConflict
	}
	run.CancelRequestedAt, run.UpdatedAt = &now, now
	return run, nil
}

func (s *fakeRunStore) ClaimRunOutboxBatch(_ context.Context, limit int, now time.Time) ([]*RunOutbox, error) {
	items := make([]*RunOutbox, 0, limit)
	for _, item := range s.outbox {
		if len(items) == limit {
			break
		}
		if item.Status == RunOutboxPending && !item.AvailableAt.After(now) {
			item.Status, item.ClaimedAt = RunOutboxProcessing, &now
			item.AttemptCount++
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *fakeRunStore) ConfirmRunOutbox(_ context.Context, id uuid.UUID, now time.Time) error {
	for _, item := range s.outbox {
		if item.ID == id {
			item.Status, item.ConfirmedAt = RunOutboxConfirmed, &now
			return nil
		}
	}
	return errors.New("outbox not found")
}

func (s *fakeRunStore) RetryRunOutbox(_ context.Context, id uuid.UUID, availableAt time.Time, message string) error {
	for _, item := range s.outbox {
		if item.ID == id {
			item.Status, item.AvailableAt, item.ErrorMessage, item.ClaimedAt = RunOutboxPending, availableAt, message, nil
			return nil
		}
	}
	return errors.New("outbox not found")
}

func (s *fakeRunStore) RequeueStaleRunOutbox(_ context.Context, claimedBefore, now time.Time) error {
	for _, item := range s.outbox {
		if item.Status == RunOutboxProcessing && item.ClaimedAt != nil && item.ClaimedAt.Before(claimedBefore) {
			item.Status, item.AvailableAt, item.ClaimedAt = RunOutboxPending, now, nil
		}
	}
	return nil
}

func (s *fakeRunStore) MarkStaleRunsUnknown(_ context.Context, updatedBefore, now time.Time) error {
	for _, run := range s.runs {
		if (run.Status == RunStatusRunning || run.Status == RunStatusCanceling) && run.UpdatedAt.Before(updatedBefore) {
			run.Status, run.CompletedAt = RunStatusUnknown, &now
		}
	}
	return nil
}

type fakeRunDispatcher struct {
	fail     bool
	enqueued []uuid.UUID
	canceled []uuid.UUID
}

func (d *fakeRunDispatcher) EnqueueRun(_ context.Context, id uuid.UUID) error {
	d.enqueued = append(d.enqueued, id)
	if d.fail {
		return errors.New("queue unavailable")
	}
	return nil
}

func (d *fakeRunDispatcher) CancelRun(_ context.Context, id uuid.UUID) error {
	d.canceled = append(d.canceled, id)
	return nil
}

type fakeDigitalHumanChat struct {
	caller                chatruntime.Caller
	config                chatruntime.RunConfig
	request               runtimedto.ChatRequest
	prepareCalls          int
	ensureCalls           int
	ensuredConversationID uuid.UUID
	skills                []skills.SkillDiscoveryMetadata
	events                []chatruntime.StreamEvent
	recoveryMessage       *runtimemodel.Message
	stopMessage           *runtimemodel.Message
	getMessageCalls       int
	runCalls              int
	followUpContext       *chatruntime.RuntimeInputFollowUpContext
	followUpPrepared      *chatruntime.PreparedChat
	followUpFailure       error
	activeConversation    *runtimemodel.Conversation
	submittedRuntimeInput *chatruntime.SubmitRuntimeInputRequest
	regenerationRequest   runtimedto.RegenerateMessageRequest
	regenerationMessageID uuid.UUID
}

func (c *fakeDigitalHumanChat) GetConversationByCaller(_ context.Context, _ chatruntime.Scope, _ chatruntime.Caller, conversationID uuid.UUID) (*runtimemodel.Conversation, error) {
	if c.activeConversation == nil || c.activeConversation.ID != conversationID {
		return nil, chatruntime.ErrNotFound
	}
	return c.activeConversation, nil
}

func (c *fakeDigitalHumanChat) SubmitRuntimeInput(_ context.Context, _ chatruntime.Scope, _ chatruntime.Caller, config chatruntime.RunConfig, request chatruntime.SubmitRuntimeInputRequest) (*chatruntime.SubmitRuntimeInputResult, error) {
	if c.activeConversation == nil || c.activeConversation.ActiveMessageID == nil || *c.activeConversation.ActiveMessageID != request.TargetMessageID {
		return nil, chatruntime.ErrConversationIdle
	}
	c.config = config
	c.submittedRuntimeInput = &request
	return &chatruntime.SubmitRuntimeInputResult{Created: true, Disposition: chatruntime.RuntimeInputDispositionCurrentRun}, nil
}

func (c *fakeDigitalHumanChat) GetRuntimeInputFollowUpContext(_ context.Context, _ uuid.UUID) (*chatruntime.RuntimeInputFollowUpContext, error) {
	if c.followUpContext == nil {
		return nil, chatruntime.ErrNotFound
	}
	return c.followUpContext, nil
}

func (c *fakeDigitalHumanChat) PrepareRuntimeInputFollowUp(_ context.Context, _ chatruntime.RunConfig, _ uuid.UUID) (*chatruntime.PreparedChat, error) {
	return c.followUpPrepared, nil
}

func (c *fakeDigitalHumanChat) FailRuntimeInputFollowUp(_ context.Context, _ uuid.UUID, cause error) error {
	c.followUpFailure = cause
	return nil
}

func (c *fakeDigitalHumanChat) EnsureConversationForCaller(_ context.Context, _ chatruntime.Scope, caller chatruntime.Caller, id uuid.UUID, _ string) (*runtimemodel.Conversation, error) {
	c.caller = caller
	c.ensureCalls++
	c.ensuredConversationID = id
	return &runtimemodel.Conversation{ID: id}, nil
}

func (c *fakeDigitalHumanChat) PrepareConfiguredChat(_ context.Context, _ chatruntime.Scope, caller chatruntime.Caller, config chatruntime.RunConfig, request runtimedto.ChatRequest) (*chatruntime.PreparedChat, error) {
	c.prepareCalls++
	c.caller, c.config, c.request = caller, config, request
	conversationID := uuid.New()
	if request.ConversationID != "" {
		if parsed, err := uuid.Parse(request.ConversationID); err == nil {
			conversationID = parsed
		}
	}
	return &chatruntime.PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: uuid.New()},
	}, nil
}

func (c *fakeDigitalHumanChat) PrepareConfiguredRootRegeneration(_ context.Context, _ chatruntime.Scope, caller chatruntime.Caller, config chatruntime.RunConfig, messageID uuid.UUID, request runtimedto.RegenerateMessageRequest) (*chatruntime.PreparedChat, error) {
	c.caller, c.config = caller, config
	c.regenerationMessageID, c.regenerationRequest = messageID, request
	conversationID := uuid.New()
	if c.recoveryMessage != nil && c.recoveryMessage.ConversationID != uuid.Nil {
		conversationID = c.recoveryMessage.ConversationID
	}
	return &chatruntime.PreparedChat{
		Conversation: &runtimemodel.Conversation{ID: conversationID},
		Message:      &runtimemodel.Message{ID: messageID},
	}, nil
}

func (c *fakeDigitalHumanChat) RunPreparedStream(ctx context.Context, _ *chatruntime.PreparedChat, _ func(string) error, callbacks ...func(chatruntime.StreamEvent) error) (*chatruntime.ChatResult, error) {
	c.runCalls++
	if len(callbacks) > 0 && callbacks[0] != nil {
		for _, event := range c.events {
			_ = callbacks[0](event)
		}
	}
	return &chatruntime.ChatResult{
		Answer: "completed answer", Status: runtimemodel.MessageStatusCompleted,
		Metadata: map[string]interface{}{
			"skill_invocations": []interface{}{map[string]interface{}{
				"runtime_id": "skill:1", "skill_id": "calculator", "tool_name": "calculate", "status": "succeeded",
				"arguments": map[string]interface{}{"api_key": "must-not-persist"}, "result": "must-not-persist",
			}},
			"generated_files": []interface{}{map[string]interface{}{
				"artifact_id": "tool_file:tool-file-1", "file_id": "tool-file-1", "tool_file_id": "tool-file-1",
				"filename": "report.pdf", "extension": ".pdf", "transfer_method": "tool_file",
				"download_url": "https://stale.example/download?secret=must-not-persist",
			}},
		},
	}, nil
}

func (c *fakeDigitalHumanChat) GetMessageByCaller(_ context.Context, _ chatruntime.Scope, caller chatruntime.Caller, _ uuid.UUID) (*runtimemodel.Message, *runtimemodel.Conversation, error) {
	c.getMessageCalls++
	c.caller = caller
	if c.recoveryMessage == nil {
		return nil, nil, errors.New("message not found")
	}
	return c.recoveryMessage, &runtimemodel.Conversation{ID: c.recoveryMessage.ConversationID}, nil
}

func (c *fakeDigitalHumanChat) StopMessage(context.Context, chatruntime.Scope, uuid.UUID) (*runtimemodel.Message, error) {
	if c.stopMessage != nil {
		return c.stopMessage, nil
	}
	return &runtimemodel.Message{}, nil
}

func (c *fakeDigitalHumanChat) ListSkills(context.Context, chatruntime.Scope) ([]skills.SkillDiscoveryMetadata, error) {
	return append([]skills.SkillDiscoveryMetadata(nil), c.skills...), nil
}

type fakeExecutionResolver struct {
	now time.Time
}

func (r fakeExecutionResolver) Resolve(_ context.Context, event invocation.Event, _ *uuid.UUID) (*ResolvedExecution, error) {
	snapshot := ExecutionSnapshot{
		SchemaVersion: executionSnapshotSchemaV1, Subject: event.Target, DigitalHumanVersion: uuid.New(),
		IdentityName: "Atlas", Persona: "Researcher", Rules: []string{"cite evidence"}, Model: "test-model",
		UseUserMemory: false, UseAgentMemory: false, ResolvedAt: r.now,
	}
	var err error
	snapshot.Revision, err = executionSnapshotRevision(snapshot)
	if err != nil {
		return nil, err
	}
	spec, err := runspec.Seal(runspec.Spec{
		InvocationID: event.ID, Subject: event.Target,
		Identity: runspec.VersionRef{Kind: "identity", ID: snapshot.DigitalHumanVersion.String(), Revision: snapshot.Revision},
		Model:    runspec.VersionRef{Kind: "model", ID: snapshot.Model, Revision: snapshot.Revision},
		Policies: []runspec.VersionRef{{Kind: "execution_snapshot", ID: snapshot.DigitalHumanVersion.String(), Revision: snapshot.Revision}},
		Budget:   runspec.Budget{MaxDurationSeconds: 300}, ResolvedAt: r.now,
	})
	return &ResolvedExecution{Spec: spec, Snapshot: snapshot}, err
}

type fakeBrainAdapter struct{}

func (fakeBrainAdapter) RunConfig(resolved *ResolvedExecution) (chatruntime.RunConfig, error) {
	return chatruntime.RunConfig{
		SystemPrompt: resolved.Snapshot.Persona, Model: resolved.Snapshot.Model,
		BillingAppID: resolved.Snapshot.Subject.ID.String(), BillingAppType: runtimemodel.ConversationCallerDigitalHuman,
		UseMemory: false, AgentMemoryEnabled: false, AgentMemoryAutoExtractionEnabled: false,
	}, nil
}

type fakeMemoryRuntimeProvider struct {
	preparation   *MemoryRuntimePreparation
	calls         int
	finalizeCalls int
}

type fakeChannelContextRuntimeProvider struct {
	preparation *ChannelContextPreparation
	err         error
}

type manualInvocationFileValidatorStub struct {
	normalized []string
	err        error
	scope      Scope
}

func (v *manualInvocationFileValidatorStub) Validate(_ context.Context, scope Scope, _ []string) ([]string, error) {
	v.scope = scope
	return append([]string(nil), v.normalized...), v.err
}

func (p *fakeChannelContextRuntimeProvider) Prepare(context.Context, *Run, *ResolvedExecution, invocation.Event, string, string) (*ChannelContextPreparation, error) {
	return p.preparation, p.err
}

func (p *fakeMemoryRuntimeProvider) Prepare(context.Context, *Run, *ResolvedExecution, invocation.Event) (*MemoryRuntimePreparation, error) {
	p.calls++
	return p.preparation, nil
}

func (p *fakeMemoryRuntimeProvider) Finalize(context.Context, *Run, *chatruntime.PreparedChat, *chatruntime.ChatResult) error {
	p.finalizeCalls++
	return nil
}

func newTestRuntimeService(t *testing.T, store *fakeRunStore, dispatcher *fakeRunDispatcher, chat *fakeDigitalHumanChat) *RuntimeService {
	t.Helper()
	now := time.Date(2026, 8, 18, 6, 0, 0, 0, time.UTC)
	service := NewRuntimeService(
		store, fakeExecutionResolver{now: now}, fakeBrainAdapter{}, chat, dispatcher,
		WithChannelTransientDispatcher(&channelTransientDispatcherStub{}),
	)
	service.now = func() time.Time { return now }
	return service
}

func testRunScope() Scope {
	return Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New()}
}

func advancingClock(start time.Time) func() time.Time {
	current := start
	return func() time.Time {
		current = current.Add(time.Second)
		return current
	}
}
