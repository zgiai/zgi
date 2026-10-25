// Package socialoauth owns persisted, one-use social authorization flows.
package socialoauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProviderConfig struct {
	AppID        string
	AppSlug      string
	ClientID     string
	ClientSecret string `json:"-"`
	RedirectURL  string
	Scopes       []string
	APIVersion   string
}
type Lifecycle interface {
	StartKeySetup(context.Context, connections.KeySetupCommand) (connections.KeySetupResult, error)
}
type Options struct {
	LifecycleFactory func(*gorm.DB) (Lifecycle, func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error), error)
	Providers        map[string]ProviderConfig
	FrontendURL      string
	HTTPClient       *http.Client
	Reconnect        func(context.Context, connections.ReconnectCommand) (connections.ReconnectResult, error)
}
type Service struct {
	db        *gorm.DB
	vault     *credentials.Store
	lifecycle Lifecycle
	opts      Options
	now       func() time.Time
}
type Page struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Flow struct {
	InstallURL   string    `json:"install_url,omitempty"`
	ID           uuid.UUID `json:"id"`
	ConnectorID  string    `json:"connector_id"`
	Status       string    `json:"status"`
	AuthorizeURL string    `json:"authorize_url,omitempty"`
	ConnectionID uuid.UUID `json:"connection_id,omitempty"`
	Pages        []Page    `json:"pages,omitempty"`
	ErrorCode    string    `json:"error_code,omitempty"`
	RenewalMode  string    `json:"renewal_mode,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type flowRecord struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID
	OwnerAccountID uuid.UUID
	ConnectorID    string
	StateHash      string `gorm:"uniqueIndex"`
	Status         string
	Envelope       string `json:"-"`
	ConnectionID   uuid.UUID
	ErrorCode      string
	ExpiresAt      time.Time
	CreatedAt      time.Time
}

func (flowRecord) TableName() string { return "connector_social_oauth_flows" }

type grantRecord struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID    uuid.UUID
	ConnectionID      uuid.UUID `gorm:"uniqueIndex"`
	ConnectorID       string
	CredentialVersion int
	CredentialDigest  string
	Envelope          string `json:"-"`
	UpdatedAt         time.Time
}

func (grantRecord) TableName() string { return "connector_social_oauth_grants" }

type secretFlow struct {
	RenewalMode                string
	Verifier                   string
	ReconnectID                uuid.UUID
	ReconnectRevision          int
	ReconnectCredentialVersion int
	Grant                      tokenGrant
	Pages                      []secretPage
}
type secretPage struct {
	Page
	Token string
}
type tokenGrant struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_token_expires_in"`
	Scope            string `json:"scope"`
	ExpiresAt        time.Time
	RefreshExpiresAt time.Time
}

func New(db *gorm.DB, vault *credentials.Store, lifecycle Lifecycle, opts Options) (*Service, error) {
	if db == nil || vault == nil || lifecycle == nil {
		return nil, fail(core.ErrorCodeInternal)
	}
	if opts.FrontendURL != "" {
		u, e := url.Parse(opts.FrontendURL)
		if e != nil || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) {
			return nil, fail(core.ErrorCodeInvalidInput)
		}
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	copyProviders := make(map[string]ProviderConfig)
	for id, c := range opts.Providers {
		if !supported(id) {
			return nil, fail(core.ErrorCodeInvalidInput)
		}
		if c.ClientID == "" && c.ClientSecret == "" && c.RedirectURL == "" {
			continue
		}
		u, e := url.Parse(c.RedirectURL)
		if e != nil || u.User != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1"))) || u.RawQuery != "" || u.Fragment != "" || c.ClientID == "" || c.ClientSecret == "" {
			return nil, fail(core.ErrorCodeInvalidInput)
		}
		if id == "github" && (!regexp.MustCompile(`^[1-9][0-9]{0,17}$`).MatchString(c.AppID) || !regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,98}[a-z0-9])?$`).MatchString(c.AppSlug)) {
			return nil, fail(core.ErrorCodeInvalidInput)
		}
		if id == "facebook-social" {
			if c.APIVersion == "" {
				c.APIVersion = "v23.0"
			}
			if !regexp.MustCompile(`^v[0-9]{1,3}\.[0-9]{1,2}$`).MatchString(c.APIVersion) {
				return nil, fail(core.ErrorCodeInvalidInput)
			}
		}
		c.Scopes = append([]string(nil), c.Scopes...)
		copyProviders[id] = c
	}
	opts.Providers = copyProviders
	return &Service{db: db, vault: vault, lifecycle: lifecycle, opts: opts, now: func() time.Time { return time.Now().UTC() }}, nil
}
func supported(id string) bool {
	return id == "github" || id == "x-social" || id == "linkedin-social" || id == "facebook-social"
}
func (s *Service) Available(id string) bool {
	if s == nil {
		return false
	}
	_, ok := s.opts.Providers[id]
	return ok
}
func fail(code core.ErrorCode) error {
	return core.NewError(code, code == core.ErrorCodeProviderUnavailable || code == core.ErrorCodeInternal, nil)
}
func random() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic("system random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }
func flowSubject(r flowRecord) credentials.Subject {
	return credentials.Subject{Purpose: credentials.OnboardingDevice, OrganizationID: r.OrganizationID, ID: r.ID, ConnectorID: r.ConnectorID, Version: 1}
}
func grantSubject(r grantRecord) credentials.Subject {
	return credentials.Subject{Purpose: credentials.ConnectionKey, OrganizationID: r.OrganizationID, ID: r.ID, ConnectorID: r.ConnectorID, Version: r.CredentialVersion}
}
func (s *Service) seal(subject credentials.Subject, v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", fail(core.ErrorCodeInternal)
	}
	defer clear(b)
	return s.vault.Protect(subject, b)
}
func (s *Service) open(subject credentials.Subject, envelope string, v any) error {
	b, e := s.vault.Open(subject, envelope)
	if e != nil {
		return fail(core.ErrorCodeInternal)
	}
	defer clear(b)
	if json.Unmarshal(b, v) != nil {
		return fail(core.ErrorCodeInternal)
	}
	return nil
}
func (s *Service) Start(ctx context.Context, org, owner uuid.UUID, id string, reconnect uuid.UUID) (Flow, error) {
	if !s.Available(id) {
		return Flow{}, fail(core.ErrorCodeConnectorUnavailable)
	}
	if org == uuid.Nil || owner == uuid.Nil {
		return Flow{}, fail(core.ErrorCodeInvalidInput)
	}
	var reconnectRevision, reconnectCredentialVersion int
	if reconnect != uuid.Nil {
		var c connectionRow
		if e := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ? AND deleted_at IS NULL", reconnect, org, owner, id).Take(&c).Error; e != nil {
			return Flow{}, fail(core.ErrorCodeNotFound)
		}
		reconnectRevision = c.Revision
		reconnectCredentialVersion = c.CredentialVersion
		if s.opts.Reconnect == nil && s.opts.LifecycleFactory == nil {
			return Flow{}, fail(core.ErrorCodeConnectorUnavailable)
		}
	}
	if e := s.CleanupExpired(ctx); e != nil {
		return Flow{}, e
	}
	state := random()
	secret := secretFlow{Verifier: random(), ReconnectID: reconnect, ReconnectRevision: reconnectRevision, ReconnectCredentialVersion: reconnectCredentialVersion}
	r := flowRecord{ID: uuid.New(), OrganizationID: org, OwnerAccountID: owner, ConnectorID: id, StateHash: digest(state), Status: "pending", ExpiresAt: s.now().Add(10 * time.Minute), CreatedAt: s.now()}
	var e error
	r.Envelope, e = s.seal(flowSubject(r), secret)
	if e != nil {
		return Flow{}, e
	}
	if e = s.db.WithContext(ctx).Create(&r).Error; e != nil {
		return Flow{}, fail(core.ErrorCodeInternal)
	}
	f := publicFlow(r)
	f.AuthorizeURL = s.authorize(id, state, secret.Verifier)
	return f, nil
}
func (s *Service) authorize(id, state, verifier string) string {
	c := s.opts.Providers[id]
	q := url.Values{"response_type": {"code"}, "client_id": {c.ClientID}, "redirect_uri": {c.RedirectURL}, "state": {state}}
	scopes := c.Scopes
	endpoint := "https://www.linkedin.com/oauth/v2/authorization"
	switch id {
	case "github":
		endpoint = "https://github.com/login/oauth/authorize"
		scopes = []string{"offline_access"}
		h := sha256.Sum256([]byte(verifier))
		q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(h[:]))
		q.Set("code_challenge_method", "S256")
	case "x-social":
		endpoint = "https://x.com/i/oauth2/authorize"
		if len(scopes) == 0 {
			scopes = []string{"tweet.read", "tweet.write", "users.read", "offline.access"}
		}
		if !contains(scopes, "offline.access") {
			scopes = append(append([]string{}, scopes...), "offline.access")
		}
		h := sha256.Sum256([]byte(verifier))
		q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(h[:]))
		q.Set("code_challenge_method", "S256")
	case "facebook-social":
		endpoint = "https://www.facebook.com/" + c.APIVersion + "/dialog/oauth"
		if len(scopes) == 0 {
			scopes = []string{"pages_show_list", "pages_read_engagement", "pages_manage_posts"}
		}
	default:
		if len(scopes) == 0 {
			scopes = []string{"openid", "profile", "w_member_social"}
		}
	}
	q.Set("scope", strings.Join(scopes, " "))
	return endpoint + "?" + q.Encode()
}
func contains(a []string, v string) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}
func publicFlow(r flowRecord) Flow {
	return Flow{ID: r.ID, ConnectorID: r.ConnectorID, Status: r.Status, ConnectionID: r.ConnectionID, ErrorCode: r.ErrorCode, ExpiresAt: r.ExpiresAt}
}
func (s *Service) Get(ctx context.Context, org, owner, id uuid.UUID) (Flow, error) {
	var r flowRecord
	if e := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND owner_account_id = ?", id, org, owner).Take(&r).Error; e != nil {
		return Flow{}, fail(core.ErrorCodeNotFound)
	}
	f := publicFlow(r)
	if (r.Status == "pending" || r.Status == "select_page" || r.Status == "install_app" || r.Status == "processing") && !s.now().Before(r.ExpiresAt) {
		if e := s.finishFailure(ctx, &r, core.ErrorCodeFlowExpired); e != nil {
			return Flow{}, e
		}
		return s.Get(ctx, org, owner, id)
	}
	if r.Status == "failed" {
		return f, nil
	}
	var secret secretFlow
	if e := s.open(flowSubject(r), r.Envelope, &secret); e != nil {
		return Flow{}, e
	}
	if r.Status == "install_app" {
		f.InstallURL = s.githubInstallURL()
	}
	if r.Status == "select_page" {
		for _, p := range secret.Pages {
			f.Pages = append(f.Pages, p.Page)
		}
	}
	if r.Status == "ready" {
		f.RenewalMode = secret.RenewalMode
	}
	return f, nil
}
func renewal(id string, g tokenGrant) string {
	if id != "facebook-social" && g.RefreshToken != "" {
		return "automatic"
	}
	return "reauthorize"
}
func (s *Service) Complete(ctx context.Context, id, state, code string, denied bool) (uuid.UUID, error) {
	if !supported(id) || len(state) < 32 || len(state) > 256 {
		return uuid.Nil, fail(core.ErrorCodeInvalidInput)
	}
	var r flowRecord
	if e := s.db.WithContext(ctx).Where("state_hash = ? AND connector_id = ?", digest(state), id).Take(&r).Error; e != nil {
		return uuid.Nil, fail(core.ErrorCodeNotFound)
	}
	if !s.now().Before(r.ExpiresAt) {
		return uuid.Nil, fail(core.ErrorCodeFlowExpired)
	}
	claim := s.db.WithContext(ctx).Model(&flowRecord{}).Where("id = ? AND status = ? AND expires_at > ?", r.ID, "pending", s.now()).Update("status", "processing")
	if claim.Error != nil {
		return uuid.Nil, fail(core.ErrorCodeInternal)
	}
	if claim.RowsAffected != 1 {
		return uuid.Nil, fail(core.ErrorCodeFlowExpired)
	}
	r.Status = "processing"
	if denied {
		return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeAccessDenied)
	}
	if code == "" || len(code) > 8192 {
		return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeInvalidInput)
	}
	var secret secretFlow
	if e := s.open(flowSubject(r), r.Envelope, &secret); e != nil {
		return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeInternal)
	}
	g, e := s.exchange(ctx, id, code, secret.Verifier, false)
	if e != nil {
		return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeOf(e))
	}
	secret.Grant = g
	if id == "github" {
		installed, err := s.githubInstalled(ctx, g.AccessToken)
		if err != nil {
			return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeOf(err))
		}
		if !installed {
			if err := s.waitForGitHubInstallation(ctx, &r, secret); err != nil {
				return r.ID, err
			}
			return r.ID, nil
		}
	}
	if id == "facebook-social" {
		pages, e := s.facebookPages(ctx, g.AccessToken)
		if e != nil {
			return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeOf(e))
		}
		secret.Pages = pages
		if len(pages) == 0 {
			return r.ID, s.finishFailure(ctx, &r, core.ErrorCodePermissionMissing)
		}
		r.Status = "select_page"
		r.Envelope, e = s.seal(flowSubject(r), secret)
		if e != nil {
			return r.ID, e
		}
		saved := s.db.WithContext(ctx).Model(&flowRecord{}).Where("id = ? AND status = ? AND expires_at > ?", r.ID, "processing", s.now()).Updates(map[string]any{"status": "select_page", "envelope": r.Envelope})
		if saved.Error != nil {
			return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeInternal)
		}
		if saved.RowsAffected != 1 {
			return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeFlowExpired)
		}
		return r.ID, nil
	}
	e = s.saveConnection(ctx, &r, &secret)
	if e != nil {
		return r.ID, s.finishFailure(ctx, &r, core.ErrorCodeOf(e))
	}
	return r.ID, nil
}
func (s *Service) finishFailure(ctx context.Context, r *flowRecord, code core.ErrorCode) error {
	envelope, e := s.seal(flowSubject(*r), secretFlow{})
	if e != nil {
		return e
	}
	if !code.Valid() {
		code = core.ErrorCodeInternal
	}
	e = s.db.WithContext(ctx).Model(r).Where("status IN ?", []string{"pending", "processing", "select_page", "install_app"}).Updates(map[string]any{"status": "failed", "error_code": string(code), "envelope": envelope}).Error
	if e != nil {
		return fail(core.ErrorCodeInternal)
	}
	return nil
}
func (s *Service) SelectPage(ctx context.Context, org, owner, id uuid.UUID, pageID string) (Flow, error) {
	var r flowRecord
	if e := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND owner_account_id = ?", id, org, owner).Take(&r).Error; e != nil {
		return Flow{}, fail(core.ErrorCodeNotFound)
	}
	var secret secretFlow
	if e := s.open(flowSubject(r), r.Envelope, &secret); e != nil {
		return Flow{}, e
	}
	if !s.now().Before(r.ExpiresAt) {
		s.finishFailure(ctx, &r, core.ErrorCodeFlowExpired)
		return Flow{}, fail(core.ErrorCodeFlowExpired)
	}
	var selected string
	for _, p := range secret.Pages {
		if p.ID == pageID {
			selected = p.Token
		}
	}
	if selected == "" {
		return Flow{}, fail(core.ErrorCodeInvalidInput)
	}
	claim := s.db.WithContext(ctx).Model(&flowRecord{}).Where("id = ? AND status = ? AND expires_at > ?", r.ID, "select_page", s.now()).Update("status", "processing")
	if claim.Error != nil || claim.RowsAffected != 1 {
		return Flow{}, fail(core.ErrorCodeFlowExpired)
	}
	secret.Grant.AccessToken = selected
	secret.Pages = nil
	if e := s.saveConnection(ctx, &r, &secret); e != nil {
		if persistErr := s.finishFailure(ctx, &r, core.ErrorCodeOf(e)); persistErr != nil {
			return Flow{}, persistErr
		}
		return Flow{}, e
	}
	return s.Get(ctx, org, owner, id)
}
func (s *Service) saveConnection(ctx context.Context, r *flowRecord, secret *secretFlow) error {
	if s.opts.LifecycleFactory == nil {
		return fail(core.ErrorCodeInternal)
	}
	completed := *r
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked flowRecord
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND organization_id = ? AND owner_account_id = ?", r.ID, r.OrganizationID, r.OwnerAccountID).Take(&locked).Error; e != nil {
			return fail(core.ErrorCodeNotFound)
		}
		if locked.Status != "processing" || !s.now().Before(locked.ExpiresAt) {
			return fail(core.ErrorCodeFlowExpired)
		}
		if secret.ReconnectID != uuid.Nil {
			var existing connectionRow
			if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ? AND deleted_at IS NULL", secret.ReconnectID, r.OrganizationID, r.OwnerAccountID, r.ConnectorID).Take(&existing).Error; e != nil {
				return fail(core.ErrorCodeNotFound)
			}
			if existing.Revision != secret.ReconnectRevision || existing.CredentialVersion != secret.ReconnectCredentialVersion {
				return fail(core.ErrorCodeConflict)
			}
		}
		lifecycle, reconnect, e := s.opts.LifecycleFactory(tx)
		if e != nil || lifecycle == nil {
			return fail(core.ErrorCodeInternal)
		}
		values := map[string]string{"token": secret.Grant.AccessToken}
		defer core.DestroySecretValues(values)
		var result connections.KeySetupResult
		if secret.ReconnectID != uuid.Nil {
			if reconnect == nil {
				return fail(core.ErrorCodeConnectorUnavailable)
			}
			rr, err := reconnect(ctx, connections.ReconnectCommand{OrganizationID: r.OrganizationID, OwnerAccountID: r.OwnerAccountID, ConnectionID: secret.ReconnectID, IdempotencyKey: "oauth:" + r.ID.String(), Values: values})
			result = rr.KeySetupResult
			e = err
		} else {
			result, e = lifecycle.StartKeySetup(ctx, connections.KeySetupCommand{OrganizationID: r.OrganizationID, OwnerAccountID: r.OwnerAccountID, ConnectorID: r.ConnectorID, IdempotencyKey: "oauth:" + r.ID.String(), Values: values})
		}
		if e != nil {
			return e
		}
		if result.Connection == nil {
			return fail(core.ErrorCodeInvalidKey)
		}
		c := result.Connection
		var current connectionRow
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND organization_id = ? AND owner_account_id = ? AND connector_id = ? AND credential_version = ? AND state = ? AND deleted_at IS NULL", c.ID, r.OrganizationID, r.OwnerAccountID, r.ConnectorID, c.CredentialVersion, core.ReadinessReady).Take(&current).Error; e != nil {
			return fail(core.ErrorCodeConflict)
		}
		g := grantRecord{ID: uuid.New(), OrganizationID: r.OrganizationID, ConnectionID: c.ID, ConnectorID: r.ConnectorID, CredentialVersion: c.CredentialVersion, CredentialDigest: digest(current.EncryptedCredentials), UpdatedAt: s.now()}
		g.Envelope, e = s.seal(grantSubject(g), secret.Grant)
		if e != nil {
			return e
		}
		if e := tx.Where("connection_id = ?", c.ID).Delete(&grantRecord{}).Error; e != nil {
			return fail(core.ErrorCodeInternal)
		}
		if e := tx.Create(&g).Error; e != nil {
			return fail(core.ErrorCodeInternal)
		}
		safe := secretFlow{RenewalMode: renewal(r.ConnectorID, secret.Grant)}
		completed.ConnectionID = c.ID
		completed.Status = "ready"
		completed.ErrorCode = ""
		completed.Envelope, e = s.seal(flowSubject(completed), safe)
		if e != nil {
			return e
		}
		if e := tx.Save(&completed).Error; e != nil {
			return fail(core.ErrorCodeInternal)
		}
		return nil
	})
	if e != nil {
		return e
	}
	*r = completed
	return nil
}

// CleanupExpired removes at most 100 flows per authorization request. A short
// retention window lets an expired completion screen still display a safe error.
func (s *Service) CleanupExpired(ctx context.Context) error {
	cutoff := s.now().Add(-time.Hour)
	ids := s.db.Model(&flowRecord{}).Select("id").Where("expires_at < ?", cutoff).Order("expires_at ASC").Limit(100)
	if e := s.db.WithContext(ctx).Where("id IN (?)", ids).Delete(&flowRecord{}).Error; e != nil {
		return fail(core.ErrorCodeInternal)
	}
	return nil
}
