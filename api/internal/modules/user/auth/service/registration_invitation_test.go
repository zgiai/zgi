package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

func TestRegistrationInvitationErrorsPreserveExpectedCauses(t *testing.T) {
	tests := []struct {
		name         string
		validate     bool
		cause        error
		wantBoundary error
		wantCode     apperror.Code
	}{
		{
			name:         "unavailable during validation",
			validate:     true,
			cause:        workspace_service.ErrOrganizationInviteUnavailable,
			wantBoundary: ErrRegistrationInvitationInvalid,
			wantCode:     AppCodeRegistrationInvitationUnavailable,
		},
		{
			name:         "unavailable during acceptance",
			cause:        workspace_service.ErrOrganizationInviteUnavailable,
			wantBoundary: ErrRegistrationInvitationAcceptance,
			wantCode:     AppCodeRegistrationInvitationUnavailable,
		},
		{
			name:         "member name conflict during acceptance",
			cause:        workspace_service.ErrMemberNameExists,
			wantBoundary: ErrRegistrationInvitationAcceptance,
			wantCode:     AppCodeRegistrationMemberNameConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gateway := &registrationInvitationErrorGateway{err: test.cause}
			var err error
			if test.validate {
				_, err = validateRegistrationInvitation(t.Context(), gateway, "invite-token")
			} else {
				_, err = acceptRegistrationInvitation(t.Context(), gateway, "invite-token", "account-id", "Member")
			}

			require.ErrorIs(t, err, test.wantBoundary)
			require.ErrorIs(t, err, test.cause)
			require.True(t, apperror.IsCode(err, test.wantCode))
		})
	}
}

func TestRegistrationInvitationUnknownAcceptanceErrorIsNotPubliclyClassified(t *testing.T) {
	databaseErr := errors.New("database connection contained private diagnostics")
	gateway := &registrationInvitationErrorGateway{err: databaseErr}

	_, err := acceptRegistrationInvitation(t.Context(), gateway, "invite-token", "account-id", "Member")

	require.ErrorIs(t, err, ErrRegistrationInvitationAcceptance)
	require.ErrorIs(t, err, databaseErr)
	_, classified := apperror.As(err)
	require.False(t, classified)
}

type registrationInvitationErrorGateway struct {
	err error
}

func (g *registrationInvitationErrorGateway) ValidateInviteLinkForRegistration(context.Context, string) (*workspace_model.OrganizationInviteLink, error) {
	if g.err != nil {
		return nil, g.err
	}
	return &workspace_model.OrganizationInviteLink{}, nil
}

func (g *registrationInvitationErrorGateway) AcceptInviteByToken(context.Context, string, string, *string) (*workspace_model.OrganizationJoinRequest, error) {
	if g.err != nil {
		return nil, g.err
	}
	return &workspace_model.OrganizationJoinRequest{}, nil
}
