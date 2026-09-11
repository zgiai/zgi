// Package mailprotocol implements bounded mail access for compiled provider profiles.
// It never consumes notification credentials, OAuth tokens or arbitrary hosts.
package mailprotocol

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	imapid "github.com/emersion/go-imap-id"
	"io"
	"net"
	"net/smtp"
	"net/textproto"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/client"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

const maxMessageBytes = 1 << 20
const maxTextBytes = 256 << 10
const maxHeaderBytes = 64 << 10

type Dialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}
type Options struct {
	Provider  string
	Dialer    Dialer
	RootCAs   *x509.CertPool
	SMTPPort  int
	HandleKey string
	Now       func() time.Time
}
type Client struct {
	profile  catalog.MailProfile
	dialer   Dialer
	roots    *x509.CertPool
	smtpPort int
	handles  cipher.AEAD
	now      func() time.Time
	schemas  *providers.Schemas
	mu       sync.Mutex
	active   map[string]bool
}

func New(options Options) (*Client, error) {
	if options.Provider == "" {
		options.Provider = "gmail"
	}
	profile, ok := catalog.MailProfileFor(options.Provider)
	if !ok {
		return nil, providers.Invalid()
	}
	if options.SMTPPort == 0 {
		options.SMTPPort = 465
	}
	if options.SMTPPort != 465 && (options.SMTPPort != 587 || profile.ID != "gmail") || len(options.HandleKey) != 32 {
		return nil, providers.Invalid()
	}
	if options.Dialer == nil {
		options.Dialer = &net.Dialer{Timeout: 15 * time.Second}
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	key := sha256.Sum256([]byte("zgi/" + profile.ID + "/reference/v1\x00" + options.HandleKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	schemas, err := providers.NewSchemas(profile.ID)
	if err != nil {
		return nil, err
	}
	return &Client{profile: profile, dialer: options.Dialer, roots: options.RootCAs, smtpPort: options.SMTPPort, handles: aead, now: options.Now, schemas: schemas, active: map[string]bool{}}, nil
}
func (c *Client) ConnectorID() string { return c.profile.ID }

func (c *Client) VerifyCredentials(ctx context.Context, in providers.CredentialInput) (providers.Verification, error) {
	if catalog.NormalizeMailValuesFor(c.profile.ID, in.Values) != nil || len(catalog.MailOperationsFor(c.profile.ID, in.MailUsage)) == 0 {
		return providers.Verification{}, providers.Invalid()
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	evidence := &core.MailEvidence{Version: 1, Provider: c.profile.ID, IdentityKind: "authenticated_login", AuthenticatedLogin: in.Values["email"], Usage: in.MailUsage, ScopeGranularity: "not_available", IMAPStatus: "not_requested", SMTPStatus: "not_requested", CheckedAt: c.now().UTC(), Operations: []core.OperationEvidence{}}
	var firstError error
	if in.MailUsage == "read" || in.MailUsage == "read_send" {
		cl, closeSession, err := c.openIMAP(ctx, in)
		if err == nil {
			_, err = cl.Select("INBOX", true)
			closeSession()
		}
		if err != nil {
			evidence.IMAPStatus = "failed"
			firstError = c.readFailure(err)
		} else {
			evidence.IMAPStatus = "verified"
		}
	}
	if in.MailUsage == "send" || in.MailUsage == "read_send" {
		cl, _, closeSession, err := c.openSMTP(ctx, in)
		if err == nil {
			_ = cl.Quit()
			closeSession()
		}
		if err != nil {
			evidence.SMTPStatus = "failed"
			if firstError == nil {
				firstError = c.readFailure(err)
			}
		} else {
			evidence.SMTPStatus = "verified"
		}
	}
	evidence.CredentialVerified = firstError == nil
	result := providers.Verification{AccountID: in.Values["email"], DisplayName: in.Values["email"], Evidence: &core.ProviderEvidence{Mail: evidence}}
	if firstError != nil {
		return result, firstError
	}
	for _, id := range catalog.MailOperationsFor(c.profile.ID, in.MailUsage) {
		availability := "verified"
		if id == c.profile.ID+".message.send" {
			availability = "conditional"
		}
		evidence.Operations = append(evidence.Operations, core.OperationEvidence{ID: id, Availability: availability, Permission: "not_scoped", Resource: "evaluated_at_execution"})
	}
	result.Operations = catalog.MailOperationsFor(c.profile.ID, in.MailUsage)
	return result, nil
}

func (c *Client) InvokeCredentials(ctx context.Context, operation string, in providers.CredentialInput, args map[string]any) (providers.Response, error) {
	if catalog.NormalizeMailValuesFor(c.profile.ID, in.Values) != nil {
		return providers.Response{}, providers.Invalid()
	}
	if in.ResourceScope.OrganizationID == uuid.Nil || in.ResourceScope.OwnerAccountID == uuid.Nil || in.ResourceScope.ConnectionID == uuid.Nil || in.ResourceScope.CredentialVersion < 1 || in.ResourceScope.Target.Validate() != nil {
		return providers.Response{}, providers.Invalid()
	}
	input, err := c.schemas.Input(operation, args)
	if err != nil {
		return providers.Response{}, err
	}
	var value map[string]any
	if operation == c.profile.ID+".message.send" {
		value, err = c.send(ctx, in, input)
	} else {
		value, err = c.read(ctx, in, operation, input)
	}
	if err != nil {
		return providers.Response{}, err
	}
	value["provider"], value["operation"] = c.profile.ID, operation
	if err = c.schemas.Output(operation, value); err != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	return providers.Response{Value: value}, nil
}

// The application owns cross-worker replay and permission checks. This local
// gate additionally prevents one process opening duplicate protocol sessions.
func (c *Client) acquire(in providers.CredentialInput, protocol string) (func(), error) {
	key := in.ResourceScope.OrganizationID.String() + ":" + in.ResourceScope.ConnectionID.String() + ":" + protocol
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active[key] {
		return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Retryable: true}
	}
	c.active[key] = true
	return func() { c.mu.Lock(); delete(c.active, key); c.mu.Unlock() }, nil
}
func (c *Client) connect(ctx context.Context, host string, port int, implicitTLS bool) (net.Conn, func(), error) {
	conn, err := c.dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return nil, nil, protocolFailure(err, false)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	closeSession := func() { stop(); _ = conn.Close() }
	deadline := time.Now().Add(30 * time.Second)
	if v, ok := ctx.Deadline(); ok && v.Before(deadline) {
		deadline = v
	}
	_ = conn.SetDeadline(deadline)
	if implicitTLS {
		secured := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, RootCAs: c.roots})
		if err := secured.HandshakeContext(ctx); err != nil {
			closeSession()
			return nil, nil, &providers.Failure{Code: core.ErrorCodeMailTLSRequired}
		}
		return secured, closeSession, nil
	}
	return conn, closeSession, nil
}
func (c *Client) openIMAP(ctx context.Context, in providers.CredentialInput) (*client.Client, func(), error) {
	release, err := c.acquire(in, "imap")
	if err != nil {
		return nil, nil, err
	}
	conn, closeConn, err := c.connect(ctx, c.profile.IMAPHost, 993, true)
	if err != nil {
		release()
		return nil, nil, err
	}
	var closeOnce sync.Once
	closeSession := func() { closeOnce.Do(func() { closeConn(); release() }) }
	cl, err := newPrivateIMAPClient(conn)
	if err != nil {
		closeSession()
		return nil, nil, protocolFailure(err, false)
	}
	cl.Timeout = 30 * time.Second
	if err = cl.Login(in.Values["email"], in.Values[c.profile.PasswordField]); err != nil {
		closeSession()
		return nil, nil, c.authenticationFailure(err)
	}

	if c.profile.RequireIMAPID {
		idClient := imapid.NewClient(cl)
		supported, supportErr := idClient.SupportID()
		if supportErr != nil {
			closeSession()
			return nil, nil, c.readFailure(supportErr)
		}
		if !supported {
			closeSession()
			return nil, nil, &providers.Failure{Code: core.ErrorCodeMailClientIdentityRequired}
		}
		identity := imapid.ID{imapid.FieldName: "ZGI"}
		if build, ok := debug.ReadBuildInfo(); ok && build.Main.Version != "" && build.Main.Version != "(devel)" {
			identity[imapid.FieldVersion] = build.Main.Version
		}
		serverIdentity, idErr := idClient.ID(identity)
		if idErr != nil {
			closeSession()
			return nil, nil, c.readFailure(idErr)
		}
		valid := len(serverIdentity) <= 30
		for key, value := range serverIdentity {
			valid = valid && len(key) <= 30 && len(value) <= 1024
		}
		if !valid {
			closeSession()
			return nil, nil, &providers.Failure{Code: core.ErrorCodeMailClientIdentityRequired}
		}
	}
	return cl, closeSession, nil
}
func (c *Client) openSMTP(ctx context.Context, in providers.CredentialInput) (*smtp.Client, net.Conn, func(), error) {
	release, err := c.acquire(in, "smtp")
	if err != nil {
		return nil, nil, nil, err
	}
	conn, closeConn, err := c.connect(ctx, c.profile.SMTPHost, c.smtpPort, c.smtpPort == 465)
	if err != nil {
		release()
		return nil, nil, nil, err
	}
	var closeOnce sync.Once
	closeSession := func() { closeOnce.Do(func() { closeConn(); release() }) }
	cl, err := smtp.NewClient(conn, c.profile.SMTPHost)
	if err != nil {
		closeSession()
		return nil, nil, nil, protocolFailure(err, false)
	}
	if c.smtpPort == 587 {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			closeSession()
			return nil, nil, nil, &providers.Failure{Code: core.ErrorCodeMailTLSRequired}
		}
		if err = cl.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.profile.SMTPHost, RootCAs: c.roots}); err != nil {
			closeSession()
			return nil, nil, nil, &providers.Failure{Code: core.ErrorCodeMailTLSRequired}
		}
	}
	if state, ok := cl.TLSConnectionState(); !ok || !state.HandshakeComplete {
		closeSession()
		return nil, nil, nil, &providers.Failure{Code: core.ErrorCodeMailTLSRequired}
	}
	auth := smtp.PlainAuth("", in.Values["email"], in.Values[c.profile.PasswordField], c.profile.SMTPHost)
	if c.profile.ID != "gmail" {
		var authErr error
		auth, authErr = c.smtpAuth(cl, in.Values["email"], in.Values[c.profile.PasswordField])
		if authErr != nil {
			closeSession()
			return nil, nil, nil, authErr
		}
	}
	if err = cl.Auth(auth); err != nil {
		closeSession()
		return nil, nil, nil, c.authenticationFailure(err)
	}
	return cl, conn, closeSession, nil
}
func authenticationFailure(err error) error {
	var n net.Error
	if errors.As(err, &n) || errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return protocolFailure(err, false)
	}
	var p *textproto.Error
	if errors.As(err, &p) && p.Code/100 == 4 {
		return protocolFailure(err, false)
	}
	// 534 alone also means browser security action; inspect only the explicit
	// documented app-password phrase, and never publish the upstream text.
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "too many simultaneous connections") || strings.Contains(text, "too many login attempts") || strings.Contains(text, "rate limit exceeded") {
		return &providers.Failure{Code: core.ErrorCodeRateLimited, Retryable: true}
	}
	if strings.Contains(text, "application-specific password required") || strings.Contains(text, "app-specific password required") {
		return &providers.Failure{Code: core.ErrorCodeMailAppPasswordRequired}
	}
	return &providers.Failure{Code: core.ErrorCodeMailAuthenticationRejected}
}
func protocolFailure(err error, mayHaveSubmitted bool) error {
	if err == nil {
		return nil
	}
	var f *providers.Failure
	if errors.As(err, &f) {
		return f
	}
	var p *textproto.Error
	if errors.As(err, &p) {
		if mayHaveSubmitted {
			if p.Code/100 == 4 || p.Code/100 == 5 {
				return &providers.Failure{Code: core.ErrorCodeMailSubmissionRejected, Retryable: p.Code/100 == 4}
			}
			return &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
		}
		return &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Retryable: p.Code/100 == 4}
	}
	if mayHaveSubmitted {
		return &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
	}
	if errors.Is(err, errResponseLimit) {
		return &providers.Failure{Code: core.ErrorCodeMailSizeExceeded}
	}
	return &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Retryable: true}
}

func (c *Client) readFailure(err error) error {
	if err != nil && c.profile.RequireIMAPID && strings.Contains(strings.ToLower(err.Error()), "unsafe login") {
		return &providers.Failure{Code: core.ErrorCodeMailClientIdentityRequired}
	}
	return protocolFailure(err, false)
}
func (c *Client) authenticationFailure(err error) error {
	if c.profile.RequireIMAPID && strings.Contains(strings.ToLower(err.Error()), "unsafe login") {
		return c.readFailure(err)
	}
	return authenticationFailure(err)
}
