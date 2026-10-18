package zgimarket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// activationFixture answers as the member named by the X-Test-Account header, an administrator
// when that account is the fixture's admin.
type activationFixture struct {
	*toolsFixture
	admin, member, other uuid.UUID
}

func newActivationFixture(t *testing.T) *activationFixture {
	t.Helper()
	base := newToolsFixture(t, false)
	require.NoError(t, base.db.AutoMigrate(&ActivationRequest{}))
	f := &activationFixture{toolsFixture: base, admin: uuid.New(), member: uuid.New(), other: uuid.New()}
	require.NoError(t, base.db.Exec(`CREATE TABLE accounts (id text, name text)`).Error)
	for id, name := range map[uuid.UUID]string{f.admin: "Ada", f.member: "Bo", f.other: "Cy"} {
		require.NoError(t, base.db.Exec(`INSERT INTO accounts VALUES (?, ?)`, id.String(), name).Error)
	}
	proxy := NewProxy(
		Config{ConsoleURL: "http://console.invalid"},
		func(*gin.Context) string { return base.org.String() },
		func(c *gin.Context) string { return c.GetHeader("X-Test-Account") },
		func(_ *gin.Context, _, account string) (bool, bool, error) {
			return true, account == f.admin.String(), nil
		},
	).WithTools(base.tools)
	base.engine = gin.New()
	base.engine.Any("/workspaces/current/zgi-market/*path", proxy.Forward)
	return f
}

func (f *activationFixture) as(t *testing.T, account uuid.UUID, method, path, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, "/workspaces/current/zgi-market/"+path, strings.NewReader(body))
	request.Header.Set("X-Test-Account", account.String())
	recorder := httptest.NewRecorder()
	f.engine.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded) //nolint:errcheck // asserted by the caller
	return recorder.Code, decoded
}

func TestAMemberAsksForAnItemAndAnAdministratorAnswers(t *testing.T) {
	f := newActivationFixture(t)
	ask := `{"listing_id":7,"listing_kind":"mcp","listing_slug":"wiki","listing_title":"Wiki","reason":"for the support agent"}`

	status, answer := f.as(t, f.member, http.MethodPost, "activation-requests", ask)
	require.Equal(t, http.StatusOK, status, answer)
	first := answer["data"].(map[string]any)
	assert.Equal(t, []any{ActivationPending, "wiki", "for the support agent"}, []any{first["status"], first["listing_slug"], first["reason"]})

	// Asking again while it is pending returns the same request.
	_, answer = f.as(t, f.member, http.MethodPost, "activation-requests", ask)
	assert.Equal(t, first["id"], answer["data"].(map[string]any)["id"])
	_, _ = f.as(t, f.other, http.MethodPost, "activation-requests", `{"listing_id":20,"listing_kind":"api","listing_slug":"items","offering_id":3}`)

	// A member reads their own; an administrator reads everyone's, with names.
	_, answer = f.as(t, f.member, http.MethodGet, "activation-requests", "")
	assert.Len(t, answer["data"].(map[string]any)["data"], 1)
	_, answer = f.as(t, f.admin, http.MethodGet, "activation-requests?status=pending", "")
	page := answer["data"].(map[string]any)
	assert.Equal(t, float64(2), page["pending"])
	names := []any{}
	for _, row := range page["data"].([]any) {
		names = append(names, row.(map[string]any)["requester_name"])
	}
	assert.ElementsMatch(t, []any{"Bo", "Cy"}, names)

	// Only an administrator answers, and a request is answered once.
	id := first["id"].(string)
	status, _ = f.as(t, f.member, http.MethodPost, "activation-requests/"+id+"/approve", "")
	assert.Equal(t, http.StatusForbidden, status)
	status, answer = f.as(t, f.admin, http.MethodPost, "activation-requests/"+id+"/approve", `{"note":"activated"}`)
	require.Equal(t, http.StatusOK, status, answer)
	approved := answer["data"].(map[string]any)
	assert.Equal(t, []any{ActivationApproved, "Ada", "activated"}, []any{approved["status"], approved["reviewer_name"], approved["review_note"]})
	status, _ = f.as(t, f.admin, http.MethodPost, "activation-requests/"+id+"/reject", "")
	assert.NotEqual(t, http.StatusOK, status)

	// Once answered, the member may ask again.
	_, answer = f.as(t, f.member, http.MethodPost, "activation-requests", ask)
	assert.NotEqual(t, id, answer["data"].(map[string]any)["id"])
}

func TestOnlyTheRequesterTakesARequestBack(t *testing.T) {
	f := newActivationFixture(t)
	_, answer := f.as(t, f.member, http.MethodPost, "activation-requests", `{"listing_id":9,"listing_kind":"skill","listing_slug":"notes"}`)
	id := answer["data"].(map[string]any)["id"].(string)

	status, _ := f.as(t, f.other, http.MethodPost, "activation-requests/"+id+"/cancel", "")
	assert.Equal(t, http.StatusNotFound, status)
	status, answer = f.as(t, f.member, http.MethodPost, "activation-requests/"+id+"/cancel", "")
	require.Equal(t, http.StatusOK, status, answer)
	assert.Equal(t, ActivationCancelled, answer["data"].(map[string]any)["status"])
}

func TestAnActivationRequestMustNameAnItem(t *testing.T) {
	f := newActivationFixture(t)
	for _, body := range []string{
		`{}`,
		`{"listing_id":7,"listing_kind":"model","listing_slug":"wiki"}`,
		`{"listing_id":7,"listing_kind":"mcp","listing_slug":"../x"}`,
		`{"listing_id":7,"listing_kind":"mcp","listing_slug":"wiki","offering_id":2}`,
		`{"listing_id":7,"listing_kind":"mcp","listing_slug":"wiki","reason":"` + strings.Repeat("长", 501) + `"}`,
		`not json`,
	} {
		status, _ := f.as(t, f.member, http.MethodPost, "activation-requests", body)
		assert.NotEqual(t, http.StatusOK, status, body)
	}
}
