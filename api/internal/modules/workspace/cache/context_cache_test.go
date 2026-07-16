package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestCurrentOrganizationCacheMissesWithoutRedis(t *testing.T) {
	previousRedis := redisutil.GetClient()
	redisutil.SetClient(nil)
	defer redisutil.SetClient(previousRedis)

	token := NewAccountScopedToken(context.Background(), "account-1")
	if got, ok := GetCurrentOrganization(context.Background(), token); ok || got != nil {
		t.Fatalf("GetCurrentOrganization() = (%v, %v), want nil false", got, ok)
	}
}

func TestCurrentOrganizationCacheInvalidatesByAccountGeneration(t *testing.T) {
	withRedis(t)
	ctx := context.Background()
	accountID := "account-1"
	token := NewAccountScopedToken(ctx, accountID)
	value := &shared_dto.CurrentOrganizationResponse{
		ID:               "organization-1",
		Name:             "Acme",
		Status:           model.OrganizationStatusActive,
		OrganizationRole: model.OrganizationRoleOwner,
	}

	SetCurrentOrganization(ctx, token, value)

	got, ok := GetCurrentOrganization(ctx, token)
	if !ok {
		t.Fatal("GetCurrentOrganization() ok = false, want true")
	}
	if got.ID != value.ID || got.OrganizationRole != value.OrganizationRole {
		t.Fatalf("GetCurrentOrganization() = %+v, want %+v", got, value)
	}

	InvalidateAccount(ctx, accountID)
	newToken := NewAccountScopedToken(ctx, accountID)
	if got, ok := GetCurrentOrganization(ctx, newToken); ok || got != nil {
		t.Fatalf("GetCurrentOrganization() after invalidate = (%v, %v), want nil false", got, ok)
	}
}

func TestAccountContextCacheInvalidatesByAccountGeneration(t *testing.T) {
	withRedis(t)
	ctx := context.Background()
	accountID := "account-1"
	organizationID := "organization-1"
	workspaceID := "workspace-1"
	token := NewAccountScopedToken(ctx, accountID)
	value := &auth_model.AccountContext{
		AccountID:             accountID,
		CurrentOrganizationID: &organizationID,
		CurrentWorkspaceID:    &workspaceID,
	}

	SetAccountContext(ctx, token, value)

	got, ok := GetAccountContext(ctx, token)
	if !ok {
		t.Fatal("GetAccountContext() ok = false, want true")
	}
	if got.AccountID != accountID || got.CurrentOrganizationID == nil || *got.CurrentOrganizationID != organizationID {
		t.Fatalf("GetAccountContext() = %+v, want organization %s", got, organizationID)
	}

	InvalidateAccount(ctx, accountID)
	newToken := NewAccountScopedToken(ctx, accountID)
	if got, ok := GetAccountContext(ctx, newToken); ok || got != nil {
		t.Fatalf("GetAccountContext() after invalidate = (%v, %v), want nil false", got, ok)
	}
}

func TestOrganizationWorkspaceCacheSkipsStaleFillAfterInvalidation(t *testing.T) {
	withRedis(t)
	ctx := context.Background()
	organizationID := "organization-1"
	accountID := "account-1"
	token := NewOrganizationWorkspaceToken(ctx, organizationID, accountID)
	value := &shared_dto.OrganizationWorkspacePaginationResponse{
		Data: []*shared_dto.OrganizationWorkspaceResponse{{
			ID:     "workspace-1",
			Name:   "Workspace",
			Status: string(model.WorkspaceStatusNormal),
		}},
		Page:  1,
		Limit: 20,
		Total: 1,
	}

	InvalidateOrganization(ctx, organizationID)
	SetOrganizationWorkspaces(ctx, token, 1, 20, "", "", value)

	newToken := NewOrganizationWorkspaceToken(ctx, organizationID, accountID)
	if got, ok := GetOrganizationWorkspaces(ctx, newToken, 1, 20, "", ""); ok || got != nil {
		t.Fatalf("GetOrganizationWorkspaces() after stale fill = (%v, %v), want nil false", got, ok)
	}
}

func TestWorkspaceMemberPermissionsCacheInvalidatesByTargetAccountGeneration(t *testing.T) {
	withRedis(t)
	ctx := context.Background()
	organizationID := "organization-1"
	requesterID := "requester-1"
	targetID := "target-1"
	workspaceID := "workspace-1"
	roleID := "role-1"
	token := NewWorkspaceMemberPermissionsToken(ctx, organizationID, workspaceID, requesterID, targetID)
	value := &shared_dto.WorkspaceMemberPermissionsResponse{
		OrganizationID:  organizationID,
		WorkspaceID:     workspaceID,
		AccountID:       targetID,
		WorkspaceRoleID: &roleID,
		Permissions:     []string{"agent.create"},
	}

	SetWorkspaceMemberPermissions(ctx, token, value)

	got, ok := GetWorkspaceMemberPermissions(ctx, token)
	if !ok {
		t.Fatal("GetWorkspaceMemberPermissions() ok = false, want true")
	}
	if got.AccountID != targetID || len(got.Permissions) != 1 || got.Permissions[0] != "agent.create" {
		t.Fatalf("GetWorkspaceMemberPermissions() = %+v, want cached permissions", got)
	}

	InvalidateAccount(ctx, targetID)
	newToken := NewWorkspaceMemberPermissionsToken(ctx, organizationID, workspaceID, requesterID, targetID)
	if got, ok := GetWorkspaceMemberPermissions(ctx, newToken); ok || got != nil {
		t.Fatalf("GetWorkspaceMemberPermissions() after target invalidate = (%v, %v), want nil false", got, ok)
	}
}

func TestWorkspaceMemberPermissionsCacheSkipsStaleFillAfterTargetInvalidation(t *testing.T) {
	withRedis(t)
	ctx := context.Background()
	organizationID := "organization-1"
	requesterID := "requester-1"
	targetID := "target-1"
	workspaceID := "workspace-1"
	token := NewWorkspaceMemberPermissionsToken(ctx, organizationID, workspaceID, requesterID, targetID)
	value := &shared_dto.WorkspaceMemberPermissionsResponse{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		AccountID:      targetID,
		Permissions:    []string{"agent.create"},
	}

	InvalidateAccount(ctx, targetID)
	SetWorkspaceMemberPermissions(ctx, token, value)

	newToken := NewWorkspaceMemberPermissionsToken(ctx, organizationID, workspaceID, requesterID, targetID)
	if got, ok := GetWorkspaceMemberPermissions(ctx, newToken); ok || got != nil {
		t.Fatalf("GetWorkspaceMemberPermissions() after stale fill = (%v, %v), want nil false", got, ok)
	}
}

func withRedis(t *testing.T) {
	t.Helper()

	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
	})

	previousRedis := redisutil.GetClient()
	redisutil.SetClient(client)
	t.Cleanup(func() {
		redisutil.SetClient(previousRedis)
	})
}
