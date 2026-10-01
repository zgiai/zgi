package runtimeconfirmation

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
)

type StatusQuery struct {
	PresentationVersion int
	OrganizationID      uuid.UUID
	OwnerAccountID      uuid.UUID
	RouteReceipt        execution.RouteClaimReceipt
}

// StatusResult contains durable status and an opt-in, owner-only safe preview.
// Raw input, credentials and host state are never returned.
type StatusResult struct {
	Presentation *Presentation  `json:"presentation,omitempty"`
	Resolution   string         `json:"resolution"`
	State        State          `json:"state,omitempty"`
	DecisionID   uuid.UUID      `json:"decision_id,omitempty,omitzero"`
	ErrorCode    core.ErrorCode `json:"error_code,omitempty"`
}

type Presentation struct {
	Version int                            `json:"version"`
	Preview *execution.ConfirmationPreview `json:"preview"`
}

// ReadStatus is strictly observational. A pending row is unresolved, not a
// failed operation. In particular, this method never routes, recovers by
// submission, records a terminal, or resumes a Chat/Workflow host.
func (service *Service) ReadStatus(ctx context.Context, query StatusQuery) (StatusResult, error) {
	if query.PresentationVersion != 0 && query.PresentationVersion != 1 {
		return StatusResult{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
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
	var presentation *Presentation
	if query.PresentationVersion == 1 {
		presentation = &Presentation{Version: 1, Preview: continuation.Frozen.Preview}
	}
	if continuation.State == execution.ConfirmationContinuationPending {
		return StatusResult{Resolution: "unresolved", Presentation: presentation}, nil
	}
	if continuation.State == execution.ConfirmationContinuationCancelled {
		command.Decision = execution.ConfirmationDecisionCancelled
	}
	result, err := projectExisting(ctx, continuation, command.Decision)
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{Resolution: "resolved", State: result.State, DecisionID: result.DecisionID, ErrorCode: result.ErrorCode, Presentation: presentation}, nil
}
