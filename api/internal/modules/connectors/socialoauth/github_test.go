package socialoauth

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"gorm.io/gorm"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func githubFixture(t *testing.T) (*Service, uuid.UUID, uuid.UUID) {
	s, o, a := fixture(t)
	s.opts.Providers["github"] = ProviderConfig{ClientID: "client", ClientSecret: "secret", RedirectURL: "https://api.example/console/api/connectors/github/oauth/callback", AppID: "12345", AppSlug: "example-app"}
	s.db.AutoMigrate(&connectionRow{})
	defs, e := catalog.BuiltinDefinitions()
	if e != nil {
		t.Fatal(e)
	}
	registry, e := catalog.New(defs...)
	if e != nil {
		t.Fatal(e)
	}
	h, _ := connections.NewCommandHasher(strings.Repeat("m", 32))
	s.opts.LifecycleFactory = func(tx *gorm.DB) (Lifecycle, func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error), error) {
		l, e := connections.NewService(registry, &lifecycleRepository{db: tx}, githubVerifier{}, s.vault, h)
		return l, nil, e
	}
	return s, o, a
}

type githubVerifier struct{}

func (githubVerifier) Verify(context.Context, connections.CandidateVerificationRequest) (connections.CandidateVerificationResult, error) {
	return connections.CandidateVerificationResult{ExternalAccountID: "123", ExternalDisplayName: "example-user", UsableOperationIDs: []string{"github.user.get", "github.repository.list"}}, nil
}
func TestGitHubAuthorizationPKCEAndRefresh(t *testing.T) {
	s, o, a := githubFixture(t)
	f, e := s.Start(context.Background(), o, a, "github", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(f.AuthorizeURL)
	if u.Host != "github.com" || u.Path != "/login/oauth/authorize" || u.Query().Get("scope") != "offline_access" || u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("code_challenge") == "" {
		t.Fatal("incorrect GitHub authorization URL")
	}
	calls := 0
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		r.ParseForm()
		if r.URL.String() != "https://github.com/login/oauth/access_token" || r.Form.Get("client_secret") != "secret" {
			t.Fatal("unsafe token exchange")
		}
		if calls == 1 && (r.Form.Get("code_verifier") != "verifier" || r.Form.Get("code") != "code" || r.Form.Get("redirect_uri") == "") {
			t.Fatal("missing PKCE")
		}
		if calls == 2 && (r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "refresh") {
			t.Fatal("bad refresh")
		}
		return response(`{"access_token":"access","refresh_token":"refresh","expires_in":28800,"refresh_token_expires_in":15897600}`, 200), nil
	})
	g, e := s.exchange(context.Background(), "github", "code", "verifier", false)
	if e != nil || g.RefreshToken != "refresh" || g.ExpiresAt.Before(s.now().Add(7*time.Hour)) {
		t.Fatal("grant missing", e)
	}
	if _, e = s.exchange(context.Background(), "github", g.RefreshToken, "", true); e != nil {
		t.Fatal(e)
	}
	s.opts.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(`{"error":"bad_verification_code","error_description":"private-secret"}`, 200), nil
	})
	if _, e = s.exchange(context.Background(), "github", "bad", "verifier", false); e == nil || strings.Contains(e.Error(), "private-secret") {
		t.Fatal("unsafe provider error")
	}
}
func TestGitHubInstallationOwnerIsolationAndNativePersistence(t *testing.T) {
	s, o, a := githubFixture(t)
	installed := false
	tokenCalls := 0
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "github.com" {
			tokenCalls++
			return response(`{"access_token":"user-secret","refresh_token":"renew-secret","expires_in":28800}`, 200), nil
		}
		if r.URL.Host != "api.github.com" || r.URL.Path != "/user/installations" || r.Header.Get("Authorization") != "Bearer user-secret" {
			t.Fatal("unexpected installation request")
		}
		if installed {
			return response(`{"total_count":1,"installations":[{"id":17,"app_id":12345,"suspended_at":null}]}`, 200), nil
		}
		return response(`{"total_count":2,"installations":[{"id":9,"app_id":999},{"id":10,"app_id":12345,"suspended_at":"2026-01-01T00:00:00Z"}]}`, 200), nil
	})
	f, e := s.Start(context.Background(), o, a, "github", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(f.AuthorizeURL)
	if _, e = s.Complete(context.Background(), "github", u.Query().Get("state"), "code", false); e != nil {
		t.Fatal(e)
	}
	f, e = s.Get(context.Background(), o, a, f.ID)
	if e != nil || f.Status != "install_app" || f.InstallURL != "https://github.com/apps/example-app/installations/new" || f.ConnectionID != uuid.Nil {
		t.Fatal("installation must precede connection", e)
	}
	if _, e = s.CompleteInstallation(context.Background(), o, uuid.New(), f.ID); e == nil {
		t.Fatal("cross-owner resume")
	}
	if _, e = s.CompleteInstallation(context.Background(), uuid.New(), a, f.ID); e == nil {
		t.Fatal("cross-org resume")
	}
	f, e = s.CompleteInstallation(context.Background(), o, a, f.ID)
	if e != nil || f.Status != "install_app" {
		t.Fatal("not resumable", e)
	}
	installed = true
	f, e = s.CompleteInstallation(context.Background(), o, a, f.ID)
	if e != nil || f.Status != "ready" || f.ConnectionID == uuid.Nil || f.RenewalMode != "automatic" {
		t.Fatal("native lifecycle incomplete", e)
	}
	if tokenCalls != 1 {
		t.Fatal("code exchanged twice")
	}
	var c connectionRow
	s.db.First(&c, "id = ?", f.ConnectionID)
	v, e := s.ResolveCredentials(context.Background(), currentRequest(c))
	if e != nil || v["token"] != "user-secret" {
		t.Fatal("native token unavailable", e)
	}
	if _, e = s.CompleteInstallation(context.Background(), o, a, f.ID); e == nil {
		t.Fatal("replay accepted")
	}
	var row flowRecord
	s.db.First(&row, "id = ?", f.ID)
	var secret secretFlow
	s.open(flowSubject(row), row.Envelope, &secret)
	if secret.Grant.AccessToken != "" || secret.Grant.RefreshToken != "" {
		t.Fatal("completed flow retains tokens")
	}
}
func TestGitHubInstallationFlowExpiry(t *testing.T) {
	s, o, a := githubFixture(t)
	r := flowRecord{ID: uuid.New(), OrganizationID: o, OwnerAccountID: a, ConnectorID: "github", StateHash: random(), Status: "install_app", ExpiresAt: s.now().Add(-time.Minute)}
	r.Envelope, _ = s.seal(flowSubject(r), secretFlow{Grant: tokenGrant{AccessToken: "secret"}})
	s.db.Create(&r)
	f, e := s.Get(context.Background(), o, a, r.ID)
	if e != nil || f.Status != "failed" {
		t.Fatal("expired flow usable", e)
	}
	if _, e = s.CompleteInstallation(context.Background(), o, a, r.ID); e == nil {
		t.Fatal("expired completion accepted")
	}
}

func TestGitHubAutomaticRefreshAndManualPATCompatibility(t *testing.T) {
	s, org, owner := githubFixture(t)
	id := uuid.New()
	env, e := s.vault.Seal(connections.CredentialSealRequest{OrganizationID: org, ConnectionID: id, ConnectorID: "github", CredentialVersion: 1, Values: map[string]string{"token": "old"}})
	if e != nil {
		t.Fatal(e)
	}
	c := connectionRow{ID: id, OrganizationID: org, OwnerAccountID: owner, ConnectorID: "github", State: "ready", CredentialVersion: 1, EncryptedCredentials: env}
	if e = s.db.Create(&c).Error; e != nil {
		t.Fatal(e)
	}
	// Existing manual PAT connections have no OAuth grant and must keep working.
	values, e := s.ResolveCredentials(context.Background(), currentRequest(c))
	if e != nil || values["token"] != "old" {
		t.Fatal("PAT fallback broken", e)
	}
	g := grantRecord{ID: uuid.New(), OrganizationID: org, ConnectionID: id, ConnectorID: "github", CredentialVersion: 1, CredentialDigest: digest(env)}
	g.Envelope, e = s.seal(grantSubject(g), tokenGrant{AccessToken: "old", RefreshToken: "refresh-old", ExpiresAt: s.now().Add(-time.Minute), RefreshExpiresAt: s.now().Add(24 * time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.db.Create(&g).Error; e != nil {
		t.Fatal(e)
	}
	calls := 0
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		r.ParseForm()
		if r.URL.String() != "https://github.com/login/oauth/access_token" || r.Form.Get("refresh_token") != "refresh-old" {
			t.Fatal("wrong refresh request")
		}
		return response(`{"access_token":"renewed","refresh_token":"refresh-new","expires_in":28800,"refresh_token_expires_in":15897600}`, 200), nil
	})
	for i := 0; i < 2; i++ {
		values, e = s.ResolveCredentials(context.Background(), currentRequest(c))
		if e != nil || values["token"] != "renewed" {
			t.Fatal("renewal unavailable", e)
		}
	}
	if calls != 1 {
		t.Fatal("refresh not reused")
	}
	s.db.First(&g, "connection_id = ?", id)
	var renewed tokenGrant
	if e = s.open(grantSubject(g), g.Envelope, &renewed); e != nil || renewed.RefreshToken != "refresh-new" {
		t.Fatal("refresh rotation not saved", e)
	}
	var saved connectionRow
	s.db.First(&saved, "id = ?", id)
	if saved.CredentialVersion != c.CredentialVersion || saved.EncryptedCredentials == env {
		t.Fatal("renewal generation invalid")
	}
}
