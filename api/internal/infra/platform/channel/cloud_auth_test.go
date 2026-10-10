package channel

import (
	"net/http"
	"testing"
)

// TestCloudSignRequestSendsInternalSecret verifies the platform-channel refresh
// carries the configured Console credential in the existing secret slot, with
// the HMAC signature kept for the compatibility path.
func TestCloudSignRequestSendsInternalSecret(t *testing.T) {
	c := NewCloud("http://console.invalid", "test-signing-key")
	req, err := http.NewRequest(http.MethodGet, "http://console.invalid"+pathChannelRouting, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	c.signRequest(req)

	if got := req.Header.Get("X-ZGI-Internal-Secret"); got != "test-signing-key" {
		t.Fatalf("X-ZGI-Internal-Secret = %q, want test-signing-key", got)
	}
	if req.Header.Get("X-Internal-Signature") == "" {
		t.Fatal("HMAC signature must still be sent")
	}
}
