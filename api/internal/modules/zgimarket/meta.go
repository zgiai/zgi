package zgimarket

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zgiai/zgi/api/pkg/response"
)

// metaPath is answered by this instance itself: what a market page needs to know about the
// caller and about how the market is reached from outside.
const metaPath = "meta"

// Meta is what a market page of this instance needs beside the market's own answers.
type Meta struct {
	// GatewayURL is the public origin of the market gateway, or empty when it is not configured.
	GatewayURL string `json:"gateway_url"`
	// Admin says whether the caller administers the organization: only an administrator
	// activates, installs, connects, and binds.
	Admin bool `json:"admin"`
}

// meta answers GET meta for a member of the organization. It reports whether it handled the
// request.
func (p *Proxy) meta(c *gin.Context) bool {
	if strings.Trim(c.Param("path"), "/") != metaPath {
		return false
	}
	if c.Request.Method != http.MethodGet {
		response.Fail(c, response.ErrNotFound)
		return true
	}
	who, ok := p.member(c)
	if !ok {
		return true
	}
	c.Header("Cache-Control", "private, no-store")
	response.Success(c, Meta{GatewayURL: p.config.GatewayURL, Admin: who.admin})
	return true
}
