// Package zgimarket lets members of an organization use ZGI Market. The market is a separate
// service behind the console: this instance forwards a member's request to the console's market
// proxy with its instance credential, and the console signs it for the market. Nothing of the
// market is stored here.
package zgimarket

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	consoleProxyPath = "/v1/internal/zgi-market/proxy/"
	requestBodyLimit = 1 << 20
	answerBodyLimit  = 8 << 20
	requestTimeout   = 30 * time.Second
	// A skill package is far larger than any other answer and takes longer to arrive.
	packageBodyLimit = 65 << 20
	packageTimeout   = 5 * time.Minute

	headerOrganizationID   = "X-ZGI-Billing-Organization-ID"
	headerAccountID        = "X-ZGI-Account-ID"
	headerOrganizationRole = "X-ZGI-Organization-Role"

	roleAdmin  = "admin"
	roleMember = "member"
)

var errAnswerTooLarge = errors.New("the market answer is too large")

// resources are the market resources a member may reach. Everything else answers not found here,
// before the console is asked.
var resources = map[string]bool{
	"categories": true, "listings": true, "subscriptions": true, "credentials": true,
	"bindings": true, "connections": true, "usage": true, "statements": true,
	// The record of the organization's own gateway and MCP calls.
	"calls": true,
}

// forwardedHeaders are the request headers handed on to the market.
var forwardedHeaders = []string{"Content-Type", "Accept", "Accept-Language", "Idempotency-Key"}

// Config is how the console is reached.
type Config struct {
	// ConsoleURL is the base address of the console API, without a path.
	ConsoleURL string
	// InternalKey is the transitional shared key; the instance credential is added on top of it.
	InternalKey string
	Client      *http.Client
	// PackageClient fetches skill packages; it allows more time than Client.
	PackageClient *http.Client
	// GatewayURL is the public origin of the market gateway, shown to members in usage examples.
	// Empty when this instance does not name one.
	GatewayURL string
	// OAuthRedirectURL is the callback address of this instance for connector authorizations: the
	// page a vendor sends the browser back to. It has to be one of the callback addresses the
	// market is configured with. Empty leaves the choice to the market, which uses its default.
	OAuthRedirectURL string
}

// Proxy forwards market requests of organization members to the console.
type Proxy struct {
	config Config
	// membership reports whether the account belongs to the organization, and whether it
	// administers it.
	membership func(c *gin.Context, organizationID, accountID string) (member, admin bool, err error)
	// organization and account read the caller from the request context.
	organization func(c *gin.Context) string
	account      func(c *gin.Context) string
	// skills imports market skill packages; nil leaves installing off.
	skills SkillInstaller
	// tools binds market MCP servers to agents and digital humans; nil leaves that off.
	tools *Tools
}

// NewProxy creates the proxy.
func NewProxy(
	config Config,
	organization, account func(c *gin.Context) string,
	membership func(c *gin.Context, organizationID, accountID string) (member, admin bool, err error),
) *Proxy {
	config.ConsoleURL = strings.TrimRight(strings.TrimSpace(config.ConsoleURL), "/")
	if config.OAuthRedirectURL = strings.TrimSpace(config.OAuthRedirectURL); !ValidOAuthRedirectURL(config.OAuthRedirectURL) {
		// An address the market would refuse is not sent; its default is used instead.
		config.OAuthRedirectURL = ""
	}
	if config.Client == nil {
		config.Client = &http.Client{
			Timeout: requestTimeout,
			// The answer of the market is handed back as it is, a redirect included.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if config.PackageClient == nil {
		config.PackageClient = &http.Client{
			Timeout:       packageTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	return &Proxy{config: config, organization: organization, account: account, membership: membership}
}

// resourceOf returns the cleaned path of a market request and its first segment.
func resourceOf(raw string) (path, resource string, ok bool) {
	path = strings.Trim(raw, "/")
	if path == "" || strings.Contains(path, "..") || strings.Contains(path, "//") || strings.ContainsAny(path, "\\\x00") {
		return "", "", false
	}
	resource, _, _ = strings.Cut(path, "/")
	return path, resource, resources[resource]
}

// skillManifestPattern is the one path under `skills` a browser may read: the file list of a skill
// package. The package itself is fetched by this instance when it installs the skill, never handed
// to a browser.
var skillManifestPattern = regexp.MustCompile(`^skills/[a-z0-9][a-z0-9._-]{0,127}/manifest$`)

// forwardable reports whether a member's request may be handed to the console.
func forwardable(method, raw string) (path string, ok bool) {
	path, _, allowed := resourceOf(raw)
	if allowed {
		return path, true
	}
	if method == http.MethodGet && skillManifestPattern.MatchString(path) {
		return path, true
	}
	return "", false
}

// caller is the member a market request is made for.
type caller struct {
	organizationID, accountID string
	admin                     bool
}

// answer is what the console returned for one market request.
type answer struct {
	status  int
	header  http.Header
	content []byte
}

// member resolves the caller of the request, or answers the refusal itself.
func (p *Proxy) member(c *gin.Context) (caller, bool) {
	who := caller{organizationID: p.organization(c), accountID: p.account(c)}
	if who.organizationID == "" || who.accountID == "" {
		response.Fail(c, response.ErrPermissionDenied)
		return caller{}, false
	}
	member, admin, err := p.membership(c, who.organizationID, who.accountID)
	if err != nil || !member {
		response.Fail(c, response.ErrPermissionDenied)
		return caller{}, false
	}
	who.admin = admin
	return who, true
}

// call sends one market request to the console as the caller. path is below the market's customer
// prefix and may carry a query; limit bounds the answer.
func (p *Proxy) call(
	ctx context.Context, who caller, method, path string, body []byte, headers http.Header, limit int64,
) (*answer, error) {
	request, err := http.NewRequestWithContext(ctx, method, p.config.ConsoleURL+consoleProxyPath+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for _, name := range forwardedHeaders {
		if value := headers.Get(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	// The caller is named by this instance, never by what the browser sent.
	request.Header.Set(headerOrganizationID, who.organizationID)
	request.Header.Set(headerAccountID, who.accountID)
	role := roleMember
	if who.admin {
		role = roleAdmin
	}
	request.Header.Set(headerOrganizationRole, role)
	p.sign(request)

	client := p.config.Client
	if limit > answerBodyLimit {
		client = p.config.PackageClient
	}
	result, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()
	content, err := io.ReadAll(io.LimitReader(result.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > limit {
		return nil, errAnswerTooLarge
	}
	return &answer{status: result.StatusCode, header: result.Header, content: content}, nil
}

// Forward hands one market request of a member to the console and returns the answer unchanged.
func (p *Proxy) Forward(c *gin.Context) {
	if p.config.ConsoleURL == "" {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market is not configured")
		return
	}
	if p.meta(c) || p.install(c) || p.toolRoute(c) || p.callLogRoute(c) || p.activationRequestRoute(c) {
		return
	}
	path, allowed := forwardable(c.Request.Method, c.Param("path"))
	if !allowed {
		response.Fail(c, response.ErrNotFound)
		return
	}
	who, ok := p.member(c)
	if !ok {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, requestBodyLimit))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, "Request body is too large")
		return
	}
	callback := path == oauthCallbackPath
	headers := c.Request.Header
	query, named := p.nameOAuthRedirect(c.Request.Method, path, c.Request.URL.RawQuery, body)
	if len(named) != len(body) || string(named) != string(body) {
		// A reauthorization may come without a body; the address makes it a JSON one.
		headers = headers.Clone()
		headers.Set("Content-Type", "application/json")
		body = named
	}
	if query != "" {
		path += "?" + query
	}
	result, err := p.call(c.Request.Context(), who, c.Request.Method, path, body, headers, answerBodyLimit)
	if err != nil {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market is unavailable")
		return
	}
	if callback {
		p.relayOAuthCallback(c, result)
		return
	}
	p.relay(c, result)
}

// relay returns an answer of the market as it is, a redirect included.
func (p *Proxy) relay(c *gin.Context, result *answer) {
	if location := result.header.Get("Location"); location != "" {
		c.Header("Location", location)
	}
	contentType := result.header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(result.status, contentType, result.content)
}

// sign authenticates the request to the console the way every official-channel call of this
// instance does: the transitional shared key, and the per-instance credential on top of it.
func (p *Proxy) sign(request *http.Request) {
	if p.config.InternalKey != "" {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(p.config.InternalKey))
		mac.Write([]byte(timestamp + "|" + request.URL.Path))
		request.Header.Set("X-ZGI-Internal-Secret", p.config.InternalKey)
		request.Header.Set("X-Internal-Timestamp", timestamp)
		request.Header.Set("X-Internal-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	consolecred.SignRequest(request)
}
