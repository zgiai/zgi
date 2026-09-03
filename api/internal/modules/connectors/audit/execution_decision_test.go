package audit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
)

const executionDecisionSecretSentinel = "TEST-CONNECTOR-RISK-DECISION-SECRET"

func TestExecutionRiskDefaultsSubmitOnlyEligibleLowOrExplicitlyConfirmedOperations(t *testing.T) {
	testCases := []struct {
		name          string
		risk          execution.RiskClassification
		interactive   bool
		confirmation  *execution.RuntimeConfirmationCommand
		wantState     execution.ExecutionState
		wantDecision  core.DecisionStatus
		wantDecisions int
		wantProvider  int
		wantOutcome   int
		wantErrorCode core.ErrorCode
		wantPrompt    bool
	}{
		{name: "interactive trusted low", risk: execution.RiskTrustedLow, interactive: true, wantState: execution.ExecutionStateCompleted, wantDecision: core.DecisionStatusSubmitted, wantDecisions: 1, wantProvider: 1, wantOutcome: 1},
		{name: "unattended trusted low", risk: execution.RiskTrustedLow, interactive: false, wantState: execution.ExecutionStateCompleted, wantDecision: core.DecisionStatusSubmitted, wantDecisions: 1, wantProvider: 1, wantOutcome: 1},
		{
			name: "trusted low rejects injected confirmation", risk: execution.RiskTrustedLow, interactive: true,
			confirmation:  confirmationCommand(execution.ConfirmationDecisionConfirmed),
			wantErrorCode: core.ErrorCodeInvalidInput,
			wantDecision:  core.DecisionStatusDenied, wantDecisions: 1,
		},
		{
			name: "trusted low rejects injected cancellation", risk: execution.RiskTrustedLow, interactive: true,
			confirmation:  confirmationCommand(execution.ConfirmationDecisionCancelled),
			wantErrorCode: core.ErrorCodeInvalidInput,
			wantDecision:  core.DecisionStatusDenied, wantDecisions: 1,
		},
		{name: "interactive high prompts", risk: execution.RiskHigh, interactive: true, wantState: execution.ExecutionStateRequiresConfirmation, wantPrompt: true},
		{name: "interactive unclassified prompts", risk: execution.RiskUnclassified, interactive: true, wantState: execution.ExecutionStateRequiresConfirmation, wantPrompt: true},
		{
			name: "interactive high confirms", risk: execution.RiskHigh, interactive: true,
			confirmation: confirmationCommand(execution.ConfirmationDecisionConfirmed),
			wantState:    execution.ExecutionStateCompleted, wantDecision: core.DecisionStatusSubmitted,
			wantDecisions: 2, wantProvider: 1, wantOutcome: 1,
		},
		{
			name: "interactive unclassified cancels", risk: execution.RiskUnclassified, interactive: true,
			confirmation: confirmationCommand(execution.ConfirmationDecisionCancelled),
			wantState:    execution.ExecutionStateCancelled, wantDecision: core.DecisionStatusCancelled, wantDecisions: 2,
		},
		{
			name: "unattended high blocks", risk: execution.RiskHigh, interactive: false,
			wantState: execution.ExecutionStateBlocked, wantDecision: core.DecisionStatusBlocked, wantDecisions: 1,
			wantErrorCode: core.ErrorCodeConfirmationRequired,
		},
		{
			name: "unattended unclassified blocks", risk: execution.RiskUnclassified, interactive: false,
			wantState: execution.ExecutionStateBlocked, wantDecision: core.DecisionStatusBlocked, wantDecisions: 1,
			wantErrorCode: core.ErrorCodeConfirmationRequired,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newRiskExecutionHarness(t, testCase.risk, testCase.interactive)
			if testCase.confirmation != nil {
				accepted := freezeRiskConfirmationContract(t, harness)
				harness.pinned.Request.AcceptedOperation = accepted
				command := *testCase.confirmation
				command.RouteReceipt = harness.pinned.Receipt
				harness.pinned.Request.Confirmation = &command
			}

			result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
			if testCase.wantErrorCode == "" {
				if err != nil {
					t.Fatalf("InvokeConnector() error = %v", err)
				}
			} else if !core.IsErrorCode(err, testCase.wantErrorCode) {
				t.Fatalf("InvokeConnector() error = %v, want %s", err, testCase.wantErrorCode)
			}
			if result.State != testCase.wantState {
				t.Fatalf("InvokeConnector().State = %q, want %q", result.State, testCase.wantState)
			}
			if harness.provider.calls != testCase.wantProvider || len(harness.recorder.outcomes) != testCase.wantOutcome {
				t.Fatalf("provider/outcomes = %d/%d, want %d/%d", harness.provider.calls, len(harness.recorder.outcomes), testCase.wantProvider, testCase.wantOutcome)
			}
			if len(harness.recorder.decisions) != testCase.wantDecisions {
				t.Fatalf("decisions = %#v, want %d", harness.recorder.decisions, testCase.wantDecisions)
			}
			if testCase.wantDecision != "" && harness.recorder.decisions[len(harness.recorder.decisions)-1].Status != testCase.wantDecision {
				t.Fatalf("decisions = %#v, want final %q", harness.recorder.decisions, testCase.wantDecision)
			}
			if testCase.wantPrompt {
				assertRuntimeConfirmationPreview(t, harness, result)
			} else if result.Confirmation != nil {
				t.Fatalf("unexpected confirmation preview = %#v", result.Confirmation)
			}
			if harness.provider.calls == 0 && len(harness.recorder.outcomes) != 0 {
				t.Fatalf("non-submitted operation created outcome = %#v", harness.recorder.outcomes)
			}
			if strings.Contains(fmt.Sprint(result), executionDecisionSecretSentinel) ||
				strings.Contains(fmt.Sprint(harness.recorder), executionDecisionSecretSentinel) {
				t.Fatal("secret sentinel escaped into execution result or audit")
			}
		})
	}
}

func TestSubmittedExecutionAlwaysRecordsExactlyOneTerminalOutcome(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		provider  execution.ProviderInvocationResult
		want      core.OutcomeStatus
		wantError core.ErrorCode
	}{
		{name: "successful", provider: execution.ProviderInvocationResult{Value: "ok", Status: core.OutcomeStatusSuccessful}, want: core.OutcomeStatusSuccessful},
		{name: "failed", provider: execution.ProviderInvocationResult{Status: core.OutcomeStatusFailed, FailureCode: core.ErrorCodeProviderUnavailable, DiagnosticCode: "provider_unavailable"}, want: core.OutcomeStatusFailed, wantError: core.ErrorCodeProviderUnavailable},
		{name: "unknown", provider: execution.ProviderInvocationResult{Status: core.OutcomeStatusUnknown}, want: core.OutcomeStatusUnknown, wantError: core.ErrorCodeOutcomeUnknown},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newRiskExecutionHarness(t, execution.RiskTrustedLow, true)
			harness.provider.result = testCase.provider

			result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
			if testCase.wantError == "" {
				if err != nil {
					t.Fatalf("InvokeConnector() error = %v", err)
				}
			} else if !core.IsErrorCode(err, testCase.wantError) {
				t.Fatalf("InvokeConnector() error = %v, want %s", err, testCase.wantError)
			}
			if harness.provider.calls != 1 || len(harness.recorder.decisions) != 1 ||
				len(harness.recorder.outcomes) != 1 || result.Outcome.Status != testCase.want {
				t.Fatalf("provider/decisions/outcomes/result = %d/%d/%d/%#v", harness.provider.calls, len(harness.recorder.decisions), len(harness.recorder.outcomes), result)
			}
			if harness.recorder.decisions[0].Status != core.DecisionStatusSubmitted || harness.recorder.outcomes[0].Status != testCase.want {
				t.Fatalf("audit = %#v / %#v", harness.recorder.decisions, harness.recorder.outcomes)
			}
		})
	}
}

func TestRuntimeConfirmationFinalizationIsAtomicAcrossRaces(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		commands  []execution.ConfirmationDecision
		wantFinal *bool
	}{
		{name: "double confirm", commands: []execution.ConfirmationDecision{execution.ConfirmationDecisionConfirmed, execution.ConfirmationDecisionConfirmed}, wantFinal: boolPointer(true)},
		{name: "double cancel", commands: []execution.ConfirmationDecision{execution.ConfirmationDecisionCancelled, execution.ConfirmationDecisionCancelled}, wantFinal: boolPointer(false)},
		{name: "confirm versus cancel", commands: []execution.ConfirmationDecision{execution.ConfirmationDecisionConfirmed, execution.ConfirmationDecisionCancelled}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			harness := newRiskExecutionHarness(t, execution.RiskHigh, true)
			harness.pinned.Request.AcceptedOperation = freezeRiskConfirmationContract(t, harness)
			errorsByCommand := invokeConfirmationCommandsConcurrently(harness, testCase.commands)
			succeeded, conflicted := 0, 0
			for _, err := range errorsByCommand {
				switch {
				case err == nil:
					succeeded++
				case core.IsErrorCode(err, core.ErrorCodeConflict):
					conflicted++
				default:
					t.Fatalf("concurrent confirmation error = %v", err)
				}
			}
			if succeeded != 1 || conflicted != len(testCase.commands)-1 {
				t.Fatalf("success/conflict = %d/%d", succeeded, conflicted)
			}

			harness.recorder.mu.Lock()
			decisions := append([]core.LifecycleDecision(nil), harness.recorder.decisions...)
			outcomes := append([]core.OperationOutcome(nil), harness.recorder.outcomes...)
			harness.recorder.mu.Unlock()
			var finalization *core.LifecycleDecision
			for index := range decisions {
				if decisions[index].Decision == "confirmation_finalization" {
					if finalization != nil {
						t.Fatalf("multiple finalizations = %#v", decisions)
					}
					copyValue := decisions[index]
					finalization = &copyValue
				}
			}
			if finalization == nil || len(decisions) != 2 {
				t.Fatalf("decisions = %#v", decisions)
			}
			confirmed, ok := finalization.Details["confirmation"].(bool)
			if !ok || (testCase.wantFinal != nil && confirmed != *testCase.wantFinal) {
				t.Fatalf("finalization = %#v", finalization)
			}
			harness.provider.mu.Lock()
			providerCalls := harness.provider.calls
			harness.provider.mu.Unlock()
			if confirmed {
				if providerCalls != 1 || len(outcomes) != 1 || countDecisionStatus(decisions, core.DecisionStatusSubmitted) != 1 {
					t.Fatalf("confirmed provider/outcomes/decisions = %d/%d/%#v", providerCalls, len(outcomes), decisions)
				}
			} else if providerCalls != 0 || len(outcomes) != 0 || countDecisionStatus(decisions, core.DecisionStatusCancelled) != 1 {
				t.Fatalf("cancelled provider/outcomes/decisions = %d/%d/%#v", providerCalls, len(outcomes), decisions)
			}
		})
	}
}

func TestRuntimeConfirmationLoserDoesNotConvergeUnknownWhileProviderLeaseIsActive(t *testing.T) {
	harness := newRiskExecutionHarness(t, execution.RiskHigh, true)
	harness.pinned.Request.AcceptedOperation = freezeRiskConfirmationContract(t, harness)
	harness.pinned.Request.Confirmation = confirmationCommand(execution.ConfirmationDecisionConfirmed)
	harness.provider.started = make(chan struct{})
	harness.provider.release = make(chan struct{})

	type invocationResult struct {
		result execution.ExecutionResult
		err    error
	}
	winnerDone := make(chan invocationResult, 1)
	go func() {
		result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
		winnerDone <- invocationResult{result: result, err: err}
	}()
	<-harness.provider.started

	if _, err := harness.service.InvokeConnector(context.Background(), harness.pinned); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("concurrent loser error = %v, want conflict", err)
	}
	recoverer, err := runtimeconfirmation.NewAuditTerminalRecoverer(harness.recorder)
	if err != nil {
		t.Fatal(err)
	}
	recoveryRequest := runtimeconfirmation.TerminalRecoveryRequest{
		OrganizationID:     harness.pinned.Request.OrganizationID,
		OwnerAccountID:     harness.pinned.Request.OwnerAccountID,
		ActingPrincipalID:  harness.pinned.Request.ActingPrincipalID,
		ConnectionID:       harness.pinned.Request.ConnectionID,
		ConnectorID:        harness.pinned.Request.ConnectorID,
		Target:             harness.pinned.Request.Target,
		OperationID:        harness.pinned.Request.OperationID,
		LogicalOperationID: harness.pinned.Request.LogicalOperationID,
		CorrelationID:      harness.pinned.Request.CorrelationID,
		RouteReceipt:       harness.pinned.Receipt,
		Decision:           execution.ConfirmationDecisionConfirmed,
	}
	if _, err = recoverer.RecoverTerminal(context.Background(), recoveryRequest); !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("active-lease recovery error = %v, want conflict", err)
	}
	harness.recorder.mu.Lock()
	outcomesWhileBlocked := len(harness.recorder.outcomes)
	harness.recorder.mu.Unlock()
	if outcomesWhileBlocked != 0 {
		t.Fatalf("active provider lease produced premature outcome: %#v", harness.recorder.outcomes)
	}

	close(harness.provider.release)
	winner := <-winnerDone
	if winner.err != nil || winner.result.Outcome.Status != core.OutcomeStatusSuccessful {
		t.Fatalf("winner = %#v, %v", winner.result, winner.err)
	}
	terminal, err := recoverer.RecoverTerminal(context.Background(), recoveryRequest)
	if err != nil || terminal.State != execution.ConfirmationContinuationCompleted ||
		terminal.DecisionID != winner.result.DecisionID || terminal.OutcomeID != winner.result.Outcome.ID {
		t.Fatalf("post-provider recovery = %#v, %v", terminal, err)
	}
	if harness.provider.callCount() != 1 {
		t.Fatalf("provider calls = %d, want 1", harness.provider.callCount())
	}
}

func invokeConfirmationCommandsConcurrently(harness *riskExecutionHarness, commands []execution.ConfirmationDecision) []error {
	start := make(chan struct{})
	errorsByCommand := make(chan error, len(commands))
	var wait sync.WaitGroup
	for _, decision := range commands {
		decision := decision
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			pinned := harness.pinned
			pinned.Request.Confirmation = confirmationCommand(decision)
			_, err := harness.service.InvokeConnector(context.Background(), pinned)
			errorsByCommand <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByCommand)
	result := make([]error, 0, len(commands))
	for err := range errorsByCommand {
		result = append(result, err)
	}
	return result
}

func countDecisionStatus(decisions []core.LifecycleDecision, status core.DecisionStatus) int {
	count := 0
	for _, decision := range decisions {
		if decision.Status == status {
			count++
		}
	}
	return count
}

func boolPointer(value bool) *bool { return &value }

func TestExecutionDecisionBuilderAllowsOnlyNonSubmittedNoOutcomeStates(t *testing.T) {
	harness := newRiskExecutionHarness(t, execution.RiskHigh, false)
	request := audit.ExecutionDecisionRequest{
		ID: uuid.New(), OrganizationID: harness.pinned.Request.OrganizationID,
		OwnerAccountID:    harness.pinned.Request.OwnerAccountID,
		ActingPrincipalID: harness.pinned.Request.ActingPrincipalID,
		ConnectorID:       harness.pinned.Request.ConnectorID, ConnectionID: harness.pinned.Request.ConnectionID,
		Target: harness.pinned.Request.Target, OperationID: harness.pinned.Request.OperationID,
		LogicalOperationID: harness.pinned.Request.LogicalOperationID,
		CorrelationID:      harness.pinned.Request.CorrelationID, ReceiptID: string(harness.pinned.Receipt),
		ExecutionPath: string(execution.PathConnectorNative), RiskClassification: string(execution.RiskHigh),
		Interactive: false, Confirmation: false, CreatedAt: time.Now().UTC(),
	}
	decision, err := audit.NewExecutionDecision(request, core.DecisionStatusBlocked)
	if err != nil {
		t.Fatalf("NewExecutionDecision() error = %v", err)
	}
	if decision.Decision != "confirmation" || decision.Status != core.DecisionStatusBlocked ||
		decision.Details["risk_classification"] != string(execution.RiskHigh) ||
		decision.Details["confirmation"] != false || decision.Details["interactive"] != false {
		t.Fatalf("NewExecutionDecision() = %#v", decision)
	}
	if _, err = audit.NewExecutionDecision(request, core.DecisionStatusSuccessful); !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatalf("terminal non-submission decision error = %v, want invalid input", err)
	}
	request.Interactive = true
	finalization, err := audit.NewConfirmationFinalizationDecision(request, true)
	if err != nil || finalization.Decision != "confirmation_finalization" ||
		finalization.Status != core.DecisionStatusSuccessful || finalization.Details["confirmation"] != true {
		t.Fatalf("NewConfirmationFinalizationDecision() = %#v, %v", finalization, err)
	}
}

func TestRuntimeConfirmationJSONContractIsMinimalAndSecretFree(t *testing.T) {
	harness := newRiskExecutionHarness(t, execution.RiskHigh, true)
	result, err := harness.service.InvokeConnector(context.Background(), harness.pinned)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{
		`"state":"requires_confirmation"`, `"route_receipt":"risk-route-receipt"`,
		`"connector_id":"github"`, `"connector_name":"GitHub"`,
		`"operation_id":"github.repository.delete"`, `"action_name":"Delete repository"`,
		`"target":{"type":"chat","id":"conversation-1"}`, `"external_effect":"delete"`,
		`"risk_classification":"high"`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("confirmation JSON missing %s: %s", required, text)
		}
	}
	for _, forbidden := range []string{
		executionDecisionSecretSentinel, `"outcome"`, `"decision_id"`, `"value"`,
		`"input"`, `"credential"`, `"policy"`, `"approval"`,
	} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("confirmation JSON contains %q: %s", forbidden, text)
		}
	}
}

func confirmationCommand(decision execution.ConfirmationDecision) *execution.RuntimeConfirmationCommand {
	return &execution.RuntimeConfirmationCommand{RouteReceipt: "risk-route-receipt", Decision: decision}
}

func freezeRiskConfirmationContract(
	t *testing.T,
	harness *riskExecutionHarness,
) *catalog.PinnedOperation {
	t.Helper()
	classification := harness.risk.classification
	harness.risk.classification = execution.RiskHigh
	promptRequest := harness.pinned
	promptRequest.Request.Confirmation = nil
	promptRequest.Request.AcceptedOperation = nil
	result, err := harness.service.InvokeConnector(context.Background(), promptRequest)
	harness.risk.classification = classification
	if err != nil || result.State != execution.ExecutionStateRequiresConfirmation ||
		result.Confirmation == nil || result.Confirmation.AcceptedOperation == nil {
		t.Fatalf("freeze runtime confirmation = (%#v, %v)", result, err)
	}
	accepted := *result.Confirmation.AcceptedOperation
	return &accepted
}

func assertRuntimeConfirmationPreview(t *testing.T, harness *riskExecutionHarness, result execution.ExecutionResult) {
	t.Helper()
	confirmation := result.Confirmation
	if confirmation == nil || confirmation.RouteReceipt != harness.pinned.Receipt ||
		confirmation.ConnectorID != harness.pinned.Request.ConnectorID || confirmation.ConnectorName != "GitHub" ||
		confirmation.OperationID != harness.pinned.Request.OperationID || confirmation.ActionName != "Delete repository" ||
		confirmation.Target != harness.pinned.Request.Target || confirmation.Risk != harness.risk.classification ||
		confirmation.Consequence.ExternalEffect != string(catalog.OperationEffectDelete) ||
		!confirmation.Consequence.DataEgress || confirmation.Consequence.ExternalDestination != "api.github.com" {
		t.Fatalf("runtime confirmation = %#v", confirmation)
	}
}

type riskExecutionHarness struct {
	service  *execution.Service
	pinned   execution.PinnedExecution
	risk     *riskDecisionResolverStub
	recorder *riskAuditRecorderSpy
	provider *riskProviderSpy
}

func newRiskExecutionHarness(t *testing.T, risk execution.RiskClassification, interactive bool) *riskExecutionHarness {
	t.Helper()
	ownerID := uuid.New()
	request := execution.ExecutionRequest{
		OrganizationID: uuid.New(), OwnerAccountID: ownerID, ActingPrincipalID: ownerID,
		BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier("github"),
		Target:      core.TargetRef{Type: core.TargetTypeChat, ID: "conversation-1"},
		OperationID: core.Identifier("github.repository.delete"), LogicalOperationID: uuid.NewString(),
		CorrelationID: uuid.NewString(), Interactive: interactive, Input: map[string]any{"private": executionDecisionSecretSentinel},
	}
	now := time.Date(2026, 9, 1, 14, 0, 0, 0, time.UTC)
	definition := catalog.Definition{
		ID: "github", PluginID: "zgi.github", PluginVersion: "1.0.0",
		SourceDialect: catalog.SourceDialectAgentPluginsV1, SourceSchemaVersion: catalog.SourceSchemaVersion1,
		ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "GitHub", Description: "GitHub connector", Classification: catalog.ClassificationConnector,
		CapabilityTypes: []catalog.CapabilityType{catalog.CapabilityTypeAction}, SetupMethod: catalog.SetupMethodKey,
		Ownership: catalog.OwnershipPersonal, Surfaces: catalog.FirstReleaseSurfaces(),
		KeyFields: []catalog.KeyField{{Name: "token", Label: "Token", Required: true, Secret: true}},
		Operations: []catalog.Operation{{
			ID: request.OperationID.String(), ToolName: "delete_github_repository", Name: "Delete repository",
			Effect: catalog.OperationEffectDelete, DataEgress: true, ExternalDestination: "api.github.com",
			RiskHint:    catalog.OperationRiskHintLow,
			InputSchema: map[string]any{"type": "object"}, OutputSchema: map[string]any{"type": "object"},
		}},
	}
	binding := core.TargetBinding{Constraint: core.OwnerConstraint(ownerID, []core.Identifier{request.OperationID}),
		ID: request.BindingID, OrganizationID: request.OrganizationID, OwnerAccountID: ownerID,
		ConnectionID: request.ConnectionID, ConnectorID: request.ConnectorID, Target: request.Target,
		Status: core.BindingStatusActive, Revision: 1,
	}
	connection := core.PersonalConnection{Constraint: core.OwnerConstraint(ownerID, []core.Identifier{request.OperationID}),
		ID: request.ConnectionID, OrganizationID: request.OrganizationID, OwnerAccountID: ownerID,
		ConnectorID: request.ConnectorID, ProviderID: core.Identifier(definition.ProviderID),
		DriverID: core.Identifier(definition.DriverID), AuthMethodID: core.Identifier(definition.AuthMethodID),
		Name: "GitHub account", EncryptedCredentials: "encrypted-envelope", CredentialVersion: 1,
		State: core.ReadinessReady, UsableOperationIDs: []core.Identifier{request.OperationID},
		UnavailableOperations: map[core.Identifier]string{}, Revision: 1,
		LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	riskResolver := &riskDecisionResolverStub{classification: risk}
	recorder := &riskAuditRecorderSpy{}
	provider := &riskProviderSpy{result: execution.ProviderInvocationResult{Value: "ok", Status: core.OutcomeStatusSuccessful}}
	service, err := execution.NewService(
		&riskDefinitionCatalogStub{definition: definition}, &riskBindingReaderStub{binding: binding},
		&riskConnectionReaderStub{connection: connection}, riskTargetAuthorizerStub{}, riskResolver,
		riskCredentialResolverStub{}, recorder, provider,
	)
	if err != nil {
		t.Fatal(err)
	}
	return &riskExecutionHarness{
		service: service,
		pinned:  execution.PinnedExecution{Request: request, Path: execution.PathConnectorNative, Receipt: "risk-route-receipt"},
		risk:    riskResolver, recorder: recorder, provider: provider,
	}
}

type riskDefinitionCatalogStub struct{ definition catalog.Definition }

func (stub *riskDefinitionCatalogStub) Get(connectorID string) (catalog.Definition, bool) {
	return stub.definition, connectorID == stub.definition.ID
}

type riskBindingReaderStub struct{ binding core.TargetBinding }

func (stub *riskBindingReaderStub) GetBinding(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.TargetBinding, error) {
	return stub.binding, nil
}

type riskConnectionReaderStub struct{ connection core.PersonalConnection }

func (stub *riskConnectionReaderStub) GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.PersonalConnection, error) {
	return stub.connection, nil
}

type riskTargetAuthorizerStub struct{}

func (riskTargetAuthorizerStub) AuthorizeTarget(context.Context, execution.TargetAuthorizationRequest) error {
	return nil
}

type riskDecisionResolverStub struct{ classification execution.RiskClassification }

func (stub *riskDecisionResolverStub) ResolveRisk(context.Context, execution.RiskRequest) (execution.RiskClassification, error) {
	return stub.classification, nil
}

type riskCredentialResolverStub struct{}

func (riskCredentialResolverStub) ResolveCredentials(context.Context, execution.CredentialResolutionRequest) (map[string]string, error) {
	return map[string]string{"token": executionDecisionSecretSentinel}, nil
}

type riskAuditRecorderSpy struct {
	mu        sync.Mutex
	decisions []core.LifecycleDecision
	outcomes  []core.OperationOutcome
	finalized map[string]struct{}
}

func (spy *riskAuditRecorderSpy) CreateDecision(_ context.Context, decision core.LifecycleDecision) error {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	spy.decisions = append(spy.decisions, decision)
	return nil
}

func (spy *riskAuditRecorderSpy) FinalizeConfirmation(
	_ context.Context,
	decision core.LifecycleDecision,
	operationDecision core.LifecycleDecision,
) error {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if spy.finalized == nil {
		spy.finalized = map[string]struct{}{}
	}
	key := decision.OrganizationID.String() + "/" + decision.ReceiptID
	if _, exists := spy.finalized[key]; exists {
		return core.NewError(core.ErrorCodeConflict, false, nil)
	}
	spy.finalized[key] = struct{}{}
	spy.decisions = append(spy.decisions, decision, operationDecision)
	return nil
}

func (spy *riskAuditRecorderSpy) RecordOutcome(_ context.Context, outcome core.OperationOutcome) error {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	spy.outcomes = append(spy.outcomes, outcome)
	return nil
}

func (spy *riskAuditRecorderSpy) ConvergeOutcome(
	_ context.Context,
	unknown core.OperationOutcome,
) (core.OperationOutcome, error) {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	for _, outcome := range spy.outcomes {
		if outcome.DecisionID == unknown.DecisionID ||
			outcome.LogicalOperationID == unknown.LogicalOperationID {
			return outcome, nil
		}
	}
	spy.outcomes = append(spy.outcomes, unknown)
	return unknown, nil
}

func (spy *riskAuditRecorderSpy) ListDecisions(
	_ context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	_ audit.DecisionListFilter,
) ([]core.LifecycleDecision, error) {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	decisions := make([]core.LifecycleDecision, 0, len(spy.decisions))
	for _, decision := range spy.decisions {
		if decision.OrganizationID == organizationID && decision.OwnerAccountID == ownerAccountID {
			decisions = append(decisions, decision)
		}
	}
	return decisions, nil
}

func (spy *riskAuditRecorderSpy) GetOutcomeByDecision(
	_ context.Context,
	organizationID uuid.UUID,
	ownerAccountID uuid.UUID,
	decisionID uuid.UUID,
) (core.OperationOutcome, error) {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	for _, outcome := range spy.outcomes {
		if outcome.OrganizationID == organizationID && outcome.OwnerAccountID == ownerAccountID &&
			outcome.DecisionID == decisionID {
			return outcome, nil
		}
	}
	return core.OperationOutcome{}, core.NewError(core.ErrorCodeNotFound, false, nil)
}

type riskProviderSpy struct {
	mu          sync.Mutex
	calls       int
	result      execution.ProviderInvocationResult
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
}

func (spy *riskProviderSpy) Invoke(_ context.Context, invocation execution.ProviderInvocation) (execution.ProviderInvocationResult, error) {
	spy.mu.Lock()
	spy.calls++
	if invocation.Credentials["token"] != executionDecisionSecretSentinel {
		spy.mu.Unlock()
		return execution.ProviderInvocationResult{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	result := spy.result
	started, release := spy.started, spy.release
	spy.mu.Unlock()
	if started != nil {
		spy.startedOnce.Do(func() { close(started) })
	}
	if release != nil {
		<-release
	}
	return result, nil
}

func (spy *riskProviderSpy) callCount() int {
	spy.mu.Lock()
	defer spy.mu.Unlock()
	return spy.calls
}
