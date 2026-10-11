// Package zgimarket lets members of an organization use ZGI Market. The market is a separate
// service behind the console: this instance forwards a member's request to the console's market
// proxy with its instance credential, and the console signs it for the market. Nothing of the
// market is stored here.
package zgimarket

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
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

	headerOrganizationID   = "X-ZGI-Billing-Organization-ID"
	headerAccountID        = "X-ZGI-Account-ID"
	headerOrganizationRole = "X-ZGI-Organization-Role"

	roleAdmin  = "admin"
	roleMember = "member"
)

// resources are the market resources a member may reach. Everything else answers not found here,
// before the console is asked.
var resources = map[string]bool{
	"categories": true, "listings": true, "subscriptions": true, "credentials": true,
	"bindings": true, "connections": true, "usage": true, "statements": true,
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
}

// NewProxy creates the proxy.
func NewProxy(
	config Config,
	organization, account func(c *gin.Context) string,
	membership func(c *gin.Context, organizationID, accountID string) (member, admin bool, err error),
) *Proxy {
	config.ConsoleURL = strings.TrimRight(strings.TrimSpace(config.ConsoleURL), "/")
	if config.Client == nil {
		config.Client = &http.Client{
			Timeout: requestTimeout,
			// The answer of the market is handed back as it is, a redirect included.
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

// Forward hands one market request of a member to the console and returns the answer unchanged.
func (p *Proxy) Forward(c *gin.Context) {
	if p.config.ConsoleURL == "" {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market is not configured")
		return
	}
	path, _, allowed := resourceOf(c.Param("path"))
	if !allowed {
		response.Fail(c, response.ErrNotFound)
		return
	}
	organizationID, accountID := p.organization(c), p.account(c)
	if organizationID == "" || accountID == "" {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	member, admin, err := p.membership(c, organizationID, accountID)
	if err != nil || !member {
		response.Fail(c, response.ErrPermissionDenied)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, requestBodyLimit))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, "Request body is too large")
		return
	}

	target := p.config.ConsoleURL + consoleProxyPath + path
	if c.Request.URL.RawQuery != "" {
		target += "?" + c.Request.URL.RawQuery
	}
	request, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target, bytes.NewReader(body))
	if err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	for _, name := range forwardedHeaders {
		if value := c.GetHeader(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	// The caller is named by this instance, never by what the browser sent.
	request.Header.Set(headerOrganizationID, organizationID)
	request.Header.Set(headerAccountID, accountID)
	role := roleMember
	if admin {
		role = roleAdmin
	}
	request.Header.Set(headerOrganizationRole, role)
	p.sign(request)

	answer, err := p.config.Client.Do(request)
	if err != nil {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market is unavailable")
		return
	}
	defer answer.Body.Close()
	content, err := io.ReadAll(io.LimitReader(answer.Body, answerBodyLimit+1))
	if err != nil || len(content) > answerBodyLimit {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market is unavailable")
		return
	}
	if location := answer.Header.Get("Location"); location != "" {
		c.Header("Location", location)
	}
	contentType := answer.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Header("Cache-Control", "private, no-store")
	c.Data(answer.StatusCode, contentType, content)
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
