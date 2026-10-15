package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
)

type externalContinuationFixture struct {
	svc            *service
	mock           sqlmock.Sqlmock
	message        *runtimemodel.Message
	scope          Scope
	caller         Caller
	config         RunConfig
	conversationID uuid.UUID
}

type sourcedConversationRepo struct {
	*callerScopedStreamConversationRepo
	source string
}

func (r sourcedConversationRepo) GetByCallerScoped(ctx context.Context, id, organizationID, accountID uuid.UUID, callerType string, callerID, externalUserID *uuid.UUID, conversationType string) (*runtimemodel.Conversation, error) {
	conversation, err := r.callerScopedStreamConversationRepo.GetByCallerScoped(ctx, id, organizationID, accountID, callerType, callerID, externalUserID, conversationType)
	if conversation != nil {
		conversation.Source = r.source
	}
	return conversation, err
}

// newExternalContinuationFixture is an API conversation whose workflow
// approval belongs to designated approvers (ui_approval_allowed=false).
func newExternalContinuationFixture(t *testing.T, status, kind string) externalContinuationFixture {
	t.Helper()
	digitalHumanID, conversationID := uuid.New(), uuid.New()
	message := &runtimemodel.Message{
		ID: uuid.New(), ConversationID: conversationID, Status: status,
		Metadata: map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{
			"workflow_run_id": "run-1", "kind": kind, "binding_id": "binding-1", "ui_approval_allowed": false,
		}},
	}
	db, mock := newAgentBindingStepTestDB(t)
	return externalContinuationFixture{
		svc: &service{repos: &repository.Repositories{
			Access: callerScopedStreamAccessRepo{},
			Conversation: sourcedConversationRepo{
				callerScopedStreamConversationRepo: &callerScopedStreamConversationRepo{allowedCallerType: runtimemodel.ConversationCallerDigitalHuman, allowedCallerID: digitalHumanID},
				source:                             runtimemodel.ConversationSourceExternalAPI,
			},
			Message: &callerScopedStreamMessageRepo{message: message},
			DB:      db,
		}},
		mock: mock, message: message, conversationID: conversationID,
		scope:  Scope{OrganizationID: uuid.New(), AccountID: uuid.New()},
		caller: Caller{Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID, Source: runtimemodel.ConversationSourceExternalAPI},
		config: RunConfig{WorkflowBindings: []AgentWorkflowBinding{{BindingID: "binding-1"}}},
	}
}

func TestExternalWorkflowContinuationClaimsADesignatedApproverApproval(t *testing.T) {
	f := newExternalContinuationFixture(t, runtimemodel.MessageStatusWaitingApproval, "")
	if _, err := f.svc.BeginWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("conversation submit error = %v, want the approval kept with its approvers", err)
	}
	f.mock.ExpectBegin()
	f.mock.ExpectExec(`UPDATE "chat_runtime_conversations"`).WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectExec(`UPDATE "chat_runtime_messages" SET "metadata"`).WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectExec(`UPDATE "chat_runtime_messages" SET "status"`).WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectCommit()
	continuation, err := f.svc.BeginExternalWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID)
	if err != nil || continuation == nil || continuation.WorkflowRunID != "run-1" {
		t.Fatalf("BeginExternalWorkflowApprovalContinuation() = %#v, %v", continuation, err)
	}
	if !workflowContinuationExternallyResumed(continuation.Metadata) {
		t.Fatalf("continuation metadata = %#v, want the external claim recorded", continuation.Metadata)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExternalWorkflowContinuationClaimsTheMessageOnce(t *testing.T) {
	f := newExternalContinuationFixture(t, runtimemodel.MessageStatusWaitingApproval, "")
	f.mock.ExpectBegin()
	f.mock.ExpectExec(`UPDATE "chat_runtime_conversations"`).WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectExec(`UPDATE "chat_runtime_messages" SET "metadata"`).WillReturnResult(sqlmock.NewResult(0, 1))
	f.mock.ExpectExec(`UPDATE "chat_runtime_messages" SET "status"`).WillReturnResult(sqlmock.NewResult(0, 0))
	f.mock.ExpectRollback()
	if _, err := f.svc.BeginExternalWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("second claim error = %v, want ErrInvalidInput", err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExternalWorkflowContinuationOnlyObservesWaitingWorkflowApprovals(t *testing.T) {
	for name, f := range map[string]externalContinuationFixture{
		"question":          newExternalContinuationFixture(t, runtimemodel.MessageStatusWaitingQuestion, ""),
		"streaming":         newExternalContinuationFixture(t, runtimemodel.MessageStatusStreaming, ""),
		"DigitalHuman task": newExternalContinuationFixture(t, runtimemodel.MessageStatusWaitingApproval, "digital_human_task"),
	} {
		if _, err := f.svc.BeginExternalWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: error = %v, want ErrInvalidInput", name, err)
		}
		if err := f.mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestExternalWorkflowContinuationRetakesItsOwnClaimAfterLosingTheWorker(t *testing.T) {
	f := newExternalContinuationFixture(t, runtimemodel.MessageStatusStreaming, "")
	f.message.Metadata["agent_workflow_continuation"].(map[string]interface{})[workflowContinuationExternalResumeKey] = true
	continuation, err := f.svc.BeginExternalWorkflowApprovalContinuation(context.Background(), f.scope, f.caller, f.config, f.conversationID, f.message.ID)
	if err != nil || continuation == nil || continuation.WorkflowRunID != "run-1" {
		t.Fatalf("re-take = %#v, %v; want the externally claimed message taken over", continuation, err)
	}
	if err := f.mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("re-take wrote to the database: %v", err)
	}

	inline := newExternalContinuationFixture(t, runtimemodel.MessageStatusStreaming, "")
	if _, err := inline.svc.BeginExternalWorkflowApprovalContinuation(context.Background(), inline.scope, inline.caller, inline.config, inline.conversationID, inline.message.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("taking over a continuation the conversation started: err = %v, want ErrInvalidInput", err)
	}
}

func TestConversationContinuationClearsAnEarlierExternalClaim(t *testing.T) {
	metadata := map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "run-1", workflowContinuationExternalResumeKey: true}}
	if !markWorkflowContinuationExternalResume(metadata, false) || workflowContinuationExternallyResumed(metadata) {
		t.Fatalf("metadata = %#v, want the external claim cleared", metadata)
	}
	if markWorkflowContinuationExternalResume(metadata, false) {
		t.Fatal("clearing an absent claim must not rewrite the metadata")
	}
	if !markWorkflowContinuationExternalResume(metadata, true) || !workflowContinuationExternallyResumed(metadata) {
		t.Fatalf("metadata = %#v, want the external claim recorded", metadata)
	}
}

func TestContinuedApprovalExposureFollowsTheWorkflowToolRule(t *testing.T) {
	digitalHumanID := uuid.New()
	webApp := func(enabled interface{}) map[string]interface{} {
		methods := map[string]interface{}{}
		if enabled != nil {
			methods["enabled"] = enabled
		}
		return map[string]interface{}{"approval_form": map[string]interface{}{"submit_methods": map[string]interface{}{"webapp": methods}}}
	}
	typed := map[string]interface{}{"submit_methods": struct {
		WebApp struct {
			Enabled *bool `json:"enabled"`
		} `json:"webapp"`
	}{}}
	for name, test := range map[string]struct {
		caller  Caller
		payload map[string]interface{}
		want    bool
	}{
		"console":                      {Caller{Source: runtimemodel.ConversationSourceConsole}, webApp(false), true},
		"webapp approval on webapp":    {Caller{Source: runtimemodel.ConversationSourceWebApp}, webApp(true), true},
		"email approval on webapp":     {Caller{Source: runtimemodel.ConversationSourceWebApp}, webApp(false), false},
		"legacy approval on api":       {Caller{Source: runtimemodel.ConversationSourceExternalAPI}, webApp(nil), true},
		"typed form on api":            {Caller{Source: runtimemodel.ConversationSourceExternalAPI}, typed, true},
		"digital human channel":        {Caller{Source: runtimemodel.ConversationSourceChannel, Type: runtimemodel.ConversationCallerDigitalHuman, ID: &digitalHumanID}, webApp(true), true},
		"agent channel":                {Caller{Source: runtimemodel.ConversationSourceChannel, Type: runtimemodel.ConversationCallerAgent}, webApp(true), false},
		"already exposed by the event": {Caller{Source: runtimemodel.ConversationSourceWebApp}, map[string]interface{}{"ui_approval_allowed": true}, true},
	} {
		if got := workflowContinuationApprovalUIAllowed(test.caller, test.payload); got != test.want {
			t.Fatalf("%s: ui approval allowed = %v, want %v", name, got, test.want)
		}
	}
}

func TestContinuationWaitsOnTheApprovalItRecords(t *testing.T) {
	for name, test := range map[string]struct {
		enabled bool
	}{
		"approval answered in the conversation": {true},
		"approval kept with its approvers":      {false},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &service{repos: &repository.Repositories{Message: &workflowContinuationCheckpointMessageRepo{}, Conversation: &workflowContinuationCheckpointConversationRepo{}}}
			continuation := &WorkflowApprovalContinuation{
				ConversationID: uuid.New(), MessageID: uuid.New(), WorkflowRunID: "run-1",
				Caller: Caller{Source: runtimemodel.ConversationSourceExternalAPI},
				Metadata: map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{
					"workflow_run_id": "run-1", "ui_approval_allowed": false, "approval_token": "",
				}},
			}
			form := map[string]interface{}{"id": "form-2", "token": "token-2", "submit_methods": map[string]interface{}{"webapp": map[string]interface{}{"enabled": test.enabled}}}
			event, err := svc.RecordWorkflowApprovalContinuationEvent(t.Context(), continuation, "approval_requested", map[string]interface{}{
				"node_id": "approval-2", "form_id": "form-2", "approval_token": "token-2", "approval_form": form,
			})
			if err != nil {
				t.Fatal(err)
			}
			record := continuation.Metadata["agent_workflow_continuation"].(map[string]interface{})
			if record["ui_approval_allowed"] != test.enabled || continuation.UIApprovalAllowed != test.enabled || event.Payload["ui_approval_allowed"] != test.enabled {
				t.Fatalf("record = %#v, event = %#v, want ui_approval_allowed %v", record, event.Payload, test.enabled)
			}
			if record["approval_form"] == nil {
				t.Fatalf("record = %#v, want the new approval's form", record)
			}
			// The approval token opens the approval page: only an approval the
			// conversation may answer carries it.
			if leaked := strings.Contains(workflowContinuationTestJSON(t, event.Payload)+workflowContinuationTestJSON(t, record), "token-2"); leaked != test.enabled {
				t.Fatalf("record = %#v, event = %#v, want the approval token exposed %v", record, event.Payload, test.enabled)
			}
			if test.enabled && record["approval_token"] != "token-2" {
				t.Fatalf("record = %#v, want the new approval's token", record)
			}
		})
	}
}

func TestContinuationEventsOtherThanAnAnswerableApprovalCarryNoApprovalToken(t *testing.T) {
	svc := &service{repos: &repository.Repositories{Message: &workflowContinuationCheckpointMessageRepo{}, Conversation: &workflowContinuationCheckpointConversationRepo{}}}
	continuation := &WorkflowApprovalContinuation{
		ConversationID: uuid.New(), MessageID: uuid.New(), WorkflowRunID: "run-1",
		Caller:   Caller{Source: runtimemodel.ConversationSourceWebApp},
		Metadata: map[string]interface{}{"agent_workflow_continuation": map[string]interface{}{"workflow_run_id": "run-1"}},
	}
	event, err := svc.AppendWorkflowApprovalContinuationStreamEvent(t.Context(), continuation, "workflow_paused", map[string]interface{}{
		"outputs": map[string]interface{}{
			"__approval_token": "token-3", "approval_token": "token-3",
			"__approval_form": map[string]interface{}{"id": "form-3", "token": "token-3"},
			"text":            "kept",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := workflowContinuationTestJSON(t, event.Payload)
	if strings.Contains(payload, "token-3") || !strings.Contains(payload, "kept") || !strings.Contains(payload, "form-3") {
		t.Fatalf("event = %s, want the approval token stripped and the rest kept", payload)
	}
}

func workflowContinuationTestJSON(t *testing.T, value interface{}) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
