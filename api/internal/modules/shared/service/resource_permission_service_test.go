package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestResourcePermissionServiceChecksWorkspacePermissions(t *testing.T) {
	t.Parallel()

	auth := &resourcePermissionAuthorizationFixture{allow: true}
	svc := NewResourcePermissionService(auth)

	allowed, err := svc.CheckSingleResourceEditPermission(context.Background(), interfaces.SingleResourcePermissionParams{
		AccountID:      "acc-1",
		TenantID:       "ws-1",
		OrganizationID: "org-1",
		CreatedBy:      "acc-other",
		PermissionCodes: []model.WorkspacePermissionCode{
			model.WorkspacePermissionAgentUpdate,
		},
	})
	if err != nil {
		t.Fatalf("CheckSingleResourceEditPermission error = %v", err)
	}
	if !allowed {
		t.Fatal("CheckSingleResourceEditPermission allowed = false, want true")
	}
	if auth.requireCalls != 1 {
		t.Fatalf("RequireWorkspacePermission calls = %d, want 1", auth.requireCalls)
	}
	wantPermissions := []model.WorkspacePermissionCode{model.WorkspacePermissionAgentUpdate}
	if !reflect.DeepEqual(auth.lastRequest.PermissionCodes, wantPermissions) {
		t.Fatalf("PermissionCodes = %#v, want %#v", auth.lastRequest.PermissionCodes, wantPermissions)
	}
}

func TestResourcePermissionServiceDeniesAuthorizationFailure(t *testing.T) {
	t.Parallel()

	auth := &resourcePermissionAuthorizationFixture{err: ErrAuthorizationDenied}
	svc := NewResourcePermissionService(auth)

	allowed, err := svc.CheckSingleResourceEditPermission(context.Background(), interfaces.SingleResourcePermissionParams{
		AccountID:      "acc-1",
		TenantID:       "ws-1",
		OrganizationID: "org-1",
		CreatedBy:      "acc-other",
		PermissionCodes: []model.WorkspacePermissionCode{
			model.WorkspacePermissionAgentUpdate,
		},
	})
	if err != nil {
		t.Fatalf("CheckSingleResourceEditPermission error = %v", err)
	}
	if allowed {
		t.Fatal("CheckSingleResourceEditPermission allowed = true, want false")
	}
}

func TestResourcePermissionServiceRequiresExplicitPermissionCodes(t *testing.T) {
	t.Parallel()

	auth := &resourcePermissionAuthorizationFixture{allow: true}
	svc := NewResourcePermissionService(auth)

	allowed, err := svc.CheckSingleResourceEditPermission(context.Background(), interfaces.SingleResourcePermissionParams{
		AccountID:      "acc-1",
		TenantID:       "ws-1",
		OrganizationID: "org-1",
		CreatedBy:      "acc-other",
	})
	if err != nil {
		t.Fatalf("CheckSingleResourceEditPermission error = %v", err)
	}
	if allowed {
		t.Fatal("CheckSingleResourceEditPermission allowed = true, want false")
	}
	if auth.requireCalls != 0 {
		t.Fatalf("RequireWorkspacePermission calls = %d, want 0", auth.requireCalls)
	}
}

func TestResourcePermissionServicePropagatesAuthorizationInfrastructureError(t *testing.T) {
	t.Parallel()

	auth := &resourcePermissionAuthorizationFixture{err: errors.New("database down")}
	svc := NewResourcePermissionService(auth)

	_, err := svc.CheckSingleResourceEditPermission(context.Background(), interfaces.SingleResourcePermissionParams{
		AccountID:      "acc-1",
		TenantID:       "ws-1",
		OrganizationID: "org-1",
		CreatedBy:      "acc-other",
		PermissionCodes: []model.WorkspacePermissionCode{
			model.WorkspacePermissionAgentUpdate,
		},
	})
	if err == nil {
		t.Fatal("CheckSingleResourceEditPermission error = nil, want error")
	}
}

type resourcePermissionAuthorizationFixture struct {
	interfaces.AuthorizationService
	allow        bool
	err          error
	requireCalls int
	lastRequest  interfaces.WorkspaceScopeRequest
}

func (f *resourcePermissionAuthorizationFixture) RequireWorkspacePermission(_ context.Context, req interfaces.WorkspaceScopeRequest) (*interfaces.WorkspaceScope, error) {
	f.requireCalls++
	f.lastRequest = req
	if f.err != nil {
		return nil, f.err
	}
	if !f.allow {
		return nil, ErrAuthorizationDenied
	}
	return &interfaces.WorkspaceScope{
		WorkspaceID:       req.WorkspaceID,
		PermissionCodes:   req.PermissionCodes,
		OrganizationScope: interfaces.OrganizationScope{OrganizationID: req.OrganizationID, AccountID: req.AccountID},
	}, nil
}
