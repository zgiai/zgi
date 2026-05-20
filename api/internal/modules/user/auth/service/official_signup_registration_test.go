package service

import (
	"context"
	"errors"
	"testing"

	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type stubOfficialSignupRegistrationOrganizationLookup struct {
	organization *workspace_model.Organization
	err          error
}

func (s *stubOfficialSignupRegistrationOrganizationLookup) GetFirstOwnedOrganization(ctx context.Context, accountID string) (*workspace_model.Organization, error) {
	return s.organization, s.err
}

type stubOfficialSignupRegistrationConsole struct {
	available bool
	mode      string
	err       error
	requests  []*platformconsole.NotifyOfficialSignupRequest
}

func (s *stubOfficialSignupRegistrationConsole) IsAvailable() bool {
	return s.available
}

func (s *stubOfficialSignupRegistrationConsole) GetMode() string {
	return s.mode
}

func (s *stubOfficialSignupRegistrationConsole) NotifyOfficialSignup(ctx context.Context, req *platformconsole.NotifyOfficialSignupRequest) (*platformconsole.NotifyOfficialSignupResponse, error) {
	s.requests = append(s.requests, req)
	if s.err != nil {
		return nil, s.err
	}
	return &platformconsole.NotifyOfficialSignupResponse{
		OrganizationID: req.OrganizationID,
		AccountID:      req.AccountID,
	}, nil
}

func TestOfficialSignupRegistrationService_Notify(t *testing.T) {
	account := &auth_model.Account{
		ID: "acc-1",
		Extensions: auth_model.JSONMap{
			"existing_key": "existing_value",
		},
	}
	orgs := &stubOfficialSignupRegistrationOrganizationLookup{
		organization: &workspace_model.Organization{ID: "org-1"},
	}
	console := &stubOfficialSignupRegistrationConsole{available: true, mode: "CLOUD"}
	service := newOfficialSignupRegistrationService(orgs, console)

	if err := service.Notify(context.Background(), account); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if len(console.requests) != 1 {
		t.Fatalf("console request count = %d, want 1", len(console.requests))
	}
	if console.requests[0].OrganizationID != "org-1" {
		t.Fatalf("organization_id = %q, want org-1", console.requests[0].OrganizationID)
	}
	if console.requests[0].AccountID != "acc-1" {
		t.Fatalf("account_id = %q, want acc-1", console.requests[0].AccountID)
	}
	if got := account.Extensions["existing_key"]; got != "existing_value" {
		t.Fatalf("account extensions mutated to %#v", account.Extensions)
	}
}

func TestOfficialSignupRegistrationService_NotifySkipsWhenNotCloud(t *testing.T) {
	account := &auth_model.Account{ID: "acc-1"}
	orgs := &stubOfficialSignupRegistrationOrganizationLookup{
		organization: &workspace_model.Organization{ID: "org-1"},
	}
	console := &stubOfficialSignupRegistrationConsole{available: true, mode: "SELF_HOSTED"}
	service := newOfficialSignupRegistrationService(orgs, console)

	if err := service.Notify(context.Background(), account); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
	if len(console.requests) != 0 {
		t.Fatalf("console request count = %d, want 0", len(console.requests))
	}
}

func TestOfficialSignupRegistrationService_NotifyPropagatesConsoleError(t *testing.T) {
	account := &auth_model.Account{ID: "acc-1"}
	orgs := &stubOfficialSignupRegistrationOrganizationLookup{
		organization: &workspace_model.Organization{ID: "org-1"},
	}
	console := &stubOfficialSignupRegistrationConsole{
		available: true,
		mode:      "CLOUD",
		err:       errors.New("console unavailable"),
	}
	service := newOfficialSignupRegistrationService(orgs, console)

	err := service.Notify(context.Background(), account)
	if err == nil || err.Error() != "console unavailable" {
		t.Fatalf("err = %v, want console unavailable", err)
	}
	if len(console.requests) != 1 {
		t.Fatalf("console request count = %d, want 1", len(console.requests))
	}
}
