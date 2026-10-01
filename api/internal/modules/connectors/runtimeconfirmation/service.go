package runtimeconfirmation

import (
	"context"

	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

// State is the complete public state machine for a resolved confirmation.
// Pending is deliberately absent because this service returns only a terminal
// projection or an error.
type State string

const (
	StateCancelled State = "cancelled"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
	StateUnknown   State = "unknown"
)

// Command contains the authenticated owner scope and the only two values a
// client may submit. The original operation identity and input are always
// restored from ConfirmationContinuationStore. RememberForSession is a
// separate governance preference and never changes the frozen request.
type Command struct {
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	RouteReceipt       execution.RouteClaimReceipt
	Decision           execution.ConfirmationDecision
	RememberForSession bool
}

// AuthorizationSnapshot carries only the non-secret operation contract needed
// to create a Tool Governance session grant after a user approval.
type AuthorizationSnapshot struct {
	SkillID              string
	ConnectionID         uuid.UUID
	BindingID            uuid.UUID
	ConnectorID          core.Identifier
	OperationID          core.Identifier
	Target               core.TargetRef
	Effect               catalog.OperationEffect
	DataEgress           bool
	ExternalDestination  string
	SensitiveDataAllowed bool
	Risk                 execution.RiskClassification
}

// ConfirmationRequestIdentity is the exact non-secret authority tuple restored
// from the encrypted continuation. It deliberately has no Input, Confirmation,
// credential, provider value, or JSON representation.
type ConfirmationRequestIdentity struct {
	OrganizationID     uuid.UUID
	OwnerAccountID     uuid.UUID
	ActingPrincipalID  uuid.UUID
	BindingID          uuid.UUID
	ConnectionID       uuid.UUID
	ConnectorID        core.Identifier
	Target             core.TargetRef
	OperationID        core.Identifier
	LogicalOperationID string
	CorrelationID      string
	Interactive        bool
}

// Result carries both the safe terminal projection and the trusted host
// continuation data. HTTP callers must project only State, DecisionID, and
// ErrorCode. JSON tags provide a second line of defense against exposing the
// origin or provider value directly.
type Result struct {
	State           State                                 `json:"state"`
	DecisionID      uuid.UUID                             `json:"decision_id,omitempty,omitzero"`
	ErrorCode       core.ErrorCode                        `json:"error_code,omitempty"`
	Origin          execution.ConfirmationOrigin          `json:"-"`
	RequestIdentity ConfirmationRequestIdentity           `json:"-"`
	Authorization   *AuthorizationSnapshot                `json:"-"`
	ExecutionResult *execution.ExecutionResult            `json:"-"`
	Presentation    *connectortools.OperationPresentation `json:"-"`
	OutcomeID       uuid.UUID                             `json:"-"`
}

// ExecutionRouter is implemented by execution.Router. Keeping the dependency
// narrow makes the continuation service independently testable without adding
// another provider submission path.
type ExecutionRouter interface {
	Route(context.Context, execution.ExecutionRequest) (execution.ExecutionResult, error)
}

// TerminalRecoverer rebuilds a safe terminal from immutable audit evidence.
// It is consulted only after the already-pinned Router reports a conflict and
// the continuation row is still pending; it must never invoke a provider.
type TerminalRecoverer interface {
	RecoverTerminal(context.Context, TerminalRecoveryRequest) (execution.ConfirmationTerminal, error)
}

// Service owns terminal continuation semantics around the shared execution
// Router and the durable owner-scoped continuation store.
type Service struct {
	store     execution.ConfirmationContinuationStore
	router    ExecutionRouter
	recoverer TerminalRecoverer
}

func NewService(
	store execution.ConfirmationContinuationStore,
	router ExecutionRouter,
	recoverer TerminalRecoverer,
) (*Service, error) {
	if store == nil || router == nil || recoverer == nil {
		return nil, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return &Service{store: store, router: router, recoverer: recoverer}, nil
}

// Continue restores the server-authoritative request, adds only the submitted
// confirmation command, and resumes the already pinned route. Sequential
// duplicates never call Router again. Concurrent losers converge on the
// winning terminal record when it is already visible, otherwise receive a
// conflict while the winner is still running.
func (service *Service) Continue(ctx context.Context, command Command) (Result, error) {
	if err := service.available(); err != nil {
		return Result{}, err
	}
	if err := validateCommand(command); err != nil {
		return Result{}, err
	}

	continuation, err := service.store.Load(
		ctx,
		command.OrganizationID,
		command.OwnerAccountID,
		command.RouteReceipt,
	)
	if err != nil {
		return Result{}, err
	}
	if continuation.State != execution.ConfirmationContinuationPending {
		return projectExisting(ctx, continuation, command.Decision)
	}
	if err := authorizeFrozenRequest(command, continuation.Frozen); err != nil {
		return Result{}, err
	}

	request := continuation.Frozen.Request
	acceptedOperation := continuation.Frozen.AcceptedOperation
	authorization := authorizationSnapshot(request, acceptedOperation)
	request.AcceptedOperation = &acceptedOperation
	request.Confirmation = &execution.RuntimeConfirmationCommand{
		RouteReceipt: command.RouteReceipt,
		Decision:     command.Decision,
	}
	result, routeErr := service.router.Route(ctx, request)
	if result.State == execution.ExecutionStateUnresolved {
		return service.resolveConcurrentWinner(ctx, command, core.NewError(core.ErrorCodeOutcomeUnknown, false, nil))
	}
	if core.IsErrorCode(routeErr, core.ErrorCodeConflict) &&
		result.State != execution.ExecutionStateCompleted &&
		result.State != execution.ExecutionStateRejected &&
		result.State != execution.ExecutionStateCancelled {
		return service.resolveConcurrentWinner(ctx, command, routeErr)
	}
	if command.Decision == execution.ConfirmationDecisionCancelled &&
		result.State != execution.ExecutionStateRejected &&
		(result.State != execution.ExecutionStateCancelled || result.DecisionID == uuid.Nil) {
		if routeErr != nil {
			return Result{}, routeErr
		}
		return Result{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	if command.Decision == execution.ConfirmationDecisionConfirmed &&
		result.State == execution.ExecutionStateCancelled {
		return Result{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}

	terminal, terminalErr := service.terminalFromRoute(ctx, command, continuation, result, routeErr)
	if terminalErr != nil {
		return Result{}, terminalErr
	}
	stored, err := service.store.RecordTerminal(
		ctx,
		command.OrganizationID,
		command.OwnerAccountID,
		command.RouteReceipt,
		terminal,
	)
	if err != nil {
		if core.IsErrorCode(err, core.ErrorCodeConflict) {
			return service.resolveConcurrentWinner(ctx, command, err)
		}
		return Result{}, err
	}
	projected, err := projectExisting(ctx, stored, command.Decision)
	if err != nil {
		return Result{}, err
	}
	if executionResultMatchesTerminal(result, terminal) {
		projected.ExecutionResult = &result
		projected.Presentation = connectortools.PresentationForExecution(result, dispatch.Identity{
			OrganizationID:     request.OrganizationID,
			OwnerAccountID:     request.OwnerAccountID,
			ActingPrincipalID:  request.ActingPrincipalID,
			BindingID:          request.BindingID,
			ConnectionID:       request.ConnectionID,
			ConnectorID:        request.ConnectorID,
			Target:             request.Target,
			OperationID:        request.OperationID,
			LogicalOperationID: request.LogicalOperationID,
			CorrelationID:      request.CorrelationID,
			Interactive:        request.Interactive,
		})
	}
	projected.Authorization = authorization
	return projected, nil
}

func (service *Service) terminalFromRoute(
	ctx context.Context,
	command Command,
	continuation execution.ConfirmationContinuation,
	result execution.ExecutionResult,
	routeErr error,
) (execution.ConfirmationTerminal, error) {
	if result.State == execution.ExecutionStateCancelled || result.State == execution.ExecutionStateCompleted || result.State == execution.ExecutionStateRejected {
		terminal, err := service.recoverer.RecoverTerminal(
			ctx,
			terminalRecoveryRequest(command, continuation),
		)
		if err != nil {
			return execution.ConfirmationTerminal{}, err
		}
		if result.State == execution.ExecutionStateRejected && (!result.Evidence.ValidFor(result.State) || result.DecisionID == uuid.Nil || terminal.DecisionID != result.DecisionID || terminal.State != execution.ConfirmationContinuationFailed || terminal.OutcomeID != uuid.Nil) {
			return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeConflict, false, nil)
		}
		if result.State == execution.ExecutionStateCancelled &&
			terminal.State != execution.ConfirmationContinuationCancelled {
			return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeInternal, false, nil)
		}
		if result.State == execution.ExecutionStateCompleted &&
			terminal.State == execution.ConfirmationContinuationCancelled {
			return execution.ConfirmationTerminal{}, core.NewError(core.ErrorCodeInternal, false, nil)
		}
		return terminal, nil
	}
	return terminalFromExecution(result, routeErr), nil
}

func executionResultMatchesTerminal(
	result execution.ExecutionResult,
	terminal execution.ConfirmationTerminal,
) bool {
	if result.State == execution.ExecutionStateRejected {
		return result.Evidence.ValidFor(result.State) && result.DecisionID != uuid.Nil && result.DecisionID == terminal.DecisionID && terminal.State == execution.ConfirmationContinuationFailed && terminal.OutcomeID == uuid.Nil
	}
	if result.State == execution.ExecutionStateCancelled {
		return result.DecisionID != uuid.Nil && result.DecisionID == terminal.DecisionID &&
			terminal.State == execution.ConfirmationContinuationCancelled
	}
	if result.State != execution.ExecutionStateCompleted || result.DecisionID == uuid.Nil ||
		result.Outcome.ID == uuid.Nil {
		return false
	}
	return result.DecisionID == terminal.DecisionID && result.Outcome.ID == terminal.OutcomeID &&
		result.Outcome.DecisionID == terminal.DecisionID
}

func (service *Service) resolveConcurrentWinner(
	ctx context.Context,
	command Command,
	conflict error,
) (Result, error) {
	current, err := service.store.Load(
		ctx,
		command.OrganizationID,
		command.OwnerAccountID,
		command.RouteReceipt,
	)
	if err != nil {
		return Result{}, err
	}
	if current.State == execution.ConfirmationContinuationPending {
		return service.recoverPendingTerminal(ctx, command, current, conflict)
	}
	return projectExisting(ctx, current, command.Decision)
}

func (service *Service) recoverPendingTerminal(
	ctx context.Context,
	command Command,
	current execution.ConfirmationContinuation,
	conflict error,
) (Result, error) {
	if service.recoverer == nil {
		return Result{}, conflict
	}
	terminal, err := service.recoverer.RecoverTerminal(ctx, terminalRecoveryRequest(command, current))
	if err != nil {
		if core.IsErrorCode(err, core.ErrorCodeConflict) || core.IsErrorCode(err, core.ErrorCodeNotFound) {
			return Result{}, conflict
		}
		return Result{}, err
	}
	stored, err := service.store.RecordTerminal(
		ctx,
		command.OrganizationID,
		command.OwnerAccountID,
		command.RouteReceipt,
		terminal,
	)
	if err != nil {
		if !core.IsErrorCode(err, core.ErrorCodeConflict) {
			return Result{}, err
		}
		stored, err = service.store.Load(
			ctx,
			command.OrganizationID,
			command.OwnerAccountID,
			command.RouteReceipt,
		)
		if err != nil {
			return Result{}, err
		}
		if stored.State == execution.ConfirmationContinuationPending {
			return Result{}, conflict
		}
	}
	return projectExisting(ctx, stored, command.Decision)
}

func terminalRecoveryRequest(
	command Command,
	continuation execution.ConfirmationContinuation,
) TerminalRecoveryRequest {
	request := continuation.Frozen.Request
	return TerminalRecoveryRequest{
		OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
		ActingPrincipalID: request.ActingPrincipalID, ConnectionID: request.ConnectionID,
		ConnectorID: request.ConnectorID, Target: request.Target, OperationID: request.OperationID,
		LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
		RouteReceipt: command.RouteReceipt, Decision: command.Decision,
	}
}

func (service *Service) available() error {
	if service == nil || service.store == nil || service.router == nil || service.recoverer == nil {
		return core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return nil
}

func validateCommand(command Command) error {
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil ||
		command.RouteReceipt == "" ||
		(command.Decision != execution.ConfirmationDecisionConfirmed &&
			command.Decision != execution.ConfirmationDecisionCancelled) {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	receiptID, err := uuid.Parse(string(command.RouteReceipt))
	if err != nil || receiptID == uuid.Nil || receiptID.String() != string(command.RouteReceipt) {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return nil
}

func authorizeFrozenRequest(command Command, frozen execution.FrozenConfirmationRequest) error {
	request := frozen.Request
	if frozen.RouteReceipt != command.RouteReceipt || request.OrganizationID != command.OrganizationID ||
		request.OwnerAccountID != command.OwnerAccountID || request.ActingPrincipalID != command.OwnerAccountID ||
		request.Confirmation != nil || !request.Interactive {
		return core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	return nil
}

func terminalFromExecution(result execution.ExecutionResult, routeErr error) execution.ConfirmationTerminal {
	switch result.State {
	case execution.ExecutionStateRejected, execution.ExecutionStateUnresolved:
		return execution.ConfirmationTerminal{State: execution.ConfirmationContinuationUnknown, DiagnosticCode: string(core.ErrorCodeOutcomeUnknown)}
	case execution.ExecutionStateCancelled:
		if result.DecisionID != uuid.Nil {
			return execution.ConfirmationTerminal{
				State: execution.ConfirmationContinuationCancelled, DecisionID: result.DecisionID,
			}
		}
		return failedTerminal(core.ErrorCodeInternal)
	case execution.ExecutionStateCompleted:
		return terminalFromCompletedExecution(result, routeErr)
	case execution.ExecutionStateBlocked, execution.ExecutionStateRequiresConfirmation:
		return failedTerminal(core.ErrorCodeInternal)
	default:
		if core.IsErrorCode(routeErr, core.ErrorCodeOutcomeUnknown) {
			return execution.ConfirmationTerminal{
				State:          execution.ConfirmationContinuationUnknown,
				DiagnosticCode: string(core.ErrorCodeOutcomeUnknown),
			}
		}
		return failedTerminal(safeExecutionErrorCode(routeErr))
	}
}

func terminalFromCompletedExecution(
	result execution.ExecutionResult,
	routeErr error,
) execution.ConfirmationTerminal {
	if result.DecisionID == uuid.Nil || result.Outcome.ID == uuid.Nil ||
		result.Outcome.DecisionID != result.DecisionID {
		return execution.ConfirmationTerminal{
			State:          execution.ConfirmationContinuationUnknown,
			DiagnosticCode: string(core.ErrorCodeOutcomeUnknown),
		}
	}
	terminal := execution.ConfirmationTerminal{
		DecisionID: result.DecisionID,
		OutcomeID:  result.Outcome.ID,
	}
	switch result.Outcome.Status {
	case core.OutcomeStatusSuccessful:
		if routeErr != nil {
			terminal.State = execution.ConfirmationContinuationUnknown
			terminal.DiagnosticCode = string(core.ErrorCodeOutcomeUnknown)
			return terminal
		}
		terminal.State = execution.ConfirmationContinuationCompleted
	case core.OutcomeStatusFailed:
		terminal.State = execution.ConfirmationContinuationFailed
		terminal.DiagnosticCode = string(safeExecutionErrorCode(routeErr))
	case core.OutcomeStatusUnknown:
		terminal.State = execution.ConfirmationContinuationUnknown
		terminal.DiagnosticCode = string(core.ErrorCodeOutcomeUnknown)
	default:
		terminal.State = execution.ConfirmationContinuationUnknown
		terminal.DiagnosticCode = string(core.ErrorCodeOutcomeUnknown)
	}
	return terminal
}

func failedTerminal(code core.ErrorCode) execution.ConfirmationTerminal {
	if !code.Valid() || code == core.ErrorCodeOutcomeUnknown {
		if code == core.ErrorCodeOutcomeUnknown {
			return execution.ConfirmationTerminal{
				State:          execution.ConfirmationContinuationUnknown,
				DiagnosticCode: string(core.ErrorCodeOutcomeUnknown),
			}
		}
		code = core.ErrorCodeInternal
	}
	return execution.ConfirmationTerminal{
		State: execution.ConfirmationContinuationFailed, DiagnosticCode: string(code),
	}
}

func safeExecutionErrorCode(err error) core.ErrorCode {
	code := core.ErrorCodeOf(err)
	if !code.Valid() {
		return core.ErrorCodeInternal
	}
	return code
}

func projectExisting(
	ctx context.Context,
	continuation execution.ConfirmationContinuation,
	decision execution.ConfirmationDecision,
) (Result, error) {
	if (continuation.State == execution.ConfirmationContinuationCancelled) !=
		(decision == execution.ConfirmationDecisionCancelled) {
		return Result{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	request := continuation.Frozen.Request
	result := Result{
		DecisionID: continuation.DecisionID,
		OutcomeID:  continuation.OutcomeID,
		Origin:     continuation.Frozen.Origin,
		RequestIdentity: ConfirmationRequestIdentity{
			OrganizationID: request.OrganizationID, OwnerAccountID: request.OwnerAccountID,
			ActingPrincipalID: request.ActingPrincipalID, BindingID: request.BindingID,
			ConnectionID: request.ConnectionID, ConnectorID: request.ConnectorID,
			Target: request.Target, OperationID: request.OperationID,
			LogicalOperationID: request.LogicalOperationID, CorrelationID: request.CorrelationID,
			Interactive: request.Interactive,
		},
		Authorization: authorizationSnapshot(request, continuation.Frozen.AcceptedOperation),
	}
	switch continuation.State {
	case execution.ConfirmationContinuationCancelled:
		result.State = StateCancelled
	case execution.ConfirmationContinuationCompleted:
		result.State = StateCompleted
	case execution.ConfirmationContinuationFailed:
		result.State = StateFailed
		result.ErrorCode = safeStoredErrorCode(continuation.DiagnosticCode, core.ErrorCodeInternal)
	case execution.ConfirmationContinuationUnknown:
		result.State = StateUnknown
		result.ErrorCode = core.ErrorCodeOutcomeUnknown
	default:
		return Result{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	result.Presentation = fallbackPresentation(ctx, request, continuation.Frozen.AcceptedOperation, result.State, result.ErrorCode)
	return result, nil
}

func fallbackPresentation(
	ctx context.Context,
	request execution.ExecutionRequest,
	operation catalog.PinnedOperation,
	state State,
	errorCode core.ErrorCode,
) *connectortools.OperationPresentation {
	status := "unknown"
	switch state {
	case StateCompleted:
		status = "successful"
	case StateFailed:
		status = "failed"
	case StateCancelled:
		status = "cancelled"
	}
	locale := execution.PresentationLocaleFromContext(ctx)
	code := strings.TrimSpace(string(errorCode))
	title := strings.TrimSpace(operation.Operation.NameI18n[locale])
	if title == "" {
		title = strings.TrimSpace(operation.Operation.Name)
	}
	if title == "" {
		title = request.OperationID.String()
	}
	return &connectortools.OperationPresentation{
		SchemaVersion: 1,
		SourceKind:    "native",
		InvocationID:  request.LogicalOperationID,
		ConnectorID:   request.ConnectorID,
		OperationID:   request.OperationID,
		DisplayStatus: status,
		Title:         title,
		Summary:       connectortools.SafeTerminalSummary(status, code, locale),
		ErrorCode:     code,
		Redacted:      false,
	}
}

func authorizationSnapshot(
	request execution.ExecutionRequest,
	accepted catalog.PinnedOperation,
) *AuthorizationSnapshot {
	if accepted.Operation.ID == "" || request.ConnectionID == uuid.Nil || request.BindingID == uuid.Nil {
		return nil
	}
	return &AuthorizationSnapshot{
		SkillID:              accepted.Operation.SkillID,
		ConnectionID:         request.ConnectionID,
		BindingID:            request.BindingID,
		ConnectorID:          request.ConnectorID,
		OperationID:          request.OperationID,
		Target:               request.Target,
		Effect:               accepted.Operation.Effect,
		DataEgress:           accepted.Operation.DataEgress,
		ExternalDestination:  accepted.Operation.ExternalDestination,
		SensitiveDataAllowed: accepted.Operation.SensitiveDataAllowed,
		Risk:                 execution.RiskHigh,
	}
}

func safeStoredErrorCode(value string, fallback core.ErrorCode) core.ErrorCode {
	code := core.ErrorCode(value)
	if !code.Valid() {
		return fallback
	}
	return code
}
