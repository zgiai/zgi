package zgimarket

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFreeProxy serves listings by path and records each subscription the member's proxy sends.
func newFreeProxy(t *testing.T, listings map[string]string) (*gin.Engine, *[]seenRequest) {
	t.Helper()
	seen := []seenRequest{}
	console := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body) //nolint:errcheck // test server
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			for path, listing := range listings {
				if strings.HasSuffix(r.URL.Path, "/listings/"+path) {
					_, _ = io.WriteString(w, `{"code":0,"data":`+listing+`}`) //nolint:errcheck // test server
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		seen = append(seen, seenRequest{r.Method, r.URL.Path, r.URL.RawQuery, string(body), r.Header.Clone()})
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"code":0,"data":{"id":1}}`) //nolint:errcheck // test server
	}))
	t.Cleanup(console.Close)
	proxy := NewProxy(
		Config{ConsoleURL: console.URL, InternalKey: "shared-key"},
		func(*gin.Context) string { return "org-1" },
		func(*gin.Context) string { return "account-1" },
		func(*gin.Context, string, string) (bool, bool, error) { return true, false, nil },
	)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Any("/workspaces/current/zgi-market/*path", proxy.Forward)
	return engine, &seen
}

func TestAMemberActivatesAFreeListingAsAnAdministratorWould(t *testing.T) {
	engine, seen := newFreeProxy(t, map[string]string{
		"api/postal": `{"id":929,"kind":"api","offerings":[{"id":5,"slug":"postal-standard"},{"id":6,"slug":"postal-pro"}],
			"prices":[{"offering_slug":"postal-standard","unit_price_micros":0},{"offering_slug":"postal-pro","unit_price_micros":900}]}`,
		"mcp/wiki":      `{"id":919,"kind":"mcp","prices":[{"unit_price_micros":1500}]}`,
		"mcp/unpriced":  `{"id":918,"kind":"mcp","prices":[]}`,
		"plugin/bundle": `{"id":860,"kind":"plugin","prices":[{"unit_price_micros":0}],"plugin":{"members":[{"kind":"mcp","slug":"wiki"}]}}`,
		"plugin/free":   `{"id":861,"kind":"plugin","prices":[{"unit_price_micros":0}],"plugin":{"members":[]}}`,
	})
	activate := func(body string) seenRequest {
		before := len(*seen)
		recorder := send(engine, http.MethodPost, "/workspaces/current/zgi-market/subscriptions", body, nil)
		require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
		require.Len(t, *seen, before+1)
		return (*seen)[before]
	}

	free := activate(`{"listing_id":929,"offering_id":5,"listing_kind":"api","listing_slug":"postal"}`)
	assert.Equal(t, roleAdmin, free.header.Get(headerOrganizationRole), "a free offering is activated with the administrator's right")
	assert.JSONEq(t, `{"listing_id":929,"offering_id":5}`, free.body, "the market gets only what it takes")
	plugin := activate(`{"listing_id":861,"listing_kind":"plugin","listing_slug":"free"}`)
	assert.Equal(t, roleAdmin, plugin.header.Get(headerOrganizationRole))

	for name, body := range map[string]string{
		"a paid offering":                  `{"listing_id":929,"offering_id":6,"listing_kind":"api","listing_slug":"postal"}`,
		"an API without an offering":       `{"listing_id":929,"listing_kind":"api","listing_slug":"postal"}`,
		"a paid MCP server":                `{"listing_id":919,"listing_kind":"mcp","listing_slug":"wiki"}`,
		"a listing without a price":        `{"listing_id":918,"listing_kind":"mcp","listing_slug":"unpriced"}`,
		"a free plugin with a paid member": `{"listing_id":860,"listing_kind":"plugin","listing_slug":"bundle"}`,
		"an id that is not the named one":  `{"listing_id":1,"offering_id":5,"listing_kind":"api","listing_slug":"postal"}`,
		"a listing named by its id only":   `{"listing_id":929,"offering_id":5}`,
	} {
		forwarded := activate(body)
		assert.Equal(t, roleMember, forwarded.header.Get(headerOrganizationRole), name+" stays the member's, which the market refuses")
	}
}
