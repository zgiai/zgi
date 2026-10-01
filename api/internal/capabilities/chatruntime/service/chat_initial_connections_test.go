package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorpreferences "github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"gorm.io/gorm"
)

func TestInitialChatConnectionInputCannotBypassExistingBindings(t *testing.T) {
	valid := runtimedto.InitialConnectorBinding{ConnectorID: "github", ConnectionID: uuid.NewString()}
	for _, test := range []struct {
		name                 string
		caller               string
		conversation, parent string
		bindings             []runtimedto.InitialConnectorBinding
		valid                bool
	}{
		{"new chat", "aichat", "", "", []runtimedto.InitialConnectorBinding{valid}, true},
		{"ordinary existing chat", "aichat", uuid.NewString(), "", nil, true},
		{"existing chat override", "aichat", uuid.NewString(), "", []runtimedto.InitialConnectorBinding{valid}, false},
		{"reply", "aichat", "", uuid.NewString(), []runtimedto.InitialConnectorBinding{valid}, false},
		{"agent caller", "agent", "", "", []runtimedto.InitialConnectorBinding{valid}, false},
		{"duplicate connector", "aichat", "", "", []runtimedto.InitialConnectorBinding{valid, valid}, false},
		{"invalid UUID", "aichat", "", "", []runtimedto.InitialConnectorBinding{{ConnectorID: "github", ConnectionID: "draft-1"}}, false},
		{"noncanonical UUID", "aichat", "", "", []runtimedto.InitialConnectorBinding{{ConnectorID: "github", ConnectionID: strings.ReplaceAll(valid.ConnectionID, "-", "")}}, false},
		{"zero UUID", "aichat", "", "", []runtimedto.InitialConnectorBinding{{ConnectorID: "github", ConnectionID: uuid.Nil.String()}}, false},
		{"invalid connector", "aichat", "", "", []runtimedto.InitialConnectorBinding{{ConnectorID: "bad id", ConnectionID: valid.ConnectionID}}, false},
		{"bounded list", "aichat", "", "", make([]runtimedto.InitialConnectorBinding, 65), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := initialChatConnections(Caller{Type: test.caller}, runtimedto.ChatRequest{ConversationID: test.conversation, ParentID: test.parent, InitialConnectorBindings: test.bindings})
			if (err == nil) != test.valid {
				t.Fatalf("validation = %v, valid = %v", err, test.valid)
			}
			if err != nil && !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
				t.Fatalf("unexpected error code: %v", err)
			}
		})
	}
}

func TestInitialChatDefaultsInputChecksEvenAnEmptySelection(t *testing.T) {
	zero, positive, negative := int64(0), int64(7), int64(-1)
	for _, test := range []struct {
		name                 string
		caller               string
		conversation, parent string
		bindings             []runtimedto.InitialConnectorBinding
		revision             *int64
		valid                bool
	}{
		{"new empty selection with revision zero", "aichat", "", "", []runtimedto.InitialConnectorBinding{}, &zero, true},
		{"new omitted selection with revision", "aichat", "", "", nil, &positive, true},
		{"negative revision", "aichat", "", "", nil, &negative, false},
		{"existing chat empty selection", "aichat", uuid.NewString(), "", []runtimedto.InitialConnectorBinding{}, nil, false},
		{"existing chat revision zero", "aichat", uuid.NewString(), "", nil, &zero, false},
		{"reply revision zero", "aichat", "", uuid.NewString(), nil, &zero, false},
		{"agent revision zero", "agent", "", "", nil, &zero, false},
		{"agent empty selection", "agent", "", "", []runtimedto.InitialConnectorBinding{}, nil, false},
		{"legacy existing chat has no initialization", "aichat", uuid.NewString(), "", nil, nil, true},
		{"legacy agent has no initialization", "agent", "", "", nil, nil, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := runtimedto.ChatRequest{ConversationID: test.conversation, ParentID: test.parent,
				InitialConnectorBindings: test.bindings, InitialConnectorDefaultsRevision: test.revision}
			selections, err := initialChatConnections(Caller{Type: test.caller}, req)
			if (err == nil) != test.valid {
				t.Fatalf("validation = %v, valid = %v", err, test.valid)
			}
			if err != nil && !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
				t.Fatalf("unexpected error code: %v", err)
			}
			if len(selections) != 0 {
				t.Fatalf("defaults must never auto-fill explicit selections: %#v", selections)
			}
		})
	}
}

func TestInitialChatDefaultsWireFormatKeepsPresence(t *testing.T) {
	for _, test := range []struct {
		body    string
		initial bool
	}{
		{`{"query":"hello"}`, false},
		{`{"query":"hello","initial_connector_bindings":[]}`, true},
		{`{"query":"hello","initial_connector_defaults_revision":0}`, true},
	} {
		var request runtimedto.ChatRequest
		if err := json.Unmarshal([]byte(test.body), &request); err != nil {
			t.Fatal(err)
		}
		if hasInitialChatConnections(request) != test.initial {
			t.Fatalf("initialization presence lost for %s", test.body)
		}
	}
}

type initialChatDiscoveryProbe struct {
	chatRuntimeResolverStub
	discover func(connectorruntimeauth.ChatCapabilityRequest) error
	validate func(connectorruntimeauth.ChatCapabilityRequest, []connectorbindings.InitialChatConnection) error
}

func (probe *initialChatDiscoveryProbe) ValidateInitialChatConnections(_ context.Context, _ *gorm.DB, request connectorruntimeauth.ChatCapabilityRequest, selections []connectorbindings.InitialChatConnection) error {
	if probe.validate != nil {
		return probe.validate(request, selections)
	}
	return nil
}

func (probe *initialChatDiscoveryProbe) ResolveChatCapabilities(_ context.Context, request connectorruntimeauth.ChatCapabilityRequest) (connectorruntimeauth.ChatCapabilitySnapshot, error) {
	return connectorruntimeauth.ChatCapabilitySnapshot{}, probe.discover(request)
}

func TestInitialChatBindingsCommitBeforeFirstTurnDiscovery(t *testing.T) {
	for _, test := range []struct {
		name            string
		fail            bool
		checkPreference bool
		failCatalog     bool
	}{
		{"manual selection committed before discovery", false, false, false},
		{"manual selection failure rolls back chat", true, false, false},
		{"reviewed default committed before discovery", false, true, false},
		{"reviewed default binding failure rolls back chat", true, true, false},
		{"plugin disabled after reviewed snapshot rolls back", false, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock := newAgentBindingStepTestDB(t)
			workspace, chatID, connectionID := uuid.New(), uuid.New(), uuid.New()
			scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New(), WorkspaceID: &workspace, SkipAccessCheck: true}
			mock.ExpectBegin()
			var defaultsRevision *int64
			if test.checkPreference {
				revision := int64(3)
				defaultsRevision = &revision
				expectInitialChatDefaultsRevision(mock, scope, revision, nil)
			}
			mock.ExpectQuery(`INSERT INTO "chat_runtime_conversations"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(chatID))
			lock := `SELECT "id" FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4.*LIMIT \$5 FOR UPDATE`
			mock.ExpectQuery(lock).WithArgs(connectionID, scope.OrganizationID, scope.AccountID, "github", 1).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(connectionID))
			metadata := mock.ExpectQuery(`SELECT connection_constraint, usable_operation_ids FROM "connector_connections"`).
				WithArgs(connectionID, scope.OrganizationID, scope.AccountID, "github", string(core.ReadinessReady), 1)
			if test.fail {
				metadata.WillReturnRows(sqlmock.NewRows([]string{"connection_constraint", "usable_operation_ids"}))
				mock.ExpectRollback()
			} else {
				constraint, _ := json.Marshal(core.OwnerConstraint(scope.AccountID, []core.Identifier{"read"}))
				metadata.WillReturnRows(sqlmock.NewRows([]string{"connection_constraint", "usable_operation_ids"}).AddRow(string(constraint), `["read"]`))
				mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings"`).WithArgs(scope.OrganizationID, scope.AccountID, "chat", chatID.String(), "github", "live", "", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
				mock.ExpectQuery(lock).WithArgs(connectionID, scope.OrganizationID, scope.AccountID, "github", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(connectionID))
				mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings".*FOR UPDATE`).WithArgs(scope.OrganizationID, scope.AccountID, "chat", chatID.String(), "github", "live", "", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
				mock.ExpectQuery(`SELECT "id" FROM "connector_connections".*deleted_at IS NULL`).WithArgs(connectionID, scope.OrganizationID, scope.AccountID, "github", string(core.ReadinessReady), 1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(connectionID))
				mock.ExpectExec(`INSERT INTO "connector_target_bindings"`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).WillReturnResult(sqlmock.NewResult(0, 1))
				if test.failCatalog {
					mock.ExpectRollback()
				} else {
					mock.ExpectCommit()
				}
			}
			discovered := false
			stop := errors.New("stop after discovery boundary")
			probe := &initialChatDiscoveryProbe{discover: func(request connectorruntimeauth.ChatCapabilityRequest) error {
				discovered = true
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatalf("discovery ran before initialization commit: %v", err)
				}
				if request.ConversationID != chatID.String() || request.OrganizationID != scope.OrganizationID || request.OwnerAccountID != scope.AccountID {
					t.Fatalf("wrong discovery target: %#v", request)
				}
				return stop
			}, validate: func(request connectorruntimeauth.ChatCapabilityRequest, selections []connectorbindings.InitialChatConnection) error {
				if request.ConversationID != chatID.String() || len(selections) != 1 || selections[0].ConnectionID != connectionID {
					t.Fatalf("wrong initialization availability check: %#v, %#v", request, selections)
				}
				if test.failCatalog {
					return core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
				}
				return nil
			}}
			svc := &service{repos: repository.NewRepositories(db), connectorChatRuntime: probe,
				skillRuntime: skills.NewRuntime(nil, nil), modelSpecResolver: toolGovernanceStreamModelSpecResolver{}}
			prepared, err := svc.PrepareChat(context.Background(), scope, runtimedto.ChatRequest{
				Query: "search repositories", Model: "test", Provider: "test", Surface: runtimedto.RuntimeSurfaceWorkChat,
				InitialConnectorBindings:         []runtimedto.InitialConnectorBinding{{ConnectorID: "github", ConnectionID: connectionID.String()}},
				InitialConnectorDefaultsRevision: defaultsRevision,
			})
			if prepared != nil || err == nil {
				t.Fatalf("unexpected prepared turn: %#v, %v", prepared, err)
			}
			if test.fail || test.failCatalog {
				if discovered || !core.IsErrorCode(err, core.ErrorCodeConnectionNotReady) {
					t.Fatalf("failure did not stop before discovery: %v", err)
				}
			} else if !discovered || !errors.Is(err, stop) {
				t.Fatalf("discovery not reached: %v; expectations: %v", err, mock.ExpectationsWereMet())
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func expectInitialChatDefaultsRevision(mock sqlmock.Sqlmock, scope Scope, revision int64, readErr error) {
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	query := mock.ExpectQuery(`SELECT \* FROM "connector_chat_preferences" WHERE organization_id = \$1 AND owner_account_id = \$2 LIMIT \$3`).
		WithArgs(scope.OrganizationID, scope.AccountID, 1)
	if readErr != nil {
		query.WillReturnError(readErr)
		return
	}
	rows := sqlmock.NewRows([]string{"organization_id", "owner_account_id", "revision", "defaults"})
	if revision > 0 {
		// An explicit empty selection must not be replaced by this stored
		// default. Successful tests therefore expect no connection reads.
		defaults := `{"github":{"mode":"selected","connection_id":"` + uuid.NewString() + `"}}`
		rows.AddRow(scope.OrganizationID, scope.AccountID, revision, defaults)
	}
	query.WillReturnRows(rows)
}

func TestInitialChatDefaultsRevisionGuardsEmptySelectionsBeforeDiscovery(t *testing.T) {
	for _, test := range []struct {
		name     string
		expected int64
		stored   int64
		readErr  error
		omitList bool
	}{
		{"revision zero with no stored preferences", 0, 0, nil, false},
		{"empty selection does not refill a stored default", 3, 3, nil, false},
		{"omitted selection does not refill a stored default", 3, 3, nil, true},
		{"empty selection with stale revision rolls back", 2, 3, nil, false},
		{"revision zero cannot ignore another tab's first default", 0, 1, nil, false},
		{"revision without a list still conflicts", 2, 3, nil, true},
		{"unreadable snapshot fails closed", 3, 3, errors.New("private database diagnostic"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock := newAgentBindingStepTestDB(t)
			workspace, chatID := uuid.New(), uuid.New()
			scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New(), WorkspaceID: &workspace, SkipAccessCheck: true}
			conflict := test.expected != test.stored
			mock.ExpectBegin()
			expectInitialChatDefaultsRevision(mock, scope, test.stored, test.readErr)
			if conflict || test.readErr != nil {
				// No conversation or binding writes may happen after a failed
				// snapshot check, and no model/discovery work may follow rollback.
				mock.ExpectRollback()
			} else {
				mock.ExpectQuery(`INSERT INTO "chat_runtime_conversations"`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(chatID))
				mock.ExpectCommit()
			}
			discovered := false
			stop := errors.New("stop after reviewed snapshot commit")
			probe := &initialChatDiscoveryProbe{discover: func(request connectorruntimeauth.ChatCapabilityRequest) error {
				discovered = true
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatalf("discovery ran before the checked transaction committed: %v", err)
				}
				if request.ConversationID != chatID.String() {
					t.Fatalf("discovery received a nonpersistent target: %#v", request)
				}
				return stop
			}}
			svc := &service{repos: repository.NewRepositories(db), connectorChatRuntime: probe,
				skillRuntime: skills.NewRuntime(nil, nil), modelSpecResolver: toolGovernanceStreamModelSpecResolver{}}
			bindings := []runtimedto.InitialConnectorBinding{}
			if test.omitList {
				bindings = nil
			}
			prepared, err := svc.PrepareChat(context.Background(), scope, runtimedto.ChatRequest{
				Query: "hello", Model: "test", Provider: "test", Surface: runtimedto.RuntimeSurfaceWorkChat,
				InitialConnectorBindings: bindings, InitialConnectorDefaultsRevision: &test.expected,
			})
			if prepared != nil || err == nil {
				t.Fatalf("unexpected prepared turn: %#v, %v", prepared, err)
			}
			switch {
			case test.readErr != nil:
				if discovered || !apperror.IsCode(err, connectorpreferences.AppCodeInternal) || !errors.Is(err, test.readErr) {
					t.Fatalf("snapshot storage failure did not stop initialization: %v", err)
				}
			case conflict:
				if discovered || !apperror.IsCode(err, connectorpreferences.AppCodeRevisionConflict) {
					t.Fatalf("snapshot conflict did not stop initialization: %v", err)
				}
			default:
				if !discovered || !errors.Is(err, stop) {
					t.Fatalf("checked empty selection not committed before discovery: %v", err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInitialChatRejectsExistingRequestBeforeAnyDependencies(t *testing.T) {
	zero := int64(0)
	for _, request := range []runtimedto.ChatRequest{
		{ConversationID: uuid.NewString(), InitialConnectorBindings: []runtimedto.InitialConnectorBinding{{ConnectorID: "github", ConnectionID: uuid.NewString()}}},
		{ConversationID: uuid.NewString(), InitialConnectorBindings: []runtimedto.InitialConnectorBinding{}},
		{ConversationID: uuid.NewString(), InitialConnectorDefaultsRevision: &zero},
		{ParentID: uuid.NewString(), InitialConnectorDefaultsRevision: &zero},
	} {
		svc := &service{}
		_, err := svc.PrepareConfiguredChat(context.Background(), Scope{OrganizationID: uuid.New(), AccountID: uuid.New(), SkipAccessCheck: true}, Caller{Type: runtimemodel.ConversationCallerAIChat}, RunConfig{}, request)
		if !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestInitialChatDiscoveryCannotSilentlyDropReviewedConnections(t *testing.T) {
	for _, mode := range []string{"ready", "plugin disabled", "zero operations", "second connection missing", "different account"} {
		t.Run(mode, func(t *testing.T) {
			org, owner, conversationID := uuid.New(), uuid.New(), uuid.New()
			snapshot := connectorChatTestSnapshot(org, owner, conversationID)
			selected := snapshot.Capabilities[0]
			required := []connectorbindings.InitialChatConnection{{ConnectorID: selected.ConnectorID, ConnectionID: selected.ConnectionID}}
			switch mode {
			case "plugin disabled":
				snapshot.Capabilities = nil
			case "zero operations":
				snapshot.Capabilities[0].Operations = nil
			case "second connection missing":
				required = append(required, connectorbindings.InitialChatConnection{ConnectorID: "web-search", ConnectionID: uuid.New()})
			case "different account":
				snapshot.Capabilities[0].ConnectionID = uuid.New()
			}
			svc := &service{connectorChatRuntime: &chatRuntimeResolverStub{snapshot: snapshot}}
			parts := &chatRequestParts{ConversationID: conversationID.String(), RequiredInitialConnectorBindings: required}
			err := svc.applyConnectorSkillConfig(context.Background(), Scope{OrganizationID: org, AccountID: owner}, Caller{Type: runtimemodel.ConversationCallerAIChat}, parts)
			if mode == "ready" {
				if err != nil || !parts.ConnectorToolsEnabled {
					t.Fatalf("valid initial selection rejected: %v", err)
				}
			} else if !core.IsErrorCode(err, core.ErrorCodeConnectionNotReady) || parts.ConnectorToolsEnabled {
				t.Fatalf("unavailable initial selection silently fell back to model-only chat: %v", err)
			}
			// The final business-Skill projection just before model dispatch has
			// the same guard for a lifecycle change after preparation.
			prepared := &PreparedChat{Conversation: &runtimemodel.Conversation{ID: conversationID},
				Scope: Scope{OrganizationID: org, AccountID: owner}, Caller: Caller{Type: runtimemodel.ConversationCallerAIChat},
				parts: &chatRequestParts{ConversationID: conversationID.String(), RequiredInitialConnectorBindings: required}}
			_, err = svc.resolvePreparedBusinessSkills(context.Background(), prepared)
			if mode != "ready" && !core.IsErrorCode(err, core.ErrorCodeConnectionNotReady) {
				t.Fatalf("pre-model projection silently dropped a reviewed connection: %v", err)
			}
			if mode == "ready" && err != nil {
				t.Fatal(err)
			}
		})
	}
}
