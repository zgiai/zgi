package execution

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestGormConfirmationContinuationStoreFreezeIsImmutableAndDoesNotPersistInput(t *testing.T) {
	fixture := newConfirmationContinuationTestFixture(t)
	const secretSentinel = "SUPER-SECRET-CONNECTOR-KEY-92746"
	fixture.frozen.Request.Input = map[string]any{
		"title": "Create release", "api_key": secretSentinel,
	}
	fixture.claim.InputDigest, _ = invocationInputDigest(fixture.frozen.Request.Input)

	expectContinuationFreeze(fixture, 1, "v1.sealed-envelope.1")
	first, err := fixture.store.Freeze(context.Background(), fixture.frozen)
	if err != nil {
		t.Fatalf("Freeze() error = %v", err)
	}
	if first.State != ConfirmationContinuationPending || first.Frozen.RouteReceipt != fixture.frozen.RouteReceipt ||
		first.Frozen.Origin != fixture.frozen.Origin {
		t.Fatalf("Freeze() = %#v", first)
	}

	expectContinuationFreeze(fixture, 0, "v1.sealed-envelope.2")
	expectContinuationLoad(fixture, fixture.pendingRecord("v1.sealed-envelope.1"))
	second, err := fixture.store.Freeze(context.Background(), fixture.frozen)
	if err != nil {
		t.Fatalf("idempotent Freeze() error = %v", err)
	}
	if second.State != first.State || !second.CreatedAt.Equal(first.CreatedAt) || !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("idempotent Freeze() = %#v, want existing %#v", second, first)
	}

	changed := fixture.frozen
	changed.Request.Input = map[string]any{
		"title": "Delete release", "api_key": secretSentinel,
	}
	fixture.mock.ExpectQuery(`SELECT .* FROM "connector_invocation_claims"`).WillReturnRows(nativeInvocationRows(fixture.claim))
	if _, err = fixture.store.Freeze(context.Background(), changed); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("changed-input Freeze() error = %v, want access denied", err)
	}

	for name, value := range map[string]any{"frozen request": fixture.frozen, "continuation": first} {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			t.Fatalf("json.Marshal(%s) error = %v", name, marshalErr)
		}
		if strings.Contains(string(encoded), secretSentinel) || strings.Contains(string(encoded), "api_key") {
			t.Fatalf("public %s JSON leaked frozen Input: %s", name, encoded)
		}
	}
	if got := fixture.protector.lastSealedInputValue("api_key"); got != secretSentinel {
		t.Fatalf("protector received Input api_key = %q, want sentinel", got)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormConfirmationContinuationStoreLoadIsOwnerScoped(t *testing.T) {
	fixture := newConfirmationContinuationTestFixture(t)
	expectContinuationFreeze(fixture, 1, "v1.sealed-envelope.1")
	if _, err := fixture.store.Freeze(context.Background(), fixture.frozen); err != nil {
		t.Fatal(err)
	}
	record := fixture.pendingRecord("v1.sealed-envelope.1")
	expectContinuationLoad(fixture, record)
	if _, err := fixture.store.Load(
		context.Background(), fixture.frozen.Request.OrganizationID,
		fixture.frozen.Request.OwnerAccountID, fixture.frozen.RouteReceipt,
	); err != nil {
		t.Fatalf("owner-scoped Load() error = %v", err)
	}
	for _, testCase := range []struct {
		name           string
		organizationID uuid.UUID
		ownerID        uuid.UUID
	}{
		{name: "other organization", organizationID: uuid.New(), ownerID: fixture.frozen.Request.OwnerAccountID},
		{name: "other owner", organizationID: fixture.frozen.Request.OrganizationID, ownerID: uuid.New()},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			expectContinuationLoadNotFound(fixture, testCase.organizationID, testCase.ownerID)
			_, err := fixture.store.Load(
				context.Background(), testCase.organizationID, testCase.ownerID, fixture.frozen.RouteReceipt,
			)
			if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
				t.Fatalf("Load() error = %v, want access denied", err)
			}
		})
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormConfirmationContinuationStoreRecordTerminalIsIdempotentOnlyForExactResult(t *testing.T) {
	fixture := newConfirmationContinuationTestFixture(t)
	expectContinuationFreeze(fixture, 1, "v1.sealed-envelope.1")
	if _, err := fixture.store.Freeze(context.Background(), fixture.frozen); err != nil {
		t.Fatal(err)
	}
	terminal := ConfirmationTerminal{
		State: ConfirmationContinuationCompleted, DecisionID: uuid.New(), OutcomeID: uuid.New(),
	}
	terminalRecord := fixture.terminalRecord("v1.sealed-envelope.1", terminal)
	expectContinuationTerminalUpdate(fixture, terminal, 1)
	expectContinuationLoad(fixture, terminalRecord)
	first, err := fixture.store.RecordTerminal(
		context.Background(), fixture.frozen.Request.OrganizationID, fixture.frozen.Request.OwnerAccountID,
		fixture.frozen.RouteReceipt, terminal,
	)
	if err != nil {
		t.Fatalf("RecordTerminal() error = %v", err)
	}
	if !continuationMatchesTerminal(first, terminal) || first.TerminalAt.IsZero() {
		t.Fatalf("RecordTerminal() = %#v", first)
	}

	expectContinuationTerminalUpdate(fixture, terminal, 0)
	expectContinuationLoad(fixture, terminalRecord)
	duplicate, err := fixture.store.RecordTerminal(
		context.Background(), fixture.frozen.Request.OrganizationID, fixture.frozen.Request.OwnerAccountID,
		fixture.frozen.RouteReceipt, terminal,
	)
	if err != nil {
		t.Fatalf("duplicate RecordTerminal() error = %v", err)
	}
	if !continuationMatchesTerminal(duplicate, terminal) || !duplicate.TerminalAt.Equal(first.TerminalAt) {
		t.Fatalf("duplicate RecordTerminal() = %#v, want existing %#v", duplicate, first)
	}

	divergent := ConfirmationTerminal{State: ConfirmationContinuationCancelled, DecisionID: uuid.New()}
	expectContinuationTerminalUpdate(fixture, divergent, 0)
	expectContinuationLoad(fixture, terminalRecord)
	if _, err = fixture.store.RecordTerminal(
		context.Background(), fixture.frozen.Request.OrganizationID, fixture.frozen.Request.OwnerAccountID,
		fixture.frozen.RouteReceipt, divergent,
	); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("divergent RecordTerminal() error = %v, want conflict", err)
	}
	expectContinuationLoad(fixture, terminalRecord)
	current, err := fixture.store.Load(
		context.Background(), fixture.frozen.Request.OrganizationID,
		fixture.frozen.Request.OwnerAccountID, fixture.frozen.RouteReceipt,
	)
	if err != nil || !continuationMatchesTerminal(current, terminal) {
		t.Fatalf("Load() after divergent replay = %#v, %v", current, err)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormConfirmationContinuationStoreRecordTerminalCASSerializesConcurrentDecisions(t *testing.T) {
	fixture := newConfirmationContinuationTestFixture(t)
	expectContinuationFreeze(fixture, 1, "v1.sealed-envelope.1")
	if _, err := fixture.store.Freeze(context.Background(), fixture.frozen); err != nil {
		t.Fatal(err)
	}
	terminals := []ConfirmationTerminal{
		{State: ConfirmationContinuationCompleted, DecisionID: uuid.New(), OutcomeID: uuid.New()},
		{State: ConfirmationContinuationCancelled, DecisionID: uuid.New()},
	}
	winnerRecord := fixture.terminalRecord("v1.sealed-envelope.1", terminals[0])
	fixture.mock.MatchExpectationsInOrder(false)
	expectContinuationTerminalUpdate(fixture, terminals[0], 1)
	expectContinuationTerminalUpdate(fixture, terminals[1], 0)
	expectContinuationLoad(fixture, winnerRecord)
	expectContinuationLoad(fixture, winnerRecord)

	type result struct {
		index        int
		continuation ConfirmationContinuation
		err          error
	}
	results := make(chan result, len(terminals))
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range terminals {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			continuation, err := fixture.store.RecordTerminal(
				context.Background(), fixture.frozen.Request.OrganizationID, fixture.frozen.Request.OwnerAccountID,
				fixture.frozen.RouteReceipt, terminals[index],
			)
			results <- result{index: index, continuation: continuation, err: err}
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)

	winner, conflicts := -1, 0
	for call := range results {
		switch {
		case call.err == nil:
			winner = call.index
			if !continuationMatchesTerminal(call.continuation, terminals[call.index]) {
				t.Fatalf("winner continuation = %#v", call.continuation)
			}
		case core.IsErrorCode(call.err, core.ErrorCodeConflict):
			conflicts++
		default:
			t.Fatalf("concurrent RecordTerminal() error = %v", call.err)
		}
	}
	if winner != 0 || conflicts != 1 {
		t.Fatalf("winner=%d conflicts=%d, want completed winner and one conflict", winner, conflicts)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormConfirmationContinuationStoreRejectsOwnerMismatchOnTerminalCAS(t *testing.T) {
	fixture := newConfirmationContinuationTestFixture(t)
	expectContinuationFreeze(fixture, 1, "v1.sealed-envelope.1")
	if _, err := fixture.store.Freeze(context.Background(), fixture.frozen); err != nil {
		t.Fatal(err)
	}
	otherOwner := uuid.New()
	terminal := ConfirmationTerminal{State: ConfirmationContinuationCancelled, DecisionID: uuid.New()}
	expectContinuationTerminalUpdateForOwner(fixture, otherOwner, terminal, 0)
	expectContinuationLoadNotFound(fixture, fixture.frozen.Request.OrganizationID, otherOwner)
	_, err := fixture.store.RecordTerminal(
		context.Background(), fixture.frozen.Request.OrganizationID, otherOwner,
		fixture.frozen.RouteReceipt, terminal,
	)
	if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
		t.Fatalf("cross-owner RecordTerminal() error = %v, want access denied", err)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormConfirmationContinuationStoreFreezesDigitalHumanOrigin(t *testing.T) {
	fixture := newDigitalHumanConfirmationContinuationTestFixture(t)
	target := fixture.frozen.Request.Target

	expectContinuationFreeze(fixture, 1, "v1.sealed-envelope.1")
	first, err := fixture.store.Freeze(context.Background(), fixture.frozen)
	if err != nil {
		t.Fatalf("Freeze() error = %v", err)
	}
	if first.State != ConfirmationContinuationPending || first.Frozen.Origin != fixture.frozen.Origin {
		t.Fatalf("Freeze() = %#v", first)
	}

	record := fixture.pendingRecord("v1.sealed-envelope.1")
	if record.OriginVersion != confirmationOriginVersionDigitalHuman ||
		record.DigitalHumanID == nil || *record.DigitalHumanID != target.ID {
		t.Fatalf("persisted digital human origin = %#v", record)
	}
	expectContinuationLoad(fixture, record)
	loaded, err := fixture.store.Load(
		context.Background(), fixture.frozen.Request.OrganizationID,
		fixture.frozen.Request.OwnerAccountID, fixture.frozen.RouteReceipt,
	)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Frozen.Origin != fixture.frozen.Origin || loaded.Frozen.Request.Target != target {
		t.Fatalf("Load() = %#v", loaded)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormConfirmationContinuationStoreBindsOmittedDigitalHumanOrigin(t *testing.T) {
	fixture := newDigitalHumanConfirmationContinuationTestFixture(t)
	target := fixture.frozen.Request.Target
	request := fixture.frozen
	// The shared runtime builds the host surface references but leaves the
	// digital human id and origin version to the store, which owns the target.
	request.Origin.OriginVersion = 0
	request.Origin.DigitalHumanID = ""

	expectContinuationFreezeForOrigin(fixture, fixture.frozen.Origin, 1, "v1.sealed-envelope.1")
	continuation, err := fixture.store.Freeze(context.Background(), request)
	if err != nil {
		t.Fatalf("Freeze() error = %v", err)
	}
	if continuation.Frozen.Origin.OriginVersion != confirmationOriginVersionDigitalHuman ||
		continuation.Frozen.Origin.DigitalHumanID != target.ID {
		t.Fatalf("bound origin = %#v, want target %s", continuation.Frozen.Origin, target.ID)
	}
	if err := fixture.mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormConfirmationContinuationStoreRejectsDriftedDigitalHumanClaim(t *testing.T) {
	for name, mutate := range map[string]func(*invocationRecord){
		"other digital human": func(claim *invocationRecord) { claim.TargetID = uuid.NewString() },
		"other version":       func(claim *invocationRecord) { claim.TargetVersionID = uuid.NewString() },
		"other instance":      func(claim *invocationRecord) { claim.BindingInstanceID = uuid.NewString() },
		"other surface":       func(claim *invocationRecord) { claim.TargetType = string(core.TargetTypeAgent) },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDigitalHumanConfirmationContinuationTestFixture(t)
			mutate(&fixture.claim)
			fixture.mock.ExpectQuery(`SELECT .* FROM "connector_invocation_claims"`).WillReturnRows(nativeInvocationRows(fixture.claim))
			if _, err := fixture.store.Freeze(context.Background(), fixture.frozen); !core.IsErrorCode(err, core.ErrorCodeAccessDenied) {
				t.Fatalf("drifted digital human claim accepted: %v", err)
			}
			if err := fixture.mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateConfirmationOriginDigitalHumanScope(t *testing.T) {
	target := core.TargetRef{
		Type: core.TargetTypeDigitalHuman, ID: uuid.NewString(),
		Scope: core.ScopePublished, VersionID: uuid.NewString(), BindingInstanceID: uuid.NewString(),
	}
	valid := ConfirmationOrigin{
		Surface: core.TargetTypeDigitalHuman, OriginVersion: confirmationOriginVersionDigitalHuman,
		DigitalHumanID: target.ID, ConversationID: uuid.NewString(),
		MessageID: uuid.NewString(), RunID: uuid.NewString(),
	}
	if err := validateConfirmationOrigin(valid, target); err != nil {
		t.Fatalf("valid digital human origin rejected: %v", err)
	}
	for name, mutate := range map[string]func(*ConfirmationOrigin){
		"other digital human":   func(origin *ConfirmationOrigin) { origin.DigitalHumanID = uuid.NewString() },
		"missing digital human": func(origin *ConfirmationOrigin) { origin.DigitalHumanID = "" },
		"wrong origin version":  func(origin *ConfirmationOrigin) { origin.OriginVersion = 1 },
		"non canonical run":     func(origin *ConfirmationOrigin) { origin.RunID = "not-a-uuid" },
		"agent field":           func(origin *ConfirmationOrigin) { origin.AgentID = target.ID },
		"workflow field":        func(origin *ConfirmationOrigin) { origin.WorkflowID = target.ID },
		"node field":            func(origin *ConfirmationOrigin) { origin.NodeID = uuid.NewString() },
		"missing conversation":  func(origin *ConfirmationOrigin) { origin.ConversationID = "" },
		"missing message":       func(origin *ConfirmationOrigin) { origin.MessageID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			origin := valid
			mutate(&origin)
			if err := validateConfirmationOrigin(origin, target); err == nil {
				t.Fatalf("origin %#v accepted for %#v, want invalid input", origin, target)
			}
		})
	}
	if err := validateConfirmationOrigin(valid, core.TargetRef{Type: core.TargetTypeChat, ID: valid.ConversationID}); err == nil {
		t.Fatal("digital human origin accepted for a chat target")
	}
	if err := validateConfirmationOrigin(ConfirmationOrigin{
		Surface: core.TargetTypeChat, OriginVersion: confirmationOriginVersionDigitalHuman,
		DigitalHumanID: target.ID, ConversationID: "conversation-1", MessageID: "message-1",
	}, core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"}); err == nil {
		t.Fatal("chat origin carrying digital human fields was accepted")
	}
}

func TestBindConfirmationOriginOnlyTouchesDigitalHumanSurface(t *testing.T) {
	target := core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: uuid.NewString()}
	frozen := FrozenConfirmationRequest{
		Request: ExecutionRequest{Target: target},
		Origin: ConfirmationOrigin{
			Surface: core.TargetTypeDigitalHuman, ConversationID: uuid.NewString(),
			MessageID: uuid.NewString(), RunID: uuid.NewString(),
		},
	}
	bound := bindConfirmationOrigin(frozen)
	if bound.Origin.OriginVersion != confirmationOriginVersionDigitalHuman || bound.Origin.DigitalHumanID != target.ID {
		t.Fatalf("bound origin = %#v", bound.Origin)
	}

	// A caller-supplied disagreement is preserved so validation rejects it
	// instead of silently rebinding the receipt to a different digital human.
	frozen.Origin.OriginVersion = 1
	frozen.Origin.DigitalHumanID = uuid.NewString()
	mismatched := bindConfirmationOrigin(frozen)
	if mismatched.Origin != frozen.Origin {
		t.Fatalf("mismatched origin was rewritten: %#v", mismatched.Origin)
	}

	chat := FrozenConfirmationRequest{
		Request: ExecutionRequest{Target: core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"}},
		Origin:  ConfirmationOrigin{Surface: core.TargetTypeChat, ConversationID: "conversation-1", MessageID: "message-1"},
	}
	if got := bindConfirmationOrigin(chat); got.Origin != chat.Origin {
		t.Fatalf("chat origin changed: %#v", got.Origin)
	}
}

type confirmationContinuationTestFixture struct {
	store     *GormConfirmationContinuationStore
	mock      sqlmock.Sqlmock
	protector *memoryContinuationPayloadProtector
	frozen    FrozenConfirmationRequest
	claim     invocationRecord
	now       time.Time
}

func newConfirmationContinuationTestFixture(t *testing.T) *confirmationContinuationTestFixture {
	t.Helper()
	request := validExecutionRequest()
	return newConfirmationContinuationTestFixtureFor(t, request, ConfirmationOrigin{
		Surface: core.TargetTypeChat, ConversationID: request.Target.ID, MessageID: "message-1",
	})
}

func newDigitalHumanConfirmationContinuationTestFixture(t *testing.T) *confirmationContinuationTestFixture {
	t.Helper()
	request := validExecutionRequest()
	request.Target = core.TargetRef{
		Type: core.TargetTypeDigitalHuman, ID: uuid.NewString(),
		Scope: core.ScopePublished, VersionID: uuid.NewString(), BindingInstanceID: uuid.NewString(),
	}
	return newConfirmationContinuationTestFixtureFor(t, request, ConfirmationOrigin{
		Surface: core.TargetTypeDigitalHuman, OriginVersion: confirmationOriginVersionDigitalHuman,
		DigitalHumanID: request.Target.ID, ConversationID: uuid.NewString(),
		MessageID: uuid.NewString(), RunID: uuid.NewString(),
	})
}

func newConfirmationContinuationTestFixtureFor(
	t *testing.T,
	request ExecutionRequest,
	origin ConfirmationOrigin,
) *confirmationContinuationTestFixture {
	t.Helper()
	db, mock := openExecutionPersistenceMock(t)
	claim := routeResolutionRequest(request)
	receiptID := uuid.New()
	definition := validDefinition()
	definition.Operations[0].ID = claim.OperationID.String()
	definition.Operations[0].ToolName = claim.OperationID.String()
	acceptedOperation := mustAcceptedOperation(t, definition, definition.Operations[0])
	frozen := FrozenConfirmationRequest{
		RouteReceipt: RouteClaimReceipt(receiptID.String()),
		Request: ExecutionRequest{
			OrganizationID: claim.OrganizationID, OwnerAccountID: claim.OwnerAccountID,
			ActingPrincipalID: claim.ActingPrincipalID, BindingID: claim.BindingID,
			ConnectionID: claim.ConnectionID, ConnectorID: claim.ConnectorID,
			Target: claim.Target, OperationID: claim.OperationID,
			LogicalOperationID: claim.LogicalOperationID, CorrelationID: claim.CorrelationID,
			Interactive: true, Input: map[string]any{"query": "zgi"},
		},
		Origin:            origin,
		AcceptedOperation: acceptedOperation,
	}
	digest, _ := invocationInputDigest(frozen.Request.Input)
	claimRecord := invocationRow(InvocationClaim{Identity: routeResolutionRequest(frozen.Request), Snapshot: InvocationSnapshot{InputDigest: digest, ContractDigest: acceptedOperation.ContractDigest, BindingRevision: 1, CredentialVersion: 1}})
	claimRecord.ReceiptID = receiptID
	claimRecord.State = "claimed"
	claimRecord.CreatedAt = testExecutionTime()
	protector := &memoryContinuationPayloadProtector{payloads: map[string]protectedTestPayload{}}
	store, err := NewGormConfirmationContinuationStore(db, protector)
	if err != nil {
		t.Fatal(err)
	}
	now := testExecutionTime().Add(time.Minute)
	store.now = func() time.Time { return now }
	return &confirmationContinuationTestFixture{
		store: store, mock: mock, protector: protector, frozen: frozen, claim: claimRecord, now: now,
	}
}

func (fixture *confirmationContinuationTestFixture) pendingRecord(envelope string) confirmationContinuationRecord {
	return confirmationContinuationRecordFromFrozen(
		fixture.frozen, ContinuationPayloadEnvelope(envelope), fixture.now,
	)
}

func (fixture *confirmationContinuationTestFixture) terminalRecord(
	envelope string,
	terminal ConfirmationTerminal,
) confirmationContinuationRecord {
	record := fixture.pendingRecord(envelope)
	record.State = string(terminal.State)
	record.DecisionID = optionalContinuationUUID(terminal.DecisionID)
	record.OutcomeID = optionalContinuationUUID(terminal.OutcomeID)
	record.DiagnosticCode = optionalContinuationString(terminal.DiagnosticCode)
	record.TerminalAt = &fixture.now
	return record
}

func expectContinuationFreeze(
	fixture *confirmationContinuationTestFixture,
	rowsAffected int64,
	envelope string,
) {
	expectContinuationFreezeForOrigin(fixture, fixture.frozen.Origin, rowsAffected, envelope)
}

func expectContinuationFreezeForOrigin(
	fixture *confirmationContinuationTestFixture,
	origin ConfirmationOrigin,
	rowsAffected int64,
	envelope string,
) {
	fixture.mock.ExpectQuery(`(?s)SELECT .* FROM "connector_invocation_claims" WHERE receipt_id = \$1 AND organization_id = \$2 AND owner_account_id = \$3.*LIMIT \$4`).
		WithArgs(fixture.claim.ReceiptID, fixture.claim.OrganizationID, fixture.claim.OwnerAccountID, 1).
		WillReturnRows(nativeInvocationRows(fixture.claim))
	fixture.mock.ExpectExec(`(?s)INSERT INTO "connector_confirmation_continuations" .*ON CONFLICT \("route_receipt"\) DO NOTHING`).
		WithArgs(
			fixture.claim.ReceiptID, fixture.claim.OrganizationID, fixture.claim.OwnerAccountID,
			origin.OriginVersion, nullableContinuationStringDriverValue(origin.DigitalHumanID),
			string(origin.Surface), nullableContinuationStringDriverValue(origin.ConversationID),
			nullableContinuationStringDriverValue(origin.MessageID), nullableContinuationStringDriverValue(origin.AgentID),
			nullableContinuationStringDriverValue(origin.WorkflowID), nullableContinuationStringDriverValue(origin.RunID),
			nullableContinuationStringDriverValue(origin.NodeID),
			envelope, string(ConfirmationContinuationPending),
			nil, nil, nil, fixture.now, fixture.now, nil,
		).
		WillReturnResult(sqlmock.NewResult(0, rowsAffected))
}

func expectContinuationLoad(fixture *confirmationContinuationTestFixture, record confirmationContinuationRecord) {
	fixture.mock.ExpectQuery(`(?s)SELECT .* FROM "connector_confirmation_continuations" WHERE route_receipt = \$1 AND organization_id = \$2 AND owner_account_id = \$3.*LIMIT \$4`).
		WithArgs(record.RouteReceipt, record.OrganizationID, record.OwnerAccountID, 1).
		WillReturnRows(confirmationContinuationRows(record))
}

func expectContinuationLoadNotFound(
	fixture *confirmationContinuationTestFixture,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
) {
	fixture.mock.ExpectQuery(`(?s)SELECT .* FROM "connector_confirmation_continuations" WHERE route_receipt = \$1 AND organization_id = \$2 AND owner_account_id = \$3.*LIMIT \$4`).
		WithArgs(fixture.claim.ReceiptID, organizationID, ownerAccountID, 1).
		WillReturnRows(sqlmock.NewRows(confirmationContinuationColumns()))
}

func expectContinuationTerminalUpdate(
	fixture *confirmationContinuationTestFixture,
	terminal ConfirmationTerminal,
	rowsAffected int64,
) {
	expectContinuationTerminalUpdateForOwner(
		fixture, fixture.frozen.Request.OwnerAccountID, terminal, rowsAffected,
	)
}

func expectContinuationTerminalUpdateForOwner(
	fixture *confirmationContinuationTestFixture,
	ownerAccountID uuid.UUID,
	terminal ConfirmationTerminal,
	rowsAffected int64,
) {
	fixture.mock.ExpectExec(`(?s)UPDATE "connector_confirmation_continuations" SET .* WHERE route_receipt = \$7 AND organization_id = \$8 AND owner_account_id = \$9 AND state = \$10`).
		WithArgs(
			continuationUUIDDriverValue(terminal.DecisionID), nullableContinuationStringDriverValue(terminal.DiagnosticCode),
			continuationUUIDDriverValue(terminal.OutcomeID), string(terminal.State), fixture.now, fixture.now,
			fixture.claim.ReceiptID, fixture.claim.OrganizationID, ownerAccountID, string(ConfirmationContinuationPending),
		).
		WillReturnResult(sqlmock.NewResult(0, rowsAffected))
}

func confirmationContinuationRows(record confirmationContinuationRecord) *sqlmock.Rows {
	return sqlmock.NewRows(confirmationContinuationColumns()).AddRow(
		record.RouteReceipt, record.OrganizationID, record.OwnerAccountID,
		record.OriginVersion, record.DigitalHumanID, record.OriginSurface,
		record.ConversationID, record.MessageID, record.AgentID, record.WorkflowID, record.RunID, record.NodeID,
		record.ProtectedPayloadEnvelope, record.State, record.DecisionID, record.OutcomeID, record.DiagnosticCode,
		record.CreatedAt, record.UpdatedAt, record.TerminalAt,
	)
}

func confirmationContinuationColumns() []string {
	return []string{
		"route_receipt", "organization_id", "owner_account_id", "origin_version", "digital_human_id", "origin_surface",
		"conversation_id", "message_id", "agent_id", "workflow_id", "run_id", "node_id",
		"protected_payload_envelope", "state", "decision_id", "outcome_id", "diagnostic_code",
		"created_at", "updated_at", "terminal_at",
	}
}

func continuationUUIDDriverValue(value uuid.UUID) driver.Value {
	if value == uuid.Nil {
		return nil
	}
	return value
}

func nullableContinuationStringDriverValue(value string) driver.Value {
	if value == "" {
		return nil
	}
	return value
}

type protectedTestPayload struct {
	organizationID uuid.UUID
	receipt        RouteClaimReceipt
	frozen         FrozenConfirmationRequest
}

type memoryContinuationPayloadProtector struct {
	mu       sync.Mutex
	next     int
	payloads map[string]protectedTestPayload
	last     FrozenConfirmationRequest
}

func (protector *memoryContinuationPayloadProtector) Seal(
	_ context.Context,
	organizationID uuid.UUID,
	receipt RouteClaimReceipt,
	frozen FrozenConfirmationRequest,
) (ContinuationPayloadEnvelope, error) {
	copy, err := cloneFrozenConfirmationRequest(frozen)
	if err != nil {
		return "", err
	}
	protector.mu.Lock()
	defer protector.mu.Unlock()
	protector.next++
	envelope := fmt.Sprintf("v1.sealed-envelope.%d", protector.next)
	protector.payloads[envelope] = protectedTestPayload{
		organizationID: organizationID, receipt: receipt, frozen: copy,
	}
	protector.last = copy
	return ContinuationPayloadEnvelope(envelope), nil
}

func (protector *memoryContinuationPayloadProtector) Open(
	_ context.Context,
	organizationID uuid.UUID,
	receipt RouteClaimReceipt,
	envelope ContinuationPayloadEnvelope,
) (FrozenConfirmationRequest, error) {
	protector.mu.Lock()
	payload, ok := protector.payloads[string(envelope)]
	protector.mu.Unlock()
	if !ok || payload.organizationID != organizationID || payload.receipt != receipt {
		return FrozenConfirmationRequest{}, fmt.Errorf("protected payload binding mismatch")
	}
	return cloneFrozenConfirmationRequest(payload.frozen)
}

func (protector *memoryContinuationPayloadProtector) lastSealedInputValue(key string) string {
	protector.mu.Lock()
	defer protector.mu.Unlock()
	input, _ := protector.last.Request.Input.(map[string]any)
	value, _ := input[key].(string)
	return value
}

func cloneFrozenConfirmationRequest(frozen FrozenConfirmationRequest) (FrozenConfirmationRequest, error) {
	encoded, err := json.Marshal(frozen.Request.Input)
	if err != nil {
		return FrozenConfirmationRequest{}, err
	}
	var input any
	if err = json.Unmarshal(encoded, &input); err != nil {
		return FrozenConfirmationRequest{}, err
	}
	copy := frozen
	copy.Request.Input = input
	encodedAccepted, err := json.Marshal(frozen.AcceptedOperation)
	if err != nil {
		return FrozenConfirmationRequest{}, err
	}
	if err := json.Unmarshal(encodedAccepted, &copy.AcceptedOperation); err != nil {
		return FrozenConfirmationRequest{}, err
	}
	copy.AcceptedOperation.Definition.PluginDigest = frozen.AcceptedOperation.Definition.PluginDigest
	return copy, nil
}

func nativeInvocationRows(records ...invocationRecord) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"receipt_id", "organization_id", "owner_account_id", "acting_principal_id", "binding_id", "connection_id", "connector_id", "target_type", "target_id", "target_scope", "target_version_id", "binding_instance_id", "operation_id", "logical_operation_id", "correlation_id", "input_digest", "contract_digest", "binding_revision", "credential_version", "state", "created_at"})
	for _, r := range records {
		rows.AddRow(r.ReceiptID, r.OrganizationID, r.OwnerAccountID, r.ActingPrincipalID, r.BindingID, r.ConnectionID, r.ConnectorID, r.TargetType, r.TargetID, r.TargetScope, r.TargetVersionID, r.BindingInstanceID, r.OperationID, r.LogicalOperationID, r.CorrelationID, r.InputDigest, r.ContractDigest, r.BindingRevision, r.CredentialVersion, r.State, r.CreatedAt)
	}
	return rows
}
