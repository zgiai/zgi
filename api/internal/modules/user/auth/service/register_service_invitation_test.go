package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	usererrors "github.com/zgiai/zgi/api/internal/errors"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInviteExistingPendingMemberCommitsTransactionAndKeepsDurableTokenOnEmailFailure(t *testing.T) {
	db, sqlDB := newInvitationTestDB(t)
	tokenMgr := newInvitationTestTokenManager(t)
	workspace := &workspace_model.Workspace{ID: "workspace-1", Name: "Workspace", Status: workspace_model.WorkspaceStatusNormal}
	invitee := &auth_model.Account{ID: "invitee-1", Email: "invitee@example.com", Status: auth_model.AccountStatusPending}
	service := &RegisterServiceImpl{
		db: db,
		accountRepo: &invitationAccountRepository{
			inviter: &auth_model.Account{ID: "inviter-1", Name: "Inviter", Status: auth_model.AccountStatusActive},
			invitee: invitee,
		},
		tenantService: &invitationWorkspaceService{
			workspace: workspace,
			member:    &workspace_model.WorkspaceMember{ID: "member-1", WorkspaceID: workspace.ID, AccountID: invitee.ID},
		},
		tokenMgr: tokenMgr,
		inviteEmailSender: func(string, string, string, string, string) error {
			return errors.New("provider unavailable")
		},
	}

	token, err := service.InviteMemberEx(
		t.Context(), workspace.ID, "inviter-1", invitee.Email, workspace_model.WorkspaceRoleNormal,
		"en-US", invitee.Name, "", "", "", true,
	)
	require.ErrorIs(t, err, usererrors.ErrInviteEmailDeliveryFailed)
	require.NotEmpty(t, token)
	require.Zero(t, sqlDB.Stats().InUse, "the repeated-invite transaction must be committed")

	stored, err := tokenMgr.GetInvitationByToken(token, "", "")
	require.NoError(t, err)
	require.Equal(t, invitee.ID, stored.AccountID)
}

func TestInviteExistingActiveAccountDefersMembershipUntilAcceptance(t *testing.T) {
	db, _ := newInvitationTestDB(t)
	tokenMgr := newInvitationTestTokenManager(t)
	organizationID := "organization-1"
	workspace := &workspace_model.Workspace{
		ID: "workspace-active", Name: "Workspace", Status: workspace_model.WorkspaceStatusNormal,
		OrganizationID: &organizationID,
	}
	invitee := &auth_model.Account{ID: "active-invitee-1", Email: "active@example.com", Status: auth_model.AccountStatusActive}
	service := &RegisterServiceImpl{
		db: db,
		accountRepo: &invitationAccountRepository{
			inviter: &auth_model.Account{ID: "inviter-1", Name: "Inviter", Status: auth_model.AccountStatusActive},
			invitee: invitee,
		},
		tenantService: &invitationWorkspaceService{workspace: workspace},
		tokenMgr:      tokenMgr,
	}

	token, err := service.InviteMemberEx(
		t.Context(), workspace.ID, "inviter-1", invitee.Email, workspace_model.WorkspaceRoleMember,
		"en-US", invitee.Name, "", "", "", false,
	)
	require.NoError(t, err, "inviting an active account must not require a membership write")
	stored, err := tokenMgr.GetInvitationByToken(token, "", "")
	require.NoError(t, err)
	require.Equal(t, invitee.ID, stored.AccountID)
	require.Equal(t, workspace.ID, stored.WorkspaceID)
	require.Equal(t, organizationID, stored.OrganizationID)
	require.Equal(t, "inviter-1", stored.InviterID)
	require.Equal(t, string(workspace_model.WorkspaceRoleMember), stored.Role)
}

func TestGenerateInviteTokenReturnsRedisFailureInsteadOfFakeToken(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	redisUtil.SetClient(client)
	server.Close()
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})

	service := &RegisterServiceImpl{tokenMgr: helper.NewTokenManager()}
	token, err := service.generateInviteToken(
		&workspace_model.Workspace{ID: "workspace-1"},
		&auth_model.Account{ID: "account-1", Email: "user@example.com"},
	)
	require.Error(t, err)
	require.Empty(t, token)
}

func newInvitationTestDB(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:invite-transaction?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, sqlDB
}

func newInvitationTestTokenManager(t *testing.T) *helper.TokenManager {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(nil)
	})
	return helper.NewTokenManager()
}

type invitationAccountRepository struct {
	auth_repo.AccountRepository
	inviter *auth_model.Account
	invitee *auth_model.Account
}

func (r *invitationAccountRepository) GetAccount(_ context.Context, id string) (*auth_model.Account, error) {
	if r.inviter != nil && r.inviter.ID == id {
		return r.inviter, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *invitationAccountRepository) GetAccountByEmail(_ context.Context, email string) (*auth_model.Account, error) {
	if r.invitee != nil && r.invitee.Email == email {
		return r.invitee, nil
	}
	return nil, gorm.ErrRecordNotFound
}

type invitationWorkspaceService struct {
	interfaces.WorkspaceManagementService
	workspace *workspace_model.Workspace
	member    *workspace_model.WorkspaceMember
}

func (s *invitationWorkspaceService) GetWorkspaceByID(context.Context, string) (*workspace_model.Workspace, error) {
	return s.workspace, nil
}

func (s *invitationWorkspaceService) CheckMemberPermission(context.Context, *workspace_model.Workspace, *auth_model.Account, *auth_model.Account, string) error {
	return nil
}

func (s *invitationWorkspaceService) GetByWorkspaceAndMember(context.Context, string, string) (*workspace_model.WorkspaceMember, error) {
	return s.member, nil
}

func (s *invitationWorkspaceService) WithTx(*gorm.DB) interfaces.WorkspaceManagementService {
	return s
}
