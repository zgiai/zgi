package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/ginext/internal/dto"
)

func TestSSOProviderTokenFromExtra(t *testing.T) {
	got := ssoProviderTokenFromExtra(map[string]any{
		ssoLoginTicketProviderKey: shared_dto.SSOProviderCasdoor,
		ssoLoginTicketIDTokenKey:  "id-token-1",
	})

	require.NotNil(t, got)
	require.Equal(t, shared_dto.SSOProviderCasdoor, got.Provider)
	require.Equal(t, "id-token-1", got.IDToken)
}

func TestSSOProviderTokenFromExtraReturnsNilWhenAbsent(t *testing.T) {
	got := ssoProviderTokenFromExtra(nil)
	require.Nil(t, got)
}
