package container

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/datatypes"
	"testing"
)

type journalResolverStub struct {
	caps    runtimeauth.ChatCapabilitySnapshot
	request dispatch.IdentityResolutionRequest
}

func (r *journalResolverStub) ResolveDigitalHumanCapabilities(context.Context, runtimeauth.DigitalHumanCapabilityRequest) (runtimeauth.ChatCapabilitySnapshot, error) {
	return r.caps, nil
}
func (r *journalResolverStub) ResolveConnectorIdentity(_ context.Context, req dispatch.IdentityResolutionRequest) (dispatch.Identity, error) {
	r.request = req
	for _, c := range r.caps.Capabilities {
		if c.Target.SameIdentity(req.Target) {
			return dispatch.Identity{OrganizationID: req.OrganizationID, OwnerAccountID: req.OwnerAccountID, ActingPrincipalID: req.ActingPrincipalID, BindingID: c.BindingID, ConnectionID: c.ConnectionID, ConnectorID: req.ConnectorID, Target: req.Target, OperationID: req.OperationID, LogicalOperationID: req.LogicalOperationID, CorrelationID: req.CorrelationID, ConversationID: req.ConversationID}, nil
		}
	}
	return dispatch.Identity{}, errors.New("not found")
}

type journalDispatcherStub struct {
	calls  int
	last   dispatch.Invocation
	ctx    context.Context
	result execution.ExecutionResult
	err    error
}

func (d *journalDispatcherStub) Dispatch(ctx context.Context, i dispatch.Invocation) (execution.ExecutionResult, error) {
	d.calls++
	d.last = i
	d.ctx = ctx
	return d.result, d.err
}

type journalAuditStub struct {
	out   core.OperationOutcome
	calls int
}

func (a *journalAuditStub) GetOutcomeByLogicalOperation(context.Context, uuid.UUID, uuid.UUID, string) (core.OperationOutcome, error) {
	a.calls++
	return a.out, nil
}
func TestGenericGatewayUsesExactConnectionAndAuthoritativeEvidence(t *testing.T) {
	for _, connector := range []string{"gmail", "x-social"} {
		t.Run(connector, func(t *testing.T) {
			org, workspace, owner, employee, version := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
			conv, msg := uuid.New(), uuid.New()
			restored := ActiveBackgroundTaskRun{WorkspaceID: workspace, OwnerID: owner, Run: digitalhuman.Run{ID: uuid.New(), OrganizationID: org, WorkspaceID: workspace, DigitalHumanID: employee, DigitalHumanVersionID: version, ConversationID: &conv, MessageID: &msg}}
			op := workjournal.Operation{ID: uuid.New(), Scope: workjournal.Scope{OrganizationID: org, WorkspaceID: workspace, OwnerID: owner, EmployeeID: employee}, EmployeeVersionID: version, ConnectorID: connector, ConnectionID: uuid.New(), OperationID: connector + ".message.send", Input: datatypes.JSON(`{"text":"reply","recipient_id":9223372036854775807}`), State: workjournal.StateExecuting, Attempts: 1}
			target := core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: employee.String(), Scope: core.ScopePublished, VersionID: version.String(), BindingInstanceID: uuid.NewString()}
			selected := runtimeauth.ChatCapability{Target: target, BindingID: uuid.New(), ConnectionID: op.ConnectionID, OwnerAccountID: owner, ConnectorID: core.Identifier(connector), Operations: nil}
			selected.Operations = append(selected.Operations, catalog.Operation{ID: op.OperationID, Effect: catalog.OperationEffectExternalSend})
			other := selected
			other.ConnectionID = uuid.New()
			other.BindingID = uuid.New()
			other.Target.BindingInstanceID = uuid.NewString()
			resolver := &journalResolverStub{caps: runtimeauth.ChatCapabilitySnapshot{Capabilities: []runtimeauth.ChatCapability{other, selected}}}
			outcome := core.OperationOutcome{ID: uuid.New(), DecisionID: uuid.New(), OrganizationID: org, OwnerAccountID: owner, ActingPrincipalID: owner, ConnectorID: core.Identifier(connector), ConnectionID: op.ConnectionID, Target: target, OperationID: core.Identifier(op.OperationID), LogicalOperationID: op.LogicalID(), Status: core.OutcomeStatusSuccessful}
			dispatcher := &journalDispatcherStub{result: execution.ExecutionResult{State: execution.ExecutionStateCompleted, Outcome: outcome, Value: map[string]any{"id": "provider-message"}}}
			audit := &journalAuditStub{out: outcome}
			gateway := &workJournalGateway{restored: restored, resolver: resolver, dispatcher: dispatcher, audit: audit}
			result, e := gateway.Execute(t.Context(), op)
			if e != nil || result.State != workjournal.StateSucceeded || dispatcher.calls != 1 || dispatcher.last.Identity.ConnectionID != op.ConnectionID {
				t.Fatal(result, e, dispatcher)
			}
			if resolver.request.Target.BindingInstanceID != target.BindingInstanceID {
				t.Fatal("wrong binding instance")
			}
			input := dispatcher.last.Input.(map[string]any)
			if input["recipient_id"] != json.Number("9223372036854775807") {
				t.Fatal("lost integer", input)
			}
			if _, ok := dispatcher.ctx.Value(workJournalClaimContextKey{}).(workJournalClaim); !ok {
				t.Fatal("missing private journal claim")
			}
			result, e = gateway.Recover(t.Context(), op)
			if e != nil || result.State != workjournal.StateSucceeded || dispatcher.calls != 1 || audit.calls != 1 {
				t.Fatal(result, e)
			}
			oldVersion := op.EmployeeVersionID
			op.EmployeeVersionID = uuid.New()
			audit.out.Target.VersionID = op.EmployeeVersionID.String()
			result, e = gateway.Recover(t.Context(), op)
			if e != nil || result.State != workjournal.StateSucceeded {
				t.Fatal("old published version could not recover", result, e)
			}
			op.EmployeeVersionID = oldVersion
			audit.out.Target.VersionID = oldVersion.String()
			audit.out.ConnectionID = uuid.New()
			result, _ = gateway.Recover(t.Context(), op)
			if result.State != workjournal.StateUnknown {
				t.Fatal("foreign receipt accepted", result)
			}
			dispatcher.err = errors.New("timeout")
			dispatcher.result = execution.ExecutionResult{}
			result, _ = gateway.Execute(t.Context(), op)
			if result.State != workjournal.StateUnknown {
				t.Fatal(result)
			}
			op.EmployeeVersionID = uuid.New()
			before := dispatcher.calls
			result, _ = gateway.Execute(t.Context(), op)
			if result.State != workjournal.StateFailed || dispatcher.calls != before {
				t.Fatal("stale version submitted", result)
			}
		})
	}
}

// This composition exercises the real native router's input copy between the
// journal-created claim and the background hook, rather than mocking that hop.
func TestGenericJournalUsesNativeRouteWithoutChangingClaimedNumbers(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	if err := db.AutoMigrate(&workjournal.Operation{}); err != nil {
		t.Fatal(err)
	}
	store, _ := workjournal.New(db, nil)
	org, workspace, owner, employee, version := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	conv, msg := uuid.New(), uuid.New()
	scope := workjournal.Scope{OrganizationID: org, WorkspaceID: workspace, OwnerID: owner, EmployeeID: employee}
	op, err := store.Prepare(t.Context(), scope, workjournal.PrepareRequest{WorkKey: "native-number-check", EmployeeVersionID: version, ConnectorID: "gmail", ConnectionID: uuid.New(), OperationID: "gmail.message.send", Input: datatypes.JSON(`{"subject":"test","text":"reply","to":["user@example.com"],"correlation_number":9223372036854775807,"value":1e0}`)})
	if err != nil {
		t.Fatal(err)
	}
	restored := ActiveBackgroundTaskRun{WorkspaceID: workspace, OwnerID: owner, Run: digitalhuman.Run{ID: uuid.New(), OrganizationID: org, WorkspaceID: workspace, DigitalHumanID: employee, DigitalHumanVersionID: version, ConversationID: &conv, MessageID: &msg}}
	target := core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: employee.String(), Scope: core.ScopePublished, VersionID: version.String(), BindingInstanceID: uuid.NewString()}
	capability := runtimeauth.ChatCapability{Target: target, BindingID: uuid.New(), ConnectionID: op.ConnectionID, OwnerAccountID: owner, ConnectorID: "gmail", Operations: []catalog.Operation{{ID: op.OperationID}}}
	authorizer := &ConnectorBackgroundAuthorization{resolver: &BackgroundTaskRunResolver{db: db}}
	native := &journalNativeClaimService{authorizer: authorizer, restored: restored}
	router, _ := execution.NewNativeRouter(journalNativeClaimStore{}, native)
	adapter, _ := dispatch.New(router)
	gateway := &workJournalGateway{restored: restored, resolver: &journalResolverStub{caps: runtimeauth.ChatCapabilitySnapshot{Capabilities: []runtimeauth.ChatCapability{capability}}}, dispatcher: adapter}
	out, err := store.Dispatch(t.Context(), scope, op.ID, gateway)
	if err != nil || out.State != workjournal.StateSucceeded {
		t.Fatal(out, err)
	}
	if native.input["correlation_number"] != json.Number("9223372036854775807") || native.input["value"] != json.Number("1e0") {
		t.Fatal("native route changed frozen numbers", native.input)
	}
}

type journalNativeClaimStore struct{}

func (journalNativeClaimStore) Claim(context.Context, execution.InvocationClaim) (execution.RouteClaimReceipt, error) {
	return execution.RouteClaimReceipt(uuid.NewString()), nil
}
func (journalNativeClaimStore) Resume(context.Context, execution.InvocationClaim, execution.RouteClaimReceipt) error {
	return errors.New("unexpected confirmation")
}

type journalNativeClaimService struct {
	authorizer *ConnectorBackgroundAuthorization
	restored   ActiveBackgroundTaskRun
	input      map[string]any
}

func (s *journalNativeClaimService) PrepareInvocation(context.Context, execution.ExecutionRequest) (execution.InvocationSnapshot, error) {
	return execution.InvocationSnapshot{}, nil
}
func (s *journalNativeClaimService) InvokeConnector(ctx context.Context, p execution.PinnedExecution) (execution.ExecutionResult, error) {
	s.input = p.Request.Input.(map[string]any)
	request := execution.BackgroundAuthorizationRequest{Request: p.Request, Connection: core.PersonalConnection{ID: p.Request.ConnectionID}}
	if err := s.authorizer.authorizeWorkJournalClaim(ctx, s.restored, request, s.input); err != nil {
		return execution.ExecutionResult{}, err
	}
	r := p.Request
	return execution.ExecutionResult{State: execution.ExecutionStateCompleted, Outcome: core.OperationOutcome{ID: uuid.New(), DecisionID: uuid.New(), OrganizationID: r.OrganizationID, OwnerAccountID: r.OwnerAccountID, ActingPrincipalID: r.ActingPrincipalID, ConnectorID: r.ConnectorID, ConnectionID: r.ConnectionID, Target: r.Target, OperationID: r.OperationID, LogicalOperationID: r.LogicalOperationID, Status: core.OutcomeStatusSuccessful}}, nil
}
