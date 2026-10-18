package zgimarket

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/pkg/response"
)

// toolsPrefix is the path below which this instance answers about market tools itself.
const toolsPrefix = "tools"

// oauthCallbackPath is the market's connector OAuth callback, which a browser page of this
// instance forwards after the vendor sent the user back.
const oauthCallbackPath = "connections/oauth/callback"

// WithTools lets organization administrators bind market MCP servers to agents and digital humans.
func (p *Proxy) WithTools(tools *Tools) *Proxy {
	if tools != nil {
		tools.proxy = p
	}
	p.tools = tools
	return p
}

// toolRoute handles the requests below tools/, which this instance answers itself. It reports
// whether the request was one of them.
//
//	GET tools/candidates                     the activated MCP listings and the connections
//	GET tools/subjects/{agent|digital_human}/{id}  the market tools of one subject
//	PUT tools/subjects/{agent|digital_human}/{id}  replace them (administrators)
func (p *Proxy) toolRoute(c *gin.Context) bool {
	path := strings.Trim(c.Param("path"), "/")
	if path != toolsPrefix && !strings.HasPrefix(path, toolsPrefix+"/") {
		return false
	}
	if p.tools == nil {
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market tools are not configured")
		return true
	}
	segments := strings.Split(path, "/")
	who, ok := p.member(c)
	if !ok {
		return true
	}
	ctx := c.Request.Context()
	c.Header("Cache-Control", "private, no-store")
	switch {
	case len(segments) == 2 && segments[1] == "candidates" && c.Request.Method == http.MethodGet:
		candidates, err := p.tools.Candidates(ctx, who)
		p.toolAnswer(c, candidates, err)
	case len(segments) == 4 && segments[1] == "subjects" && c.Request.Method == http.MethodGet:
		binding, err := p.tools.Binding(ctx, who, segments[2], segments[3])
		p.toolAnswer(c, binding, err)
	case len(segments) == 4 && segments[1] == "subjects" && c.Request.Method == http.MethodPut:
		if !who.admin {
			response.Fail(c, response.ErrPermissionDenied)
			return true
		}
		var input BindInput
		body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, requestBodyLimit))
		if err != nil || json.Unmarshal(body, &input) != nil {
			response.Fail(c, response.ErrInvalidParams)
			return true
		}
		binding, err := p.tools.Bind(ctx, who, segments[2], segments[3], input)
		p.toolAnswer(c, binding, err)
	default:
		response.Fail(c, response.ErrNotFound)
	}
	return true
}

func (p *Proxy) toolAnswer(c *gin.Context, data any, err error) {
	var refused *marketAnswerError
	switch {
	case err == nil:
		response.Success(c, data)
	case errors.As(err, &refused):
		// The market's own answer, such as a connection that is not connected.
		p.relay(c, refused.answer)
	case errors.Is(err, ErrSubjectNotFound):
		response.Fail(c, response.ErrNotFound)
	case errors.Is(err, ErrListingNotBindable):
		response.FailWithMessage(c, response.ErrInvalidParams, err.Error())
	default:
		response.FailWithMessage(c, response.ErrServiceUnavailable, "ZGI Market is unavailable")
	}
}

// relayOAuthCallback answers the forwarded OAuth callback with the page to open next as data: a
// browser page made the request with fetch, which cannot read the redirect itself.
func (p *Proxy) relayOAuthCallback(c *gin.Context, result *answer) {
	location := result.header.Get("Location")
	if result.status != http.StatusFound || location == "" {
		p.relay(c, result)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("Referrer-Policy", "no-referrer")
	// The market sends a callback it cannot tie to a started authorization, such as one with an
	// unknown state, to its own website. A browser that arrived here stays here: the page shows
	// that the authorization failed instead of opening another site.
	if own := originOf(p.config.OAuthRedirectURL); own != "" && originOf(location) != own {
		c.JSON(http.StatusConflict, gin.H{
			"code": "zgi_market.oauth.not_completed", "message": "The authorization was not completed",
			"error_code": oauthFailureCode(location),
		})
		return
	}
	response.Success(c, gin.H{"redirect": location})
}
