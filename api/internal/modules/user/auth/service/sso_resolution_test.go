package service

import (
	"testing"

	shared_dto "github.com/zgiai/ginext/internal/dto"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	"github.com/stretchr/testify/require"
)

func TestSelectPreferredSSOAccountPrefersEmailMatch(t *testing.T) {
	t.Parallel()

	emailAccount := &auth_model.Account{ID: "acc-email"}
	mobileAccount := &auth_model.Account{ID: "acc-mobile"}

	got := selectPreferredSSOAccount(emailAccount, mobileAccount)
	require.NotNil(t, got)
	require.Equal(t, "acc-email", got.ID)
}

func TestDefaultSSOAccountNameFallsBackToPhone(t *testing.T) {
	t.Parallel()

	got := defaultSSOAccountName(&shared_dto.SSOIdentity{
		PhoneNumber: "+14155552671",
	})

	require.Equal(t, "user-2671", got)
}
