package weixin

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"io"
	"net/http"
	"strings"
	"testing"
)

type reliabilityTransport func(*http.Request) (*http.Response, error)

func (f reliabilityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestOfficialSparseUpdatesPreserveCursor(t *testing.T) {
	for _, tc := range []struct {
		body   string
		ok     bool
		cursor string
	}{
		{`{"msgs":[]}`, true, "previous"},
		{`{"get_updates_buf":"","msgs":[]}`, true, "previous"},
		{`{"get_updates_buf":"next"}`, true, "next"},
		{`{}`, false, "previous"}, {`{"ret":0}`, false, "previous"},
		{`{"msgs":null}`, false, "previous"}, {`{"msgs":[] ,"ret":1}`, false, "previous"},
		{`{"msgs":[{"message_type":1,"message_state":2}],"get_updates_buf":"next"}`, false, "previous"},
		{`{"msgs":[],"msgs":[]}`, false, "previous"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			c, _ := New(&http.Client{Transport: reliabilityTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}, true, "bot", nil)
			state := messaging.State{Cursor: "previous"}
			err := c.pull(context.Background(), providers.CredentialInput{Values: map[string]string{"token": "fixture"}}, &state)
			if (err == nil) != tc.ok || state.Cursor != tc.cursor {
				t.Fatalf("success=%v cursor=%q", err == nil, state.Cursor)
			}
			if err != nil && strings.Contains(tc.body, "message_type") && !providers.FailureOf(err).Unknown {
				t.Fatal("malformed consuming batch may replay")
			}
		})
	}
}
