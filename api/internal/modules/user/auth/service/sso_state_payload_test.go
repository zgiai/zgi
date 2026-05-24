package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSOStateCallbackURLFromExtra(t *testing.T) {
	t.Parallel()

	got := ssoStateCallbackURLFromExtra(map[string]any{
		ssoStateFrontendCallbackURLKey: "https://region-a.example.com/sso/callback",
	})

	require.Equal(t, "https://region-a.example.com/sso/callback", got)
}

func TestSSOStateCallbackURLFromExtraReturnsEmptyWhenAbsent(t *testing.T) {
	t.Parallel()

	got := ssoStateCallbackURLFromExtra(nil)

	require.Empty(t, got)
}
