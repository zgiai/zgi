package socialoauth

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeLifecycle struct{ calls int }

func (f *fakeLifecycle) StartKeySetup(_ context.Context, c connections.KeySetupCommand) (connections.KeySetupResult, error) {
	f.calls++
	return connections.KeySetupResult{}, nil
}
func fixture(t *testing.T) (*Service, uuid.UUID, uuid.UUID) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&flowRecord{}, &grantRecord{}); e != nil {
		t.Fatal(e)
	}
	v, _ := credentials.NewStore("test", map[string]string{"test": strings.Repeat("x", 32)})
	s, e := New(db, v, &fakeLifecycle{}, Options{Providers: map[string]ProviderConfig{"x-social": {ClientID: "client", ClientSecret: "secret", RedirectURL: "https://api.example/callback"}}, HTTPClient: &http.Client{}})
	if e != nil {
		t.Fatal(e)
	}
	return s, uuid.New(), uuid.New()
}
func TestStartEncryptedStatePKCEAndOwnerIsolation(t *testing.T) {
	s, org, owner := fixture(t)
	f, e := s.Start(context.Background(), org, owner, "x-social", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(f.AuthorizeURL)
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || !strings.Contains(q.Get("scope"), "offline.access") {
		t.Fatalf("bad authorization query %v", q)
	}
	var row flowRecord
	s.db.First(&row, "id = ?", f.ID)
	b, _ := json.Marshal(row)
	if strings.Contains(string(b), q.Get("state")) {
		t.Fatal("plaintext state persisted")
	}
	if _, e = s.Get(context.Background(), org, uuid.New(), f.ID); e == nil {
		t.Fatal("cross-owner flow access")
	}
	if _, e = s.Complete(context.Background(), "linkedin-social", q.Get("state"), "", true); e == nil {
		t.Fatal("cross provider callback")
	}
	id, e := s.Complete(context.Background(), "x-social", q.Get("state"), "", true)
	if e != nil || id != f.ID {
		t.Fatalf("denial %v", e)
	}
	if _, e = s.Complete(context.Background(), "x-social", q.Get("state"), "", true); e == nil {
		t.Fatal("replay accepted")
	}
	f, _ = s.Get(context.Background(), org, owner, f.ID)
	if f.Status != "failed" || f.ErrorCode == "" {
		t.Fatal(f)
	}
}
func TestExpiredFlowAndUnavailableProvider(t *testing.T) {
	s, org, owner := fixture(t)
	if s.Available("facebook-social") {
		t.Fatal("unconfigured")
	}
	if _, e := s.Start(context.Background(), org, owner, "facebook-social", uuid.Nil); e == nil {
		t.Fatal("unconfigured accepted")
	}
	f, _ := s.Start(context.Background(), org, owner, "x-social", uuid.Nil)
	u, _ := url.Parse(f.AuthorizeURL)
	s.db.Model(&flowRecord{}).Where("id = ?", f.ID).Update("expires_at", time.Now().Add(-time.Minute))
	if _, e := s.Complete(context.Background(), "x-social", u.Query().Get("state"), "code", false); e == nil {
		t.Fatal("expired accepted")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(body string, status int) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestExchangeUsesFixedEndpointsAndSanitizesFailure(t *testing.T) {
	s, _, _ := fixture(t)
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.x.com/2/oauth2/token" {
			t.Fatal(r.URL)
		}
		r.ParseForm()
		if r.Form.Get("code_verifier") != "verifier" || r.Form.Get("code") != "code" {
			t.Fatal("missing PKCE")
		}
		if r.Header.Get("Authorization") == "" {
			t.Fatal("missing client authentication")
		}
		return response(`{"access_token":"token-secret","refresh_token":"refresh-secret","expires_in":7200}`, 200), nil
	})
	g, e := s.exchange(context.Background(), "x-social", "code", "verifier", false)
	if e != nil || g.AccessToken != "token-secret" || g.RefreshToken != "refresh-secret" || g.ExpiresAt.IsZero() {
		t.Fatal(g, e)
	}
	s.opts.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(`{"error":"token-secret refresh-secret"}`, 400), nil
	})
	_, e = s.exchange(context.Background(), "x-social", "code", "verifier", false)
	if e == nil || strings.Contains(e.Error(), "secret") {
		t.Fatal(e)
	}
}
func TestRefreshRotationCurrentVersionManualReplacementAndOwner(t *testing.T) {
	s, org, owner := fixture(t)
	if e := s.db.AutoMigrate(&connectionRow{}); e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := s.db.DB()
	sqlDB.SetMaxOpenConns(1)
	id := uuid.New()
	env, e := s.vault.Seal(connections.CredentialSealRequest{OrganizationID: org, ConnectionID: id, ConnectorID: "x-social", CredentialVersion: 1, Values: map[string]string{"token": "old"}})
	if e != nil {
		t.Fatal(e)
	}
	c := connectionRow{Revision: 7, ID: id, OrganizationID: org, OwnerAccountID: owner, ConnectorID: "x-social", State: string(core.ReadinessReady), CredentialVersion: 1, EncryptedCredentials: env}
	if e = s.db.Create(&c).Error; e != nil {
		t.Fatal(e)
	}
	g := grantRecord{ID: uuid.New(), OrganizationID: org, ConnectionID: id, ConnectorID: "x-social", CredentialVersion: 1}
	g.CredentialDigest = digest(c.EncryptedCredentials)
	g.Envelope, e = s.seal(grantSubject(g), tokenGrant{AccessToken: "old", RefreshToken: "refresh-secret", ExpiresAt: time.Now().Add(-time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	s.db.Create(&g)
	var calls atomic.Int32
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		r.ParseForm()
		if r.Form.Get("refresh_token") != "refresh-secret" {
			t.Fatal("bad refresh grant")
		}
		return response(`{"access_token":"new","refresh_token":"rotated","expires_in":7200}`, 200), nil
	})
	req := currentRequest(c)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := s.ResolveCredentials(context.Background(), req)
			if e != nil || v["token"] != "new" {
				t.Errorf("resolve %v %v", v, e)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("refresh not serialized", calls.Load())
	}
	var refreshed connectionRow
	s.db.First(&refreshed, "id = ?", id)
	if refreshed.Revision != c.Revision || refreshed.CredentialVersion != c.CredentialVersion || refreshed.EncryptedCredentials == c.EncryptedCredentials {
		t.Fatal("renewal changed authorization generation or did not rotate token")
	}
	s.db.First(&g, "connection_id = ?", id)
	var token tokenGrant
	if e = s.open(grantSubject(g), g.Envelope, &token); e != nil || token.RefreshToken != "rotated" || g.CredentialVersion != 1 {
		t.Fatal("rotation not saved", e)
	}
	if _, e = s.ResolveConnectionCredentials(context.Background(), core.PersonalConnection{ID: id, OrganizationID: org, OwnerAccountID: uuid.New(), ConnectorID: "x-social", CredentialVersion: 1, EncryptedCredentials: env}); e == nil {
		t.Fatal("cross owner resolve")
	}
	manual, e := s.vault.Seal(connections.CredentialSealRequest{OrganizationID: org, ConnectionID: id, ConnectorID: "x-social", CredentialVersion: 1, Values: map[string]string{"token": "manual"}})
	if e != nil {
		t.Fatal(e)
	}
	s.db.Model(&connectionRow{}).Where("id = ?", id).Updates(map[string]any{"credential_version": 1, "encrypted_credentials": manual})
	if _, e = s.ResolveCredentials(context.Background(), req); e == nil {
		t.Fatal("stale OAuth resurrected")
	}
	req.CredentialVersion = 1
	req.EncryptedCredentials = manual
	v, e := s.ResolveCredentials(context.Background(), req)
	if e != nil || v["token"] != "manual" || calls.Load() != 1 {
		t.Fatal(v, e, calls.Load())
	}
}

type lifecycleRepository struct {
	connections.Repository
	db      *gorm.DB
	attempt core.KeySetupAttempt
}

func (r *lifecycleRepository) ClaimSetupAttempt(_ context.Context, a core.KeySetupAttempt) (core.KeySetupAttempt, bool, error) {
	r.attempt = a
	return a, true, nil
}
func (r *lifecycleRepository) CompleteSetupReady(_ context.Context, org, owner, id uuid.UUID, c core.PersonalConnection) (core.KeySetupAttempt, error) {
	a := r.attempt
	a.State = core.SetupStateReady
	a.ResultConnectionID = c.ID
	return a, r.db.Create(&connectionRow{ID: c.ID, OrganizationID: c.OrganizationID, OwnerAccountID: c.OwnerAccountID, ConnectorID: c.ConnectorID.String(), CredentialVersion: c.CredentialVersion, EncryptedCredentials: c.EncryptedCredentials, State: string(c.State)}).Error
}
func (r *lifecycleRepository) CompleteSetupFailed(_ context.Context, org, owner, id uuid.UUID, code core.ErrorCode) (core.KeySetupAttempt, error) {
	a := r.attempt
	a.State = core.SetupStateFailed
	a.ErrorCode = code
	return a, nil
}

type candidateVerifier struct{ t *testing.T }

func (v candidateVerifier) Verify(_ context.Context, r connections.CandidateVerificationRequest) (connections.CandidateVerificationResult, error) {
	if len(r.Credentials) != 1 || r.Credentials["token"] == "" {
		v.t.Fatal("native credential schema mismatch")
	}
	return connections.CandidateVerificationResult{ExternalAccountID: "account-1", ExternalDisplayName: "Account", UsableOperationIDs: []string{r.ConnectorID + ".account.get"}}, nil
}
func TestCompleteUsesRealNativeLifecycleAndEncryptedGrant(t *testing.T) {
	s, org, owner := fixture(t)
	s.db.AutoMigrate(&connectionRow{})
	defs, e := catalog.New(catalog.SocialMCPDefinitions()...)
	if e != nil {
		t.Fatal(e)
	}
	hasher, _ := connections.NewCommandHasher(strings.Repeat("m", 32))
	lifecycle, e := connections.NewService(defs, &lifecycleRepository{db: s.db}, candidateVerifier{t}, s.vault, hasher)
	if e != nil {
		t.Fatal(e)
	}
	s.lifecycle = lifecycle
	s.opts.LifecycleFactory = func(tx *gorm.DB) (Lifecycle, func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error), error) {
		l, e := connections.NewService(defs, &lifecycleRepository{db: tx}, candidateVerifier{t}, s.vault, hasher)
		return l, nil, e
	}
	s.opts.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(`{"access_token":"token-secret","refresh_token":"refresh-secret","expires_in":7200}`, 200), nil
	})
	f, e := s.Start(context.Background(), org, owner, "x-social", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(f.AuthorizeURL)
	id, e := s.Complete(context.Background(), "x-social", u.Query().Get("state"), "code", false)
	if e != nil {
		t.Fatal(e)
	}
	f, e = s.Get(context.Background(), org, owner, id)
	if e != nil || f.Status != "ready" || f.RenewalMode != "automatic" || f.ConnectionID == uuid.Nil {
		t.Fatal(f, e)
	}
	var grant grantRecord
	s.db.First(&grant, "connection_id = ?", f.ConnectionID)
	if strings.Contains(grant.Envelope, "secret") {
		t.Fatal("unencrypted grant")
	}
	var c connectionRow
	s.db.First(&c, "id = ?", f.ConnectionID)
	v, e := s.ResolveCredentials(context.Background(), currentRequest(c))
	if e != nil || v["token"] != "token-secret" {
		t.Fatal(v, e)
	}
	var row flowRecord
	s.db.First(&row, "id = ?", id)
	var secret secretFlow
	s.open(flowSubject(row), row.Envelope, &secret)
	if secret.Grant.AccessToken != "" || secret.Grant.RefreshToken != "" || len(secret.Pages) != 0 {
		t.Fatal("completed flow retains tokens")
	}
}
func TestFacebookPageSelectionIsOwnerScopedAndOneUse(t *testing.T) {
	s, org, owner := fixture(t)
	s.db.AutoMigrate(&connectionRow{})
	defs, e := catalog.New(catalog.SocialMCPDefinitions()...)
	if e != nil {
		t.Fatal(e)
	}
	h, _ := connections.NewCommandHasher(strings.Repeat("m", 32))
	l, _ := connections.NewService(defs, &lifecycleRepository{db: s.db}, candidateVerifier{t}, s.vault, h)
	s.lifecycle = l
	s.opts.LifecycleFactory = func(tx *gorm.DB) (Lifecycle, func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error), error) {
		l, e := connections.NewService(defs, &lifecycleRepository{db: tx}, candidateVerifier{t}, s.vault, h)
		return l, nil, e
	}
	s.opts.Providers["facebook-social"] = ProviderConfig{ClientID: "fb", ClientSecret: "fb-secret", RedirectURL: "https://api.example/callback", APIVersion: "v23.0"}
	var exchanges int
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v23.0/me/accounts" {
			if r.Header.Get("Authorization") != "Bearer long-user" {
				t.Fatal("Page listing must use long-lived user token")
			}
			return response(`{"data":[{"id":"page-1","name":"Page One","access_token":"page-secret"}]}`, 200), nil
		}
		exchanges++
		r.ParseForm()
		if exchanges == 1 {
			return response(`{"access_token":"short-user","expires_in":3600}`, 200), nil
		}
		if r.Form.Get("fb_exchange_token") != "short-user" {
			t.Fatal("missing long-lived exchange")
		}
		return response(`{"access_token":"long-user","expires_in":5184000}`, 200), nil
	})
	f, e := s.Start(context.Background(), org, owner, "facebook-social", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(f.AuthorizeURL)
	_, e = s.Complete(context.Background(), "facebook-social", u.Query().Get("state"), "code", false)
	if e != nil {
		t.Fatal(e)
	}
	f, e = s.Get(context.Background(), org, owner, f.ID)
	if e != nil || f.Status != "select_page" || len(f.Pages) != 1 {
		t.Fatal(f, e)
	}
	b, _ := json.Marshal(f)
	if strings.Contains(string(b), "secret") || strings.Contains(string(b), "long-user") {
		t.Fatal("public Page leak")
	}
	if _, e = s.SelectPage(context.Background(), org, uuid.New(), f.ID, "page-1"); e == nil {
		t.Fatal("other owner selected")
	}
	if _, e = s.SelectPage(context.Background(), org, owner, f.ID, "bad"); e == nil {
		t.Fatal("unlisted page selected")
	}
	f, e = s.SelectPage(context.Background(), org, owner, f.ID, "page-1")
	if e != nil || f.Status != "ready" {
		t.Fatal(f, e)
	}
	var c connectionRow
	s.db.First(&c, "id = ?", f.ConnectionID)
	v, e := s.ResolveCredentials(context.Background(), currentRequest(c))
	if e != nil || v["token"] != "page-secret" {
		t.Fatal("did not store Page token", v, e)
	}
	if _, e = s.SelectPage(context.Background(), org, owner, f.ID, "page-1"); e == nil {
		t.Fatal("repeat Page selection accepted")
	}
}

func TestNonRefreshableExpiryAndTransientFailurePreserveCredential(t *testing.T) {
	for _, tc := range []struct {
		id, refresh string
		status      int
		want        core.ErrorCode
	}{{"linkedin-social", "", 0, core.ErrorCodeInvalidKey}, {"facebook-social", "unexpected-refresh", 0, core.ErrorCodeInvalidKey}, {"x-social", "refresh", 503, core.ErrorCodeProviderUnavailable}} {
		t.Run(tc.id, func(t *testing.T) {
			s, org, owner := fixture(t)
			s.db.AutoMigrate(&connectionRow{})
			c := connectionRow{ID: uuid.New(), OrganizationID: org, OwnerAccountID: owner, ConnectorID: tc.id, State: string(core.ReadinessReady), CredentialVersion: 1}
			c.EncryptedCredentials, _ = s.vault.Seal(connections.CredentialSealRequest{OrganizationID: org, ConnectionID: c.ID, ConnectorID: tc.id, CredentialVersion: 1, Values: map[string]string{"token": "expired"}})
			s.db.Create(&c)
			g := grantRecord{ID: uuid.New(), OrganizationID: org, ConnectionID: c.ID, ConnectorID: tc.id, CredentialVersion: 1}
			g.CredentialDigest = digest(c.EncryptedCredentials)
			g.Envelope, _ = s.seal(grantSubject(g), tokenGrant{AccessToken: "expired", RefreshToken: tc.refresh, ExpiresAt: time.Now().Add(-time.Hour)})
			s.db.Create(&g)
			var calls int
			s.opts.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return response(`{"error":"secret"}`, tc.status), nil
			})
			_, e := s.ResolveCredentials(context.Background(), currentRequest(c))
			if !core.IsErrorCode(e, tc.want) {
				t.Fatal(e)
			}
			if (tc.status == 0 && calls != 0) || (tc.status != 0 && calls != 1) {
				t.Fatal("unsupported refresh request", calls)
			}
			var after connectionRow
			s.db.First(&after, "id = ?", c.ID)
			if after.CredentialVersion != 1 || after.EncryptedCredentials != c.EncryptedCredentials {
				t.Fatal("failed renewal changed credential")
			}
		})
	}
}
func TestProviderRedirectDoesNotForwardSecrets(t *testing.T) {
	s, _, _ := fixture(t)
	var calls int
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		res := response("", 302)
		res.Header.Set("Location", "https://attacker.invalid/steal")
		return res, nil
	})
	if _, e := s.exchange(context.Background(), "x-social", "code", "verifier", false); e == nil || calls != 1 {
		t.Fatal("redirect followed", e, calls)
	}
}

func TestNativePersistenceRollsBackIfGrantSaveFails(t *testing.T) {
	s, org, owner := fixture(t)
	s.db.AutoMigrate(&connectionRow{})
	defs, e := catalog.New(catalog.SocialMCPDefinitions()...)
	if e != nil {
		t.Fatal(e)
	}
	h, _ := connections.NewCommandHasher(strings.Repeat("m", 32))
	s.opts.LifecycleFactory = func(tx *gorm.DB) (Lifecycle, func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error), error) {
		l, e := connections.NewService(defs, &lifecycleRepository{db: tx}, candidateVerifier{t}, s.vault, h)
		return l, nil, e
	}
	s.opts.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(`{"access_token":"secret","refresh_token":"refresh","expires_in":7200}`, 200), nil
	})
	if e := s.db.Callback().Create().Before("gorm:create").Register("test:fail-oauth-grant", func(tx *gorm.DB) {
		if tx.Statement.Table == "connector_social_oauth_grants" {
			tx.AddError(errors.New("grant persistence unavailable"))
		}
	}); e != nil {
		t.Fatal(e)
	}
	f, e := s.Start(context.Background(), org, owner, "x-social", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(f.AuthorizeURL)
	id, e := s.Complete(context.Background(), "x-social", u.Query().Get("state"), "code", false)
	if e != nil || id != f.ID {
		t.Fatal(e, id)
	}
	var count int64
	s.db.Model(&connectionRow{}).Count(&count)
	if count != 0 {
		t.Fatal("native connection committed without OAuth grant")
	}
	f, e = s.Get(context.Background(), org, owner, id)
	if e != nil || f.Status != "failed" {
		t.Fatal(f, e)
	}
}
func TestFailedAndExpiredProcessingFlowsEraseSecrets(t *testing.T) {
	s, org, owner := fixture(t)
	f, e := s.Start(context.Background(), org, owner, "x-social", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	var r flowRecord
	s.db.First(&r, "id = ?", f.ID)
	r.Status = "processing"
	r.ExpiresAt = time.Now().Add(-time.Minute)
	r.Envelope, _ = s.seal(flowSubject(r), secretFlow{Verifier: "secret-verifier", Grant: tokenGrant{AccessToken: "secret", RefreshToken: "secret"}, Pages: []secretPage{{Page: Page{ID: "page"}, Token: "secret"}}})
	s.db.Save(&r)
	f, e = s.Get(context.Background(), org, owner, f.ID)
	if e != nil || f.Status != "failed" {
		t.Fatal(f, e)
	}
	s.db.First(&r, "id = ?", f.ID)
	var secret secretFlow
	if e = s.open(flowSubject(r), r.Envelope, &secret); e != nil || secret.Verifier != "" || secret.Grant.AccessToken != "" || secret.Grant.RefreshToken != "" || len(secret.Pages) != 0 {
		t.Fatal("expired processing secrets retained", e)
	}
}

func TestCleanupIsBoundedAndCallbackClaimIsAtomic(t *testing.T) {
	s, org, owner := fixture(t)
	sqlDB, _ := s.db.DB()
	sqlDB.SetMaxOpenConns(1)
	old := make([]flowRecord, 105)
	for i := range old {
		old[i] = flowRecord{ID: uuid.New(), OrganizationID: org, OwnerAccountID: owner, ConnectorID: "x-social", StateHash: digest(uuid.NewString()), Status: "pending", Envelope: "expired-encrypted", ExpiresAt: time.Now().UTC().Add(-2 * time.Hour)}
	}
	if e := s.db.Create(&old).Error; e != nil {
		t.Fatal(e)
	}
	if e := s.CleanupExpired(context.Background()); e != nil {
		t.Fatal(e)
	}
	var count int64
	s.db.Model(&flowRecord{}).Count(&count)
	if count != 5 {
		t.Fatal("unbounded cleanup", count)
	}
	f, e := s.Start(context.Background(), org, owner, "x-social", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(f.AuthorizeURL)
	var successes atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.Complete(context.Background(), "x-social", u.Query().Get("state"), "", true); e == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("callback not single use", successes.Load())
	}
}

func TestStaleFailureCannotOverwriteReadyFlow(t *testing.T) {
	s, org, owner := fixture(t)
	f, _ := s.Start(context.Background(), org, owner, "x-social", uuid.Nil)
	var stale flowRecord
	s.db.First(&stale, "id = ?", f.ID)
	s.db.Model(&flowRecord{}).Where("id = ?", f.ID).Updates(map[string]any{"status": "ready", "connection_id": uuid.New()})
	if e := s.finishFailure(context.Background(), &stale, core.ErrorCodeFlowExpired); e != nil {
		t.Fatal(e)
	}
	f, e := s.Get(context.Background(), org, owner, f.ID)
	if e != nil || f.Status != "ready" {
		t.Fatal("stale terminalization overwrote committed authorization", f, e)
	}
}
func TestFacebookExchangeCannotResurrectTerminalFlow(t *testing.T) {
	s, org, owner := fixture(t)
	s.opts.Providers["facebook-social"] = ProviderConfig{ClientID: "fb", ClientSecret: "secret", RedirectURL: "https://api.example/callback", APIVersion: "v23.0"}
	f, e := s.Start(context.Background(), org, owner, "facebook-social", uuid.Nil)
	if e != nil {
		t.Fatal(e)
	}
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/me/accounts") {
			var terminal flowRecord
			s.db.First(&terminal, "id = ?", f.ID)
			if e := s.finishFailure(context.Background(), &terminal, core.ErrorCodeFlowExpired); e != nil {
				t.Fatal(e)
			}
			return response(`{"data":[{"id":"page","name":"Page","access_token":"secret-page"}]}`, 200), nil
		}
		return response(`{"access_token":"secret-user","expires_in":7200}`, 200), nil
	})
	u, _ := url.Parse(f.AuthorizeURL)
	if _, e = s.Complete(context.Background(), "facebook-social", u.Query().Get("state"), "code", false); e != nil {
		t.Fatal(e)
	}
	f, e = s.Get(context.Background(), org, owner, f.ID)
	if e != nil || f.Status != "failed" || len(f.Pages) != 0 {
		t.Fatal("stale provider response resurrected expired flow", f, e)
	}
}

func TestOAuthReconnectRejectsChangedAuthorizationGeneration(t *testing.T) {
	for _, field := range []string{"revision", "credential_version"} {
		t.Run(field, func(t *testing.T) {
			s, org, owner := fixture(t)
			s.db.AutoMigrate(&connectionRow{})
			c := connectionRow{ID: uuid.New(), OrganizationID: org, OwnerAccountID: owner, ConnectorID: "x-social", Revision: 4, CredentialVersion: 3, State: "ready", EncryptedCredentials: "existing-envelope"}
			if e := s.db.Create(&c).Error; e != nil {
				t.Fatal(e)
			}
			g := grantRecord{ID: uuid.New(), OrganizationID: org, ConnectionID: c.ID, ConnectorID: c.ConnectorID, CredentialVersion: 3, CredentialDigest: digest(c.EncryptedCredentials), Envelope: "existing-grant"}
			if e := s.db.Create(&g).Error; e != nil {
				t.Fatal(e)
			}
			var reconnectCalls int
			s.opts.LifecycleFactory = func(*gorm.DB) (Lifecycle, func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error), error) {
				return &fakeLifecycle{}, func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error) {
					reconnectCalls++
					return connections.ReconnectResult{}, fail(core.ErrorCodeInvalidKey)
				}, nil
			}
			f, e := s.Start(context.Background(), org, owner, "x-social", c.ID)
			if e != nil {
				t.Fatal(e)
			}
			if e = s.db.Model(&connectionRow{}).Where("id = ?", c.ID).Update(field, gorm.Expr(field+" + 1")).Error; e != nil {
				t.Fatal(e)
			}
			s.opts.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return response(`{"access_token":"new-token","refresh_token":"new-refresh","expires_in":7200}`, 200), nil
			})
			u, _ := url.Parse(f.AuthorizeURL)
			id, e := s.Complete(context.Background(), "x-social", u.Query().Get("state"), "code", false)
			if e != nil {
				t.Fatal(e)
			}
			if reconnectCalls != 0 {
				t.Fatal("stale OAuth flow invoked native reconnect")
			}
			f, e = s.Get(context.Background(), org, owner, id)
			if e != nil || f.Status != "failed" || f.ErrorCode != string(core.ErrorCodeConflict) {
				t.Fatal(f, e)
			}
			var after grantRecord
			s.db.First(&after, "id = ?", g.ID)
			if after.Envelope != g.Envelope || after.CredentialDigest != g.CredentialDigest {
				t.Fatal("stale OAuth flow replaced newer grant")
			}
		})
	}
}
