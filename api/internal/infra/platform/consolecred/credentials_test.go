package consolecred

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/credentials"
)

// expectedHTTP recomputes the instance signature from first principles, deliberately NOT
// through the production SignedString, so a change to the signed-string format is caught
// here instead of being mirrored by the test.
func expectedHTTP(secret, nonce, timestamp, method, path string, body []byte) string {
	sum := sha256.Sum256(body)
	signed := nonce + "|" + timestamp + "|" + method + "|" + path + "|" + hex.EncodeToString(sum[:])
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	return hex.EncodeToString(mac.Sum(nil))
}

// wantSignatureFromRequest checks the four instance headers of a signed request against an
// independently computed signature and returns the nonce so callers can assert freshness.
func wantSignatureFromRequest(t *testing.T, req *http.Request, secret, path string, body []byte) string {
	t.Helper()
	nonce := req.Header.Get(HeaderNonce)
	if nonce == "" {
		t.Fatalf("missing %s header", HeaderNonce)
	}
	timestamp := req.Header.Get(HeaderTimestamp)
	if timestamp == "" {
		t.Fatalf("missing %s header", HeaderTimestamp)
	}
	want := expectedHTTP(secret, nonce, timestamp, strings.ToUpper(req.Method), path, body)
	if got := req.Header.Get(HeaderSignature); got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
	return nonce
}

func TestSignRequestAddsInstanceHeaders(t *testing.T) {
	Global().Clear()
	t.Cleanup(func() { Global().Clear() })
	Configure("instance-42", "top-secret")

	req := httptest.NewRequest("POST", "https://console.example.com/v1/internal/pre/heartbeat", nil)
	SignRequest(req)

	if got := req.Header.Get(HeaderInstanceID); got != "instance-42" {
		t.Fatalf("%s = %q", HeaderInstanceID, got)
	}
	wantSignatureFromRequest(t, req, "top-secret", "/v1/internal/pre/heartbeat", nil)
}

// The exact wire format is the contract with the console verifier: pin it literally.
func TestSignedStringFormat(t *testing.T) {
	const (
		nonce     = "0123456789abcdef0123456789abcdef"
		timestamp = "1750000000"
	)
	digest := BodyDigest([]byte(`{"a":1}`))
	want := nonce + "|" + timestamp + "|POST|/v1/internal/pre/heartbeat|" + digest
	if got := SignedString(nonce, timestamp, "POST", "/v1/internal/pre/heartbeat", digest); got != want {
		t.Fatalf("SignedString() = %q, want %q", got, want)
	}
	if got := Sign("secret", nonce, timestamp, "POST", "/v1/internal/pre/heartbeat", digest); got != expectedHTTP("secret", nonce, timestamp, "POST", "/v1/internal/pre/heartbeat", []byte(`{"a":1}`)) {
		t.Fatalf("Sign() = %q, want the HMAC of the literal signed string", got)
	}
}

// A request signed for one (nonce, timestamp, method, path, body) must not verify for any
// other: this is the replay/cross-endpoint protection.
func TestSignatureCoversNonceTimestampMethodPathAndBody(t *testing.T) {
	const (
		secret    = "top-secret"
		nonce     = "0123456789abcdef0123456789abcdef"
		timestamp = "1750000000"
		path      = "/v1/internal/pre/heartbeat"
	)
	digest := BodyDigest([]byte(`{"a":1}`))
	base := Sign(secret, nonce, timestamp, "POST", path, digest)

	mutations := []struct {
		name string
		got  string
	}{
		{"changed body", Sign(secret, nonce, timestamp, "POST", path, BodyDigest([]byte(`{"a":2}`)))},
		{"changed method", Sign(secret, nonce, timestamp, "GET", path, digest)},
		{"changed path", Sign(secret, nonce, timestamp, "POST", "/v1/internal/pre/commands", digest)},
		{"changed nonce", Sign(secret, "fedcba9876543210fedcba9876543210", timestamp, "POST", path, digest)},
		{"changed timestamp", Sign(secret, nonce, "1750000001", "POST", path, digest)},
		{"changed secret", Sign("other-secret", nonce, timestamp, "POST", path, digest)},
	}
	for _, mutation := range mutations {
		if mutation.got == base {
			t.Fatalf("%s: signature did not change (%q)", mutation.name, mutation.got)
		}
	}
}

// The body digest must be taken from the exact bytes sent, through both the GetBody path
// (net/http installs it for in-memory bodies) and the buffer-and-restore path, and signing
// must leave the request body readable for the transport.
func TestSignRequestDigestsTheExactBody(t *testing.T) {
	Global().Clear()
	t.Cleanup(func() { Global().Clear() })
	Configure("instance-42", "top-secret")

	body := []byte(`{"organizations":[{"id":"1"}]}`)
	builders := []struct {
		name  string
		build func() *http.Request
	}{
		{"httptest reader", func() *http.Request {
			return httptest.NewRequest("POST", "https://console.example.com/v1/internal/pre/organizations", bytes.NewReader(body))
		}},
		{"net/http reader", func() *http.Request {
			req, err := http.NewRequestWithContext(context.Background(), "POST", "https://console.example.com/v1/internal/pre/organizations", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			return req
		}},
		{"net/http buffer", func() *http.Request {
			req, err := http.NewRequestWithContext(context.Background(), "POST", "https://console.example.com/v1/internal/pre/organizations", bytes.NewBuffer(body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			return req
		}},
	}
	for _, builder := range builders {
		t.Run(builder.name, func(t *testing.T) {
			req := builder.build()
			SignRequest(req)
			wantSignatureFromRequest(t, req, "top-secret", "/v1/internal/pre/organizations", body)

			// The signature covers the body, so the exact bytes must differ from a digest
			// over other content.
			wrong := expectedHTTP("top-secret", req.Header.Get(HeaderNonce), req.Header.Get(HeaderTimestamp), "POST", "/v1/internal/pre/organizations", []byte(`{"tampered":true}`))
			if req.Header.Get(HeaderSignature) == wrong {
				t.Fatalf("signature verified against a different body")
			}

			// Signing must not consume the request body.
			sent, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("read body after signing: %v", err)
			}
			if !bytes.Equal(sent, body) {
				t.Fatalf("body after signing = %q, want %q", sent, body)
			}
		})
	}
}

func TestNonceIsFreshPerRequestAnd32LowercaseHex(t *testing.T) {
	Global().Clear()
	t.Cleanup(func() { Global().Clear() })
	Configure("instance-42", "top-secret")

	seen := make(map[string]bool)
	for i := 0; i < 32; i++ {
		req := httptest.NewRequest("GET", "https://console.example.com/v1/internal/pre/commands", nil)
		SignRequest(req)
		nonce := req.Header.Get(HeaderNonce)
		if len(nonce) != 32 {
			t.Fatalf("nonce = %q (len %d), want 32 lowercase hex chars", nonce, len(nonce))
		}
		if strings.Trim(nonce, "0123456789abcdef") != "" {
			t.Fatalf("nonce = %q, want lowercase hex only", nonce)
		}
		if seen[nonce] {
			t.Fatalf("nonce %q repeated across requests", nonce)
		}
		seen[nonce] = true
	}

	nonce, err := NewNonce()
	if err != nil {
		t.Fatalf("NewNonce() error = %v", err)
	}
	if len(nonce) != 32 || strings.Trim(nonce, "0123456789abcdef") != "" {
		t.Fatalf("NewNonce() = %q, want 32 lowercase hex chars", nonce)
	}
}

func TestEmptyBodyDigestsAsSHA256OfEmpty(t *testing.T) {
	const sha256OfEmpty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := BodyDigest(nil); got != sha256OfEmpty {
		t.Fatalf("BodyDigest(nil) = %q, want %q", got, sha256OfEmpty)
	}
	if got := BodyDigest([]byte{}); got != sha256OfEmpty {
		t.Fatalf("BodyDigest(empty) = %q, want %q", got, sha256OfEmpty)
	}

	Global().Clear()
	t.Cleanup(func() { Global().Clear() })
	Configure("instance-42", "top-secret")

	req := httptest.NewRequest("GET", "https://console.example.com/v1/internal/pre/desired-state", nil)
	SignRequest(req)
	wantSignatureFromRequest(t, req, "top-secret", "/v1/internal/pre/desired-state", nil)
	want := expectedHTTP("top-secret", req.Header.Get(HeaderNonce), req.Header.Get(HeaderTimestamp), "GET", "/v1/internal/pre/desired-state", []byte{})
	if got := req.Header.Get(HeaderSignature); got != want {
		t.Fatalf("signature = %q, want the signature over sha256(\"\")", got)
	}
}

// The query string is sent on the wire but is not part of the signed path, exactly like the
// console verifier which reads r.URL.Path.
func TestQueryStringIsSentButNotSigned(t *testing.T) {
	Global().Clear()
	t.Cleanup(func() { Global().Clear() })
	Configure("instance-42", "top-secret")

	const (
		path  = "/v1/internal/pre/commands"
		query = "after_seq=7&limit=100"
	)
	req := httptest.NewRequest("GET", "https://console.example.com"+path+"?"+query, nil)
	SignRequest(req)

	if got := req.URL.RawQuery; got != query {
		t.Fatalf("RawQuery = %q, want %q: the query must still be sent", got, query)
	}
	nonce := req.Header.Get(HeaderNonce)
	timestamp := req.Header.Get(HeaderTimestamp)
	want := expectedHTTP("top-secret", nonce, timestamp, "GET", path, nil)
	if got := req.Header.Get(HeaderSignature); got != want {
		t.Fatalf("signature = %q, want the signature over the queryless path (%q)", got, want)
	}
	if withQuery := expectedHTTP("top-secret", nonce, timestamp, "GET", path+"?"+query, nil); req.Header.Get(HeaderSignature) == withQuery {
		t.Fatalf("signature covers the query string, which the console verifier strips")
	}
}

func TestSignRequestIsNoOpWithoutCredential(t *testing.T) {
	Global().Clear()
	req := httptest.NewRequest("GET", "https://console.example.com/v1/internal/pre/commands", nil)
	SignRequest(req)

	// Without a credential the legacy shared-secret path must stay in charge.
	if req.Header.Get(HeaderInstanceID) != "" || req.Header.Get(HeaderSignature) != "" || req.Header.Get(HeaderNonce) != "" {
		t.Fatalf("instance headers must not be set without a loaded credential")
	}
}

func TestClearDropsTheCredential(t *testing.T) {
	Configure("instance-1", "secret")
	if !Global().Loaded() {
		t.Fatalf("credential must be loaded after Configure")
	}
	Global().Clear()
	if Global().Loaded() {
		t.Fatalf("credential must be gone after Clear")
	}
	if Global().Stale() {
		t.Fatalf("a cleared store must not report a stale link")
	}
	req := httptest.NewRequest("GET", "https://console.example.com/v1/internal/pre/commands", nil)
	SignRequest(req)
	if req.Header.Get(HeaderSignature) != "" {
		t.Fatalf("a cleared credential must stop signing official calls")
	}
}

// The stale gate is the explicit store flag, not Clear(): the sync client keeps signing so
// the link can recover, while the official HTTP channel is refused.
func TestStaleFlagSuspendsOnlyTheOfficialChannel(t *testing.T) {
	Global().Clear()
	t.Cleanup(func() { Global().Clear() })
	Configure("instance-1", "secret")
	Global().SetStale(true)

	official := httptest.NewRequest("POST", "https://console.example.com/v1/internal/channels/routing", nil)
	SignRequest(official)
	if official.Header.Get(HeaderInstanceID) != "" || official.Header.Get(HeaderSignature) != "" {
		t.Fatalf("the official channel must not be signed while stale")
	}

	sync := httptest.NewRequest("POST", "https://console.example.com/v1/internal/pre/heartbeat", nil)
	SignSyncRequest(sync)
	if sync.Header.Get(HeaderInstanceID) != "instance-1" || sync.Header.Get(HeaderSignature) == "" {
		t.Fatalf("the sync client must keep signing while stale")
	}
	wantSignatureFromRequest(t, sync, "secret", "/v1/internal/pre/heartbeat", nil)

	Global().SetStale(false)
	officialAgain := httptest.NewRequest("POST", "https://console.example.com/v1/internal/channels/routing", nil)
	SignRequest(officialAgain)
	if officialAgain.Header.Get(HeaderInstanceID) != "instance-1" {
		t.Fatalf("clearing the stale flag must re-enable the official channel")
	}
}

// While stale the gRPC official channel fails closed with a local error instead of sending
// an unsigned call.
func TestGRPCCredentialsRefuseWhileStale(t *testing.T) {
	Global().Clear()
	t.Cleanup(func() { Global().Clear() })
	Configure("instance-9", "grpc-secret")
	Global().SetStale(true)

	ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{
		Method: "/zgi.rpc.v1.BillingService/CheckCreditBalance",
	})
	metadata, err := NewGRPCCredentials().GetRequestMetadata(ctx)
	if !errors.Is(err, ErrConsoleLinkStale) {
		t.Fatalf("error = %v, want ErrConsoleLinkStale", err)
	}
	if len(metadata) != 0 {
		t.Fatalf("metadata = %v, want none while stale", metadata)
	}
	if !Global().Loaded() {
		t.Fatalf("the credential must stay loaded while stale")
	}
}

// A no-credential (rollout) store sends nothing; a stale store refuses, and the two are
// distinguishable by the error.
func TestGRPCCredentialsWithoutCredentialSendNothing(t *testing.T) {
	Global().Clear()
	metadata, err := NewGRPCCredentials().GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("GetRequestMetadata() error = %v", err)
	}
	if len(metadata) != 0 {
		t.Fatalf("metadata = %v, want empty during the rollout window", metadata)
	}
}

func TestGRPCCredentialsCarryBearerAndHMAC(t *testing.T) {
	Configure("instance-9", "grpc-secret")
	t.Cleanup(func() { Global().Clear() })

	method := "/zgi.rpc.v1.BillingService/CheckCreditBalance"
	ctx := credentials.NewContextWithRequestInfo(context.Background(), credentials.RequestInfo{Method: method})

	metadata, err := NewGRPCCredentials().GetRequestMetadata(ctx)
	if err != nil {
		t.Fatalf("GetRequestMetadata() error = %v", err)
	}
	if metadata["authorization"] != "Bearer grpc-secret" {
		t.Fatalf("authorization = %q", metadata["authorization"])
	}
	if metadata[strings.ToLower(HeaderInstanceID)] != "instance-9" {
		t.Fatalf("instance id metadata = %q", metadata[strings.ToLower(HeaderInstanceID)])
	}
	timestamp := metadata[strings.ToLower(HeaderTimestamp)]
	if timestamp == "" {
		t.Fatalf("missing timestamp metadata")
	}
	// The signature must cover the full method so it cannot be replayed on another RPC.
	// gRPC deliberately keeps the pre-nonce shape: the marshaled message is not available
	// to PerRPCCredentials and the console gRPC interceptor verifies timestamp|method.
	want := signGRPCMethod("grpc-secret", timestamp, method)
	if got := metadata[strings.ToLower(HeaderSignature)]; got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
	if _, ok := metadata[strings.ToLower(HeaderNonce)]; ok {
		t.Fatalf("gRPC must not send %s: its interceptor does not verify a nonce", HeaderNonce)
	}
}

func TestConfigureRejectsIncompleteCredential(t *testing.T) {
	Configure("instance-1", "secret")
	Configure("instance-1", "")
	if Global().Loaded() {
		t.Fatalf("a credential without a secret must not be loaded")
	}
	Global().Clear()
}
