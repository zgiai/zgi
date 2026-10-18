package execution

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

var diagnosticCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

const (
	defaultProviderInvocationTimeout = 2 * time.Minute
	maximumProviderInvocationTimeout = 30 * time.Minute
)

// DefinitionCatalog is the immutable operation contract required at dispatch.
type DefinitionCatalog interface {
	Get(connectorID string) (catalog.Definition, bool)
}

// DefinitionVersionPinner is implemented by the production plugin manager.
// It freezes both the accepted definition and operation contract under the
// same lifecycle lock used for activation, disablement, and removal.
type DefinitionVersionPinner interface {
	Pin(connectorID, operationID string) (catalog.PinnedOperation, error)
}

// BindingReader and ConnectionReader deliberately use organization+owner
// scoping. A globally supplied UUID is never sufficient for authorization.
type BindingReader interface {
	GetBinding(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.TargetBinding, error)
}

type ConnectionReader interface {
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.PersonalConnection, error)
}

// TargetAuthorizer verifies current target/run/binding eligibility for the
// owner Acting Principal. It must not inspect or resolve credentials.
type TargetAuthorizer interface {
	AuthorizeTarget(context.Context, TargetAuthorizationRequest) error
}

type TargetAuthorizationRequest struct {
	OrganizationID    uuid.UUID
	OwnerAccountID    uuid.UUID
	ActingPrincipalID uuid.UUID
	Binding           core.TargetBinding
	Connection        core.PersonalConnection
	OperationID       core.Identifier
	Interactive       bool
}

// RiskClassification is a trusted platform classification, never a member
// setting or an untrusted plugin declaration.
type RiskClassification string

const (
	RiskTrustedLow   RiskClassification = "trusted_low"
	RiskHigh         RiskClassification = "high"
	RiskUnclassified RiskClassification = "unclassified"
)

// RiskResolver supplies platform-owned execution risk. Trusted-low operations
// run directly; high and unclassified operations require interactive
// confirmation unless the server matches an explicit session grant. Member
// settings cannot manufacture a grant or alter the pinned operation.
type RiskResolver interface {
	ResolveRisk(context.Context, RiskRequest) (RiskClassification, error)
}

type RiskRequest struct {
	OrganizationID    uuid.UUID
	OwnerAccountID    uuid.UUID
	ActingPrincipalID uuid.UUID
	ConnectorID       core.Identifier
	ConnectionID      uuid.UUID
	Target            core.TargetRef
	OperationID       core.Identifier
	Interactive       bool
}

// CredentialResolver opens one encrypted Connector credential into a fresh,
// request-scoped map. Service destroys that map on every post-resolution exit.
type CredentialResolver interface {
	ResolveCredentials(context.Context, CredentialResolutionRequest) (map[string]string, error)
}

type CredentialResolutionRequest struct {
	OrganizationID       uuid.UUID
	ConnectionID         uuid.UUID
	ConnectorID          core.Identifier
	CredentialVersion    int
	EncryptedCredentials string `json:"-"`
}

// DecisionOutcomeRecorder matches the non-secret audit repository boundary.
// A submitted decision is written before provider invocation and exactly one
// terminal outcome is attempted after the single invocation.
type DecisionOutcomeRecorder interface {
	CreateDecision(context.Context, core.LifecycleDecision) error
	FinalizeConfirmation(context.Context, core.LifecycleDecision, core.LifecycleDecision) error
	RecordOutcome(context.Context, core.OperationOutcome) error
	ConvergeOutcome(context.Context, core.OperationOutcome) (core.OperationOutcome, error)
}

type ProviderInvocation struct {
	BindingID          uuid.UUID
	BindingRevision    int
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	ConnectionID       uuid.UUID
	ConnectorID        core.Identifier
	ProviderID         core.Identifier
	DriverID           core.Identifier
	AuthMethodID       core.Identifier
	CredentialVersion  int
	Target             core.TargetRef
	OperationID        core.Identifier
	LogicalOperationID string
	Credentials        map[string]string `json:"-"`
	Input              any               `json:"-"`
}

// ProviderInvocationResult carries a provider response and its safe terminal
// classification. A non-nil error without an explicit failed classification is
// treated as unknown because submission may have occurred.
type ProviderInvocationResult struct {
	Value             any
	Status            core.OutcomeStatus
	ProviderRequestID string
	DiagnosticCode    string
	FailureCode       core.ErrorCode
	Retryable         bool
}

// NativeProviderInvoker is a leaf provider adapter, not the legacy Executor.
type NativeProviderInvoker interface {
	Invoke(context.Context, ProviderInvocation) (ProviderInvocationResult, error)
}

type ExecutionResult struct {
	Evidence     *DispatchEvidence     `json:"-"`
	Display      *OperationDisplay     `json:"-"`
	State        ExecutionState        `json:"state"`
	DecisionID   uuid.UUID             `json:"decision_id,omitempty,omitzero"`
	Confirmation *RuntimeConfirmation  `json:"confirmation,omitempty"`
	Outcome      core.OperationOutcome `json:"outcome,omitempty,omitzero"`
	Value        any                   `json:"value,omitempty"`
}

// Service executes only a durably pinned Connector-native operation.
type Service struct {
	resultProjector  *apptransport.Projector
	catalog          DefinitionCatalog
	bindings         BindingReader
	connections      ConnectionReader
	targetAuthorizer TargetAuthorizer
	risk             RiskResolver
	credentials      CredentialResolver
	audit            DecisionOutcomeRecorder
	provider         NativeProviderInvoker
	now              func() time.Time
	newID            func() uuid.UUID
	providerTimeout  time.Duration
}

func NewService(
	definitionCatalog DefinitionCatalog,
	bindings BindingReader,
	connections ConnectionReader,
	targetAuthorizer TargetAuthorizer,
	risk RiskResolver,
	credentials CredentialResolver,
	auditRecorder DecisionOutcomeRecorder,
	provider NativeProviderInvoker,
	projectors ...*apptransport.Projector,
) (*Service, error) {
	if definitionCatalog == nil || bindings == nil || connections == nil || targetAuthorizer == nil ||
		risk == nil || credentials == nil || auditRecorder == nil || provider == nil {
		return nil, connectorError(core.ErrorCodeInternal, false)
	}
	var projector *apptransport.Projector
	if len(projectors) > 0 {
		projector = projectors[0]
	}
	return &Service{
		resultProjector: projector,
		catalog:         definitionCatalog, bindings: bindings, connections: connections,
		targetAuthorizer: targetAuthorizer, risk: risk, credentials: credentials,
		audit: auditRecorder, provider: provider,
		now: func() time.Time { return time.Now().UTC() }, newID: uuid.New,
		providerTimeout: defaultProviderInvocationTimeout,
	}, nil
}

func (service *Service) InvokeConnector(ctx context.Context, pinned PinnedExecution) (ExecutionResult, error) {
	progress := &dispatchProgress{}
	result, err := service.invokePinned(ctx, pinned, PathConnectorNative, progress)
	if pinned.Path != PathConnectorNative {
		return result, err
	}
	result, err = service.withPreSubmissionDenial(ctx, pinned, result, err, progress)
	result.Display = progress.display
	return service.presentResult(ctx, result), err
}

// withPreSubmissionDenial records a real, valid acting identity without reading
// the rejected connection. A durable submission, terminal outcome, or duplicate
// confirmation cannot be reclassified as a denial by this boundary.
func (service *Service) withPreSubmissionDenial(ctx context.Context, pinned PinnedExecution, result ExecutionResult, cause error, progress *dispatchProgress) (ExecutionResult, error) {
	if cause == nil || result.State != "" || result.Outcome.ID != uuid.Nil {
		return result, cause
	}
	if service.available() != nil || !validReceipt(pinned.Receipt) || validateExecutionRequest(pinned.Request) != nil {
		return UnresolvedResult(core.ErrorCodeOf(cause)), cause
	}
	if progress.submissionAttempted && !(pinned.freshClaim && progress.definitelyNotWritten) {
		return service.recoverAfterFailure(ctx, pinned, cause)
	}
	if progress.submissionAttempted {
		prior, _ := service.RecoverInvocation(ctx, pinned)
		if prior.State != ExecutionStateUnresolved || (prior.Evidence != nil && prior.Evidence.Disposition == DispatchInProgress) {
			return prior, cause
		}
	}
	decision, err := service.newExecutionDecision(pinned, RiskUnclassified, core.DecisionStatusDenied, false,
		service.newID(), service.now().UTC(), time.Time{})
	if err != nil {
		return UnresolvedResult(core.ErrorCodeInternal), cause
	}
	decision.Details["diagnostic_code"] = string(core.ErrorCodeOf(cause))
	if rejector, ok := service.audit.(executionRejector); ok {
		err = rejector.RejectExecution(ctx, decision)
		if err == nil {
			return rejectedResult(decision.ID, core.ErrorCodeOf(cause)), cause
		}
	} else if !core.IsErrorCode(cause, core.ErrorCodeConflict) {
		err = service.audit.CreateDecision(ctx, decision)
		if err == nil && pinned.freshClaim && pinned.Request.Confirmation == nil {
			return rejectedResult(decision.ID, core.ErrorCodeOf(cause)), cause
		}
	} else {
		err = connectorError(core.ErrorCodeConflict, false)
	}
	if pinned.freshClaim && !progress.submissionAttempted && pinned.Request.Confirmation == nil {
		return rejectedResult(uuid.Nil, core.ErrorCodeInternal), cause
	}
	return service.recoverAfterFailure(ctx, pinned, cause)
}

// invokePinned retains the Router's immutable native claim and receipt through
// authorization, confirmation and provider submission.
func (service *Service) invokePinned(ctx context.Context, pinned PinnedExecution, expected Path, progress *dispatchProgress) (ExecutionResult, error) {
	if err := service.available(); err != nil {
		return ExecutionResult{}, err
	}
	request := pinned.Request
	if pinned.Path != expected || !validReceipt(pinned.Receipt) {
		return ExecutionResult{}, connectorError(core.ErrorCodeInvalidInput, false)
	}
	if err := validateExecutionRequest(request); err != nil {
		return ExecutionResult{}, err
	}
	if request.ActingPrincipalID != request.OwnerAccountID {
		return ExecutionResult{}, connectorError(core.ErrorCodeAccessDenied, false)
	}

	binding, err := service.bindings.GetBinding(ctx, request.OrganizationID, request.OwnerAccountID, request.BindingID)
	if err != nil {
		return ExecutionResult{}, preserveError(err, core.ErrorCodeTargetNotReady)
	}
	if err := authorizeBinding(request, binding); err != nil {
		return ExecutionResult{}, err
	}

	connection, err := service.connections.GetConnection(ctx, request.OrganizationID, request.OwnerAccountID, request.ConnectionID)
	if err != nil {
		return ExecutionResult{}, preserveError(err, core.ErrorCodeConnectionNotReady)
	}
	if err := authorizeConnection(request, binding, connection); err != nil {
		return ExecutionResult{}, err
	}

	// A DigitalHuman host has no historical Connection boundary for its own
	// target type, so the exact owner-scoped Target Binding supplies that one
	// boundary while every operation, owner, expiry, and deny restriction is
	// still enforced by both layers.
	if !core.ConstraintsAllow(
		connection.Constraint, binding.Constraint,
		request.OwnerAccountID, request.OperationID, request.Target, service.now().UTC(),
	) {
		return ExecutionResult{}, connectorError(core.ErrorCodeAccessDenied, false)
	}
	acceptedOperation, ok := acceptedCatalogOperation(service.catalog, request)
	if !ok {
		return ExecutionResult{}, connectorError(core.ErrorCodeConnectorUnavailable, false)
	}
	definition, operation := acceptedOperation.Definition, acceptedOperation.Operation
	if pinned.Snapshot != nil {
		digest, digestErr := invocationInputDigest(request.Input)
		if digestErr != nil || digest != pinned.Snapshot.InputDigest || binding.Revision != pinned.Snapshot.BindingRevision ||
			connection.CredentialVersion != pinned.Snapshot.CredentialVersion || acceptedOperation.ContractDigest != pinned.Snapshot.ContractDigest {
			return ExecutionResult{}, connectorError(core.ErrorCodeConflict, false)
		}
	}
	if err := authorizeDefinition(request, connection, definition); err != nil {
		return ExecutionResult{}, err
	}
	progress.display = &OperationDisplay{Name: operation.Name, NameI18n: operation.NameI18n}

	if err := service.targetAuthorizer.AuthorizeTarget(ctx, TargetAuthorizationRequest{
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, Binding: binding, Connection: connection,
		OperationID: request.OperationID, Interactive: request.Interactive,
	}); err != nil {
		return ExecutionResult{}, preserveError(err, core.ErrorCodeAccessDenied)
	}

	risk, err := service.risk.ResolveRisk(withPinnedRiskContract(ctx, definition, operation), RiskRequest{
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, ConnectorID: request.ConnectorID,
		ConnectionID: request.ConnectionID, Target: request.Target,
		OperationID: request.OperationID, Interactive: request.Interactive,
	})
	if err != nil {
		return ExecutionResult{}, preserveError(err, core.ErrorCodeAccessDenied)
	}
	if risk != RiskTrustedLow && risk != RiskHigh && risk != RiskUnclassified {
		return ExecutionResult{}, connectorError(core.ErrorCodeAccessDenied, false)
	}
	confirmation := request.Confirmation
	requiresConfirmedFinalization := false
	if confirmation != nil && confirmation.RouteReceipt != pinned.Receipt {
		return ExecutionResult{}, connectorError(core.ErrorCodeInvalidInput, false)
	}

	// DigitalHuman is governed by an explicit platform-owned decision that is
	// stricter than the shared Chat/Agent/Workflow path. It is evaluated before
	// session grants so a reusable grant can never preauthorize a DigitalHuman
	// invocation, and before credentials so a trusted-low classification can
	// never reach a provider.
	digitalHuman := digitalHumanApproval(request.Target, risk, request.Interactive, confirmation, pinned.Receipt)
	if digitalHuman.governs() {
		// Rules 1 and 3: neither a reusable session grant nor a caller-supplied
		// preauthorization flag is authority for a DigitalHuman invocation.
		request.Preauthorized = false
		pinned.Request.Preauthorized = false
	}
	if digitalHuman == digitalHumanApprovalFailClosed {
		decisionID, decisionErr := service.recordExecutionDecision(ctx, pinned, risk, core.DecisionStatusDenied, false)
		result := ExecutionResult{State: ExecutionStateBlocked, DecisionID: decisionID}
		if decisionErr != nil {
			return result, decisionErr
		}
		return result, connectorError(core.ErrorCodeAccessDenied, false)
	}
	if digitalHuman == digitalHumanApprovalNonInteractiveBlocked {
		decisionID, decisionErr := service.recordExecutionDecision(ctx, pinned, risk, core.DecisionStatusBlocked, false)
		result := ExecutionResult{State: ExecutionStateBlocked, DecisionID: decisionID}
		if decisionErr != nil {
			return result, decisionErr
		}
		return result, connectorError(core.ErrorCodeConfirmationRequired, false)
	}

	_, sessionGrantMatched := connectorSessionGrantMatches(request, definition, operation, risk)
	// Binding-level direct authorization is explicit in the binding constraint.
	// Legacy bindings have no mode and therefore remain ask_each_time.
	bindingDirectAuthorization := confirmation == nil && binding.Constraint != nil &&
		binding.Constraint.DirectlyAuthorized() &&
		binding.Constraint.Allows(request.OwnerAccountID, request.OperationID, request.Target, service.now().UTC()) &&
		bindingAuthorizationFresh(binding.Constraint, connection, acceptedOperation.ContractDigest)
	preauthorized := confirmation == nil && digitalHuman.sessionGrantPreauthorizes() &&
		(sessionGrantMatched || bindingDirectAuthorization)
	request.Preauthorized = preauthorized
	pinned.Request.Preauthorized = preauthorized
	if risk != RiskTrustedLow && !preauthorized {
		if !request.Interactive {
			decisionID, decisionErr := service.recordExecutionDecision(ctx, pinned, risk, core.DecisionStatusBlocked, false)
			result := ExecutionResult{State: ExecutionStateBlocked, DecisionID: decisionID}
			if decisionErr != nil {
				return result, decisionErr
			}
			return result, connectorError(core.ErrorCodeConfirmationRequired, false)
		}
		if confirmation == nil {
			confirmationResult := service.confirmationWithPreview(ctx, pinned, acceptedOperation, binding, connection, risk)
			// A governed DigitalHuman approval is never presented without a
			// complete, non-secret preview. Failing closed here keeps the
			// decision ahead of every credential and provider access.
			if !digitalHumanApprovalPreviewAllowed(digitalHuman, confirmationResult.Preview) {
				return service.denyDigitalHumanIncompletePreview(ctx, pinned, risk)
			}
			return ExecutionResult{
				State:        ExecutionStateRequiresConfirmation,
				Confirmation: confirmationResult,
			}, nil
		}
		if confirmation.Decision == ConfirmationDecisionCancelled {
			decision, decisionErr := service.finalizeRuntimeConfirmation(
				ctx, pinned, risk, false, service.now().UTC(), time.Time{},
			)
			if decisionErr != nil {
				return ExecutionResult{}, decisionErr
			}
			return ExecutionResult{State: ExecutionStateCancelled, DecisionID: decision.ID}, nil
		}
		// A confirmed DigitalHuman continuation is accepted only when the
		// frozen request still rebuilds a complete preview from the current
		// connection. This is re-checked before finalization and credentials.
		if digitalHuman.governs() &&
			!digitalHumanApprovalPreviewAllowed(digitalHuman, service.confirmationPreview(ctx, pinned, binding, connection)) {
			return service.denyDigitalHumanIncompletePreview(ctx, pinned, risk)
		}
		requiresConfirmedFinalization = true
	} else if confirmation != nil {
		return ExecutionResult{}, connectorError(core.ErrorCodeInvalidInput, false)
	}

	credentials, err := resolveCredentials(ctx, service.credentials, CredentialResolutionRequest{
		OrganizationID: request.OrganizationID, ConnectionID: connection.ID,
		ConnectorID: connection.ConnectorID, CredentialVersion: connection.CredentialVersion,
		EncryptedCredentials: connection.EncryptedCredentials,
	})
	if err != nil {
		return ExecutionResult{}, preserveError(err, core.ErrorCodeInternal)
	}
	deferredCredentials := credentials
	defer core.DestroySecretValues(deferredCredentials)
	if len(credentials) == 0 {
		return ExecutionResult{}, connectorError(core.ErrorCodeInternal, false)
	}

	now := service.now().UTC()
	outcomeID := service.newID()
	executionLeaseExpiresAt := now.Add(service.invocationTimeout(connection, request.OperationID))
	if outcomeID == uuid.Nil || now.IsZero() || !executionLeaseExpiresAt.After(now) {
		return ExecutionResult{}, connectorError(core.ErrorCodeInternal, false)
	}
	var decision core.LifecycleDecision
	if requiresConfirmedFinalization {
		progress.submissionAttempted = true
		decision, err = service.finalizeRuntimeConfirmation(
			ctx, pinned, risk, true, now, executionLeaseExpiresAt,
		)
	} else {
		decisionID := service.newID()
		if decisionID == uuid.Nil {
			return ExecutionResult{}, connectorError(core.ErrorCodeInternal, false)
		}
		decision, err = service.newExecutionDecision(
			pinned, risk, core.DecisionStatusSubmitted, false, decisionID, now, executionLeaseExpiresAt,
		)
		if err == nil {
			progress.submissionAttempted = true
			err = service.audit.CreateDecision(ctx, decision)
			progress.definitelyNotWritten = connectoraudit.DecisionDefinitelyNotWritten(err)
		}
	}
	if err != nil {
		return ExecutionResult{}, preserveError(err, core.ErrorCodeInternal)
	}
	decisionID := decision.ID

	providerCtx, cancelProvider := context.WithDeadline(ctx, executionLeaseExpiresAt)
	providerResult, providerErr, providerPanicked := invokeProvider(providerCtx, service.provider, ProviderInvocation{
		BindingID: binding.ID, BindingRevision: binding.Revision,
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, ConnectionID: request.ConnectionID,
		ConnectorID: request.ConnectorID, ProviderID: connection.ProviderID,
		DriverID: connection.DriverID, AuthMethodID: connection.AuthMethodID,
		CredentialVersion: connection.CredentialVersion,
		Target:            request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, Credentials: credentials, Input: request.Input,
	})
	cancelProvider()
	providerRequestID := safeProviderRequestID(providerResult.ProviderRequestID, credentials)
	providerResult.DiagnosticCode = safeProviderMetadata(providerResult.DiagnosticCode, credentials)
	core.DestroySecretValues(credentials)
	credentials = nil

	status, diagnosticCode, failureCode, retryable := normalizeProviderResult(providerResult, providerErr, providerPanicked)
	outcome := core.OperationOutcome{
		ID: outcomeID, DecisionID: decisionID, OrganizationID: request.OrganizationID,
		OwnerAccountID: request.OwnerAccountID, ActingPrincipalID: request.ActingPrincipalID,
		ConnectorID: request.ConnectorID, ConnectionID: request.ConnectionID, Target: request.Target,
		OperationID: request.OperationID, LogicalOperationID: request.LogicalOperationID,
		Status: status, ProviderRequestID: providerRequestID,
		DiagnosticCode: diagnosticCode, CreatedAt: service.now().UTC(),
	}
	if outcome.CreatedAt.IsZero() {
		outcome.CreatedAt = now
	}
	result := ExecutionResult{State: ExecutionStateCompleted, DecisionID: decisionID, Outcome: outcome}
	if status == core.OutcomeStatusSuccessful {
		result.Value = providerResult.Value
	}
	if err := service.audit.RecordOutcome(ctx, outcome); err != nil {
		unknown := outcome
		unknown.Status = core.OutcomeStatusUnknown
		unknown.ProviderRequestID = ""
		unknown.DiagnosticCode = string(core.ErrorCodeOutcomeUnknown)
		durable, convergeErr := service.audit.ConvergeOutcome(ctx, unknown)
		if convergeErr != nil {
			unresolved := UnresolvedResult(core.ErrorCodeOutcomeUnknown)
			unresolved.DecisionID = decisionID
			return unresolved, connectorError(core.ErrorCodeOutcomeUnknown, false)
		}
		result.Outcome = durable
		if durable.ID != outcome.ID || durable.Status != core.OutcomeStatusSuccessful {
			result.Value = nil
		}
		return result, executionOutcomeError(durable, failureCode, retryable)
	}

	switch status {
	case core.OutcomeStatusSuccessful:
		return result, nil
	case core.OutcomeStatusUnknown:
		return result, connectorError(core.ErrorCodeOutcomeUnknown, false)
	case core.OutcomeStatusFailed:
		return result, connectorError(failureCode, retryable)
	default:
		return result, connectorError(core.ErrorCodeOutcomeUnknown, false)
	}
}

	// bindingAuthorizationFresh invalidates a persisted direct choice when the
	// connection was reconnected, rotated, its operation policy revision
	// changed, or the admitted operation contract changed. A direct row without
	// a complete snapshot is treated as ask_each_time rather than inheriting
	// compatibility authority.
func bindingAuthorizationFresh(constraint *core.AccessConstraint, connection core.PersonalConnection, contractDigest string) bool {
	if constraint == nil || !constraint.DirectlyAuthorized() {
		return false
	}
	if constraint.AuthorizationConnectionRevision < 1 || constraint.AuthorizationCredentialVersion < 1 || constraint.AuthorizationContractDigest == "" {
		return false
	}
	return constraint.AuthorizationConnectionRevision == connection.Revision &&
		constraint.AuthorizationCredentialVersion == connection.CredentialVersion &&
		constraint.AuthorizationContractDigest == contractDigest
}

func executionOutcomeError(
	outcome core.OperationOutcome,
	failureCode core.ErrorCode,
	retryable bool,
) error {
	switch outcome.Status {
	case core.OutcomeStatusSuccessful:
		return nil
	case core.OutcomeStatusUnknown:
		return connectorError(core.ErrorCodeOutcomeUnknown, false)
	case core.OutcomeStatusFailed:
		if !failureCode.Valid() {
			failureCode = core.ErrorCodeInternal
		}
		return connectorError(failureCode, retryable)
	default:
		return connectorError(core.ErrorCodeOutcomeUnknown, false)
	}
}

func acceptedCatalogOperation(
	definitionCatalog DefinitionCatalog,
	request ExecutionRequest,
) (catalog.PinnedOperation, bool) {
	if request.AcceptedOperation != nil {
		if request.Confirmation == nil || !validAcceptedOperationSnapshot(
			*request.AcceptedOperation, request.ConnectorID, request.OperationID,
		) {
			return catalog.PinnedOperation{}, false
		}
		return *request.AcceptedOperation, true
	}
	if request.Confirmation != nil {
		return catalog.PinnedOperation{}, false
	}
	return pinCatalogOperation(
		definitionCatalog,
		request.ConnectorID.String(),
		request.OperationID.String(),
	)
}

func pinCatalogOperation(
	definitionCatalog DefinitionCatalog,
	connectorID string,
	operationID string,
) (catalog.PinnedOperation, bool) {
	if pinner, ok := definitionCatalog.(DefinitionVersionPinner); ok {
		pinned, err := pinner.Pin(connectorID, operationID)
		if err != nil || !validAcceptedOperationSnapshot(
			pinned, core.Identifier(connectorID), core.Identifier(operationID),
		) {
			return catalog.PinnedOperation{}, false
		}
		return pinned, true
	}
	definition, ok := definitionCatalog.Get(connectorID)
	if !ok {
		return catalog.PinnedOperation{}, false
	}
	operation, ok := catalogOperation(definition, operationID)
	if !ok {
		return catalog.PinnedOperation{}, false
	}
	contractDigest, err := catalog.DefinitionContractDigest(definition)
	if err != nil {
		return catalog.PinnedOperation{}, false
	}
	pinned := catalog.PinnedOperation{
		ConnectorID: definition.ID, PluginID: definition.PluginID,
		PluginVersion: definition.PluginVersion, PluginDigest: definition.PluginDigest,
		ContractDigest: contractDigest, Definition: definition, Operation: operation,
	}
	if !validAcceptedOperationSnapshot(
		pinned, core.Identifier(connectorID), core.Identifier(operationID),
	) {
		return catalog.PinnedOperation{}, false
	}
	return pinned, true
}

func validAcceptedOperationSnapshot(
	pinned catalog.PinnedOperation,
	connectorID core.Identifier,
	operationID core.Identifier,
) bool {
	definition := pinned.Definition
	if !connectorID.Valid() || !operationID.Valid() || catalog.Validate(definition) != nil ||
		pinned.ConnectorID != connectorID.String() || pinned.ConnectorID != definition.ID ||
		pinned.PluginID != definition.PluginID || pinned.PluginVersion != definition.PluginVersion ||
		pinned.PluginDigest != definition.PluginDigest || pinned.Operation.ID != operationID.String() {
		return false
	}
	contractDigest, err := catalog.DefinitionContractDigest(definition)
	if err != nil || contractDigest == "" || pinned.ContractDigest != contractDigest {
		return false
	}
	operation, exists := catalogOperation(definition, operationID.String())
	return exists && reflect.DeepEqual(operation, pinned.Operation)
}

func (service *Service) finalizeRuntimeConfirmation(
	ctx context.Context,
	pinned PinnedExecution,
	risk RiskClassification,
	confirmed bool,
	createdAt time.Time,
	executionLeaseExpiresAt time.Time,
) (core.LifecycleDecision, error) {
	request := pinned.Request
	finalizationID, operationDecisionID := service.newID(), service.newID()
	if finalizationID == uuid.Nil || operationDecisionID == uuid.Nil || createdAt.IsZero() {
		return core.LifecycleDecision{}, connectorError(core.ErrorCodeInternal, false)
	}
	decision, err := connectoraudit.NewConfirmationFinalizationDecision(connectoraudit.ExecutionDecisionRequest{
		ID: finalizationID, OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, ConnectorID: request.ConnectorID,
		ConnectionID: request.ConnectionID, Target: request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
		ReceiptID: string(pinned.Receipt), ExecutionPath: string(pinned.Path),
		RiskClassification: string(risk), Interactive: request.Interactive,
		Confirmation: confirmed, Preauthorized: false, CreatedAt: createdAt,
	}, confirmed)
	if err != nil {
		return core.LifecycleDecision{}, connectorError(core.ErrorCodeInternal, false)
	}
	status := core.DecisionStatusCancelled
	if confirmed {
		status = core.DecisionStatusSubmitted
	}
	operationDecision, err := service.newExecutionDecision(
		pinned, risk, status, confirmed, operationDecisionID, createdAt, executionLeaseExpiresAt,
	)
	if err != nil {
		return core.LifecycleDecision{}, err
	}
	if err := service.audit.FinalizeConfirmation(ctx, decision, operationDecision); err != nil {
		return core.LifecycleDecision{}, preserveError(err, core.ErrorCodeInternal)
	}
	return operationDecision, nil
}

func (service *Service) newExecutionDecision(
	pinned PinnedExecution,
	risk RiskClassification,
	status core.DecisionStatus,
	confirmed bool,
	decisionID uuid.UUID,
	createdAt time.Time,
	executionLeaseExpiresAt time.Time,
) (core.LifecycleDecision, error) {
	request := pinned.Request
	decision, err := connectoraudit.NewExecutionDecision(connectoraudit.ExecutionDecisionRequest{
		ID: decisionID, OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, ConnectorID: request.ConnectorID,
		ConnectionID: request.ConnectionID, Target: request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
		ReceiptID: string(pinned.Receipt), ExecutionPath: string(pinned.Path),
		RiskClassification: string(risk), Interactive: request.Interactive,
		Confirmation: confirmed, Preauthorized: request.Preauthorized, CreatedAt: createdAt,
		ExecutionLeaseExpiresAt: executionLeaseExpiresAt,
	}, status)
	if err != nil {
		return core.LifecycleDecision{}, connectorError(core.ErrorCodeInternal, false)
	}
	return decision, nil
}

func (service *Service) recordExecutionDecision(
	ctx context.Context,
	pinned PinnedExecution,
	risk RiskClassification,
	status core.DecisionStatus,
	confirmed bool,
) (uuid.UUID, error) {
	decisionID := service.newID()
	now := service.now().UTC()
	if decisionID == uuid.Nil || now.IsZero() {
		return uuid.Nil, connectorError(core.ErrorCodeInternal, false)
	}
	decision, err := service.newExecutionDecision(
		pinned, risk, status, confirmed, decisionID, now, time.Time{},
	)
	if err != nil {
		return uuid.Nil, connectorError(core.ErrorCodeInternal, false)
	}
	if err := service.audit.CreateDecision(ctx, decision); err != nil {
		return uuid.Nil, preserveError(err, core.ErrorCodeInternal)
	}
	return decisionID, nil
}

// denyDigitalHumanIncompletePreview is the fail-closed DigitalHuman policy for
// an approval that cannot be presented safely. It records a real, valid denial
// with the acting identity before returning, and because it runs before
// credential resolution and provider selection it never opens a secret or
// invokes a remote action. No request input or preview value is persisted.
func (service *Service) denyDigitalHumanIncompletePreview(
	ctx context.Context,
	pinned PinnedExecution,
	risk RiskClassification,
) (ExecutionResult, error) {
	decisionID, decisionErr := service.recordExecutionDecision(ctx, pinned, risk, core.DecisionStatusDenied, false)
	result := ExecutionResult{State: ExecutionStateBlocked, DecisionID: decisionID}
	if decisionErr != nil {
		return result, decisionErr
	}
	return result, connectorError(core.ErrorCodeAccessDenied, false)
}

func (service *Service) available() error {
	if service == nil || service.catalog == nil || service.bindings == nil || service.connections == nil ||
		service.targetAuthorizer == nil || service.risk == nil || service.credentials == nil ||
		service.audit == nil || service.provider == nil || service.now == nil || service.newID == nil {
		return connectorError(core.ErrorCodeInternal, false)
	}
	if service.providerTimeout <= 0 || service.providerTimeout > maximumProviderInvocationTimeout {
		return connectorError(core.ErrorCodeInternal, false)
	}
	return nil
}

func authorizeBinding(request ExecutionRequest, binding core.TargetBinding) error {
	if binding.Validate() != nil || binding.OrganizationID != request.OrganizationID ||
		binding.OwnerAccountID != request.OwnerAccountID || binding.ID != request.BindingID ||
		binding.ConnectionID != request.ConnectionID || binding.ConnectorID != request.ConnectorID ||
		!binding.Target.SameIdentity(request.Target) {
		return connectorError(core.ErrorCodeAccessDenied, false)
	}
	if binding.Status != core.BindingStatusActive {
		return connectorError(core.ErrorCodeTargetNotReady, false)
	}
	return nil
}

func authorizeConnection(request ExecutionRequest, binding core.TargetBinding, connection core.PersonalConnection) error {
	if connection.Validate() != nil || connection.OrganizationID != request.OrganizationID ||
		connection.OwnerAccountID != request.OwnerAccountID || connection.ID != request.ConnectionID ||
		connection.ID != binding.ConnectionID || connection.ConnectorID != request.ConnectorID ||
		connection.DeletedAt != nil {
		return connectorError(core.ErrorCodeAccessDenied, false)
	}
	if connection.State != core.ReadinessReady {
		return connectorError(core.ErrorCodeConnectionNotReady, false)
	}
	if !operationEligible(connection, request.OperationID) {
		return connectorError(core.ErrorCodeAccessDenied, false)
	}
	return nil
}

func authorizeDefinition(request ExecutionRequest, connection core.PersonalConnection, definition catalog.Definition) error {
	if definition.ID != request.ConnectorID.String() || definition.ProviderID != connection.ProviderID.String() ||
		definition.DriverID != connection.DriverID.String() || definition.AuthMethodID != connection.AuthMethodID.String() ||
		!definition.Supports(surfaceForTarget(request.Target.Type)) || !definitionHasOperation(definition, request.OperationID) {
		return connectorError(core.ErrorCodeConnectorUnavailable, false)
	}
	return nil
}

func operationEligible(connection core.PersonalConnection, operationID core.Identifier) bool {
	if _, unavailable := connection.UnavailableOperations[operationID]; unavailable {
		return false
	}
	for _, candidate := range connection.UsableOperationIDs {
		if candidate == operationID {
			return true
		}
	}
	return false
}

func definitionHasOperation(definition catalog.Definition, operationID core.Identifier) bool {
	for _, operation := range definition.Operations {
		if operation.ID == operationID.String() {
			return true
		}
	}
	return false
}

func surfaceForTarget(targetType core.TargetType) catalog.Surface {
	switch targetType {
	case core.TargetTypeChat:
		return catalog.SurfaceChat
	case core.TargetTypeAgent:
		return catalog.SurfaceAgent
	case core.TargetTypeWorkflow:
		return catalog.SurfaceWorkflow
	case core.TargetTypeDigitalHuman:
		return catalog.SurfaceDigitalHuman
	default:
		return ""
	}
}

func cloneTarget(target core.TargetRef) *core.TargetRef {
	copyValue := target
	return &copyValue
}

func resolveCredentials(
	ctx context.Context,
	resolver CredentialResolver,
	request CredentialResolutionRequest,
) (credentials map[string]string, err error) {
	defer func() {
		if recover() != nil {
			core.DestroySecretValues(credentials)
			credentials = nil
			err = connectorError(core.ErrorCodeInternal, false)
		}
	}()
	return resolver.ResolveCredentials(ctx, request)
}

func invokeProvider(
	ctx context.Context,
	provider NativeProviderInvoker,
	invocation ProviderInvocation,
) (result ProviderInvocationResult, err error, panicked bool) {
	defer func() {
		if recover() != nil {
			result = ProviderInvocationResult{}
			err = nil
			panicked = true
		}
	}()
	result, err = provider.Invoke(ctx, invocation)
	return result, err, false
}

func normalizeProviderResult(
	result ProviderInvocationResult,
	providerErr error,
	panicked bool,
) (core.OutcomeStatus, string, core.ErrorCode, bool) {
	if panicked || (providerErr != nil && result.Status != core.OutcomeStatusFailed) || !result.Status.Valid() {
		return core.OutcomeStatusUnknown, string(core.ErrorCodeOutcomeUnknown), core.ErrorCodeOutcomeUnknown, false
	}
	if result.Status == core.OutcomeStatusSuccessful {
		if providerErr != nil {
			return core.OutcomeStatusUnknown, string(core.ErrorCodeOutcomeUnknown), core.ErrorCodeOutcomeUnknown, false
		}
		return core.OutcomeStatusSuccessful, "", "", false
	}
	if result.Status == core.OutcomeStatusUnknown {
		return core.OutcomeStatusUnknown, safeDiagnosticCode(result.DiagnosticCode, core.ErrorCodeOutcomeUnknown), core.ErrorCodeOutcomeUnknown, false
	}
	failureCode := result.FailureCode
	if !failureCode.Valid() {
		failureCode = core.ErrorCodeInternal
	}
	return core.OutcomeStatusFailed, safeDiagnosticCode(result.DiagnosticCode, failureCode), failureCode, result.Retryable
}

func safeProviderRequestID(value string, credentials map[string]string) string {
	if !validOpaqueText(value, 128) || strings.ContainsAny(value, " \t") || matchesCredential(value, credentials) {
		return ""
	}
	return value
}

func safeProviderMetadata(value string, credentials map[string]string) string {
	if matchesCredential(value, credentials) {
		return ""
	}
	return value
}

func matchesCredential(value string, credentials map[string]string) bool {
	if value == "" {
		return false
	}
	for _, credential := range credentials {
		if credential != "" && strings.Contains(value, credential) {
			return true
		}
	}
	return false
}

func safeDiagnosticCode(value string, fallback core.ErrorCode) string {
	if diagnosticCodePattern.MatchString(value) {
		return value
	}
	if fallback.Valid() && diagnosticCodePattern.MatchString(string(fallback)) {
		return string(fallback)
	}
	return string(core.ErrorCodeInternal)
}

func connectorError(code core.ErrorCode, retryable bool) error {
	return core.NewError(code, retryable, nil)
}

func preserveError(err error, fallback core.ErrorCode) error {
	if err == nil {
		return nil
	}
	var connectorErr *core.Error
	if errors.As(err, &connectorErr) && connectorErr != nil && connectorErr.Code.Valid() {
		return connectorError(connectorErr.Code, connectorErr.Retryable)
	}
	return connectorError(fallback, false)
}
