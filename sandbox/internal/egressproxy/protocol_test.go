package egressproxy

import (
	"bytes"
	"context"
	"testing"
)

func TestFrameRoundTripIsBounded(t *testing.T) {
	var buffer bytes.Buffer
	input := Frame{Type: FrameTypeConnect, Target: "packages.example:443"}
	if err := WriteFrame(&buffer, input); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	output, err := ReadFrame(&buffer)
	if err != nil || output != input {
		t.Fatalf("round trip = %+v, %v", output, err)
	}
}

func TestPolicyAllowsOnlyExactOrSingleSubdomainWildcard(t *testing.T) {
	policy := Policy{AllowedHosts: []string{"pypi.org", "*.pythonhosted.org"}}
	if err := policy.Validate(); err != nil {
		t.Fatalf("validate policy: %v", err)
	}
	for host, want := range map[string]bool{
		"pypi.org":                      true,
		"files.pythonhosted.org":        true,
		"mirror.files.pythonhosted.org": false,
		"pythonhosted.org":              false,
		"evilpythonhosted.org":          false,
		"registry.npmjs.org":            false,
	} {
		if got := policy.Allows(host); got != want {
			t.Errorf("Allows(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestPolicyRejectsUnsafeDeclarations(t *testing.T) {
	for _, hosts := range [][]string{{}, {"127.0.0.1"}, {"*."}, {"pypi.org", "pypi.org"}, {"pypi.org:443"}} {
		if err := (Policy{AllowedHosts: hosts}).Validate(); err == nil {
			t.Errorf("unsafe policy %v was accepted", hosts)
		}
	}
}

func TestResolveAndDialRejectsUnallowlistedPrivateAndNonHTTPSTargets(t *testing.T) {
	policy := Policy{AllowedHosts: []string{"localhost"}}
	if _, err := policy.ResolveAndDial(context.Background(), "example.com:443"); err == nil {
		t.Fatal("unallowlisted host was accepted")
	}
	if _, err := policy.ResolveAndDial(context.Background(), "localhost:8443"); err == nil {
		t.Fatal("non-HTTPS port was accepted")
	}
	if _, err := policy.ResolveAndDial(context.Background(), "localhost:443"); err == nil {
		t.Fatal("private localhost address was accepted")
	}
}

func TestValidateTargetURLIsHTTPS443WithoutCredentials(t *testing.T) {
	for raw, want := range map[string]string{
		"https://pypi.org/simple":                    "pypi.org:443",
		"https://files.pythonhosted.org:443/pkg.whl": "files.pythonhosted.org:443",
	} {
		got, err := ValidateTargetURL(raw)
		if err != nil || got != want {
			t.Errorf("ValidateTargetURL(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"http://pypi.org/simple",
		"https://pypi.org:8443/simple",
		"https://user:password@pypi.org/simple",
		"https://",
	} {
		if _, err := ValidateTargetURL(raw); err == nil {
			t.Errorf("ValidateTargetURL(%q) accepted an unsafe URL", raw)
		}
	}
}
