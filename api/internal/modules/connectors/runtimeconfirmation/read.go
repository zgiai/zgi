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
	// OneShot marks a confirmation whose surface confirms each operation once
	// (DigitalHuman), so the client does not offer to remember it for the
	// session. It comes from the trusted frozen origin.
	OneShot bool `json:"one_shot,omitempty"`
}

type PendingResult struct {
	Items    []execution.PendingConfirmationRef `json:"items"`
	Page     int                                `json:"page"`
	PageSize int                                `json:"page_size"`
	HasMore  bool                               `json:"has_more"`
}

func (service *Service) ListPending(ctx context.Context, organizationID, ownerAccountID uuid.UUID, page, pageSize int) (PendingResult, error) {
	if service == nil || service.store == nil {
		return PendingResult{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	reader, ok := service.store.(interface {
		ListPending(context.Context, uuid.UUID, uuid.UUID, int, int) ([]execution.PendingConfirmationRef, bool, error)
	})
	if !ok {
		return PendingResult{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	items, more, err := reader.ListPending(ctx, organizationID, ownerAccountID, page, pageSize)
	if err != nil {
		return PendingResult{}, err
	}
	return PendingResult{Items: items, Page: page, PageSize: pageSize, HasMore: more}, nil
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
	continuation, err := service.loadForDecider(ctx, command)
	if err != nil {
		return StatusResult{}, err
	}
	if owner := continuation.Frozen.Request.OwnerAccountID; owner != query.OwnerAccountID {
		// A requester reads a confirmation frozen with the owner's delegated
		// credential only for a DigitalHuman run they requested.
		guardRequest, guardErr := digitalHumanGuardRequest(continuation)
		if guardErr != nil {
			return StatusResult{}, guardErr
		}
		guardRequest.DeciderID = query.OwnerAccountID
		if err := service.digitalHumanGuard.AuthorizeDecider(ctx, guardRequest); err != nil {
			return StatusResult{}, err
		}
		command.OwnerAccountID = owner
	}
	if err := authorizeFrozenRequest(command, continuation.Frozen); err != nil {
		return StatusResult{}, err
	}
	var presentation *Presentation
	if query.PresentationVersion == 1 {
		presentation = &Presentation{
			Version: 1, Preview: continuation.Frozen.Preview,
			OneShot: continuation.Frozen.Origin.Surface == core.TargetTypeDigitalHuman,
		}
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
