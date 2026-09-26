package consolecred

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/credentials"
)

func TestSignRequestAddsInstanceHeaders(t *testing.T) {
	Global().Clear()
	t.Cleanup(func() { Global().Clear() })
	Configure("instance-42", "top-secret")

	req := httptest.NewRequest("POST", "https://console.example.com/v1/internal/pre/heartbeat", nil)
	SignRequest(req)

	if got := req.Header.Get(HeaderInstanceID); got != "instance-42" {
		t.Fatalf("%s = %q", HeaderInstanceID, got)
	}
	timestamp := req.Header.Get(HeaderTimestamp)
	if timestamp == "" {
		t.Fatalf("missing timestamp header")
	}
	want := Sign("top-secret", timestamp, "/v1/internal/pre/heartbeat")
	if got := req.Header.Get(HeaderSignature); got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestSignRequestIsNoOpWithoutCredential(t *testing.T) {
	Global().Clear()
	req := httptest.NewRequest("GET", "https://console.example.com/v1/internal/pre/commands", nil)
	SignRequest(req)

	// Without a credential the legacy shared-secret path must stay in charge.
	if req.Header.Get(HeaderInstanceID) != "" || req.Header.Get(HeaderSignature) != "" {
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
	want := Sign("grpc-secret", timestamp, method)
	if got := metadata[strings.ToLower(HeaderSignature)]; got != want {
		t.Fatalf("signature = %q, want %q", got, want)
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
