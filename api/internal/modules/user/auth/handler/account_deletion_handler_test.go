package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
)

func TestAccountDeletionHandlerSendsCode(t *testing.T) {
	service := &accountDeletionHandlerService{
		account: &auth_model.Account{ID: "account-1", Email: "user@example.com"},
		token:   "deletion-token",
		code:    "123456",
	}
	handler := NewAccountHandler(service, nil)
	c, recorder := newAccountContextHandlerTestContext(http.MethodPost, "/account/delete/verification-code", nil)
	c.Set("account_id", "account-1")

	handler.SendAccountDeletionCode(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, service.emailSent)
	require.Contains(t, recorder.Body.String(), "deletion-token")
}

func TestAccountDeletionHandlerRequiresValidCodeBeforeDeleting(t *testing.T) {
	service := &accountDeletionHandlerService{}
	handler := NewAccountHandler(service, nil)
	c, recorder := newAccountContextHandlerTestContext(
		http.MethodPost,
		"/account/delete/confirm",
		[]byte(`{"token":"deletion-token","code":"000000"}`),
	)
	c.Set("account_id", "account-1")

	handler.ConfirmAccountDeletion(c)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.False(t, service.deleted)
	require.Equal(t, "account-1", service.verifiedAccountID)

	service.valid = true
	c, recorder = newAccountContextHandlerTestContext(
		http.MethodPost,
		"/account/delete/confirm",
		[]byte(`{"token":"deletion-token","code":"123456"}`),
	)
	c.Set("account_id", "account-1")
	handler.ConfirmAccountDeletion(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.True(t, service.deleted)
}

type accountDeletionHandlerService struct {
	interfaces.AccountService
	account           *auth_model.Account
	token             string
	code              string
	valid             bool
	emailSent         bool
	deleted           bool
	verifiedAccountID string
}

func (f *accountDeletionHandlerService) LoadLoggedInAccount(context.Context, string) (*auth_model.Account, error) {
	return f.account, nil
}

func (f *accountDeletionHandlerService) GenerateAccountDeletionVerificationCode(context.Context, *auth_model.Account) (string, string, error) {
	return f.token, f.code, nil
}

func (f *accountDeletionHandlerService) SendAccountDeletionVerificationEmail(context.Context, *auth_model.Account, string) error {
	f.emailSent = true
	return nil
}

func (f *accountDeletionHandlerService) VerifyAccountDeletionCode(_ context.Context, accountID, _, _ string) (bool, error) {
	f.verifiedAccountID = accountID
	return f.valid, nil
}

func (f *accountDeletionHandlerService) DeleteAccount(context.Context, string) error {
	f.deleted = true
	return nil
}
