package digitalhuman

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// pendingConfirmationFixture builds one durably waiting DigitalHuman run whose
// ChatRuntime message carries a complete, server-authored confirmation event.
type pendingConfirmationFixture struct {
	store          *fakeRunStore
	chat           *fakeDigitalHumanChat
	service        *RuntimeService
	scope          Scope
	runID          uuid.UUID
	digitalHumanID uuid.UUID
	versionID      uuid.UUID
	conversationID uuid.UUID
	messageID      uuid.UUID
	receipt        uuid.UUID
	instanceID     string
}

func newPendingConfirmationFixture(t *testing.T) *pendingConfirmationFixture {
	t.Helper()
	store, dispatcher := newFakeRunStore(), &fakeRunDispatcher{}
	fixture := &pendingConfirmationFixture{
		store: store, scope: testRunScope(),
		runID: uuid.New(), digitalHumanID: uuid.New(), versionID: uuid.New(),
		conversationID: uuid.New(), messageID: uuid.New(), receipt: uuid.New(), instanceID: uuid.NewString(),
	}
	fixture.chat = &fakeDigitalHumanChat{recoveryMessage: fixture.waitingMessage(fixture.requiredEvent())}
	fixture.service = newTestRuntimeService(t, store, dispatcher, fixture.chat)
	store.runs[fixture.runID] = fixture.waitingRun()
	return fixture
}

func (f *pendingConfirmationFixture) waitingRun() *Run {
	return &Run{
		ID: f.runID, OrganizationID: f.scope.OrganizationID, WorkspaceID: f.scope.WorkspaceID,
		DigitalHumanID: f.digitalHumanID, DigitalHumanVersionID: f.versionID,
		Status: RunStatusWaiting, WaitingReason: runtimemodel.MessageStatusWaitingApproval,
		WaitingReceipt: pendingConfirmationUUID(f.receipt),
		ConversationID: pendingConfirmationUUID(f.conversationID), MessageID: pendingConfirmationUUID(f.messageID),
	}
}

func (f *pendingConfirmationFixture) waitingMessage(event map[string]interface{}) *runtimemodel.Message {
	metadata := map[string]interface{}{
		"model_invocations": []interface{}{map[string]interface{}{
			"kind": "model_call", "phase": "skill_planning", "status": "success",
			"request":    map[string]interface{}{"model": "test-model"},
			"response":   map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "content": "calling connector"}},
			"runtime_id": "model_call:skill_planning:0:1",
		}},
	}
	if event != nil {
		metadata[connectorConfirmationEventsMetadataKey] = []interface{}{event}
	}
	return &runtimemodel.Message{
		ID: f.messageID, ConversationID: f.conversationID,
		Query: "send it", Status: runtimemodel.MessageStatusWaitingApproval,
		ModelName: "test-model", Metadata: metadata,
	}
}

func (f *pendingConfirmationFixture) requiredEvent() map[string]interface{} {
	return map[string]interface{}{
		"event_type":      connectorConfirmationRequiredEventType,
		"route_receipt":   f.receipt.String(),
		"conversation_id": f.conversationID.String(),
		"message_id":      f.messageID.String(),
		"state":           "requires_confirmation",
		"status":          runtimemodel.MessageStatusWaitingApproval,
		"blocking":        true,
		"confirmation":    f.validConfirmation(),
	}
}

func (f *pendingConfirmationFixture) validConfirmation() map[string]interface{} {
	return map[string]interface{}{
		"route_receipt":  f.receipt.String(),
		"connector_id":   "github",
		"connector_name": "GitHub",
		"operation_id":   "github.issue.create",
		"action_name":    "Create issue",
		"target": map[string]interface{}{
			"type": connectorcore.TargetTypeDigitalHuman, "id": f.digitalHumanID.String(),
			"scope": connectorcore.ScopePublished, "version_id": f.versionID.String(),
			"binding_instance_id": f.instanceID,
		},
		"consequence":         map[string]interface{}{"external_effect": "create_issue", "data_egress": true},
		"risk_classification": string(connectorexecution.RiskHigh),
	}
}

func (f *pendingConfirmationFixture) event() map[string]interface{} {
	events, _ := f.chat.recoveryMessage.Metadata[connectorConfirmationEventsMetadataKey].([]interface{})
	if len(events) == 0 {
		return nil
	}
	record, _ := events[0].(map[string]interface{})
	return record
}

func (f *pendingConfirmationFixture) confirmationRecord() map[string]interface{} {
	record, _ := f.event()["confirmation"].(map[string]interface{})
	return record
}

func (f *pendingConfirmationFixture) targetRecord() map[string]interface{} {
	record, _ := f.confirmationRecord()["target"].(map[string]interface{})
	return record
}

func (f *pendingConfirmationFixture) consequenceRecord() map[string]interface{} {
	record, _ := f.confirmationRecord()["consequence"].(map[string]interface{})
	return record
}

func (f *pendingConfirmationFixture) replaceEvents(events []interface{}) {
	f.chat.recoveryMessage.Metadata[connectorConfirmationEventsMetadataKey] = events
}

func TestRuntimeServiceProjectsVerifiedPendingConnectorConfirmation(t *testing.T) {
	fixture := newPendingConfirmationFixture(t)

	log, err := fixture.service.GetRunRuntimeLog(t.Context(), fixture.scope, fixture.digitalHumanID, fixture.runID)
	if err != nil {
		t.Fatalf("GetRunRuntimeLog() error = %v", err)
	}
	confirmation := log.PendingConnectorConfirmation
	if confirmation == nil {
		t.Fatal("pending_connector_confirmation was omitted for a verified waiting run")
	}
	if string(confirmation.RouteReceipt) != fixture.receipt.String() {
		t.Fatalf("route receipt = %q, want %s", confirmation.RouteReceipt, fixture.receipt)
	}
	if confirmation.ConnectorID != "github" || confirmation.ConnectorName != "GitHub" ||
		confirmation.OperationID != "github.issue.create" || confirmation.ActionName != "Create issue" {
		t.Fatalf("confirmation display = %#v", confirmation)
	}
	if confirmation.Target.Type != connectorcore.TargetTypeDigitalHuman ||
		confirmation.Target.ID != fixture.digitalHumanID.String() ||
		confirmation.Target.VersionID != fixture.versionID.String() ||
		confirmation.Target.BindingInstanceID != fixture.instanceID {
		t.Fatalf("confirmation target = %#v", confirmation.Target)
	}
	if confirmation.Risk != connectorexecution.RiskHigh {
		t.Fatalf("confirmation risk = %q, want high", confirmation.Risk)
	}
	// Preview and the frozen operation authority belong to the owner-scoped
	// receipt status endpoint, never to this projection.
	if confirmation.Preview != nil || confirmation.AcceptedOperation != nil {
		t.Fatalf("projection carried non-public authority: %#v", confirmation)
	}
	// Existing Steps and Detail survive the new field.
	if log.Detail == nil || len(log.Steps) != 1 || log.Steps[0].Type != "model_call" {
		t.Fatalf("runtime log steps/detail = %#v", log)
	}
	encoded, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"pending_connector_confirmation"`)) {
		t.Fatalf("projection missing from JSON: %s", encoded)
	}
	if bytes.Contains(encoded, []byte(`"accepted_operation"`)) || bytes.Contains(encoded, []byte(`"preview"`)) {
		t.Fatalf("projection JSON exposed non-public fields: %s", encoded)
	}
}

func TestRuntimeServiceWithholdsPendingConnectorConfirmationOnUnverifiedState(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*pendingConfirmationFixture)
	}{
		{"run not waiting", func(f *pendingConfirmationFixture) { f.store.runs[f.runID].Status = RunStatusSucceeded }},
		{"waiting reason not approval", func(f *pendingConfirmationFixture) {
			f.store.runs[f.runID].WaitingReason = runtimemodel.MessageStatusWaitingQuestion
		}},
		{"nil waiting receipt", func(f *pendingConfirmationFixture) { f.store.runs[f.runID].WaitingReceipt = nil }},
		{"zero waiting receipt", func(f *pendingConfirmationFixture) {
			f.store.runs[f.runID].WaitingReceipt = pendingConfirmationUUID(uuid.Nil)
		}},
		{"nil run message id", func(f *pendingConfirmationFixture) { f.store.runs[f.runID].MessageID = nil }},
		{"nil run conversation id", func(f *pendingConfirmationFixture) { f.store.runs[f.runID].ConversationID = nil }},
		{"mismatched run message id", func(f *pendingConfirmationFixture) {
			f.store.runs[f.runID].MessageID = pendingConfirmationUUID(uuid.New())
		}},
		{"mismatched run conversation id", func(f *pendingConfirmationFixture) {
			f.store.runs[f.runID].ConversationID = pendingConfirmationUUID(uuid.New())
		}},
		{"mismatched run receipt", func(f *pendingConfirmationFixture) {
			f.store.runs[f.runID].WaitingReceipt = pendingConfirmationUUID(uuid.New())
		}},
		{"message not waiting approval", func(f *pendingConfirmationFixture) {
			f.chat.recoveryMessage.Status = runtimemodel.MessageStatusCompleted
		}},
		{"mismatched event message id", func(f *pendingConfirmationFixture) { f.event()["message_id"] = uuid.NewString() }},
		{"mismatched event conversation id", func(f *pendingConfirmationFixture) {
			f.event()["conversation_id"] = uuid.NewString()
		}},
		{"mismatched event receipt", func(f *pendingConfirmationFixture) { f.event()["route_receipt"] = uuid.NewString() }},
		{"missing confirmation record", func(f *pendingConfirmationFixture) { delete(f.event(), "confirmation") }},
		{"mismatched confirmation receipt", func(f *pendingConfirmationFixture) {
			f.confirmationRecord()["route_receipt"] = uuid.NewString()
		}},
		{"target is not digital human", func(f *pendingConfirmationFixture) {
			f.targetRecord()["type"] = string(connectorcore.TargetTypeChat)
		}},
		{"target id mismatch", func(f *pendingConfirmationFixture) { f.targetRecord()["id"] = uuid.NewString() }},
		{"target version mismatch", func(f *pendingConfirmationFixture) {
			f.targetRecord()["version_id"] = uuid.NewString()
		}},
		{"target scope not published", func(f *pendingConfirmationFixture) {
			f.targetRecord()["scope"] = string(connectorcore.ScopeDraft)
		}},
		{"target binding instance not a uuid", func(f *pendingConfirmationFixture) {
			f.targetRecord()["binding_instance_id"] = "not-a-uuid"
		}},
		{"target binding instance missing", func(f *pendingConfirmationFixture) {
			delete(f.targetRecord(), "binding_instance_id")
		}},
		{"trusted low risk", func(f *pendingConfirmationFixture) {
			f.confirmationRecord()["risk_classification"] = string(connectorexecution.RiskTrustedLow)
		}},
		{"risk missing", func(f *pendingConfirmationFixture) { delete(f.confirmationRecord(), "risk_classification") }},
		{"connector name missing", func(f *pendingConfirmationFixture) { f.confirmationRecord()["connector_name"] = "  " }},
		{"action name missing", func(f *pendingConfirmationFixture) { f.confirmationRecord()["action_name"] = "" }},
		{"connector id missing", func(f *pendingConfirmationFixture) { f.confirmationRecord()["connector_id"] = "" }},
		{"external effect missing", func(f *pendingConfirmationFixture) {
			f.consequenceRecord()["external_effect"] = ""
		}},
		{"duplicate required events", func(f *pendingConfirmationFixture) {
			f.replaceEvents([]interface{}{f.event(), pendingConfirmationClone(f.event())})
		}},
		{"malformed event element", func(f *pendingConfirmationFixture) {
			f.replaceEvents([]interface{}{f.event(), "not-an-object"})
		}},
		{"resolved event for the same receipt", func(f *pendingConfirmationFixture) {
			f.replaceEvents([]interface{}{f.event(), map[string]interface{}{
				"event_type": connectorConfirmationResolvedEventType, "route_receipt": f.receipt.String(),
			}})
		}},
		{"malformed event container", func(f *pendingConfirmationFixture) {
			f.chat.recoveryMessage.Metadata[connectorConfirmationEventsMetadataKey] = "not-a-list"
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newPendingConfirmationFixture(t)
			testCase.mutate(fixture)
			log, err := fixture.service.GetRunRuntimeLog(t.Context(), fixture.scope, fixture.digitalHumanID, fixture.runID)
			if err != nil {
				t.Fatalf("GetRunRuntimeLog() error = %v", err)
			}
			if log.PendingConnectorConfirmation != nil {
				t.Fatalf("unverified confirmation was projected: %#v", log.PendingConnectorConfirmation)
			}
		})
	}
}

func TestRuntimeServicePendingConnectorConfirmationOmitsSecretsAndFrozenInput(t *testing.T) {
	const (
		credentialSecret = "connector-credential-fixture"
		inputSecret      = "provider-operation-input"
		previewSecret    = "preview-provider-value"
		metadataSecret   = "raw-runtime-metadata"
	)
	fixture := newPendingConfirmationFixture(t)
	event := fixture.event()
	event["input"] = map[string]interface{}{"body": inputSecret}
	event["runtime_metadata"] = map[string]interface{}{"raw": metadataSecret}
	confirmation := fixture.confirmationRecord()
	confirmation["preview"] = map[string]interface{}{"provider_value": previewSecret}
	confirmation["accepted_operation"] = map[string]interface{}{"credential_ref": credentialSecret}

	log, err := fixture.service.GetRunRuntimeLog(t.Context(), fixture.scope, fixture.digitalHumanID, fixture.runID)
	if err != nil {
		t.Fatalf("GetRunRuntimeLog() error = %v", err)
	}
	if log.PendingConnectorConfirmation == nil {
		t.Fatal("verified confirmation was withheld")
	}
	if log.PendingConnectorConfirmation.Preview != nil || log.PendingConnectorConfirmation.AcceptedOperation != nil {
		t.Fatalf("projection retained non-public authority: %#v", log.PendingConnectorConfirmation)
	}
	encoded, err := json.Marshal(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{credentialSecret, inputSecret, previewSecret, metadataSecret} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("runtime log leaked %q: %s", secret, encoded)
		}
	}
}

func TestRuntimeServiceRequiresOwnerScopeForPendingConnectorConfirmation(t *testing.T) {
	fixture := newPendingConfirmationFixture(t)

	wrongOrganization := fixture.scope
	wrongOrganization.OrganizationID = uuid.New()
	if _, err := fixture.service.GetRunRuntimeLog(t.Context(), wrongOrganization, fixture.digitalHumanID, fixture.runID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("wrong organization error = %v, want ErrRunNotFound", err)
	}
	wrongWorkspace := fixture.scope
	wrongWorkspace.WorkspaceID = uuid.New()
	if _, err := fixture.service.GetRunRuntimeLog(t.Context(), wrongWorkspace, fixture.digitalHumanID, fixture.runID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("wrong workspace error = %v, want ErrRunNotFound", err)
	}
	if _, err := fixture.service.GetRunRuntimeLog(t.Context(), fixture.scope, uuid.New(), fixture.runID); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("wrong digital human error = %v, want ErrRunNotFound", err)
	}
}

func TestRuntimeServiceOmitsPendingConnectorConfirmationWithoutError(t *testing.T) {
	fixture := newPendingConfirmationFixture(t)
	delete(fixture.chat.recoveryMessage.Metadata, connectorConfirmationEventsMetadataKey)
	log, err := fixture.service.GetRunRuntimeLog(t.Context(), fixture.scope, fixture.digitalHumanID, fixture.runID)
	if err != nil || log.PendingConnectorConfirmation != nil {
		t.Fatalf("no-event runtime log = %#v, err %v", log, err)
	}

	messageLess := newPendingConfirmationFixture(t)
	messageLess.store.runs[messageLess.runID].MessageID = nil
	runLess, err := messageLess.service.GetRunRuntimeLog(t.Context(), messageLess.scope, messageLess.digitalHumanID, messageLess.runID)
	if err != nil || runLess.PendingConnectorConfirmation != nil {
		t.Fatalf("message-less runtime log = %#v, err %v", runLess, err)
	}
}

func pendingConfirmationClone(record map[string]interface{}) map[string]interface{} {
	encoded, err := json.Marshal(record)
	if err != nil {
		return nil
	}
	cloned := map[string]interface{}{}
	if json.Unmarshal(encoded, &cloned) != nil {
		return nil
	}
	return cloned
}

func pendingConfirmationUUID(value uuid.UUID) *uuid.UUID {
	cloned := value
	return &cloned
}
