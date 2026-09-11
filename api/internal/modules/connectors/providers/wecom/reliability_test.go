package wecom

import (
	"context"
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"io"
	"net/http"
	"strings"
	"testing"
)

type reliabilityTransport func(*http.Request) (*http.Response, error)

func (f reliabilityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestSparseOfficialEnvelopesAndInnerRefusal(t *testing.T) {
	for _, tc := range []struct {
		inner string
		code  core.ErrorCode
	}{
		{`{"result":"{\"sessions\":[]}","error":{}}`, ""},
		{`{"result":"{\"sessions_count\":0}"}`, ""},
		{`{"result":"{\"sessions\":[]}","error":{"code":853004}}`, core.ErrorCodeInvalidAppCredentials},
		{`{"result":"{\"sessions\":[]}","error":{"code":7}}`, core.ErrorCodePermissionMissing},
	} {
		t.Run(tc.inner, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"results_json": tc.inner})
			c, _ := New(&http.Client{Transport: reliabilityTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}, true, nil)
			data, err := c.call(context.Background(), "/message/aibot/sessions/list", "fixture", map[string]any{}, false)
			if tc.code != "" {
				if err == nil || providers.FailureOf(err).Code != tc.code {
					t.Fatal("lost explicit refusal")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = parseSessions(data); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, data := range []map[string]any{{}, {"sessions": nil}, {"sessions_count": float64(1)}} {
		if _, err := parseSessions(data); err == nil {
			t.Fatal("unproven empty result succeeded")
		}
	}
}
