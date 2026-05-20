package service

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSSOMobileAlreadyE164(t *testing.T) {
	t.Parallel()

	got, err := normalizeSSOMobile("+1 415 555 2671", "")
	require.NoError(t, err)
	require.Equal(t, "+14155552671", got)
}

func TestNormalizeSSOMobileBuildsFromCountryCode(t *testing.T) {
	t.Parallel()

	got, err := normalizeSSOMobile("138 0013 8000", "86")
	require.NoError(t, err)
	require.Equal(t, "+8613800138000", got)
}

func TestNormalizeSSOMobileBuildsFromISORegionCode(t *testing.T) {
	t.Parallel()

	got, err := normalizeSSOMobile("18735357273", "CN")
	require.NoError(t, err)
	require.Equal(t, "+8618735357273", got)
}

func TestNormalizeSSOMobileRejectsNationalNumberWithoutCountryCode(t *testing.T) {
	t.Parallel()

	_, err := normalizeSSOMobile("13800138000", "")
	require.Error(t, err)
}

func TestBuildFrontendSSORedirectAddsTicket(t *testing.T) {
	t.Parallel()

	got, err := BuildFrontendSSORedirect("https://app.example.com/sso/callback", "ticket-1", "", "")
	require.NoError(t, err)
	require.Equal(t, "https://app.example.com/sso/callback?ticket=ticket-1", got)
}

func TestBuildFrontendSSORedirectPreservesExistingQuery(t *testing.T) {
	t.Parallel()

	got, err := BuildFrontendSSORedirect("https://app.example.com/sso/callback?from=login", "", "invalid_state", "")
	require.NoError(t, err)

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "login", parsed.Query().Get("from"))
	require.Equal(t, "invalid_state", parsed.Query().Get("error"))
}

func TestBuildFrontendSSORedirectAddsReason(t *testing.T) {
	t.Parallel()

	got, err := BuildFrontendSSORedirect("https://app.example.com/sso/callback", "", "exchange_failed", "identity_contact_required")
	require.NoError(t, err)

	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "exchange_failed", parsed.Query().Get("error"))
	require.Equal(t, "identity_contact_required", parsed.Query().Get("reason"))
}

func TestIdentityFromClaimsAcceptsCasdoorPhoneClaims(t *testing.T) {
	t.Parallel()

	identity, err := identityFromClaims(map[string]any{
		"sub":         "sub-1",
		"email":       "User@Example.com",
		"displayName": "Case Door",
		"phone":       "18735357273",
		"countryCode": "CN",
	})
	require.NoError(t, err)
	require.Equal(t, "sub-1", identity.Subject)
	require.Equal(t, "user@example.com", identity.Email)
	require.Equal(t, "Case Door", identity.Name)
	require.Equal(t, "+8618735357273", identity.PhoneNumber)
}

func TestIdentityFromClaimsIgnoresInvalidOptionalPhoneClaims(t *testing.T) {
	t.Parallel()

	identity, err := identityFromClaims(map[string]any{
		"sub":         "sub-2",
		"email":       "test@example.com",
		"displayName": "Test User",
		"phone":       "25458376106",
		"countryCode": "US",
	})
	require.NoError(t, err)
	require.Equal(t, "sub-2", identity.Subject)
	require.Equal(t, "test@example.com", identity.Email)
	require.Equal(t, "Test User", identity.Name)
	require.Empty(t, identity.PhoneNumber)
}

func TestIdentityFromClaimsRejectsInvalidPhoneWithoutEmail(t *testing.T) {
	t.Parallel()

	_, err := identityFromClaims(map[string]any{
		"sub":         "sub-3",
		"displayName": "Phone Only",
		"phone":       "25458376106",
		"countryCode": "US",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrOIDCInvalidPhoneClaim)
}

func TestIdentityFromClaimsRequiresEmailOrPhone(t *testing.T) {
	t.Parallel()

	_, err := identityFromClaims(map[string]any{
		"sub":         "sub-4",
		"displayName": "No Contact",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrOIDCIdentityContactAbsent)
}
