package consoleauth

import (
	"context"
	"testing"
)

func TestCredentialsAttachBearerToken(t *testing.T) {
	metadata, err := Credentials{Token: "sk-zgi-downstream"}.GetRequestMetadata(
		context.Background(),
		"/zgi.rpc.v1.BillingService/PreDeductQuota",
	)
	if err != nil {
		t.Fatalf("GetRequestMetadata() error = %v", err)
	}
	if got := metadata["authorization"]; got != "Bearer sk-zgi-downstream" {
		t.Fatalf("authorization = %q, want %q", got, "Bearer sk-zgi-downstream")
	}
	if (Credentials{}).RequireTransportSecurity() {
		t.Fatal("internal-network credentials must not require transport security")
	}
}
