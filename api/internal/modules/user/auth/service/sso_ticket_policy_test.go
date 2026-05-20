package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSSOLoginTicketRemainingWindow(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()

	remaining, legacy, err := ssoLoginTicketRemainingWindow(map[string]any{
		ssoLoginTicketIssuedAtUnixKey: now.Add(-2 * time.Minute).Unix(),
	}, now)

	require.NoError(t, err)
	require.False(t, legacy)
	require.Equal(t, time.Minute, remaining)
}

func TestSSOLoginTicketRemainingWindowTreatsMissingIssuedAtAsLegacy(t *testing.T) {
	t.Parallel()

	remaining, legacy, err := ssoLoginTicketRemainingWindow(nil, time.Unix(1_700_000_000, 0).UTC())

	require.NoError(t, err)
	require.True(t, legacy)
	require.Zero(t, remaining)
}

func TestSSOLoginTicketRemainingWindowReturnsZeroAfterGraceWindow(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_700_000_000, 0).UTC()

	remaining, legacy, err := ssoLoginTicketRemainingWindow(map[string]any{
		ssoLoginTicketIssuedAtUnixKey: now.Add(-ssoLoginTicketGraceWindow).Unix(),
	}, now)

	require.NoError(t, err)
	require.False(t, legacy)
	require.Zero(t, remaining)
}
