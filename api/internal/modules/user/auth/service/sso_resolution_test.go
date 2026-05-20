package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	auth_model "github.com/zgiai/zgi/api/internal/modules/user/auth/model"
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
