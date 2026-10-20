package container

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"gorm.io/datatypes"
)

type journalRuntimeResolver interface {
	runtimeauth.DigitalHumanRuntimeResolver
}
type journalOutcomeReader interface {
	GetOutcomeByLogicalOperation(context.Context, uuid.UUID, uuid.UUID, string) (core.OperationOutcome, error)
}
type workJournalGateway struct {
	restored   ActiveBackgroundTaskRun
	resolver   journalRuntimeResolver
	dispatcher dispatch.Dispatcher
	audit      journalOutcomeReader
}

type workJournalClaimContextKey struct{}
type workJournalClaim struct {
	OperationID uuid.UUID
	InputDigest string
}

func (g *workJournalGateway) valid(op workjournal.Operation) bool {
	if g == nil {
		return false
	}
	r := g.restored.Run
	return !g.restored.SocialOperations && op.ID != uuid.Nil && op.OrganizationID == r.OrganizationID && op.WorkspaceID == g.restored.WorkspaceID && op.OwnerID == g.restored.OwnerID && op.EmployeeID == r.DigitalHumanID && op.ConnectionID != uuid.Nil && core.Identifier(op.ConnectorID).Valid() && core.Identifier(op.OperationID).Valid() && r.ID != uuid.Nil && r.ConversationID != nil && *r.ConversationID != uuid.Nil && r.MessageID != nil && *r.MessageID != uuid.Nil
}
func (g *workJournalGateway) Execute(ctx context.Context, op workjournal.Operation) (workjournal.DispatchResult, error) {
	failed := workjournal.DispatchResult{State: workjournal.StateFailed, Receipt: datatypes.JSON(`{"submission":"not_submitted"}`)}
	if !g.valid(op) || op.EmployeeVersionID != g.restored.Run.DigitalHumanVersionID || op.State != workjournal.StateExecuting || op.Attempts != 1 || g.resolver == nil || g.dispatcher == nil {
		return failed, backgroundDenied(nil)
	}
	r := g.restored.Run
	snapshot, err := g.resolver.ResolveDigitalHumanCapabilities(ctx, runtimeauth.DigitalHumanCapabilityRequest{OrganizationID: op.OrganizationID, OwnerAccountID: op.OwnerID, DigitalHumanID: op.EmployeeID.String(), Scope: core.ScopePublished, VersionID: op.EmployeeVersionID.String()})
	if err != nil {
		return failed, err
	}
	// Select the exact connection's binding instance, including when the same
	// connector is attached several times. Resolve again immediately before send.
	var target core.TargetRef
	var bindingID uuid.UUID
	for _, capability := range snapshot.Capabilities {
		if capability.ConnectionID != op.ConnectionID || capability.OwnerAccountID != op.OwnerID || capability.ConnectorID.String() != op.ConnectorID || capability.Target.Type != core.TargetTypeDigitalHuman || capability.Target.ID != op.EmployeeID.String() || capability.Target.EffectiveScope() != core.ScopePublished || capability.Target.VersionID != op.EmployeeVersionID.String() {
			continue
		}
		for _, operation := range capability.Operations {
			if operation.ID == op.OperationID {
				if bindingID != uuid.Nil {
					return failed, backgroundDenied(nil)
				}
				bindingID = capability.BindingID
				target = capability.Target
			}
		}
	}
	if bindingID == uuid.Nil {
		return failed, backgroundDenied(nil)
	}
	identity, err := g.resolver.ResolveConnectorIdentity(ctx, dispatch.IdentityResolutionRequest{ConversationID: r.ConversationID.String(), OrganizationID: op.OrganizationID, OwnerAccountID: op.OwnerID, ActingPrincipalID: op.OwnerID, ConnectorID: core.Identifier(op.ConnectorID), Target: target, OperationID: core.Identifier(op.OperationID), LogicalOperationID: op.LogicalID(), CorrelationID: op.LogicalID(), Interactive: false})
	if err != nil {
		return failed, err
	}
	if identity.BindingID != bindingID || identity.ConnectionID != op.ConnectionID || identity.OwnerAccountID != op.OwnerID || identity.OrganizationID != op.OrganizationID || !identity.Target.SameIdentity(target) {
		return failed, backgroundDenied(nil)
	}
	var input map[string]any
	decoder := json.NewDecoder(bytes.NewReader(op.Input))
	decoder.UseNumber()
	if decoder.Decode(&input) != nil || input == nil {
		return failed, backgroundDenied(nil)
	}
	digest, err := backgroundInputDigest(input)
	if err != nil {
		return failed, err
	}
	dispatchCtx := execution.WithBackgroundIdentity(ctx, r.ID, *r.MessageID)
	dispatchCtx = context.WithValue(dispatchCtx, workJournalClaimContextKey{}, workJournalClaim{OperationID: op.ID, InputDigest: digest})
	result, err := g.dispatcher.Dispatch(dispatchCtx, dispatch.Invocation{Identity: identity, Input: input})
	if result.State == execution.ExecutionStateCompleted && g.matches(op, result.Outcome) {
		return journalNativeOutcome(result.Outcome, result.Value), nil
	}
	if result.Evidence.ValidFor(result.State) && result.Evidence.Disposition == execution.DispatchNotSubmitted {
		return failed, err
	}
	return workjournal.DispatchResult{State: workjournal.StateUnknown}, err
}
func (g *workJournalGateway) Recover(ctx context.Context, op workjournal.Operation) (workjournal.DispatchResult, error) {
	unknown := workjournal.DispatchResult{State: workjournal.StateUnknown}
	if !g.valid(op) || g.audit == nil {
		return unknown, backgroundDenied(nil)
	}
	outcome, err := g.audit.GetOutcomeByLogicalOperation(ctx, op.OrganizationID, op.OwnerID, op.LogicalID())
	if err != nil {
		return unknown, err
	}
	if !g.matches(op, outcome) {
		return unknown, backgroundDenied(nil)
	}
	return journalNativeOutcome(outcome, nil), nil
}
func (g *workJournalGateway) matches(op workjournal.Operation, out core.OperationOutcome) bool {
	return out.Validate() == nil && out.OrganizationID == op.OrganizationID && out.OwnerAccountID == op.OwnerID && out.ActingPrincipalID == op.OwnerID && out.ConnectorID.String() == op.ConnectorID && out.ConnectionID == op.ConnectionID && out.OperationID.String() == op.OperationID && out.LogicalOperationID == op.LogicalID() && out.Target.Type == core.TargetTypeDigitalHuman && out.Target.ID == op.EmployeeID.String() && out.Target.EffectiveScope() == core.ScopePublished && out.Target.VersionID == op.EmployeeVersionID.String()
}
func journalNativeOutcome(out core.OperationOutcome, value any) workjournal.DispatchResult {
	state := workjournal.StateUnknown
	switch out.Status {
	case core.OutcomeStatusSuccessful:
		state = workjournal.StateSucceeded
	case core.OutcomeStatusFailed:
		state = workjournal.StateFailed
	}
	receipt := map[string]any{"execution_outcome": out}
	if value != nil {
		receipt["result"] = value
	}
	data, err := json.Marshal(receipt)
	if err != nil || len(data) > workjournal.MaxPayloadBytes {
		delete(receipt, "result")
		receipt["result_omitted"] = true
		data, _ = json.Marshal(receipt)
	}
	id := out.ID
	return workjournal.DispatchResult{State: state, Receipt: data, EvidenceID: &id}
}
