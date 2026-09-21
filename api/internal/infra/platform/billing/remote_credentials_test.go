package billing

import (
	"context"
	"testing"
)

func TestConsoleAPICredentialsAttachBearerToken(t *testing.T) {
	metadata, err := consoleAPICredentials{token: "sk-zgi-downstream"}.GetRequestMetadata(
		context.Background(),
		"/zgi.rpc.v1.BillingService/PreDeductQuota",
	)
	if err != nil {
		t.Fatalf("GetRequestMetadata() error = %v", err)
	}
	if got := metadata["authorization"]; got != "Bearer sk-zgi-downstream" {
		t.Fatalf("authorization = %q, want %q", got, "Bearer sk-zgi-downstream")
	}
	if (consoleAPICredentials{}).RequireTransportSecurity() {
		t.Fatal("internal-network credentials must not require transport security")
	}
}
