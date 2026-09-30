package service

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRecordConnectorConfirmationResolutionCompletesOwnerScopedMessage(t *testing.T) {
	db, mock := newConnectorConfirmationResolutionTestDB(t)
	organizationID := uuid.New()
	ownerID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	receipt := uuid.NewString()
	decisionID := uuid.New()
	metadataJSON, err := json.Marshal(pendingConnectorResolutionMetadata(receipt))
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT m\.\* FROM chat_runtime_messages AS m.*JOIN chat_runtime_conversations AS c.*FOR UPDATE`).
		WithArgs(messageID, organizationID, ownerID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "conversation_id", "answer", "status", "metadata"}).
			AddRow(messageID, conversationID, "", runtimemodel.MessageStatusWaitingApproval, metadataJSON))
	mock.ExpectQuery(`(?s)SELECT \* FROM "chat_runtime_conversations".*organization_id.*account_id.*LIMIT`).
		WithArgs(conversationID, organizationID, ownerID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "organization_id", "account_id", "caller_type", "conversation_type",
		}).AddRow(
			conversationID, organizationID, ownerID,
			runtimemodel.ConversationCallerAIChat, runtimemodel.ConversationTypeChat,
		))
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .*"status"=.*WHERE .*id = .*status IN`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc := &service{repos: repository.NewRepositories(db)}
	err = svc.RecordConnectorConfirmationResolution(
		context.Background(),
		connectorconfirmation.Command{
			OrganizationID: organizationID, OwnerAccountID: ownerID,
			RouteReceipt: connectorexecution.RouteClaimReceipt(receipt),
			Decision:     connectorexecution.ConfirmationDecisionConfirmed,
		},
		connectorconfirmation.Result{
			State: connectorconfirmation.StateCompleted, DecisionID: decisionID,
			Origin: connectorexecution.ConfirmationOrigin{
				Surface: connectorcore.TargetTypeChat, ConversationID: conversationID.String(), MessageID: messageID.String(),
			},
		},
	)
	if err != nil {
		t.Fatalf("RecordConnectorConfirmationResolution() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRecordConnectorConfirmationResolutionRejectsOwnerMismatch(t *testing.T) {
	db, mock := newConnectorConfirmationResolutionTestDB(t)
	organizationID := uuid.New()
	ownerID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT m\.\* FROM chat_runtime_messages AS m.*FOR UPDATE`).
		WithArgs(sqlmock.AnyArg(), organizationID, ownerID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	err := (&service{repos: repository.NewRepositories(db)}).RecordConnectorConfirmationResolution(
		context.Background(),
		connectorconfirmation.Command{
			OrganizationID: organizationID, OwnerAccountID: ownerID,
			RouteReceipt: connectorexecution.RouteClaimReceipt(uuid.NewString()),
			Decision:     connectorexecution.ConfirmationDecisionCancelled,
		},
		connectorconfirmation.Result{
			State: connectorconfirmation.StateCancelled, DecisionID: uuid.New(),
			Origin: connectorexecution.ConfirmationOrigin{
				Surface:        connectorcore.TargetTypeChat,
				ConversationID: uuid.NewString(), MessageID: uuid.NewString(),
			},
		},
	)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeAccessDenied) {
		t.Fatalf("error = %v, want access denied", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRecordConnectorConfirmationResolutionPersistsRememberedConnectorGrant(t *testing.T) {
	db, mock := newConnectorConfirmationResolutionTestDB(t)
	organizationID, ownerID := uuid.New(), uuid.New()
	conversationID, messageID := uuid.New(), uuid.New()
	connectionID, bindingID := uuid.New(), uuid.New()
	receipt := uuid.NewString()
	metadataJSON, err := json.Marshal(pendingConnectorResolutionMetadata(receipt))
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT m\.\* FROM chat_runtime_messages AS m.*JOIN chat_runtime_conversations AS c.*FOR UPDATE`).
		WithArgs(messageID, organizationID, ownerID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "conversation_id", "answer", "status", "metadata"}).
			AddRow(messageID, conversationID, "", runtimemodel.MessageStatusWaitingApproval, metadataJSON))
	mock.ExpectQuery(`(?s)SELECT \* FROM "chat_runtime_conversations".*organization_id.*account_id.*LIMIT`).
		WithArgs(conversationID, organizationID, ownerID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "caller_type", "conversation_type"}).
			AddRow(conversationID, organizationID, ownerID, runtimemodel.ConversationCallerAIChat, runtimemodel.ConversationTypeChat))
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_conversations" SET .*"metadata"=.*WHERE .*id = .*deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE "chat_runtime_messages" SET .*"status"=.*WHERE .*id = .*status IN`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result := connectorconfirmation.Result{
		State: connectorconfirmation.StateCompleted, DecisionID: uuid.New(),
		Origin: connectorexecution.ConfirmationOrigin{Surface: connectorcore.TargetTypeChat, ConversationID: conversationID.String(), MessageID: messageID.String()},
		RequestIdentity: connectorconfirmation.ConfirmationRequestIdentity{
			OrganizationID: organizationID, OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
			BindingID: bindingID, ConnectionID: connectionID, ConnectorID: "qqmail",
			Target:      connectorcore.TargetRef{Type: connectorcore.TargetTypeChat, ID: conversationID.String()},
			OperationID: "qqmail.message.send", LogicalOperationID: "logical-1",
			CorrelationID: "connector-correlation-" + messageID.String(), Interactive: true,
		},
		Authorization: &connectorconfirmation.AuthorizationSnapshot{
			SkillID: "connector-runtime", ConnectionID: connectionID, BindingID: bindingID,
			ConnectorID: "qqmail", OperationID: "qqmail.message.send",
			Target: connectorcore.TargetRef{Type: connectorcore.TargetTypeChat, ID: conversationID.String()},
			Effect: catalog.OperationEffectExternalSend, DataEgress: true,
			ExternalDestination: "qqmail", SensitiveDataAllowed: true,
			Risk: connectorexecution.RiskHigh,
		},
	}
	err = (&service{repos: repository.NewRepositories(db)}).RecordConnectorConfirmationResolution(
		context.Background(), connectorconfirmation.Command{
			OrganizationID: organizationID, OwnerAccountID: ownerID,
			RouteReceipt: connectorexecution.RouteClaimReceipt(receipt),
			Decision:     connectorexecution.ConfirmationDecisionConfirmed, RememberForSession: true,
		}, result)
	if err != nil {
		t.Fatalf("RecordConnectorConfirmationResolution() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func newConnectorConfirmationResolutionTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB, PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

func TestApplyConnectorConfirmationResolutionPersistsOnlySafeTerminal(t *testing.T) {
	conversationID := uuid.New()
	messageID := uuid.New()
	receipt := uuid.New()
	decisionID := uuid.New()
	secret := "connector-provider-value-sentinel"
	resolution, err := validateConnectorConfirmationResolution(
		connectorconfirmation.Command{
			OrganizationID: uuid.New(), OwnerAccountID: uuid.New(),
			RouteReceipt: connectorexecution.RouteClaimReceipt(receipt.String()),
			Decision:     connectorexecution.ConfirmationDecisionConfirmed,
		},
		connectorconfirmation.Result{
			State: connectorconfirmation.StateCompleted, DecisionID: decisionID,
			Origin: connectorexecution.ConfirmationOrigin{
				Surface: connectorcore.TargetTypeChat, ConversationID: conversationID.String(), MessageID: messageID.String(),
			},
			ExecutionResult: &connectorexecution.ExecutionResult{Value: map[string]interface{}{"secret": secret}},
		},
	)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	metadata := pendingConnectorResolutionMetadata(receipt.String())
	metadata["preserved"] = true
	updated, created, err := applyConnectorConfirmationResolution(metadata, resolution)
	if err != nil || !created || updated["preserved"] != true {
		t.Fatalf("apply: created=%v err=%v metadata=%#v", created, err, updated)
	}
	invocations := skillInvocationsFromMetadata(updated["skill_invocations"])
	if len(invocations) != 1 || invocations[0]["status"] != "completed" {
		t.Fatalf("invocations = %#v", invocations)
	}
	terminal := governanceMapFromAny(invocations[0]["result"])
	if len(terminal) != 2 || terminal["state"] != "completed" || terminal["decision_id"] != decisionID.String() {
		t.Fatalf("terminal = %#v", terminal)
	}
	events := connectorConfirmationEventsFromAny(updated[connectorConfirmationEventsMetadataKey])
	if len(events) != 1 || !resolution.matchesEvent(events[0]) {
		t.Fatalf("events = %#v", events)
	}
	encoded, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if bytes.Contains(encoded, []byte(secret)) {
		t.Fatalf("provider value leaked: %s", encoded)
	}
}

func TestApplyConnectorConfirmationResolutionDuplicateIsIdempotentAndConflictIsRejected(t *testing.T) {
	resolution := connectorConfirmationResolution{
		routeReceipt: uuid.NewString(), state: connectorconfirmation.StateCompleted, decisionID: uuid.NewString(),
		surface: connectorcore.TargetTypeAgent, conversationID: uuid.New(), messageID: uuid.New(), agentID: uuid.New(),
	}
	updated, created, err := applyConnectorConfirmationResolution(
		pendingConnectorResolutionMetadata(resolution.routeReceipt), resolution,
	)
	if err != nil || !created {
		t.Fatalf("first apply: created=%v err=%v", created, err)
	}
	duplicate, created, err := applyConnectorConfirmationResolution(updated, resolution)
	if err != nil || created {
		t.Fatalf("duplicate apply: created=%v err=%v", created, err)
	}
	if events := connectorConfirmationEventsFromAny(duplicate[connectorConfirmationEventsMetadataKey]); len(events) != 1 {
		t.Fatalf("duplicate events = %#v", events)
	}
	conflict := resolution
	conflict.state = connectorconfirmation.StateFailed
	conflict.decisionID = ""
	conflict.errorCode = string(connectorcore.ErrorCodeProviderUnavailable)
	if _, _, err := applyConnectorConfirmationResolution(updated, conflict); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		t.Fatalf("conflict error = %v", err)
	}
}

func TestConnectorSessionGrantFromResolutionKeepsConnectorIdentity(t *testing.T) {
	now := time.Now().UTC()
	organizationID, ownerID := uuid.New(), uuid.New()
	conversationID := uuid.New()
	connectionID, bindingID := uuid.New(), uuid.New()
	resolution := connectorConfirmationResolution{
		organizationID: organizationID, ownerAccountID: ownerID,
		routeReceipt: uuid.NewString(), conversationID: conversationID,
		authorization: &connectorconfirmation.AuthorizationSnapshot{
			SkillID: "connector-runtime", ConnectionID: connectionID, BindingID: bindingID,
			ConnectorID: "qqmail", OperationID: "qqmail.message.send",
			Target: connectorcore.TargetRef{Type: connectorcore.TargetTypeChat, ID: conversationID.String()},
			Effect: catalog.OperationEffectExternalSend, DataEgress: true,
			ExternalDestination: "mail", SensitiveDataAllowed: true,
			Risk: connectorexecution.RiskHigh,
		},
	}
	grant := connectorSessionGrantFromResolution(resolution, now)
	if grant["connection_id"] != connectionID.String() || grant["binding_id"] != bindingID.String() ||
		grant["tool_id"] != "qqmail.message.send" || grant["remember_for_session"] != nil {
		t.Fatalf("grant = %#v", grant)
	}
	if got := grant["expires_at"].(string); got == "" {
		t.Fatal("grant expiry is empty")
	}
}

func TestConnectorConfirmationResolutionValidatesAgentIdentityAndTerminal(t *testing.T) {
	organizationID := uuid.New()
	ownerID := uuid.New()
	conversationID := uuid.New()
	messageID := uuid.New()
	agentID := uuid.New()
	receipt := uuid.New()
	command := connectorconfirmation.Command{
		OrganizationID: organizationID, OwnerAccountID: ownerID,
		RouteReceipt: connectorexecution.RouteClaimReceipt(receipt.String()),
		Decision:     connectorexecution.ConfirmationDecisionConfirmed,
	}
	result := connectorconfirmation.Result{
		State: connectorconfirmation.StateFailed, ErrorCode: connectorcore.ErrorCodeProviderUnavailable,
		Origin: connectorexecution.ConfirmationOrigin{
			Surface: connectorcore.TargetTypeAgent, ConversationID: conversationID.String(),
			MessageID: messageID.String(), AgentID: agentID.String(),
		},
	}
	resolution, err := validateConnectorConfirmationResolution(command, result)
	if err != nil {
		t.Fatalf("valid Agent resolution: %v", err)
	}
	conversation := &runtimemodel.Conversation{
		ID: conversationID, OrganizationID: organizationID, AccountID: ownerID,
		CallerType: runtimemodel.ConversationCallerAgent, CallerID: &agentID,
		ConversationType: runtimemodel.ConversationTypeChat,
	}
	if !connectorResolutionConversationMatches(conversation, command, resolution) {
		t.Fatal("matching Agent conversation was rejected")
	}
	wrongAgent := uuid.New()
	conversation.CallerID = &wrongAgent
	if connectorResolutionConversationMatches(conversation, command, resolution) {
		t.Fatal("wrong Agent caller was accepted")
	}

	result.Origin.RunID = uuid.NewString()
	if _, err := validateConnectorConfirmationResolution(command, result); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeAccessDenied) {
		t.Fatalf("Agent run identity error = %v", err)
	}
	result.Origin.RunID = ""
	result.State = connectorconfirmation.StateUnknown
	result.ErrorCode = connectorcore.ErrorCodeProviderUnavailable
	if _, err := validateConnectorConfirmationResolution(command, result); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidInput) {
		t.Fatalf("unsafe unknown terminal error = %v", err)
	}
	command.RouteReceipt = connectorexecution.RouteClaimReceipt(strings.ToUpper(receipt.String()))
	if _, err := validateConnectorConfirmationResolution(command, result); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidInput) {
		t.Fatalf("non-canonical receipt error = %v", err)
	}
}

func pendingConnectorResolutionMetadata(receipt string) map[string]interface{} {
	return map[string]interface{}{
		"skill_invocations": []interface{}{map[string]interface{}{
			"kind": "tool_call",
			"result": map[string]interface{}{
				"state":        string(connectorexecution.ExecutionStateRequiresConfirmation),
				"confirmation": map[string]interface{}{"route_receipt": receipt},
			},
		}},
	}
}
