package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const executionSecretSentinel = "TEST-CONNECTOR-EXECUTION-SECRET-SENTINEL"

func TestServiceAuthorizesBeforeCredentialResolutionAndRecordsOneOutcome(t *testing.T) {
	harness := newExecutionHarness(t)
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil {
		t.Fatalf("InvokeConnector() error = %v", err)
	}
	if result.Value != "provider-value" || result.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("InvokeConnector() = %#v", result)
	}
	wantOrder := []string{"binding", "connection", "target_authority", "risk", "credentials", "decision", "provider", "outcome"}
	if !reflect.DeepEqual(harness.events.values, wantOrder) {
		t.Fatalf("events = %#v, want %#v", harness.events.values, wantOrder)
	}
	if harness.credentials.calls != 1 || harness.provider.calls != 1 || !harness.provider.sawCredential {
		t.Fatalf("credentials/provider calls = %d/%d, saw credential = %v", harness.credentials.calls, harness.provider.calls, harness.provider.sawCredential)
	}
	if len(harness.credentials.returned) != 0 {
		t.Fatalf("request-scoped credentials retained after provider call: %#v", harness.credentials.returned)
	}
	if len(harness.audit.decisions) != 1 || len(harness.audit.outcomes) != 1 {
		t.Fatalf("decision/outcome count = %d/%d", len(harness.audit.decisions), len(harness.audit.outcomes))
	}
	decision := harness.audit.decisions[0]
	if decision.Status != core.DecisionStatusSubmitted || decision.ReceiptID != string(harness.pinned.Receipt) ||
		decision.Details["execution_path"] != string(PathConnectorNative) ||
		decision.Details["risk_classification"] != string(RiskTrustedLow) {
		t.Fatalf("decision = %#v", decision)
	}
	if strings.Contains(fmt.Sprint(decision), executionSecretSentinel) ||
		strings.Contains(fmt.Sprint(harness.audit.outcomes[0]), executionSecretSentinel) {
		t.Fatal("secret sentinel escaped into decision or outcome")
	}
}

func TestServiceUsesMatchingSessionGrantWithoutConfirmation(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.risk.classification = RiskHigh
	definition := validDefinition()
	definition.Operations[0].Effect = catalog.OperationEffectExternalSend
	definition.Operations[0].DataEgress = true
	definition.Operations[0].ExternalDestination = "github"
	definition.Operations[0].InputSchema = map[string]interface{}{"type": "object"}
	definition.Operations[0].OutputSchema = map[string]interface{}{"type": "object"}
	definition.Operations[0].RiskHint = catalog.OperationRiskHintHigh
	harness.service.catalog = &definitionCatalogStub{definition: definition}
	request := &harness.pinned.Request
	request.ConversationID = request.Target.ID
	request.SessionGrants = []toolgovernance.SessionGrant{{
		ConversationID: request.ConversationID,
		OrganizationID: request.OrganizationID.String(), UserID: request.OwnerAccountID.String(),
		ProviderType: "connector", ProviderID: definition.ID,
		ConnectionID: request.ConnectionID.String(), BindingID: request.BindingID.String(),
		ToolID: definition.Operations[0].ID, Effect: toolgovernance.EffectExternalSend,
		AssetType: "chat", DataEgress: true, ExternalDestination: "github",
		RiskLevel: toolgovernance.RiskLevelHigh,
		Assets:    []toolgovernance.AssetRef{{ID: request.Target.ID, Type: "chat"}},
		ExpiresAt: time.Now().Add(time.Hour),
	}}
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || result.State != ExecutionStateCompleted || result.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("InvokeConnector() = %#v, %v", result, err)
	}
	if result.Confirmation != nil || harness.provider.calls != 1 || len(harness.audit.outcomes) != 1 {
		t.Fatalf("session grant did not bypass confirmation: result=%#v provider=%d outcomes=%d", result, harness.provider.calls, len(harness.audit.outcomes))
	}
}

func TestPinCatalogOperationPrefersAcceptedVersionPinner(t *testing.T) {
	definition := validDefinition()
	pinner := &definitionVersionPinnerStub{
		definitionCatalogStub: definitionCatalogStub{definition: catalog.Definition{}},
		pinned:                mustAcceptedOperation(t, definition, definition.Operations[0]),
	}
	pinned, ok := pinCatalogOperation(
		pinner, definition.ID, definition.Operations[0].ID,
	)
	if !ok || pinner.pinCalls != 1 || pinned.Definition.ID != definition.ID ||
		pinned.Operation.ID != definition.Operations[0].ID {
		t.Fatalf("pinCatalogOperation() = %#v, %v; calls=%d", pinned, ok, pinner.pinCalls)
	}
}

func TestServiceFailsClosedBeforeCredentialOrProviderAccess(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		mutate   func(*executionHarness)
		wantCode core.ErrorCode
	}{
		{
			name: "non-owner acting principal",
			mutate: func(h *executionHarness) {
				h.pinned.Request.ActingPrincipalID = uuid.New()
			},
			wantCode: core.ErrorCodeAccessDenied,
		},
		{
			name: "inactive binding",
			mutate: func(h *executionHarness) {
				h.bindings.binding.Status = core.BindingStatusDisabled
				disabledAt := time.Now().UTC()
				h.bindings.binding.DisabledAt = &disabledAt
			},
			wantCode: core.ErrorCodeTargetNotReady,
		},
		{
			name: "connection requires reconnect",
			mutate: func(h *executionHarness) {
				h.connections.connection.State = core.ReadinessReconnectRequired
			},
			wantCode: core.ErrorCodeConnectionNotReady,
		},
		{
			name: "operation is not verified usable",
			mutate: func(h *executionHarness) {
				h.connections.connection.UsableOperationIDs = []core.Identifier{"github.list_issues"}
				h.connections.connection.UnavailableOperations = map[core.Identifier]string{
					core.Identifier("github.create_issue"): "connector_capability_unavailable",
				}
			},
			wantCode: core.ErrorCodeAccessDenied,
		},
		{
			name: "target authority rejects owner",
			mutate: func(h *executionHarness) {
				h.target.err = core.NewError(core.ErrorCodeAccessDenied, false, nil)
			},
			wantCode: core.ErrorCodeAccessDenied,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newExecutionHarness(t)
			testCase.mutate(harness)
			if _, err := harness.service.InvokeConnector(context.Background(), harness.pinned); !core.IsErrorCode(err, testCase.wantCode) {
				t.Fatalf("InvokeConnector() error = %v, want %s", err, testCase.wantCode)
			}
			if harness.credentials.calls != 0 || harness.provider.calls != 0 || len(harness.audit.decisions) != 1 || len(harness.audit.outcomes) != 0 {
				t.Fatalf("denied call reached credentials/provider/decision/outcome = %d/%d/%d/%d",
					harness.credentials.calls, harness.provider.calls, len(harness.audit.decisions), len(harness.audit.outcomes))
			}
			decision := harness.audit.decisions[0]
			if decision.Status != core.DecisionStatusDenied || decision.ActingPrincipalID != harness.pinned.Request.ActingPrincipalID ||
				decision.OwnerAccountID != harness.pinned.Request.OwnerAccountID || decision.Details["execution_path"] != string(PathConnectorNative) ||
				decision.ReceiptID != string(harness.pinned.Receipt) || decision.Validate() != nil {
				t.Fatalf("automatic denial lost real actor or route identity: %#v", decision)
			}
			if testCase.name == "non-owner acting principal" && !reflect.DeepEqual(harness.events.values, []string{"decision"}) {
				t.Fatalf("non-owner touched protected state: %#v", harness.events.values)
			}
		})
	}
}

func TestServiceDoesNotManufactureDenialAuditForMalformedIdentity(t *testing.T) {
	for _, malformed := range []string{"actor", "owner", "receipt", "operation", "path"} {
		t.Run(malformed, func(t *testing.T) {
			h := newExecutionHarness(t)
			switch malformed {
			case "actor":
				h.pinned.Request.ActingPrincipalID = uuid.Nil
			case "owner":
				h.pinned.Request.OwnerAccountID = uuid.Nil
			case "receipt":
				h.pinned.Receipt = ""
			case "operation":
				h.pinned.Request.OperationID = "invalid operation"
			case "path":
				h.pinned.Path = Path("retired")
			}
			if _, err := h.service.InvokeConnector(context.Background(), h.pinned); err == nil || len(h.audit.decisions) != 0 || h.credentials.calls != 0 || h.provider.calls != 0 {
				t.Fatalf("malformed identity %s audited or executed: %v", malformed, err)
			}
		})
	}
}

func TestServiceReturnsRuntimeConfirmationBeforeCredentialOrProviderAccess(t *testing.T) {
	for _, risk := range []RiskClassification{RiskHigh, RiskUnclassified} {
		t.Run(string(risk), func(t *testing.T) {
			harness := newExecutionHarness(t)
			harness.risk.classification = risk
			result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
			if err != nil || result.State != ExecutionStateRequiresConfirmation || result.Confirmation == nil {
				t.Fatalf("InvokeConnector() = %#v, %v", result, err)
			}
			if result.Confirmation.RouteReceipt != harness.pinned.Receipt || result.Confirmation.Risk != risk ||
				result.Confirmation.ConnectorID != harness.pinned.Request.ConnectorID ||
				result.Confirmation.OperationID != harness.pinned.Request.OperationID ||
				result.Confirmation.Target != harness.pinned.Request.Target {
				t.Fatalf("confirmation = %#v", result.Confirmation)
			}
			if result.Confirmation.AcceptedOperation == nil ||
				result.Confirmation.AcceptedOperation.PluginVersion != validDefinition().PluginVersion ||
				result.Confirmation.AcceptedOperation.ContractDigest == "" {
				t.Fatalf("accepted operation = %#v", result.Confirmation.AcceptedOperation)
			}
			serialized, marshalErr := json.Marshal(result.Confirmation)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			if strings.Contains(string(serialized), result.Confirmation.AcceptedOperation.ContractDigest) {
				t.Fatalf("public confirmation exposed accepted contract: %s", serialized)
			}
			if harness.credentials.calls != 0 || harness.provider.calls != 0 || len(harness.audit.decisions) != 0 || len(harness.audit.outcomes) != 0 {
				t.Fatalf("prompt reached credentials/provider/decision/outcome = %d/%d/%d/%d",
					harness.credentials.calls, harness.provider.calls, len(harness.audit.decisions), len(harness.audit.outcomes))
			}
		})
	}
}

func TestServiceDirectBindingAuthorizationSkipsConfirmationForMatchingOperation(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.risk.classification = RiskHigh
	harness.bindings.binding.Constraint.AuthorizationMode = core.AuthorizationModeDirect
	digest, err := catalog.DefinitionContractDigest(validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	harness.bindings.binding.Constraint.AuthorizationConnectionRevision = harness.connections.connection.Revision
	harness.bindings.binding.Constraint.AuthorizationCredentialVersion = harness.connections.connection.CredentialVersion
	harness.bindings.binding.Constraint.AuthorizationContractDigest = digest
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || result.State != ExecutionStateCompleted || result.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("direct binding execution = %#v, %v", result, err)
	}
	if harness.provider.calls != 1 || len(harness.audit.decisions) != 1 || !harness.audit.decisions[0].Details["preauthorized"].(bool) {
		t.Fatalf("direct binding audit/provider = %d/%#v", harness.provider.calls, harness.audit.decisions)
	}
}

func TestServiceDirectBindingAuthorizationExpiresAfterConnectionRevisionChanges(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.risk.classification = RiskHigh
	harness.bindings.binding.Constraint.AuthorizationMode = core.AuthorizationModeDirect
	harness.bindings.binding.Constraint.AuthorizationConnectionRevision = 1
	harness.bindings.binding.Constraint.AuthorizationCredentialVersion = 1
	harness.connections.connection.Revision = 2

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || result.State != ExecutionStateRequiresConfirmation || result.Confirmation == nil {
		t.Fatalf("stale direct binding = %#v, %v", result, err)
	}
	if harness.provider.calls != 0 || len(harness.audit.decisions) != 0 {
		t.Fatalf("stale direct authorization executed/audited = provider:%d decisions:%d", harness.provider.calls, len(harness.audit.decisions))
	}
}

func freshDirectAuthorization(t *testing.T, harness *executionHarness) {
	t.Helper()
	digest, err := catalog.DefinitionContractDigest(validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	constraint := harness.bindings.binding.Constraint
	constraint.AuthorizationMode = core.AuthorizationModeDirect
	constraint.AuthorizationConnectionRevision = harness.connections.connection.Revision
	constraint.AuthorizationCredentialVersion = harness.connections.connection.CredentialVersion
	constraint.AuthorizationContractDigest = digest
}

func TestServiceDirectBindingAuthorizationExpiresAfterCredentialVersionChanges(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.risk.classification = RiskHigh
	freshDirectAuthorization(t, harness)
	harness.connections.connection.CredentialVersion += 1

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || result.State != ExecutionStateRequiresConfirmation || result.Confirmation == nil {
		t.Fatalf("credential-rotated direct binding = %#v, %v", result, err)
	}
	if harness.provider.calls != 0 || len(harness.audit.decisions) != 0 {
		t.Fatalf("rotated credential direct authorization executed/audited = provider:%d decisions:%d", harness.provider.calls, len(harness.audit.decisions))
	}
}

func TestServiceDirectBindingAuthorizationExpiresAfterContractDigestChanges(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.risk.classification = RiskHigh
	freshDirectAuthorization(t, harness)
	digest := harness.bindings.binding.Constraint.AuthorizationContractDigest
	last := digest[len(digest)-1]
	replacement := byte('1')
	if last == '1' {
		replacement = '0'
	}
	harness.bindings.binding.Constraint.AuthorizationContractDigest = digest[:len(digest)-1] + string(replacement)

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || result.State != ExecutionStateRequiresConfirmation || result.Confirmation == nil {
		t.Fatalf("contract-changed direct binding = %#v, %v", result, err)
	}
	if harness.provider.calls != 0 || len(harness.audit.decisions) != 0 {
		t.Fatalf("contract-changed direct authorization executed/audited = provider:%d decisions:%d", harness.provider.calls, len(harness.audit.decisions))
	}
}

func TestServiceBlocksNonInteractiveHighRiskWithoutProviderCall(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.risk.classification = RiskHigh
	harness.pinned.Request.Interactive = false

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if result.State != ExecutionStateBlocked || !core.IsErrorCode(err, core.ErrorCodeConfirmationRequired) {
		t.Fatalf("non-interactive high-risk = %#v, %v", result, err)
	}
	if harness.provider.calls != 0 || harness.credentials.calls != 0 {
		t.Fatalf("blocked operation reached provider/credentials = %d/%d", harness.provider.calls, harness.credentials.calls)
	}
	if len(harness.audit.decisions) != 1 || harness.audit.decisions[0].Status != core.DecisionStatusBlocked {
		t.Fatalf("blocked decision = %#v", harness.audit.decisions)
	}
}

func TestServiceResumesConfirmationWithFrozenAcceptedContractWithoutRepinning(t *testing.T) {
	harness := newExecutionHarness(t)
	definition := validDefinition()
	accepted := mustAcceptedOperation(t, definition, definition.Operations[0])
	pinner := &definitionVersionPinnerStub{
		definitionCatalogStub: definitionCatalogStub{definition: definition},
		pinned:                accepted,
	}
	service, err := NewService(
		pinner, harness.bindings, harness.connections, harness.target, harness.risk,
		harness.credentials, harness.audit, harness.provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	service.now = harness.service.now
	service.newID = harness.service.newID
	harness.risk.classification = RiskHigh

	pending, err := service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || pending.State != ExecutionStateRequiresConfirmation || pending.Confirmation == nil ||
		pending.Confirmation.AcceptedOperation == nil || pinner.pinCalls != 1 {
		t.Fatalf("pending confirmation = (%#v, %v), pin calls = %d", pending, err, pinner.pinCalls)
	}

	// The active catalog may update or become unavailable while the user is
	// deciding. A continuation must use the encrypted accepted snapshot and
	// must not consult Pin again.
	updated := definition
	updated.PluginVersion = "1.1.0"
	updated.Name = "GitHub updated display"
	pinner.pinned = mustAcceptedOperation(t, updated, updated.Operations[0])
	pinner.err = errors.New("current active definition must not be consulted")
	request := harness.pinned.Request
	frozenAccepted := *pending.Confirmation.AcceptedOperation
	request.AcceptedOperation = &frozenAccepted
	request.Confirmation = &RuntimeConfirmationCommand{
		RouteReceipt: harness.pinned.Receipt, Decision: ConfirmationDecisionConfirmed,
	}
	resumed := harness.pinned
	resumed.Request = request
	result, err := service.InvokeConnector(context.Background(), resumed)
	if err != nil || result.State != ExecutionStateCompleted || result.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("resumed execution = (%#v, %v)", result, err)
	}
	if pinner.pinCalls != 1 || harness.provider.calls != 1 || len(harness.audit.outcomes) != 1 {
		t.Fatalf("pin/provider/outcome calls = %d/%d/%d", pinner.pinCalls, harness.provider.calls, len(harness.audit.outcomes))
	}
}

func TestRouterDoesNotFallbackOnConnectorAuthorizationOrReadinessErrors(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*executionHarness)
		code   core.ErrorCode
	}{
		{
			name: "owner authorization",
			mutate: func(h *executionHarness) {
				h.pinned.Request.ActingPrincipalID = uuid.New()
			},
			code: core.ErrorCodeAccessDenied,
		},
		{
			name: "connection readiness",
			mutate: func(h *executionHarness) {
				h.connections.connection.State = core.ReadinessReconnectRequired
			},
			code: core.ErrorCodeConnectionNotReady,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newExecutionHarness(t)
			testCase.mutate(harness)
			claimer := &memoryInvocationStore{}
			router, err := NewNativeRouter(claimer, harness.service)
			if err != nil {
				t.Fatal(err)
			}

			if _, err := router.Route(context.Background(), harness.pinned.Request); !core.IsErrorCode(err, testCase.code) {
				t.Fatalf("Route() error = %v, want %s", err, testCase.code)
			}
			if claimer.claims != 0 || harness.credentials.calls != 0 || harness.provider.calls != 0 {
				t.Fatalf("claim/legacy/credentials/provider = %d/%d/%d/%d",
					claimer.claims, 0, harness.credentials.calls, harness.provider.calls)
			}
		})
	}
}

func TestServiceRequiresConnectorNativeRouteClaim(t *testing.T) {
	for _, mutate := range []func(*PinnedExecution){
		func(pinned *PinnedExecution) { pinned.Path = Path("retired") },
		func(pinned *PinnedExecution) { pinned.Receipt = "" },
	} {
		harness := newExecutionHarness(t)
		mutate(&harness.pinned)
		if _, err := harness.service.InvokeConnector(context.Background(), harness.pinned); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
			t.Fatalf("InvokeConnector() error = %v, want invalid input", err)
		}
		if harness.bindings.calls != 0 || harness.credentials.calls != 0 || harness.provider.calls != 0 {
			t.Fatalf("unpinned call reached binding/credentials/provider = %d/%d/%d", harness.bindings.calls, harness.credentials.calls, harness.provider.calls)
		}
	}
}

func TestServiceDecisionFailureSubmitsNothingAndDestroysCredential(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.audit.decisionErr = core.NewError(core.ErrorCodeInternal, true, nil)
	if _, err := harness.service.InvokeConnector(context.Background(), harness.pinned); !core.IsErrorCode(err, core.ErrorCodeInternal) {
		t.Fatalf("InvokeConnector() error = %v, want internal", err)
	}
	if harness.provider.calls != 0 || len(harness.audit.outcomes) != 0 {
		t.Fatalf("decision failure reached provider/outcome = %d/%d", harness.provider.calls, len(harness.audit.outcomes))
	}
	if len(harness.credentials.returned) != 0 {
		t.Fatalf("credentials retained after decision failure: %#v", harness.credentials.returned)
	}
}

func TestServiceProviderPanicBecomesOneUnknownOutcomeWithoutRetry(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.provider.panicValue = executionSecretSentinel
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeOutcomeUnknown) {
		t.Fatalf("InvokeConnector() error = %v, want unknown outcome", err)
	}
	if harness.provider.calls != 1 || len(harness.audit.outcomes) != 1 || result.Outcome.Status != core.OutcomeStatusUnknown {
		t.Fatalf("provider calls/outcomes/result = %d/%d/%#v", harness.provider.calls, len(harness.audit.outcomes), result)
	}
	if result.Outcome.DiagnosticCode != string(core.ErrorCodeOutcomeUnknown) ||
		strings.Contains(fmt.Sprint(result), executionSecretSentinel) {
		t.Fatalf("unsafe unknown outcome = %#v", result)
	}
}

func TestServiceProviderFailureRecordsExactlyOneTerminalOutcome(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.provider.result = ProviderInvocationResult{
		Status: core.OutcomeStatusFailed, FailureCode: core.ErrorCodeProviderUnavailable,
		DiagnosticCode: "provider_unavailable", Retryable: true,
	}
	harness.provider.err = errors.New(executionSecretSentinel)
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeProviderUnavailable) {
		t.Fatalf("InvokeConnector() error = %v, want provider unavailable", err)
	}
	if result.Outcome.Status != core.OutcomeStatusFailed || len(harness.audit.outcomes) != 1 {
		t.Fatalf("result/outcomes = %#v/%d", result, len(harness.audit.outcomes))
	}
	if strings.Contains(err.Error(), executionSecretSentinel) || strings.Contains(fmt.Sprint(result), executionSecretSentinel) {
		t.Fatal("provider cause escaped execution boundary")
	}
}

func TestServiceRedactsCredentialEchoedAsProviderMetadata(t *testing.T) {
	harness := newExecutionHarness(t)
	credential := "secret-token-123"
	harness.credentials.secretValue = credential
	harness.provider.result = ProviderInvocationResult{
		Status: core.OutcomeStatusFailed, FailureCode: core.ErrorCodeProviderUnavailable,
		ProviderRequestID: credential, DiagnosticCode: credential,
	}
	harness.provider.err = errors.New("provider failed")

	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeProviderUnavailable) {
		t.Fatalf("InvokeConnector() error = %v", err)
	}
	if result.Outcome.ProviderRequestID != "" || result.Outcome.DiagnosticCode != string(core.ErrorCodeProviderUnavailable) {
		t.Fatalf("credential-bearing metadata was retained: %#v", result.Outcome)
	}
	if strings.Contains(fmt.Sprint(harness.audit.outcomes), credential) {
		t.Fatal("credential escaped into persisted outcome")
	}
}

func TestServiceOutcomePersistenceFailureDoesNotResubmit(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.audit.outcomeErr = core.NewError(core.ErrorCodeInternal, true, nil)
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeOutcomeUnknown) || result.Outcome.Status != core.OutcomeStatusUnknown {
		t.Fatalf("InvokeConnector() = %#v, %v", result, err)
	}
	if harness.provider.calls != 1 || harness.audit.outcomeCalls != 1 || len(harness.audit.outcomes) != 1 ||
		result.Outcome.ID == uuid.Nil || result.Outcome.DecisionID != result.DecisionID ||
		harness.audit.outcomes[0].ID != result.Outcome.ID {
		t.Fatalf("provider/outcome attempts/evidence = %d/%d/%#v", harness.provider.calls, harness.audit.outcomeCalls, result)
	}
}

func TestServiceDoesNotReturnUnpersistedOutcomeIdentityWhenConvergenceFails(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.audit.outcomeErr = core.NewError(core.ErrorCodeInternal, true, nil)
	harness.audit.convergeErr = core.NewError(core.ErrorCodeInternal, true, nil)
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if !core.IsErrorCode(err, core.ErrorCodeOutcomeUnknown) {
		t.Fatalf("InvokeConnector() = %#v, %v", result, err)
	}
	if harness.provider.calls != 1 || result.DecisionID == uuid.Nil || result.Outcome.ID != uuid.Nil ||
		len(harness.audit.outcomes) != 0 {
		t.Fatalf("unpersisted outcome escaped = provider %d, result %#v, outcomes %#v",
			harness.provider.calls, result, harness.audit.outcomes)
	}
}

func TestServiceRuntimeConfirmationFinalizationFailureNeverSubmitsOrCancels(t *testing.T) {
	for _, decision := range []ConfirmationDecision{
		ConfirmationDecisionConfirmed,
		ConfirmationDecisionCancelled,
	} {
		t.Run(string(decision), func(t *testing.T) {
			harness := newExecutionHarness(t)
			resume := frozenConfirmationRequest(t, harness, decision)
			harness.audit.decisionErr = core.NewError(core.ErrorCodeInternal, true, nil)

			result, err := harness.service.InvokeConnector(context.Background(), resume)
			if !core.IsErrorCode(err, core.ErrorCodeInternal) {
				t.Fatalf("InvokeConnector() = %#v, %v", result, err)
			}
			if result.State != ExecutionStateUnresolved || !result.Evidence.ValidFor(result.State) || result.DecisionID != uuid.Nil || harness.provider.calls != 0 ||
				len(harness.audit.decisions) != 0 || len(harness.audit.outcomes) != 0 {
				t.Fatalf("failed finalization produced evidence/submission = %#v, provider=%d decisions=%#v outcomes=%#v",
					result, harness.provider.calls, harness.audit.decisions, harness.audit.outcomes)
			}
		})
	}
}

func TestServiceDuplicateConfirmedContinuationNeverResubmitsProvider(t *testing.T) {
	harness := newExecutionHarness(t)
	resume := frozenConfirmationRequest(t, harness, ConfirmationDecisionConfirmed)
	first, err := harness.service.InvokeConnector(context.Background(), resume)
	if err != nil || first.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("first InvokeConnector() = %#v, %v", first, err)
	}
	if _, err = harness.service.InvokeConnector(context.Background(), resume); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("duplicate InvokeConnector() error = %v, want conflict", err)
	}
	if harness.provider.calls != 1 || len(harness.audit.outcomes) != 1 {
		t.Fatalf("duplicate provider/outcomes = %d/%d", harness.provider.calls, len(harness.audit.outcomes))
	}
}

func TestServiceSubmittedLeaseMatchesProviderContextDeadline(t *testing.T) {
	harness := newExecutionHarness(t)
	harness.service.providerTimeout = 45 * time.Second
	resume := frozenConfirmationRequest(t, harness, ConfirmationDecisionConfirmed)
	result, err := harness.service.InvokeConnector(context.Background(), resume)
	if err != nil || result.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("InvokeConnector() = %#v, %v", result, err)
	}
	if len(harness.audit.decisions) != 2 {
		t.Fatalf("confirmation decisions = %#v", harness.audit.decisions)
	}
	dispatch := harness.audit.decisions[1]
	leaseText, ok := dispatch.Details["execution_lease_expires_at"].(string)
	leaseExpiresAt, parseErr := time.Parse(time.RFC3339Nano, leaseText)
	want := harness.service.now().UTC().Add(harness.service.providerTimeout)
	if !ok || parseErr != nil || !leaseExpiresAt.Equal(want) ||
		!harness.provider.hasDeadline || !harness.provider.deadline.Equal(want) {
		t.Fatalf("lease/deadline = %q (%v), %v/%v; want %v",
			leaseText, parseErr, harness.provider.deadline, harness.provider.hasDeadline, want)
	}
}

func frozenConfirmationRequest(
	t *testing.T,
	harness *executionHarness,
	decision ConfirmationDecision,
) PinnedExecution {
	t.Helper()
	harness.risk.classification = RiskHigh
	pending, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil || pending.State != ExecutionStateRequiresConfirmation || pending.Confirmation == nil ||
		pending.Confirmation.AcceptedOperation == nil {
		t.Fatalf("pending confirmation = %#v, %v", pending, err)
	}
	resume := harness.pinned
	accepted := *pending.Confirmation.AcceptedOperation
	resume.Request.AcceptedOperation = &accepted
	resume.Request.Confirmation = &RuntimeConfirmationCommand{
		RouteReceipt: harness.pinned.Receipt,
		Decision:     decision,
	}
	return resume
}

type executionHarness struct {
	service     *Service
	pinned      PinnedExecution
	events      *eventLog
	bindings    *bindingReaderSpy
	connections *connectionReaderSpy
	target      *targetAuthorizerSpy
	risk        *riskResolverSpy
	credentials *credentialResolverSpy
	audit       *auditRecorderSpy
	provider    *providerInvokerSpy
}

func newExecutionHarness(t *testing.T) *executionHarness {
	t.Helper()
	return newExecutionHarnessForRequest(t, validExecutionRequest())
}

func newExecutionHarnessForRequest(t *testing.T, request ExecutionRequest) *executionHarness {
	t.Helper()
	now := time.Date(2026, 9, 1, 13, 0, 0, 0, time.UTC)
	definition := validDefinition()
	binding := core.TargetBinding{
		Constraint: core.OwnerConstraint(request.OwnerAccountID, []core.Identifier{request.OperationID}),
		ID:         request.BindingID, OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ConnectionID: request.ConnectionID, ConnectorID: request.ConnectorID, Target: request.Target,
		Status: core.BindingStatusActive, Revision: 1,
	}
	connection := core.PersonalConnection{
		Constraint: core.OwnerConstraint(request.OwnerAccountID, []core.Identifier{request.OperationID}),
		ID:         request.ConnectionID, OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ConnectorID: request.ConnectorID, ProviderID: core.Identifier(definition.ProviderID),
		DriverID: core.Identifier(definition.DriverID), AuthMethodID: core.Identifier(definition.AuthMethodID),
		Name: "GitHub account", EncryptedCredentials: "encrypted-credential-envelope", CredentialVersion: 1,
		State: core.ReadinessReady, UsableOperationIDs: []core.Identifier{request.OperationID},
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
		risk:        &riskResolverSpy{classification: RiskTrustedLow, events: events},
		credentials: &credentialResolverSpy{events: events},
		audit:       &auditRecorderSpy{events: events},
		provider: &providerInvokerSpy{
			events: events,
			result: ProviderInvocationResult{Value: "provider-value", Status: core.OutcomeStatusSuccessful, ProviderRequestID: "provider-request-1"},
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

type eventLog struct{ values []string }

func (events *eventLog) add(value string) { events.values = append(events.values, value) }

type definitionCatalogStub struct{ definition catalog.Definition }

func (stub *definitionCatalogStub) Get(connectorID string) (catalog.Definition, bool) {
	if connectorID != stub.definition.ID {
		return catalog.Definition{}, false
	}
	return stub.definition, true
}

type definitionVersionPinnerStub struct {
	definitionCatalogStub
	pinned   catalog.PinnedOperation
	err      error
	pinCalls int
}

func (stub *definitionVersionPinnerStub) Pin(connectorID, operationID string) (catalog.PinnedOperation, error) {
	stub.pinCalls++
	if connectorID != stub.pinned.ConnectorID || operationID != stub.pinned.Operation.ID {
		return catalog.PinnedOperation{}, errors.New("unexpected pin identity")
	}
	return stub.pinned, stub.err
}

type bindingReaderSpy struct {
	binding core.TargetBinding
	err     error
	calls   int
	events  *eventLog
}

func (reader *bindingReaderSpy) GetBinding(_ context.Context, organizationID, ownerID, bindingID uuid.UUID) (core.TargetBinding, error) {
	reader.calls++
	reader.events.add("binding")
	if organizationID != reader.binding.OrganizationID || ownerID != reader.binding.OwnerAccountID || bindingID != reader.binding.ID {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeNotFound, false, nil)
	}
	return reader.binding, reader.err
}

type connectionReaderSpy struct {
	connection core.PersonalConnection
	err        error
	calls      int
	events     *eventLog
}

func (reader *connectionReaderSpy) GetConnection(_ context.Context, organizationID, ownerID, connectionID uuid.UUID) (core.PersonalConnection, error) {
	reader.calls++
	reader.events.add("connection")
	if organizationID != reader.connection.OrganizationID || ownerID != reader.connection.OwnerAccountID || connectionID != reader.connection.ID {
		return core.PersonalConnection{}, core.NewError(core.ErrorCodeNotFound, false, nil)
	}
	return reader.connection, reader.err
}

type targetAuthorizerSpy struct {
	err    error
	calls  int
	events *eventLog
}

func (authorizer *targetAuthorizerSpy) AuthorizeTarget(_ context.Context, request TargetAuthorizationRequest) error {
	authorizer.calls++
	authorizer.events.add("target_authority")
	if request.ActingPrincipalID != request.OwnerAccountID || request.Binding.OwnerAccountID != request.OwnerAccountID ||
		request.Connection.OwnerAccountID != request.OwnerAccountID {
		return core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return authorizer.err
}

type riskResolverSpy struct {
	classification RiskClassification
	err            error
	calls          int
	events         *eventLog
}

func (resolver *riskResolverSpy) ResolveRisk(_ context.Context, _ RiskRequest) (RiskClassification, error) {
	resolver.calls++
	resolver.events.add("risk")
	return resolver.classification, resolver.err
}

type credentialResolverSpy struct {
	calls       int
	returned    map[string]string
	secretValue string
	err         error
	events      *eventLog
}

func (resolver *credentialResolverSpy) ResolveCredentials(_ context.Context, request CredentialResolutionRequest) (map[string]string, error) {
	resolver.calls++
	resolver.events.add("credentials")
	if request.EncryptedCredentials == "" {
		return nil, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	secretValue := resolver.secretValue
	if secretValue == "" {
		secretValue = executionSecretSentinel
	}
	resolver.returned = map[string]string{"token": secretValue}
	return resolver.returned, resolver.err
}

type auditRecorderSpy struct {
	decisions    []core.LifecycleDecision
	outcomes     []core.OperationOutcome
	decisionErr  error
	outcomeErr   error
	convergeErr  error
	finalized    map[string]struct{}
	outcomeCalls int
	events       *eventLog
}

func (recorder *auditRecorderSpy) CreateDecision(_ context.Context, decision core.LifecycleDecision) error {
	recorder.events.add("decision")
	if recorder.decisionErr != nil {
		return recorder.decisionErr
	}
	recorder.decisions = append(recorder.decisions, decision)
	return nil
}

func (recorder *auditRecorderSpy) FinalizeConfirmation(
	_ context.Context,
	finalization core.LifecycleDecision,
	operationDecision core.LifecycleDecision,
) error {
	recorder.events.add("confirmation")
	if recorder.decisionErr != nil {
		return recorder.decisionErr
	}
	if recorder.finalized == nil {
		recorder.finalized = map[string]struct{}{}
	}
	key := finalization.OrganizationID.String() + "/" + finalization.ReceiptID
	if _, exists := recorder.finalized[key]; exists {
		return core.NewError(core.ErrorCodeConflict, false, nil)
	}
	recorder.finalized[key] = struct{}{}
	recorder.decisions = append(recorder.decisions, finalization, operationDecision)
	return nil
}

func (recorder *auditRecorderSpy) RecordOutcome(_ context.Context, outcome core.OperationOutcome) error {
	recorder.outcomeCalls++
	recorder.events.add("outcome")
	if recorder.outcomeErr != nil {
		return recorder.outcomeErr
	}
	recorder.outcomes = append(recorder.outcomes, outcome)
	return nil
}

func (recorder *auditRecorderSpy) ConvergeOutcome(
	_ context.Context,
	unknown core.OperationOutcome,
) (core.OperationOutcome, error) {
	if recorder.convergeErr != nil {
		return core.OperationOutcome{}, recorder.convergeErr
	}
	for _, outcome := range recorder.outcomes {
		if outcome.DecisionID == unknown.DecisionID ||
			outcome.LogicalOperationID == unknown.LogicalOperationID {
			return outcome, nil
		}
	}
	recorder.outcomes = append(recorder.outcomes, unknown)
	return unknown, nil
}

type providerInvokerSpy struct {
	calls         int
	sawCredential bool
	result        ProviderInvocationResult
	err           error
	panicValue    any
	deadline      time.Time
	hasDeadline   bool
	events        *eventLog
}

func (invoker *providerInvokerSpy) Invoke(ctx context.Context, invocation ProviderInvocation) (ProviderInvocationResult, error) {
	invoker.calls++
	invoker.events.add("provider")
	invoker.sawCredential = invocation.Credentials["token"] != ""
	invoker.deadline, invoker.hasDeadline = ctx.Deadline()
	if invoker.panicValue != nil {
		panic(invoker.panicValue)
	}
	return invoker.result, invoker.err
}

func validDefinition() catalog.Definition {
	return catalog.Definition{
		ID: "github", PluginID: "github", PluginVersion: "1.0.0",
		SourceDialect: catalog.SourceDialectAgentPluginsV1, SourceSchemaVersion: catalog.SourceSchemaVersion1,
		ProviderID: "github", DriverID: "github_api", AuthMethodID: "personal_access_token",
		Name: "GitHub", Description: "GitHub connector", Classification: catalog.ClassificationConnector,
		CapabilityTypes: []catalog.CapabilityType{catalog.CapabilityTypeAction},
		SetupMethod:     catalog.SetupMethodKey, Ownership: catalog.OwnershipPersonal,
		Surfaces:  catalog.FirstReleaseSurfaces(),
		KeyFields: []catalog.KeyField{{Name: "token", Label: "Token", Required: true, Secret: true}},
		Operations: []catalog.Operation{
			{ID: "github.create_issue", ToolName: "github.create_issue", Name: "Create issue"},
			{ID: "github.list_issues", ToolName: "github.list_issues", Name: "List issues"},
		},
	}
}

func mustAcceptedOperation(
	t *testing.T,
	definition catalog.Definition,
	operation catalog.Operation,
) catalog.PinnedOperation {
	t.Helper()
	digest, err := catalog.DefinitionContractDigest(definition)
	if err != nil {
		t.Fatalf("definition contract digest: %v", err)
	}
	return catalog.PinnedOperation{
		ConnectorID: definition.ID, PluginID: definition.PluginID,
		PluginVersion: definition.PluginVersion, PluginDigest: definition.PluginDigest,
		ContractDigest: digest, Definition: definition, Operation: operation,
	}
}
