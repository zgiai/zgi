package execution

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func validDigitalHumanTarget() core.TargetRef {
	return core.TargetRef{
		Type:              core.TargetTypeDigitalHuman,
		ID:                uuid.NewString(),
		Scope:             core.ScopeDraft,
		BindingInstanceID: uuid.NewString(),
	}
}

// digitalHumanServerOpsTarget mirrors the durable published host identity a
// DigitalHuman run dispatches for. The exact published version and resource
// instance are part of the identity the confirmation receipt is pinned to.
func digitalHumanServerOpsTarget() core.TargetRef {
	return core.TargetRef{
		Type:              core.TargetTypeDigitalHuman,
		ID:                uuid.NewString(),
		Scope:             core.ScopePublished,
		VersionID:         uuid.NewString(),
		BindingInstanceID: uuid.NewString(),
	}
}

func TestDigitalHumanApprovalDecisionEnforcesPlatformBoundary(t *testing.T) {
	receipt := RouteClaimReceipt("route-claim-receipt")
	staleReceipt := RouteClaimReceipt("stale-route-claim-receipt")
	confirmed := &RuntimeConfirmationCommand{RouteReceipt: receipt, Decision: ConfirmationDecisionConfirmed}
	staleConfirmation := &RuntimeConfirmationCommand{RouteReceipt: staleReceipt, Decision: ConfirmationDecisionConfirmed}

	for _, testCase := range []struct {
		name         string
		target       core.TargetRef
		risk         RiskClassification
		interactive  bool
		confirmation *RuntimeConfirmationCommand
		want         digitalHumanApprovalDecision
	}{
		{
			name:   "chat keeps the shared path",
			target: core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"},
			risk:   RiskTrustedLow, want: digitalHumanApprovalShared,
		},
		{
			name:   "agent keeps the shared path",
			target: core.TargetRef{Type: core.TargetTypeAgent, ID: uuid.NewString(), Scope: core.ScopeDraft},
			risk:   RiskHigh, want: digitalHumanApprovalShared,
		},
		{
			name:   "workflow keeps the shared path",
			target: core.TargetRef{Type: core.TargetTypeWorkflow, ID: uuid.NewString(), Scope: core.ScopeDraft, BindingInstanceID: uuid.NewString()},
			risk:   RiskUnclassified, want: digitalHumanApprovalShared,
		},
		{
			name:   "trusted low fails closed",
			target: validDigitalHumanTarget(), risk: RiskTrustedLow,
			interactive: true, want: digitalHumanApprovalFailClosed,
		},
		{
			name:   "trusted low with a matching confirmation still fails closed",
			target: validDigitalHumanTarget(), risk: RiskTrustedLow,
			interactive: true, confirmation: confirmed, want: digitalHumanApprovalFailClosed,
		},
		{
			name:   "unknown classification fails closed",
			target: validDigitalHumanTarget(), risk: RiskClassification("unrecognized"),
			interactive: true, want: digitalHumanApprovalFailClosed,
		},
		{
			name:   "confirmation for another receipt fails closed",
			target: validDigitalHumanTarget(), risk: RiskHigh,
			interactive: true, confirmation: staleConfirmation, want: digitalHumanApprovalFailClosed,
		},
		{
			name:   "non-interactive high is blocked",
			target: validDigitalHumanTarget(), risk: RiskHigh,
			want: digitalHumanApprovalNonInteractiveBlocked,
		},
		{
			name:   "non-interactive unclassified is blocked",
			target: validDigitalHumanTarget(), risk: RiskUnclassified,
			want: digitalHumanApprovalNonInteractiveBlocked,
		},
		{
			name:   "interactive high needs a fresh confirmation",
			target: validDigitalHumanTarget(), risk: RiskHigh,
			interactive: true, want: digitalHumanApprovalFreshConfirmation,
		},
		{
			name:   "interactive unclassified needs a fresh confirmation",
			target: validDigitalHumanTarget(), risk: RiskUnclassified,
			interactive: true, want: digitalHumanApprovalFreshConfirmation,
		},
		{
			name:   "matching confirmation resumes the shared gate",
			target: validDigitalHumanTarget(), risk: RiskHigh,
			interactive: true, confirmation: confirmed, want: digitalHumanApprovalFreshConfirmation,
		},
		{
			name:   "non-interactive matching confirmation is still blocked",
			target: validDigitalHumanTarget(), risk: RiskHigh,
			confirmation: confirmed, want: digitalHumanApprovalNonInteractiveBlocked,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got := digitalHumanApproval(
				testCase.target, testCase.risk, testCase.interactive, testCase.confirmation, receipt,
			)
			if got != testCase.want {
				t.Fatalf("digitalHumanApproval() = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestDigitalHumanApprovalSessionGrantsNeverPreauthorize(t *testing.T) {
	digitalHuman := digitalHumanApproval(validDigitalHumanTarget(), RiskHigh, true, nil, "route-claim-receipt")
	if !digitalHuman.governs() {
		t.Fatal("DigitalHuman invocation did not engage the explicit approval boundary")
	}
	if digitalHuman.sessionGrantPreauthorizes() {
		t.Fatal("a session grant was allowed to preauthorize a DigitalHuman invocation")
	}

	shared := digitalHumanApproval(
		core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"}, RiskHigh, true, nil, "route-claim-receipt",
	)
	if shared.governs() || !shared.sessionGrantPreauthorizes() {
		t.Fatalf("shared session-grant behavior changed: decision = %d", shared)
	}
}

func TestDigitalHumanGovernanceManifestRequiresPerInvocationApproval(t *testing.T) {
	definition := validDefinition()
	operation := definition.Operations[0]

	digitalHuman := connectorGovernanceManifest(definition, operation, RiskHigh, validDigitalHumanTarget())
	if !digitalHuman.ApprovalEveryInvocation ||
		digitalHuman.DefaultApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk {
		t.Fatalf("DigitalHuman manifest = %#v", digitalHuman)
	}

	chat := connectorGovernanceManifest(
		definition, operation, RiskHigh, core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"},
	)
	if chat.ApprovalEveryInvocation {
		t.Fatalf("Chat manifest unexpectedly requires approval every invocation: %#v", chat)
	}
}

// digitalHumanServerOpsRequest is the typed identity a DigitalHuman run
// dispatches for the built-in SSH Server Operations connector.
func digitalHumanServerOpsRequest(operationID string, input any, interactive bool) ExecutionRequest {
	ownerID := uuid.New()
	return ExecutionRequest{
		ConversationID: uuid.NewString(),
		OrganizationID: uuid.New(), OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
		BindingID: uuid.New(), ConnectionID: uuid.New(),
		ConnectorID:        core.Identifier(catalog.ServerOpsSSHConnectorID),
		Target:             digitalHumanServerOpsTarget(),
		OperationID:        core.Identifier(operationID),
		LogicalOperationID: uuid.NewString(), CorrelationID: uuid.NewString(),
		Interactive: interactive, Input: input,
	}
}

// newDigitalHumanServerOpsHarness wires the real execution Service over fakes
// for the Digital-Human-only SSH contract. The Connection keeps the historical
// owner constraint and the exact owner-scoped Target Binding supplies the
// DigitalHuman host boundary, which is the persisted production shape.
func newDigitalHumanServerOpsHarness(t *testing.T, request ExecutionRequest) *executionHarness {
	t.Helper()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	definition := catalog.ServerOpsSSHDefinition()
	operations := []core.Identifier{request.OperationID}

	bindingConstraint := core.OwnerConstraint(request.OwnerAccountID, operations)
	bindingConstraint.TargetBoundaries = []core.TargetBoundary{{
		Type: request.Target.Type, ID: request.Target.ID,
		Scope: string(request.Target.EffectiveScope()), VersionID: request.Target.VersionID,
	}}
	binding := core.TargetBinding{
		Constraint: bindingConstraint,
		ID:         request.BindingID, OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ConnectionID: request.ConnectionID, ConnectorID: request.ConnectorID, Target: request.Target,
		Status: core.BindingStatusActive, Revision: 1,
	}

	identity := serverOpsSSHTestIdentity
	connection := core.PersonalConnection{
		Constraint: core.OwnerConstraint(request.OwnerAccountID, operations),
		ID:         request.ConnectionID, OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ConnectorID: request.ConnectorID, ProviderID: core.Identifier(definition.ProviderID),
		DriverID: core.Identifier(definition.DriverID), AuthMethodID: core.Identifier(definition.AuthMethodID),
		Name: "Production server", ExternalAccountID: &identity,
		EncryptedCredentials: "encrypted-credential-envelope", CredentialVersion: 1,
		State: core.ReadinessReady, UsableOperationIDs: operations,
		UnavailableOperations: map[core.Identifier]string{}, Revision: 1,
		LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now,
	}

	events := &eventLog{}
	harness := &executionHarness{
		pinned:      PinnedExecution{Request: request, Path: PathConnectorNative, Receipt: "route-claim-receipt"},
		events:      events,
		bindings:    &bindingReaderSpy{binding: binding, events: events},
		connections: &connectionReaderSpy{connection: connection, events: events},
		target:      &targetAuthorizerSpy{events: events},
		risk:        &riskResolverSpy{classification: RiskHigh, events: events},
		credentials: &credentialResolverSpy{events: events},
		audit:       &auditRecorderSpy{events: events},
		provider: &providerInvokerSpy{
			events: events,
			result: ProviderInvocationResult{Value: "serverops-value", Status: core.OutcomeStatusSuccessful, ProviderRequestID: "provider-request-1"},
		},
	}
	service, err := NewService(
		&definitionCatalogStub{definition: definition}, harness.bindings, harness.connections,
		harness.target, harness.risk, harness.credentials, harness.audit, harness.provider,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.now = func() time.Time { return now }
	service.newID = uuid.New
	harness.service = service
	return harness
}

func digitalHumanServerOpsOperation(t *testing.T, definition catalog.Definition, operationID core.Identifier) catalog.Operation {
	t.Helper()
	for _, operation := range definition.Operations {
		if operation.ID == operationID.String() {
			return operation
		}
	}
	t.Fatalf("operation %q is missing from the SSH definition", operationID)
	return catalog.Operation{}
}

// digitalHumanServerOpsSessionGrant builds a grant that the shared
// Chat/Agent/Workflow matcher would accept, so the test proves the
// DigitalHuman boundary ignores it rather than merely failing to match it.
func digitalHumanServerOpsSessionGrant(request ExecutionRequest, definition catalog.Definition, operation catalog.Operation) toolgovernance.SessionGrant {
	return toolgovernance.SessionGrant{
		ConversationID: request.ConversationID,
		OrganizationID: request.OrganizationID.String(), UserID: request.OwnerAccountID.String(),
		SkillID: operation.SkillID, ProviderType: "connector", ProviderID: definition.ID,
		ConnectionID: request.ConnectionID.String(), BindingID: request.BindingID.String(),
		ToolID: operation.ID, Effect: toolgovernance.Effect(operation.Effect),
		AssetType: string(request.Target.Type), DataEgress: operation.DataEgress,
		ExternalDestination:  operation.ExternalDestination,
		SensitiveDataAllowed: operation.SensitiveDataAllowed,
		RiskLevel:            toolgovernance.RiskLevelHigh,
		Assets:               []toolgovernance.AssetRef{{ID: request.Target.ID, Type: string(request.Target.Type)}},
		ExpiresAt:            time.Now().Add(time.Hour),
	}
}

func confirmedDigitalHumanRequest(t *testing.T, harness *executionHarness, pending ExecutionResult) PinnedExecution {
	t.Helper()
	if pending.Confirmation == nil || pending.Confirmation.AcceptedOperation == nil {
		t.Fatalf("pending confirmation is incomplete: %#v", pending)
	}
	resume := harness.pinned
	accepted := *pending.Confirmation.AcceptedOperation
	resume.Request.AcceptedOperation = &accepted
	resume.Request.Confirmation = &RuntimeConfirmationCommand{
		RouteReceipt: harness.pinned.Receipt, Decision: ConfirmationDecisionConfirmed,
	}
	return resume
}

func TestDigitalHumanOnlyServerOpsSurfaceEligibility(t *testing.T) {
	definition := catalog.ServerOpsSSHDefinition()
	if !definition.Supports(catalog.SurfaceDigitalHuman) {
		t.Fatal("SSH Server Operations no longer supports the DigitalHuman surface")
	}
	for _, surface := range catalog.FirstReleaseSurfaces() {
		if definition.Supports(surface) {
			t.Fatalf("SSH Server Operations unexpectedly supports %q", surface)
		}
	}
	if surfaceForTarget(core.TargetTypeDigitalHuman) != catalog.SurfaceDigitalHuman {
		t.Fatal("DigitalHuman surface gate is closed")
	}
	if surfaceForTarget(core.TargetTypeChat) != catalog.SurfaceChat ||
		surfaceForTarget(core.TargetTypeAgent) != catalog.SurfaceAgent ||
		surfaceForTarget(core.TargetTypeWorkflow) != catalog.SurfaceWorkflow ||
		surfaceForTarget(core.TargetType("unknown")) != "" {
		t.Fatal("shared Chat/Agent/Workflow surface mapping changed")
	}
}

func TestDigitalHumanServicePathRequiresFreshInteractiveConfirmation(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, nil, true)
	harness := newDigitalHumanServerOpsHarness(t, request)

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || result.State != ExecutionStateRequiresConfirmation || result.Confirmation == nil {
		t.Fatalf("InvokeConnector() = %#v, %v", result, err)
	}
	confirmation := result.Confirmation
	if confirmation.RouteReceipt != harness.pinned.Receipt ||
		confirmation.ConnectorID != request.ConnectorID || confirmation.OperationID != request.OperationID ||
		confirmation.Target != request.Target || confirmation.Risk != RiskHigh ||
		confirmation.AcceptedOperation == nil {
		t.Fatalf("confirmation identity = %#v", confirmation)
	}
	if !completeDigitalHumanPreview(confirmation.Preview) ||
		confirmation.Preview.AccountIdentity != serverOpsSSHTestIdentity ||
		len(confirmation.Preview.Fields) == 0 {
		t.Fatalf("preview is not complete: %#v", confirmation.Preview)
	}
	encoded, marshalErr := json.Marshal(confirmation.Preview)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, secret := range []string{"encrypted-credential-envelope", "ENCRYPTED-PRIVATE-KEY-ENVELOPE"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("credential material %q escaped into the preview", secret)
		}
	}
	if harness.credentials.calls != 0 || harness.provider.calls != 0 ||
		len(harness.audit.decisions) != 0 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("confirmation prompt reached credentials/provider/decisions/outcomes = %d/%d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.decisions), len(harness.audit.outcomes))
	}
}

func TestDigitalHumanServicePathConfirmedReceiptInvokesProviderOnce(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionServiceStatus, map[string]any{"service_name": "nginx.service"}, true)
	harness := newDigitalHumanServerOpsHarness(t, request)

	pending, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || pending.State != ExecutionStateRequiresConfirmation {
		t.Fatalf("pending InvokeConnector() = %#v, %v", pending, err)
	}
	result, err := harness.service.InvokeConnector(context.Background(), confirmedDigitalHumanRequest(t, harness, pending))
	if err != nil || result.State != ExecutionStateCompleted || result.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("confirmed InvokeConnector() = %#v, %v", result, err)
	}
	if harness.credentials.calls != 1 || harness.provider.calls != 1 || !harness.provider.sawCredential ||
		len(harness.audit.outcomes) != 1 {
		t.Fatalf("credentials/provider/outcomes = %d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.outcomes))
	}
}

func TestDigitalHumanServicePathStaleReceiptDeniedBeforeDispatch(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, nil, true)
	harness := newDigitalHumanServerOpsHarness(t, request)

	pending, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || pending.State != ExecutionStateRequiresConfirmation {
		t.Fatalf("pending InvokeConnector() = %#v, %v", pending, err)
	}
	resume := confirmedDigitalHumanRequest(t, harness, pending)
	resume.Request.Confirmation.RouteReceipt = RouteClaimReceipt("stale-route-claim-receipt")

	if _, err := harness.service.InvokeConnector(context.Background(), resume); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatalf("stale receipt error = %v, want invalid input", err)
	}
	if harness.credentials.calls != 0 || harness.provider.calls != 0 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("stale receipt reached credentials/provider/outcomes = %d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.outcomes))
	}
}

func TestDigitalHumanServicePathNonInteractiveHighBlocked(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, nil, false)
	harness := newDigitalHumanServerOpsHarness(t, request)

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeConfirmationRequired) || result.State != ExecutionStateBlocked {
		t.Fatalf("non-interactive InvokeConnector() = %#v, %v", result, err)
	}
	if harness.credentials.calls != 0 || harness.provider.calls != 0 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("non-interactive block reached credentials/provider/outcomes = %d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.outcomes))
	}
	if len(harness.audit.decisions) != 1 || harness.audit.decisions[0].Status != core.DecisionStatusBlocked {
		t.Fatalf("non-interactive decisions = %#v", harness.audit.decisions)
	}
}

func TestDigitalHumanServicePathTrustedLowDeniedBeforeDispatch(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, nil, true)
	harness := newDigitalHumanServerOpsHarness(t, request)
	harness.risk.classification = RiskTrustedLow

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || result.State != ExecutionStateBlocked {
		t.Fatalf("trusted-low InvokeConnector() = %#v, %v", result, err)
	}
	if harness.credentials.calls != 0 || harness.provider.calls != 0 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("trusted-low denial reached credentials/provider/outcomes = %d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.outcomes))
	}
	if len(harness.audit.decisions) != 1 || harness.audit.decisions[0].Status != core.DecisionStatusDenied {
		t.Fatalf("trusted-low decisions = %#v", harness.audit.decisions)
	}
}

func TestDigitalHumanServicePathSessionGrantNeverPreauthorizes(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, nil, true)
	definition := catalog.ServerOpsSSHDefinition()
	operation := digitalHumanServerOpsOperation(t, definition, request.OperationID)
	request.SessionGrants = []toolgovernance.SessionGrant{
		digitalHumanServerOpsSessionGrant(request, definition, operation),
	}
	// A caller-supplied preauthorization flag is not authority either.
	request.Preauthorized = true
	if _, matched := connectorSessionGrantMatches(request, definition, operation, RiskHigh); !matched {
		t.Fatal("test session grant does not match the shared governance manifest")
	}

	harness := newDigitalHumanServerOpsHarness(t, request)
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || result.State != ExecutionStateRequiresConfirmation || result.Confirmation == nil {
		t.Fatalf("session grant preauthorized a DigitalHuman dispatch: %#v, %v", result, err)
	}
	if harness.credentials.calls != 0 || harness.provider.calls != 0 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("preauthorized attempt reached credentials/provider/outcomes = %d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.outcomes))
	}
}

func TestDigitalHumanServicePathIncompletePreviewDeniedBeforeCredential(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionServiceStatus, map[string]any{"service_name": "nginx"}, true)
	harness := newDigitalHumanServerOpsHarness(t, request)

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || result.State != ExecutionStateBlocked {
		t.Fatalf("incomplete-preview InvokeConnector() = %#v, %v", result, err)
	}
	if harness.credentials.calls != 0 || harness.provider.calls != 0 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("incomplete preview reached credentials/provider/outcomes = %d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.outcomes))
	}
	if len(harness.audit.decisions) != 1 || harness.audit.decisions[0].Status != core.DecisionStatusDenied {
		t.Fatalf("incomplete-preview decisions = %#v", harness.audit.decisions)
	}
}

func TestDigitalHumanServicePathConfirmedIncompletePreviewDeniedBeforeCredential(t *testing.T) {
	request := digitalHumanServerOpsRequest(serverops.ActionSystemSnapshot, nil, true)
	harness := newDigitalHumanServerOpsHarness(t, request)

	pending, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || pending.State != ExecutionStateRequiresConfirmation {
		t.Fatalf("pending InvokeConnector() = %#v, %v", pending, err)
	}
	// The verified host identity disappears before the approval resumes, so the
	// frozen request can no longer rebuild a complete preview.
	harness.connections.connection.ExternalAccountID = nil

	resume := confirmedDigitalHumanRequest(t, harness, pending)
	result, err := harness.service.InvokeConnector(context.Background(), resume)
	if !core.IsErrorCode(err, core.ErrorCodeAccessDenied) || result.State != ExecutionStateBlocked {
		t.Fatalf("confirmed incomplete-preview InvokeConnector() = %#v, %v", result, err)
	}
	if harness.credentials.calls != 0 || harness.provider.calls != 0 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("confirmed incomplete preview reached credentials/provider/outcomes = %d/%d/%d",
			harness.credentials.calls, harness.provider.calls, len(harness.audit.outcomes))
	}
	if len(harness.audit.decisions) != 1 || harness.audit.decisions[0].Status != core.DecisionStatusDenied {
		t.Fatalf("confirmed incomplete-preview decisions = %#v", harness.audit.decisions)
	}
}
