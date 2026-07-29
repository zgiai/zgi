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
