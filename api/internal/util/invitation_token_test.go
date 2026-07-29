package util

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

func TestInvitationReservationSupportsGenericLegacyAndDualKeys(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		generic    bool
		legacy     bool
		lookupWith bool
	}{
		{name: "generic only with legacy lookup fallback", generic: true, lookupWith: true},
		{name: "legacy only", legacy: true, lookupWith: true},
		{name: "dual keys", generic: true, legacy: true, lookupWith: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			previousClient := redisUtil.GetClient()
			redisUtil.SetClient(client)
			t.Cleanup(func() {
				_ = client.Close()
				redisUtil.SetClient(previousClient)
			})

			tm := NewTokenManager()
			const (
				token       = "invitation-token"
				workspaceID = "workspace-1"
				email       = "invitee@example.com"
				accountID   = "account-1"
			)
			if testCase.generic {
				require.NoError(t, tm.StoreInvitationToken(workspaceID, email, accountID, token, 1))
			}
			legacyKey := tm.invitationLegacyKey(workspaceID, email, token)
			if testCase.legacy {
				require.NoError(t, client.Set(t.Context(), legacyKey, accountID, time.Hour).Err())
			}

			lookupWorkspace, lookupEmail := "", ""
			if testCase.lookupWith {
				lookupWorkspace, lookupEmail = workspaceID, email
			}
			data, err := tm.GetInvitationByToken(token, lookupWorkspace, lookupEmail)
			require.NoError(t, err)
			require.Equal(t, &InvitationData{AccountID: accountID, Email: email, WorkspaceID: workspaceID}, data)

			genericTTLBefore := server.TTL(tm.getInvitationTokenKey(token))
			legacyTTLBefore := server.TTL(legacyKey)
			reservation, err := tm.ReserveInvitationToken(t.Context(), token, lookupWorkspace, lookupEmail)
			require.NoError(t, err)
			require.Equal(t, accountID, reservation.Data.AccountID)
			_, err = tm.ReserveInvitationToken(t.Context(), token, "", "")
			require.Error(t, err, "all aliases must share one global claim")
			if testCase.generic {
				require.Equal(t, genericTTLBefore, server.TTL(tm.getInvitationTokenKey(token)))
			}
			if testCase.legacy {
				require.Equal(t, legacyTTLBefore, server.TTL(legacyKey))
			}

			require.NoError(t, tm.ConsumeInvitationReservation(t.Context(), token, reservation))
			require.False(t, server.Exists(tm.getInvitationTokenKey(token)))
			require.False(t, server.Exists(legacyKey))
			_, err = tm.ReserveInvitationToken(t.Context(), token, workspaceID, email)
			require.Error(t, err)
		})
	}
}

func TestInvitationReservationReleaseAllowsRetryAndPreservesTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousClient := redisUtil.GetClient()
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(previousClient)
	})

	tm := NewTokenManager()
	require.NoError(t, tm.StoreInvitationToken("workspace-1", "invitee@example.com", "account-1", "retry-token", 1))
	key := tm.getInvitationTokenKey("retry-token")
	ttlBefore := server.TTL(key)
	reservation, err := tm.ReserveInvitationToken(t.Context(), "retry-token", "", "")
	require.NoError(t, err)
	require.NoError(t, tm.ReleaseInvitationReservation(t.Context(), "retry-token", reservation))
	require.Equal(t, ttlBefore, server.TTL(key))

	retry, err := tm.ReserveInvitationToken(t.Context(), "retry-token", "workspace-1", "invitee@example.com")
	require.NoError(t, err)
	require.NoError(t, tm.ConsumeInvitationReservation(t.Context(), "retry-token", retry))
}

func TestInvitationReservationRejectsMismatchedGenericPayload(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousClient := redisUtil.GetClient()
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(previousClient)
	})

	tm := NewTokenManager()
	require.NoError(t, tm.StoreInvitationToken("workspace-1", "invitee@example.com", "account-1", "bound-token", 1))
	_, err := tm.ReserveInvitationToken(t.Context(), "bound-token", "workspace-2", "attacker@example.com")
	require.ErrorContains(t, err, "payload does not match")
	require.Error(t, tm.RevokeInvitationToken("workspace-2", "attacker@example.com", "bound-token"))
	data, getErr := tm.GetInvitationByToken("bound-token", "", "")
	require.NoError(t, getErr)
	require.NotNil(t, data, "mismatched revoke bindings must not delete a generic invitation")
}

func TestRevokeInvitationTokenCleansMatchingDualAliasesExactlyOnce(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousClient := redisUtil.GetClient()
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(previousClient)
	})

	tm := NewTokenManager()
	const (
		token       = "dual-revoke-token"
		workspaceID = "workspace-1"
		email       = "invitee@example.com"
		accountID   = "account-1"
	)
	require.NoError(t, tm.StoreInvitationToken(workspaceID, email, accountID, token, 1))
	legacyKey := tm.invitationLegacyKey(workspaceID, email, token)
	require.NoError(t, client.Set(t.Context(), legacyKey, accountID, time.Hour).Err())

	require.NoError(t, tm.RevokeInvitationToken(workspaceID, email, token))
	require.False(t, server.Exists(tm.getInvitationTokenKey(token)))
	require.False(t, server.Exists(legacyKey))
	require.Error(t, tm.RevokeInvitationToken(workspaceID, email, token))
}

func TestInvitationReservationRejectsNonExpiringSource(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousClient := redisUtil.GetClient()
	redisUtil.SetClient(client)
	t.Cleanup(func() {
		_ = client.Close()
		redisUtil.SetClient(previousClient)
	})

	tm := NewTokenManager()
	key := tm.getInvitationTokenKey("persistent-token")
	require.NoError(t, client.Set(t.Context(), key, `{"account_id":"account-1","email":"invitee@example.com","workspace_id":"workspace-1"}`, 0).Err())
	_, err := tm.ReserveInvitationToken(t.Context(), "persistent-token", "", "")
	require.Error(t, err)
	require.False(t, server.Exists(key), "a non-expiring invitation must not survive reservation validation")
}
