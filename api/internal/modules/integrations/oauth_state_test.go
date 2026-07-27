package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type memoryOAuthStateRepository struct {
	state *IntegrationOAuthState
}

func (repository *memoryOAuthStateRepository) Create(_ context.Context, state *IntegrationOAuthState) error {
	copyValue := *state
	copyValue.RequestedScopes = append([]string(nil), state.RequestedScopes...)
	repository.state = &copyValue
	return nil
}

func (repository *memoryOAuthStateRepository) Consume(_ context.Context, digest, browserBindingDigest string, now time.Time) (*IntegrationOAuthState, error) {
	if repository.state == nil || repository.state.StateDigest != digest ||
		repository.state.BrowserBindingDigest != browserBindingDigest ||
		repository.state.Status != OAuthStatePending || !repository.state.ExpiresAt.After(now) {
		return nil, NewError(ErrorCodeAuthInvalid, "integration OAuth state is expired or already used", nil)
	}
	repository.state.Status = OAuthStateConsumed
	consumedAt := now.UTC()
	repository.state.ConsumedAt = &consumedAt
	copyValue := *repository.state
	copyValue.RequestedScopes = append([]string(nil), repository.state.RequestedScopes...)
	repository.state.EncryptedVerifier = ""
	return &copyValue, nil
}

func TestOAuthStateUsesOneTimeDigestPKCEAndEncryptedVerifier(t *testing.T) {
	repository := &memoryOAuthStateRepository{}
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	service := NewOAuthStateService(repository, cipher, 5*time.Minute).
		WithAllowedRedirectURIs([]string{"https://app.example.com/oauth/callback"})
	organizationID := uuid.New()
	accountID := uuid.New()
	browserBindingDigest := testOAuthBrowserBindingDigest(t, 1)
	authorization, err := service.Create(context.Background(), OAuthStateCreateRequest{
		OrganizationID: organizationID, AccountID: accountID, FlowID: uuid.New(),
		BrowserBindingDigest: browserBindingDigest,
		IntegrationID:        "github", DriverID: "github-rest", AuthMethodID: "github_oauth",
		RedirectURI: "https://app.example.com/oauth/callback", RequestedScopes: []string{"repo", "repo", "user:email"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorization.State == "" || authorization.CodeChallenge == "" || repository.state == nil {
		t.Fatalf("authorization state = %#v persisted=%#v", authorization, repository.state)
	}
	if repository.state.StateDigest != oauthStateDigest(authorization.State) || repository.state.StateDigest == authorization.State {
		t.Fatalf("persisted state is not a digest: %#v", repository.state)
	}
	if repository.state.AuthMethodID != "github_oauth" || len(repository.state.RequestedScopes) != 2 {
		t.Fatalf("persisted OAuth metadata = %#v", repository.state)
	}
	verifierCredentials, err := cipher.DecryptCredentials(repository.state.EncryptedVerifier, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: repository.state.ID, IntegrationID: "github", CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier := verifierCredentials["pkce_verifier"]
	destroyCredentialMap(verifierCredentials)
	challengeDigest := sha256.Sum256([]byte(verifier))
	if got := base64.RawURLEncoding.EncodeToString(challengeDigest[:]); got != authorization.CodeChallenge {
		t.Fatalf("PKCE challenge = %q, want %q", authorization.CodeChallenge, got)
	}

	consumed, err := service.Consume(context.Background(), authorization.State, browserBindingDigest)
	if err != nil {
		t.Fatal(err)
	}
	if consumed.CodeVerifier != verifier || consumed.AuthMethodID != "github_oauth" || consumed.OrganizationID != organizationID || consumed.AccountID != accountID {
		t.Fatalf("consumed OAuth state = %#v", consumed)
	}
	if _, err := service.Consume(context.Background(), authorization.State, browserBindingDigest); ErrorCode(err) != ErrorCodeAuthInvalid {
		t.Fatalf("second Consume() error = %v", err)
	}
	encoded, err := json.Marshal(repository.state)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "{}" {
		t.Fatalf("OAuth state JSON exposed internal material: %s", encoded)
	}
}

func TestOAuthStateRejectsUntrustedRedirects(t *testing.T) {
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	service := NewOAuthStateService(&memoryOAuthStateRepository{}, cipher, time.Minute).
		WithAllowedRedirectURIs([]string{"https://app.example.com/oauth/callback"})
	base := OAuthStateCreateRequest{
		OrganizationID: uuid.New(), AccountID: uuid.New(), FlowID: uuid.New(), IntegrationID: "github", DriverID: "github-rest",
		AuthMethodID: "github_oauth", BrowserBindingDigest: testOAuthBrowserBindingDigest(t, 2),
	}
	withoutPolicy := NewOAuthStateService(&memoryOAuthStateRepository{}, cipher, time.Minute)
	requestWithoutPolicy := base
	requestWithoutPolicy.RedirectURI = "https://app.example.com/oauth/callback"
	if _, err := withoutPolicy.Create(context.Background(), requestWithoutPolicy); ErrorCode(err) != ErrorCodeDisabled {
		t.Fatalf("Create() without redirect policy error = %v", err)
	}
	for _, redirectURI := range []string{
		"https://app.example.com/oauth/other",
		"https://app.example.com/oauth/callback#token",
		"http://evil.example/oauth/callback",
		"javascript:alert(1)",
	} {
		request := base
		request.RedirectURI = redirectURI
		if _, err := service.Create(context.Background(), request); err == nil {
			t.Fatalf("Create(%q) error = nil", redirectURI)
		}
	}
}

func TestOAuthStateRejectsDifferentBrowserWithoutConsumingState(t *testing.T) {
	repository := &memoryOAuthStateRepository{}
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	service := NewOAuthStateService(repository, cipher, time.Minute).
		WithAllowedRedirectURIs([]string{"https://app.example.com/oauth/callback"})
	browserA := testOAuthBrowserBindingDigest(t, 20)
	authorization, err := service.Create(context.Background(), OAuthStateCreateRequest{
		OrganizationID: uuid.New(), AccountID: uuid.New(), FlowID: uuid.New(),
		BrowserBindingDigest: browserA,
		IntegrationID:        "github", DriverID: "github-rest", AuthMethodID: "github_oauth",
		RedirectURI: "https://app.example.com/oauth/callback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Consume(context.Background(), authorization.State, testOAuthBrowserBindingDigest(t, 21)); ErrorCode(err) != ErrorCodeAuthInvalid {
		t.Fatalf("different-browser Consume() error = %v", err)
	}
	if repository.state == nil || repository.state.Status != OAuthStatePending ||
		repository.state.EncryptedVerifier == "" {
		t.Fatalf("different-browser callback consumed state = %#v", repository.state)
	}
	if _, err := service.Consume(context.Background(), authorization.State, browserA); err != nil {
		t.Fatalf("starting-browser Consume() error = %v", err)
	}
}

func TestOAuthBrowserBindingDigestRejectsWeakOrMalformedValues(t *testing.T) {
	for _, value := range []string{"", "default", base64.RawURLEncoding.EncodeToString(make([]byte, 31)), strings.Repeat("a", 64)} {
		if _, err := OAuthBrowserBindingDigest(value); ErrorCode(err) != ErrorCodeAuthInvalid {
			t.Fatalf("OAuthBrowserBindingDigest(%q) error = %v", value, err)
		}
	}
}

func testOAuthBrowserBindingDigest(t *testing.T, marker byte) string {
	t.Helper()
	raw := make([]byte, 32)
	for index := range raw {
		raw[index] = marker
	}
	digest, err := OAuthBrowserBindingDigest(base64.RawURLEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatalf("OAuthBrowserBindingDigest() error = %v", err)
	}
	return digest
}
