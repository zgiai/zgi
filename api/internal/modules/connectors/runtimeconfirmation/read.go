package runtimeconfirmation

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

type StatusQuery struct {
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	RouteReceipt   execution.RouteClaimReceipt
}

// StatusResult contains only durable public facts, never credentials, input or host state.
type StatusResult struct {
	Resolution string         `json:"resolution"`
	State      State          `json:"state,omitempty"`
	DecisionID uuid.UUID      `json:"decision_id,omitempty,omitzero"`
	ErrorCode  core.ErrorCode `json:"error_code,omitempty"`
}

// ReadStatus is strictly observational. A pending row is unresolved, not a
// failed operation. In particular, this method never routes, recovers by
// submission, records a terminal, or resumes a Chat/Workflow host.
func (service *Service) ReadStatus(ctx context.Context, query StatusQuery) (StatusResult, error) {
	if service == nil || service.store == nil {
		return StatusResult{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	command := Command{OrganizationID: query.OrganizationID, OwnerAccountID: query.OwnerAccountID, RouteReceipt: query.RouteReceipt, Decision: execution.ConfirmationDecisionConfirmed}
	if err := validateCommand(command); err != nil {
		return StatusResult{}, err
	}
	continuation, err := service.store.Load(ctx, query.OrganizationID, query.OwnerAccountID, query.RouteReceipt)
	if err != nil {
		return StatusResult{}, err
	}
	if err := authorizeFrozenRequest(command, continuation.Frozen); err != nil {
		return StatusResult{}, err
	}
	if continuation.State == execution.ConfirmationContinuationPending {
		return StatusResult{Resolution: "unresolved"}, nil
	}
	if continuation.State == execution.ConfirmationContinuationCancelled {
		command.Decision = execution.ConfirmationDecisionCancelled
	}
	result, err := projectExisting(continuation, command.Decision)
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{Resolution: "resolved", State: result.State, DecisionID: result.DecisionID, ErrorCode: result.ErrorCode}, nil
}
