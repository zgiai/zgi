package integrations

import (
	"strings"
	"testing"
)

func TestProviderInstallOpaqueReferencesAndReturnPath(t *testing.T) {
	firstRef, err := randomProviderInstallReference()
	if err != nil {
		t.Fatal(err)
	}
	secondRef, err := randomProviderInstallReference()
	if err != nil {
		t.Fatal(err)
	}
	state, err := randomProviderInstallState()
	if err != nil {
		t.Fatal(err)
	}
	if firstRef == secondRef || len(firstRef) < 40 || len(state) != 64 || !sha256HexPattern.MatchString(state) {
		t.Fatalf("opaque reference contract failed: first=%q second=%q state=%q", firstRef, secondRef, state)
	}
	if providerInstallDigest(firstRef) == firstRef || !sha256HexPattern.MatchString(providerInstallDigest(firstRef)) {
		t.Fatal("provider install references must be persisted only as SHA-256 digests")
	}
	if got := safeProviderInstallReturnPath("/console/integrations?view=connected"); got != "/console/integrations?view=connected" {
		t.Fatalf("safe return path=%q", got)
	}
	for _, unsafe := range []string{"https://evil.example/", "//evil.example/", "/api/private", "/console/x\r\nLocation:https://evil.example"} {
		if got := safeProviderInstallReturnPath(unsafe); got != "/console/integrations" {
			t.Fatalf("unsafe return path %q accepted as %q", unsafe, got)
		}
	}
}

func TestProviderInstallGrantDestroyClearsSecrets(t *testing.T) {
	grant := ProviderInstallGrant{
		ExternalTenantHash: strings.Repeat("a", 64),
		AccountDisplayName: "Example",
		Credentials:        map[string]string{"permanent_code": "secret"},
		Config:             map[string]any{"authorization_mode": "suite"},
		GrantedScopes:      []string{"wecom:app"},
	}
	grant.Destroy()
	if grant.ExternalTenantHash != "" || grant.AccountDisplayName != "" || grant.Credentials != nil || grant.Config != nil || grant.GrantedScopes != nil {
		t.Fatalf("grant was not cleared: %#v", grant)
	}
}
