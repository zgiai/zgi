package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

var (
	ErrRegistrationInvitationInvalid    = errors.New("registration invitation is invalid")
	ErrRegistrationInvitationAcceptance = errors.New("registration invitation could not be accepted")
)

type registrationInvitationGateway interface {
	ValidateInviteLinkForRegistration(ctx context.Context, token string) (*workspace_model.OrganizationInviteLink, error)
	AcceptInviteByToken(ctx context.Context, token, accountID string, name *string) (*workspace_model.OrganizationJoinRequest, error)
}

func validateRegistrationInvitation(ctx context.Context, gateway registrationInvitationGateway, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	if gateway == nil {
		return "", fmt.Errorf("%w: invitation service is unavailable", ErrRegistrationInvitationInvalid)
	}
	if _, err := gateway.ValidateInviteLinkForRegistration(ctx, token); err != nil {
		return "", registrationInvitationError(
			ErrRegistrationInvitationInvalid,
			err,
			"auth.registration.invitation.validate",
		)
	}
	return token, nil
}

func acceptRegistrationInvitation(
	ctx context.Context,
	gateway registrationInvitationGateway,
	token, accountID, name string,
) (*shared_dto.RegistrationInvitation, error) {
	if token == "" {
		return nil, nil
	}
	trimmedName := strings.TrimSpace(name)
	var memberName *string
	if trimmedName != "" {
		memberName = &trimmedName
	}
	req, err := gateway.AcceptInviteByToken(ctx, token, accountID, memberName)
	if err != nil {
		return nil, registrationInvitationError(
			ErrRegistrationInvitationAcceptance,
			err,
			"auth.registration.invitation.accept",
		)
	}
	if req == nil {
		return nil, fmt.Errorf("%w: empty acceptance result", ErrRegistrationInvitationAcceptance)
	}
	return &shared_dto.RegistrationInvitation{
		Status:         string(req.Status),
		OrganizationID: req.OrganizationID,
		WorkspaceID:    req.WorkspaceID,
	}, nil
}

func registrationInvitationError(boundary, cause error, operation string) error {
	wrapped := fmt.Errorf("%w: %w", boundary, cause)
	switch {
	case errors.Is(cause, workspace_service.ErrOrganizationInviteUnavailable):
		return apperror.Wrap(
			wrapped,
			AppCodeRegistrationInvitationUnavailable,
			apperror.WithOperation(operation),
		)
	case errors.Is(cause, workspace_service.ErrMemberNameExists):
		return apperror.Wrap(
			wrapped,
			AppCodeRegistrationMemberNameConflict,
			apperror.WithOperation(operation),
		)
	default:
		return wrapped
	}
}
