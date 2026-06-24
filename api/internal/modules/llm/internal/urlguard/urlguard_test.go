package urlguard

import (
	"context"
	"net"
	"testing"
)

type fakeResolver map[string][]net.IPAddr

func (r fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return r[host], nil
}

func TestValidateBaseURLRejectsUnsafeTargets(t *testing.T) {
	tests := []string{
		"http://127.0.0.1:11434",
		"http://localhost:11434",
		"http://10.0.0.1:8000",
		"http://192.168.1.10:8000",
		"http://169.254.169.254/latest/meta-data",
		"http://[::ffff:127.0.0.1]:8080",
		"file:///etc/passwd",
		"https://safe.example.com@127.0.0.1",
		"/relative/path",
		"https://api.example.com/v1#fragment",
	}

	for _, raw := range tests {
		if err := ValidateBaseURL(context.Background(), raw, Policy{}); err == nil {
			t.Fatalf("ValidateBaseURL(%q) error = nil, want rejection", raw)
		}
	}
}

func TestValidateBaseURLAllowsPublicTargets(t *testing.T) {
	policy := Policy{
		Resolver: fakeResolver{
			"api.example.com": {{IP: net.ParseIP("93.184.216.34")}},
		},
	}

	if err := ValidateBaseURL(context.Background(), "https://api.example.com/v1", policy); err != nil {
		t.Fatalf("ValidateBaseURL(public) error = %v, want nil", err)
	}
}

func TestValidateBaseURLRejectsDomainResolvingToPrivateIP(t *testing.T) {
	policy := Policy{
		Resolver: fakeResolver{
			"evil.example.com": {{IP: net.ParseIP("127.0.0.1")}},
		},
	}

	err := ValidateBaseURL(context.Background(), "https://evil.example.com/v1", policy)
	if err == nil {
		t.Fatal("ValidateBaseURL(domain resolving to loopback) error = nil, want rejection")
	}
}

func TestValidateBaseURLAllowsPrivateWhenPolicyAllowsIt(t *testing.T) {
	policy := Policy{AllowPrivate: true}

	if err := ValidateBaseURL(context.Background(), "http://localhost:11434", policy); err != nil {
		t.Fatalf("ValidateBaseURL(ollama private allowed) error = %v, want nil", err)
	}
}
