package service

import (
	"context"
	"fmt"
	"strings"

	platformconsole "github.com/zgiai/ginext/internal/infra/platform/console"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/ginext/internal/modules/workspace/model"
	"github.com/zgiai/ginext/pkg/logger"
)

type officialSignupRegistrationOrganizationLookup interface {
	GetFirstOwnedOrganization(ctx context.Context, accountID string) (*workspace_model.Organization, error)
}

type officialSignupRegistrationConsole interface {
	IsAvailable() bool
	GetMode() string
	NotifyOfficialSignup(ctx context.Context, req *platformconsole.NotifyOfficialSignupRequest) (*platformconsole.NotifyOfficialSignupResponse, error)
}

type officialSignupRegistrationService struct {
	organizations officialSignupRegistrationOrganizationLookup
	console       officialSignupRegistrationConsole
}

func newOfficialSignupRegistrationService(
	organizations officialSignupRegistrationOrganizationLookup,
	console officialSignupRegistrationConsole,
) *officialSignupRegistrationService {
	return &officialSignupRegistrationService{
		organizations: organizations,
		console:       console,
	}
}

func (s *officialSignupRegistrationService) SetOrganizationLookup(organizations officialSignupRegistrationOrganizationLookup) {
	if s == nil {
		return
	}
	s.organizations = organizations
}

func (s *officialSignupRegistrationService) Notify(ctx context.Context, account *auth_model.Account) error {
	if !s.enabled() || account == nil {
		return nil
	}
	if s.organizations == nil {
		return fmt.Errorf("official signup registration organization lookup is not configured")
	}

	organization, err := s.organizations.GetFirstOwnedOrganization(ctx, account.ID)
	if err != nil {
		return fmt.Errorf("failed to load owned organization: %w", err)
	}
	if organization == nil || strings.TrimSpace(organization.ID) == "" {
		return fmt.Errorf("owned organization not found")
	}

	_, err = s.console.NotifyOfficialSignup(ctx, &platformconsole.NotifyOfficialSignupRequest{
		OrganizationID: organization.ID,
		AccountID:      account.ID,
	})
	return err
}

func (s *officialSignupRegistrationService) enabled() bool {
	return s != nil && s.console != nil && s.console.IsAvailable() && strings.EqualFold(s.console.GetMode(), "CLOUD")
}

func (s *AccountService) notifyOfficialSignupRegistration(ctx context.Context, account *auth_model.Account) {
	if s == nil || s.officialSignupRegistration == nil || account == nil {
		return
	}
	if err := s.officialSignupRegistration.Notify(ctx, account); err != nil {
		logger.Warn("Failed to notify official signup registration for account %s: %v", account.ID, err)
	}
}
