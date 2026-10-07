package egressproxy

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"
)

type idleBridgeConnection struct {
	net.Conn
	reading     chan bool
	deadlineSet bool
}

func (c *idleBridgeConnection) SetDeadline(deadline time.Time) error {
	c.deadlineSet = !deadline.IsZero()
	return c.Conn.SetDeadline(deadline)
}

func (c *idleBridgeConnection) Read(buffer []byte) (int, error) {
	select {
	case c.reading <- c.deadlineSet:
	default:
	}
	return c.Conn.Read(buffer)
}

func TestQueuedBridgeWaitsForUseUntilLifecycleCancellation(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()
	connection := &idleBridgeConnection{Conn: server, reading: make(chan bool, 1)}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ServeConnection(ctx, connection, Policy{AllowedHosts: []string{"pypi.org"}}) }()
	select {
	case deadlineSet := <-connection.reading:
		if deadlineSet {
			t.Fatal("queued bridge would expire before the next installer uses it")
		}
	case <-time.After(time.Second):
		t.Fatal("bridge did not wait for its first request")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled idle bridge unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("lifecycle cancellation did not close an idle bridge")
	}
}

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

func TestBlockedAddressRejectsIPv6AndSpecialNetworks(t *testing.T) {
	for _, address := range []string{"2001:db8::1", "127.0.0.1", "10.0.0.1", "169.254.169.254"} {
		if !blockedAddress(net.ParseIP(address)) {
			t.Fatalf("blockedAddress(%q) = false", address)
		}
	}
	if blockedAddress(net.ParseIP("151.101.64.223")) {
		t.Fatal("public IPv4 address was blocked")
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
