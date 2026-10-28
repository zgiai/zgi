package service

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	workspacecache "github.com/zgiai/zgi/api/internal/modules/workspace/cache"
	"github.com/zgiai/zgi/api/internal/modules/workspace/memberstatus"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/apperror"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

type memberStatusOrganizationContextService struct {
	fakeOrganizationContextService
	disabled map[string]bool
}

func (f *memberStatusOrganizationContextService) IsOrganizationMemberDisabled(_ context.Context, organizationID, _ string) (bool, error) {
	return f.disabled[organizationID], nil
}

type failingAccountContextRepository struct {
	fakeAccountContextRepository
}

func (f *failingAccountContextRepository) UpdateAccountContext(context.Context, *auth_model.AccountContext) error {
	return errors.New("write failed")
}

func TestCheckOrganizationAccessRejectsDisabledMembershipAndForeignWorkspace(t *testing.T) {
	organizationID := "org-1"
	otherOrganizationID := "org-2"
	workspaceID := "ws-1"
	foreignWorkspaceID := "ws-foreign"
	newService := func(accountID string, currentWorkspaceID string, disabled bool) *AccountService {
		return &AccountService{
			accountRepo: &fakeAccountContextRepository{ctxModel: &auth_model.AccountContext{
				AccountID:             accountID,
				CurrentOrganizationID: &organizationID,
				CurrentWorkspaceID:    &currentWorkspaceID,
			}},
			workspaceManagementService: &fakeWorkspaceContextService{
				workspaces: map[string]*workspace_model.Workspace{
					workspaceID:        {ID: workspaceID, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &organizationID},
					foreignWorkspaceID: {ID: foreignWorkspaceID, Status: workspace_model.WorkspaceStatusNormal, OrganizationID: &otherOrganizationID},
				},
				joins: map[string]bool{workspaceID + ":" + accountID: true, foreignWorkspaceID + ":" + accountID: true},
			},
			organizationService: &memberStatusOrganizationContextService{
				fakeOrganizationContextService: fakeOrganizationContextService{
					members: map[string]bool{organizationID: true, otherOrganizationID: true},
				},
				disabled: map[string]bool{organizationID: disabled},
			},
		}
	}
	ctx := context.Background()

	t.Run("active member", func(t *testing.T) {
		accountID := uuid.NewString()
		require.NoError(t, newService(accountID, workspaceID, false).CheckOrganizationAccess(ctx, accountID, organizationID))
	})

	t.Run("disabled member", func(t *testing.T) {
		accountID := uuid.NewString()
		err := newService(accountID, workspaceID, true).CheckOrganizationAccess(ctx, accountID, organizationID)
		require.True(t, apperror.IsCode(err, memberstatus.AppCodeMemberDisabled), "error = %v", err)
	})

	t.Run("disabled member of an organization outside the account context", func(t *testing.T) {
		accountID := uuid.NewString()
		svc := newService(accountID, workspaceID, false)
		svc.organizationService.(*memberStatusOrganizationContextService).disabled[otherOrganizationID] = true
		err := svc.CheckOrganizationAccess(ctx, accountID, otherOrganizationID)
		require.True(t, apperror.IsCode(err, memberstatus.AppCodeMemberDisabled), "error = %v", err)
	})

	t.Run("organization outside the account context skips the workspace pairing", func(t *testing.T) {
		accountID := uuid.NewString()
		require.NoError(t, newService(accountID, workspaceID, false).CheckOrganizationAccess(ctx, accountID, otherOrganizationID))
	})

	t.Run("cached current workspace from another organization", func(t *testing.T) {
		server := miniredis.RunT(t)
		client := redis.NewClient(&redis.Options{Addr: server.Addr()})
		previousClient := redisUtil.GetClient()
		redisUtil.SetClient(client)
		t.Cleanup(func() {
			redisUtil.SetClient(previousClient)
			_ = client.Close()
		})
		accountID := uuid.NewString()
		svc := newService(accountID, workspaceID, false)
		// A stale cached context bypasses the repair; the guard still refuses
		// a current workspace outside the resolved organization.
		workspacecache.SetAccountContext(ctx, workspacecache.NewAccountScopedToken(ctx, accountID), &auth_model.AccountContext{
			AccountID: accountID, CurrentOrganizationID: &organizationID, CurrentWorkspaceID: &foreignWorkspaceID,
		})

		require.ErrorIs(t, svc.CheckOrganizationAccess(ctx, accountID, organizationID), interfaces.ErrCurrentWorkspaceOutsideOrganization)
	})
}

func TestAccountContextRepairFailsClosedWhenRevocationCannotBePersisted(t *testing.T) {
	ctx := context.Background()
	revokedOrganizationID := "org-revoked"
	accountID := uuid.NewString()
	svc := &AccountService{
		accountRepo: &failingAccountContextRepository{fakeAccountContextRepository: fakeAccountContextRepository{
			ctxModel: &auth_model.AccountContext{AccountID: accountID, CurrentOrganizationID: &revokedOrganizationID},
		}},
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService: &fakeOrganizationContextService{
			members: map[string]bool{revokedOrganizationID: false},
		},
	}

	_, err := svc.GetAccountContext(ctx, accountID)
	require.Error(t, err, "a revoked organization that cannot be cleared must not be served")
	_, err = svc.EnsureCurrentOrganizationID(ctx, accountID)
	require.Error(t, err)
}

func TestAccountContextRepairKeepsServingWhenOnlyADefaultCannotBePersisted(t *testing.T) {
	ctx := context.Background()
	organizationID := "org-default"
	accountID := uuid.NewString()
	svc := &AccountService{
		accountRepo: &failingAccountContextRepository{fakeAccountContextRepository: fakeAccountContextRepository{
			ctxModel: &auth_model.AccountContext{AccountID: accountID},
		}},
		workspaceManagementService: &fakeWorkspaceContextService{},
		organizationService: &fakeOrganizationContextService{
			members:     map[string]bool{organizationID: true},
			firstJoined: &workspace_model.Organization{ID: organizationID},
		},
	}

	ctxModel, err := svc.GetAccountContext(ctx, accountID)
	require.NoError(t, err)
	require.NotNil(t, ctxModel.CurrentOrganizationID)
	require.Equal(t, organizationID, *ctxModel.CurrentOrganizationID)
}

func TestAccountContextRepairRevokedScope(t *testing.T) {
	organizationID, workspaceID, otherID := "org-1", "ws-1", "other"
	require.False(t, accountContextRepairRevokedScope("", "", &auth_model.AccountContext{CurrentOrganizationID: &organizationID}))
	require.False(t, accountContextRepairRevokedScope(organizationID, workspaceID, &auth_model.AccountContext{CurrentOrganizationID: &organizationID, CurrentWorkspaceID: &workspaceID}))
	require.True(t, accountContextRepairRevokedScope(organizationID, "", &auth_model.AccountContext{}))
	require.True(t, accountContextRepairRevokedScope(organizationID, "", &auth_model.AccountContext{CurrentOrganizationID: &otherID}))
	require.True(t, accountContextRepairRevokedScope(organizationID, workspaceID, &auth_model.AccountContext{CurrentOrganizationID: &organizationID}))
}
