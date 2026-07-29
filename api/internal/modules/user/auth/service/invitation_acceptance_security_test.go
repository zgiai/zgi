package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
	auth_repo "github.com/zgiai/zgi/api/internal/modules/user/auth/repository"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	helper "github.com/zgiai/zgi/api/internal/util"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInvitedAccountActivationFailureDoesNotAuthenticateAndKeepsTokenForRetry(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const (
		email = "invitee@example.com"
		token = "retryable-invitation"
	)
	repo := &invitationAcceptanceAccountRepository{
		account:   auth_model.Account{ID: "invitee-1", Email: email, Status: auth_model.AccountStatusPending},
		updateErr: errors.New("database unavailable"),
	}
	require.NoError(t, tokenMgr.StoreInvitationToken("workspace-1", email, repo.account.ID, token, 1))
	service := &AccountService{
		accountRepo:     repo,
		tokenMgr:        tokenMgr,
		registerService: &RegisterServiceImpl{tokenMgr: tokenMgr},
	}

	account, err := service.Authenticate(t.Context(), email, "new-password", token)
	require.ErrorContains(t, err, "failed to activate invited account")
	require.Nil(t, account, "a failed activation must not proceed to login-token issuance")
	require.Equal(t, auth_model.AccountStatusPending, repo.account.Status)
	stored, err := tokenMgr.GetInvitationByToken(token, "", "")
	require.NoError(t, err)
	require.NotNil(t, stored, "the invitation must remain retryable after the database failure")

	repo.updateErr = nil
	account, err = service.Authenticate(t.Context(), email, "new-password", token)
	require.NoError(t, err)
	require.Equal(t, auth_model.AccountStatusActive, account.Status)
	require.Equal(t, auth_model.AccountStatusActive, repo.account.Status)
	stored, err = tokenMgr.GetInvitationByToken(token, "", "")
	require.NoError(t, err)
	require.Nil(t, stored, "successful activation must consume the invitation")

	replay := service.preCheckLogin(t.Context(), &shared_dto.LoginReq{
		Email:       email,
		Password:    "new-password",
		InviteToken: token,
	})
	require.NotNil(t, replay, "a consumed invitation must fail validation on replay")
}

func TestExistingAccountCanAuthenticateAndConsumeDirectedInvitation(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const (
		email    = "existing-invitee@example.com"
		token    = "existing-account-invitation"
		password = "ExistingPassword1"
	)
	hash, salt, err := helper.HashPasswordPBKDF2(password)
	require.NoError(t, err)
	repo := &invitationAcceptanceAccountRepository{account: auth_model.Account{
		ID: "existing-1", Email: email, Status: auth_model.AccountStatusActive,
		Password: &hash, PasswordSalt: &salt,
	}}
	require.NoError(t, tokenMgr.StoreInvitationTokenWithDetails(helper.InvitationData{
		AccountID: repo.account.ID, Email: email, Role: string(workspace_model.WorkspaceRoleMember),
	}, token, 1))
	service := &AccountService{accountRepo: repo, tokenMgr: tokenMgr}

	account, err := service.Authenticate(t.Context(), email, password, token)
	require.NoError(t, err)
	require.Equal(t, repo.account.ID, account.ID)
	state, err := tokenMgr.GetInvitationTokenState(token)
	require.NoError(t, err)
	require.Equal(t, "used", state)
}

func TestActiveAccountCannotUseActivationEndpointToReplacePassword(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const (
		email       = "protected-existing@example.com"
		token       = "protected-existing-invitation"
		oldPassword = "ExistingPassword1"
	)
	hash, salt, err := helper.HashPasswordPBKDF2(oldPassword)
	require.NoError(t, err)
	repo := &invitationAcceptanceAccountRepository{account: auth_model.Account{
		ID: "protected-existing-1", Email: email, Status: auth_model.AccountStatusActive,
		Password: &hash, PasswordSalt: &salt,
	}}
	require.NoError(t, tokenMgr.StoreInvitationToken("", email, repo.account.ID, token, 1))
	service := &AccountService{accountRepo: repo, tokenMgr: tokenMgr}

	_, err = service.Activate(t.Context(), "", email, token, "Attacker", "ReplacedPassword2", "en-US", "UTC")
	require.Error(t, err)
	stored := repo.account
	require.Equal(t, auth_model.AccountStatusActive, stored.Status)
	matches, compareErr := helper.ComparePasswordPBKDF2(oldPassword, *stored.Password, *stored.PasswordSalt)
	require.NoError(t, compareErr)
	require.True(t, matches, "activation must not replace an active account password")
	invitation, getErr := tokenMgr.GetInvitationByToken(token, "", "")
	require.NoError(t, getErr)
	require.NotNil(t, invitation, "rejected activation must leave the invitation available for password-authenticated login")
}

func TestActivePasswordlessAccountCannotSetPasswordThroughInvitationLogin(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const (
		email = "passwordless-existing@example.com"
		token = "passwordless-existing-invitation"
	)
	repo := &invitationAcceptanceAccountRepository{account: auth_model.Account{
		ID: "passwordless-existing-1", Email: email, Status: auth_model.AccountStatusActive,
	}}
	require.NoError(t, tokenMgr.StoreInvitationToken("", email, repo.account.ID, token, 1))
	service := &AccountService{accountRepo: repo, tokenMgr: tokenMgr}

	_, err := service.Authenticate(t.Context(), email, "InjectedPassword1", token)
	require.ErrorContains(t, err, "password not set")
	require.Nil(t, repo.account.Password)
	invitation, getErr := tokenMgr.GetInvitationByToken(token, "", "")
	require.NoError(t, getErr)
	require.NotNil(t, invitation)
}

func TestUnavailableAccountCannotBeReactivatedByInvitation(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const token = "disabled-account-invitation"
	account := auth_model.Account{ID: "disabled-1", Email: "disabled@example.com", Status: auth_model.AccountStatusBanned}
	require.NoError(t, tokenMgr.StoreInvitationToken("", account.Email, account.ID, token, 1))
	service := &AccountService{accountRepo: &invitationAcceptanceAccountRepository{account: account}, tokenMgr: tokenMgr}

	result, valid := service.ActivateCheck(t.Context(), "", account.Email, token)
	require.False(t, valid)
	require.Equal(t, "account_unavailable", result["status"])
}

func TestActivateCheckReturnsExistingAccountInvitationDetails(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const token = "detailed-existing-invitation"
	account := auth_model.Account{ID: "existing-2", Email: "details@example.com", Name: "Invitee", Status: auth_model.AccountStatusActive}
	member := &workspace_model.WorkspaceMember{ID: "member-2", WorkspaceID: "workspace-2", AccountID: account.ID, Role: workspace_model.WorkspaceRoleMember}
	require.NoError(t, tokenMgr.StoreInvitationTokenWithDetails(helper.InvitationData{
		AccountID: account.ID, Email: account.Email, WorkspaceID: member.WorkspaceID, Role: string(member.Role),
	}, token, 1))
	service := &AccountService{
		accountRepo: &invitationAcceptanceAccountRepository{account: account},
		tokenMgr:    tokenMgr,
		workspaceManagementService: &invitationWorkspaceService{
			workspace: &workspace_model.Workspace{ID: member.WorkspaceID, Name: "Invited Workspace", Status: workspace_model.WorkspaceStatusNormal},
			member:    member,
		},
	}

	result, valid := service.ActivateCheck(t.Context(), member.WorkspaceID, account.Email, token)
	require.True(t, valid)
	data, ok := result["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, data["account_exists"])
	require.Equal(t, string(member.Role), data["role"])
	require.Equal(t, "Invited Workspace", data["workspace_name"])
}

func TestActivateCheckReportsBoundEmailMismatch(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const token = "email-bound-invitation"
	require.NoError(t, tokenMgr.StoreInvitationToken("", "invited@example.com", "account-3", token, 1))
	service := &AccountService{tokenMgr: tokenMgr}

	result, valid := service.ActivateCheck(t.Context(), "", "attacker@example.com", token)
	require.False(t, valid)
	require.Equal(t, "email_mismatch", result["status"])
	data, ok := result["data"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "invited@example.com", data["email"])
}

func TestExistingAccountInvitationCreatesMembershipOnlyAfterPasswordVerification(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:existing-invite-membership-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&workspace_model.WorkspaceMember{}))
	workspace := &workspace_model.Workspace{ID: "workspace-deferred", Status: workspace_model.WorkspaceStatusNormal}
	service := &AccountService{
		db:                            db,
		workspaceManagementService:    &invitationWorkspaceService{workspace: workspace},
		organizationManagementService: invitationOrganizationManagementService{},
	}
	invitation := helper.InvitationData{
		AccountID: "active-account", WorkspaceID: workspace.ID,
		InviterID: "inviter-1", Role: string(workspace_model.WorkspaceRoleMember),
	}

	var before int64
	require.NoError(t, db.Model(&workspace_model.WorkspaceMember{}).Count(&before).Error)
	require.Zero(t, before)
	require.NoError(t, service.acceptWorkspaceInvitation(t.Context(), invitation.AccountID, invitation))
	var member workspace_model.WorkspaceMember
	require.NoError(t, db.Where("workspace_id = ? AND account_id = ?", workspace.ID, invitation.AccountID).First(&member).Error)
	require.Equal(t, workspace_model.WorkspaceRoleMember, member.Role)
	require.NotNil(t, member.InvitedBy)
	require.Equal(t, invitation.InviterID, *member.InvitedBy)
}

type invitationOrganizationManagementService struct {
	interfaces.OrganizationManagementService
}

func (s invitationOrganizationManagementService) WithTx(*gorm.DB) interfaces.OrganizationManagementService {
	return s
}

func TestAccountActivateDatabaseFailureReleasesGenericInvitation(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const (
		email = "direct-invitee@example.com"
		token = "direct-retryable-invitation"
	)
	repo := &invitationAcceptanceAccountRepository{
		account:   auth_model.Account{ID: "direct-1", Email: email, Status: auth_model.AccountStatusPending},
		updateErr: errors.New("database unavailable"),
	}
	require.NoError(t, tokenMgr.StoreInvitationToken("", email, repo.account.ID, token, 1))
	service := &AccountService{accountRepo: repo, tokenMgr: tokenMgr}

	_, err := service.Activate(t.Context(), "", email, token, "Invitee", "Password1", "en-US", "UTC")
	require.ErrorContains(t, err, "failed to update account")

	reservation, err := tokenMgr.ReserveInvitationToken(t.Context(), token, "", email)
	require.NoError(t, err, "database failure must release the invitation claim")
	require.NoError(t, tokenMgr.ReleaseInvitationReservation(t.Context(), token, reservation))
}

func TestRegisterActivateDatabaseFailureReleasesInvitation(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const (
		email       = "workspace-invitee@example.com"
		token       = "workspace-retryable-invitation"
		workspaceID = "workspace-1"
	)
	repo := &invitationAcceptanceAccountRepository{
		account:   auth_model.Account{ID: "workspace-invitee-1", Email: email, Status: auth_model.AccountStatusPending},
		updateErr: errors.New("database unavailable"),
	}
	require.NoError(t, tokenMgr.StoreInvitationToken(workspaceID, email, repo.account.ID, token, 1))
	service := &RegisterServiceImpl{
		accountRepo: repo,
		tenantService: &invitationWorkspaceService{workspace: &workspace_model.Workspace{
			ID: workspaceID, Status: workspace_model.WorkspaceStatusNormal,
		}},
		tokenMgr: tokenMgr,
	}

	_, err := service.Activate(t.Context(), workspaceID, email, token, "Invitee", "", "en-US", "UTC")
	require.ErrorContains(t, err, "failed to update account")

	reservation, err := tokenMgr.ReserveInvitationToken(t.Context(), token, workspaceID, email)
	require.NoError(t, err, "database failure must release the invitation claim")
	require.NoError(t, tokenMgr.ReleaseInvitationReservation(t.Context(), token, reservation))
}

func TestJoinByInvitationConsumesOnlyAfterSuccessfulTransaction(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		migrate     bool
		wantErr     bool
		tokenExists bool
	}{
		{name: "commit succeeds", migrate: true, tokenExists: false},
		{name: "transaction fails", migrate: false, wantErr: true, tokenExists: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tokenMgr := newInvitationTestTokenManager(t)
			db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:invite-acceptance-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
			require.NoError(t, err)
			if testCase.migrate {
				require.NoError(t, db.AutoMigrate(&workspace_model.WorkspaceMember{}))
			}

			const token = "join-invitation"
			account := auth_model.Account{ID: "invitee-1", Email: "invitee@example.com", Status: auth_model.AccountStatusPending}
			workspace := &workspace_model.Workspace{ID: "workspace-1", Status: workspace_model.WorkspaceStatusNormal}
			member := &workspace_model.WorkspaceMember{ID: "member-1", WorkspaceID: workspace.ID, AccountID: account.ID}
			require.NoError(t, tokenMgr.StoreInvitationToken(workspace.ID, account.Email, account.ID, token, 1))
			service := &RegisterServiceImpl{
				db:          db,
				accountRepo: &invitationAcceptanceAccountRepository{account: account},
				tenantService: &invitationWorkspaceService{
					workspace: workspace,
					member:    member,
				},
				tokenMgr: tokenMgr,
			}

			_, err = service.JoinByInvitation(t.Context(), account.ID, token)
			if testCase.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			stored, getErr := tokenMgr.GetInvitationByToken(token, "", "")
			require.NoError(t, getErr)
			if testCase.tokenExists {
				require.NotNil(t, stored, "a failed transaction must preserve the invitation")
			} else {
				require.Nil(t, stored, "a committed invitation must be consumed")
				_, replayErr := service.JoinByInvitation(t.Context(), account.ID, token)
				require.Error(t, replayErr)
			}
		})
	}
}

func TestConcurrentInvitationAuthenticationAllowsOnlyOnePasswordMutation(t *testing.T) {
	tokenMgr := newInvitationTestTokenManager(t)
	const (
		email = "concurrent-invitee@example.com"
		token = "concurrent-invitation"
	)
	repo := &concurrentInvitationAccountRepository{account: auth_model.Account{
		ID: "invitee-concurrent", Email: email, Status: auth_model.AccountStatusPending,
	}}
	require.NoError(t, tokenMgr.StoreInvitationToken("workspace-1", email, repo.account.ID, token, 1))
	service := &AccountService{accountRepo: repo, tokenMgr: tokenMgr}

	passwords := []string{"FirstPassword1", "SecondPassword2"}
	type result struct {
		password string
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, len(passwords))
	var wg sync.WaitGroup
	for _, password := range passwords {
		password := password
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.Authenticate(t.Context(), email, password, token)
			results <- result{password: password, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successfulPassword := ""
	for result := range results {
		if result.err == nil {
			require.Empty(t, successfulPassword, "only one concurrent claimant may succeed")
			successfulPassword = result.password
		}
	}
	require.NotEmpty(t, successfulPassword)
	stored := repo.snapshot()
	require.Equal(t, auth_model.AccountStatusActive, stored.Status)
	require.NotNil(t, stored.Password)
	require.NotNil(t, stored.PasswordSalt)
	matches, err := helper.ComparePasswordPBKDF2(successfulPassword, *stored.Password, *stored.PasswordSalt)
	require.NoError(t, err)
	require.True(t, matches, "the only successful claimant must own the persisted password")
}

type invitationAcceptanceAccountRepository struct {
	auth_repo.AccountRepository
	account   auth_model.Account
	updateErr error
}

type concurrentInvitationAccountRepository struct {
	auth_repo.AccountRepository
	mu      sync.Mutex
	account auth_model.Account
}

func (r *concurrentInvitationAccountRepository) GetAccountByEmail(_ context.Context, email string) (*auth_model.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.account.Email != email {
		return nil, gorm.ErrRecordNotFound
	}
	account := r.account
	return &account, nil
}

func (r *concurrentInvitationAccountRepository) UpdateAccount(_ context.Context, account *auth_model.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.account = *account
	return nil
}

func (r *concurrentInvitationAccountRepository) snapshot() auth_model.Account {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.account
}

func (r *invitationAcceptanceAccountRepository) GetAccount(_ context.Context, id string) (*auth_model.Account, error) {
	if r.account.ID != id {
		return nil, gorm.ErrRecordNotFound
	}
	account := r.account
	return &account, nil
}

func (r *invitationAcceptanceAccountRepository) GetAccountByEmail(_ context.Context, email string) (*auth_model.Account, error) {
	if r.account.Email != email {
		return nil, gorm.ErrRecordNotFound
	}
	account := r.account
	return &account, nil
}

func (r *invitationAcceptanceAccountRepository) UpdateAccount(_ context.Context, account *auth_model.Account) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.account = *account
	return nil
}

func (r *invitationAcceptanceAccountRepository) SelectAccountAndTenantAccountJoin(
	_ context.Context,
	invitationData map[string]string,
	_ workspace_model.Workspace,
) (*auth_model.AccountAndJoin, error) {
	if invitationData["email"] != r.account.Email {
		return nil, gorm.ErrRecordNotFound
	}
	account := r.account
	return &auth_model.AccountAndJoin{Account: account}, nil
}
