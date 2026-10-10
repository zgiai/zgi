package consolesync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
)

// PullCommands carries its cursor in the query string. The sync client must send the query
// but sign only the path, exactly like the console verifier, which reads r.URL.Path.
func TestPullCommandsSignsTheQuerylessPathButSendsTheQuery(t *testing.T) {
	consolecred.Configure("instance-1", "secret")
	t.Cleanup(func() { consolecred.Global().Clear() })

	calls := make(chan capturedCall, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- captureConsoleCall(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"commands":[],"server_time":"2026-09-26T03:00:00Z","next_poll_interval_sec":30,"desired_version":1}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, 5*time.Second)
	if _, err := client.PullCommands(context.Background(), 7, 100); err != nil {
		t.Fatalf("PullCommands() error = %v", err)
	}

	call := receiveCall(t, calls)
	if call.Method != http.MethodGet {
		t.Fatalf("method = %q, want GET", call.Method)
	}
	if call.RawQuery != "after_seq=7&limit=100" {
		t.Fatalf("RawQuery = %q, want the cursor to still be sent", call.RawQuery)
	}
	if call.Path != apiPrefix+pathCommands {
		t.Fatalf("path = %q, want %q", call.Path, apiPrefix+pathCommands)
	}

	nonce := call.Header.Get(consolecred.HeaderNonce)
	if len(nonce) != 32 {
		t.Fatalf("%s = %q, want 32 lowercase hex chars", consolecred.HeaderNonce, nonce)
	}
	timestamp := call.Header.Get(consolecred.HeaderTimestamp)
	signature := call.Header.Get(consolecred.HeaderSignature)
	want := consolecred.Sign("secret", nonce, timestamp, http.MethodGet, apiPrefix+pathCommands, consolecred.BodyDigest(nil))
	if signature != want {
		t.Fatalf("signature = %q, want the signature over the queryless path (%q)", signature, want)
	}
	withQuery := consolecred.Sign("secret", nonce, timestamp, http.MethodGet, apiPrefix+pathCommands+"?after_seq=7&limit=100", consolecred.BodyDigest(nil))
	if signature == withQuery {
		t.Fatalf("signature covers the query string, which the console verifier strips")
	}
}
