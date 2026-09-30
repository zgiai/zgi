package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
)

func TestQueryConsoleSettlementUsesAttemptAndOrganization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/settlements/attempt-1" || r.URL.Query().Get("organization_id") != "org-1" {
			t.Fatalf("unexpected query: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"found":true,"settlement_id":"s-1","deduction_id":"s-1","attempt_id":"attempt-1","organization_id":"org-1","settlement_status":"SETTLED","reserved_credits":20,"settled_credits":12,"refunded_credits":8,"actual_quantity":3,"pricing_snapshot":{"rule_id":"rule-1"},"request_id":"req-1"}`))
	}))
	defer server.Close()
	got, err := queryConsoleSettlement(context.Background(), server.URL+"/settlements", "org-1", "attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found || got.SettlementID != "s-1" || got.DeductionID != "s-1" || got.AttemptID != "attempt-1" || got.SettledCredits != 12 || got.RefundedCredits != 8 || string(got.PricingSnapshot) != `{"rule_id":"rule-1"}` {
		t.Fatalf("unexpected response: %+v", got)
	}
}

// TestQueryConsoleSettlementSendsInternalSecret verifies the settlement status
// query carries the configured Console credential in the existing secret slot.
func TestQueryConsoleSettlementSendsInternalSecret(t *testing.T) {
	prev := appconfig.GlobalConfig
	t.Cleanup(func() { appconfig.GlobalConfig = prev })
	appconfig.GlobalConfig = &appconfig.Config{Console: appconfig.ConsoleConfig{
		InternalAPIKey: "shared-secret",
	}}

	var gotSecret string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecret = r.Header.Get("X-ZGI-Internal-Secret")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"found":false}`))
	}))
	defer server.Close()

	if _, err := queryConsoleSettlement(context.Background(), server.URL+"/settlements", "org-1", "attempt-1"); err != nil {
		t.Fatal(err)
	}
	if gotSecret != "shared-secret" {
		t.Fatalf("X-ZGI-Internal-Secret = %q, want shared-secret", gotSecret)
	}
}

// TestBuildConsoleAuthHookSendsInternalSecret covers the media/proxy outbound
// path that reuses the same credential.
func TestBuildConsoleAuthHookSendsInternalSecret(t *testing.T) {
	prev := appconfig.GlobalConfig
	t.Cleanup(func() { appconfig.GlobalConfig = prev })
	appconfig.GlobalConfig = &appconfig.Config{Console: appconfig.ConsoleConfig{
		InternalAPIKey: "hmac-key",
	}}

	s := &llmGatewayServiceImpl{}
	hook := s.buildConsoleAuthHook(uuid.Nil)
	if hook == nil {
		t.Fatal("hook must be built when the internal key is configured")
	}
	req, err := http.NewRequest(http.MethodPost, "http://console.invalid/v1/internal/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	hook(req)
	if got := req.Header.Get("X-ZGI-Internal-Secret"); got != "hmac-key" {
		t.Fatalf("X-ZGI-Internal-Secret = %q, want hmac-key", got)
	}
	if req.Header.Get("X-Internal-Signature") == "" {
		t.Fatal("HMAC signature must still be sent")
	}
}

func TestWaitForConsoleSettlementReturnsFailedFinalImmediately(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"found":true,"settlement_id":"s-1","deduction_id":"s-1","attempt_id":"attempt-1","organization_id":"org-1","settlement_status":"FAILED_FINAL","retryable":false}`))
	}))
	defer server.Close()

	got, err := waitForConsoleSettlement(context.Background(), server.URL+"/settlements", "org-1", "attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("FAILED_FINAL queries = %d, want 1", calls)
	}
	if !got.Found || got.SettlementStatus != "FAILED_FINAL" || got.Retryable {
		t.Fatalf("response = %+v", got)
	}
}

// The settlement query is an official-channel call: with an instance credential loaded
// it must carry the per-instance headers on top of the shared secret, like every other
// Pre -> Console call.
func TestQueryConsoleSettlementSignsWithInstanceCredential(t *testing.T) {
	prev := appconfig.GlobalConfig
	t.Cleanup(func() { appconfig.GlobalConfig = prev; consolecred.Global().Clear() })
	appconfig.GlobalConfig = &appconfig.Config{Console: appconfig.ConsoleConfig{InternalAPIKey: "shared-secret"}}
	consolecred.Global().Clear()
	consolecred.Configure("instance-42", "instance-secret")

	var got http.Header
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"found":false}`))
	}))
	defer server.Close()

	if _, err := queryConsoleSettlement(context.Background(), server.URL+"/settlements", "org-1", "attempt-1"); err != nil {
		t.Fatal(err)
	}
	if got.Get("X-ZGI-Internal-Secret") != "shared-secret" {
		t.Fatalf("shared secret must stay in place, got %q", got.Get("X-ZGI-Internal-Secret"))
	}
	if got.Get("X-Internal-Instance-Id") != "instance-42" {
		t.Fatalf("X-Internal-Instance-Id = %q", got.Get("X-Internal-Instance-Id"))
	}
	nonce, ts, sig := got.Get("X-Internal-Nonce"), got.Get("X-Internal-Timestamp"), got.Get("X-Internal-Signature")
	if nonce == "" || ts == "" || sig == "" {
		t.Fatalf("missing instance headers: nonce=%q ts=%q sig=%q", nonce, ts, sig)
	}
	if want := consolecred.Sign("instance-secret", nonce, ts, http.MethodGet, gotPath, consolecred.BodyDigest(nil)); sig != want {
		t.Fatalf("signature does not cover method/path/empty body")
	}
}

// Without a loaded credential, or while the link is stale, the request is exactly what
// it was before: the shared secret only.
func TestQueryConsoleSettlementUnsignedWithoutUsableCredential(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func()
	}{
		{"not configured", func() { consolecred.Global().Clear() }},
		{"link stale", func() {
			consolecred.Global().Clear()
			consolecred.Configure("instance-42", "instance-secret")
			consolecred.Global().SetStale(true)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prev := appconfig.GlobalConfig
			t.Cleanup(func() { appconfig.GlobalConfig = prev; consolecred.Global().Clear() })
			appconfig.GlobalConfig = &appconfig.Config{Console: appconfig.ConsoleConfig{InternalAPIKey: "shared-secret"}}
			tc.setup()

			var got http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.Header.Clone()
				_, _ = w.Write([]byte(`{"found":false}`))
			}))
			defer server.Close()
			if _, err := queryConsoleSettlement(context.Background(), server.URL+"/settlements", "org-1", "attempt-1"); err != nil {
				t.Fatal(err)
			}
			if got.Get("X-ZGI-Internal-Secret") != "shared-secret" {
				t.Fatalf("shared secret = %q", got.Get("X-ZGI-Internal-Secret"))
			}
			for _, h := range []string{"X-Internal-Instance-Id", "X-Internal-Timestamp", "X-Internal-Signature", "X-Internal-Nonce"} {
				if got.Get(h) != "" {
					t.Fatalf("%s must not be sent, got %q", h, got.Get(h))
				}
			}
		})
	}
}
