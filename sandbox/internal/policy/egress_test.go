package policy

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/config"
)

func TestCheckEgressDestinationAllowsPublicHTTPSDestination(t *testing.T) {
	service := NewService(config.FromEnv())

	decision, err := service.checkEgressDestination(context.Background(), "workflow-safe", "https://example.com/path", fixedEgressResolver("93.184.216.34"))
	if err != nil {
		t.Fatalf("check egress destination: %v", err)
	}
	if !decision.Allowed || decision.Code != "egress_allowed" {
		t.Fatalf("expected allowed decision, got %+v", decision)
	}
	if decision.Protocol != "https" || decision.Host != "example.com" || decision.Port != 443 {
		t.Fatalf("unexpected normalized destination: %+v", decision)
	}
	if len(decision.ResolvedIPs) != 1 || decision.ResolvedIPs[0] != "93.184.216.34" {
		t.Fatalf("unexpected resolved IPs: %+v", decision)
	}
}

func TestCheckEgressDestinationBlocksDeniedCIDRRanges(t *testing.T) {
	service := NewService(config.FromEnv())

	cases := map[string]string{
		"https://127.0.0.1":          "127.0.0.0/8",
		"https://10.1.2.3":           "10.0.0.0/8",
		"https://169.254.169.254":    "169.254.0.0/16",
		"https://[fe80::1]":          "fe80::/10",
		"https://metadata.internal":  "169.254.0.0/16",
		"https://rebinding.internal": "10.0.0.0/8",
	}
	for destination, deniedCIDR := range cases {
		decision, err := service.checkEgressDestination(context.Background(), "workflow-safe", destination, resolverByHost(map[string][]string{
			"metadata.internal":  {"169.254.169.254"},
			"rebinding.internal": {"93.184.216.34", "10.10.10.10"},
		}))
		if err != nil {
			t.Fatalf("check %s: %v", destination, err)
		}
		if decision.Allowed || decision.Code != "egress_denied_cidr" || decision.DeniedCIDR != deniedCIDR {
			t.Fatalf("expected %s to be denied by %s, got %+v", destination, deniedCIDR, decision)
		}
	}
}

func TestCheckEgressDestinationRejectsProtocolPortAndDisabledPolicy(t *testing.T) {
	service := NewService(config.FromEnv())

	disabled, err := service.checkEgressDestination(context.Background(), "deny-by-default", "https://example.com", fixedEgressResolver("93.184.216.34"))
	if err != nil {
		t.Fatalf("check disabled policy: %v", err)
	}
	if disabled.Allowed || disabled.Code != "egress_denied_network_disabled" {
		t.Fatalf("expected disabled policy denial, got %+v", disabled)
	}

	protocol, err := service.checkEgressDestination(context.Background(), "workflow-safe", "http://example.com", fixedEgressResolver("93.184.216.34"))
	if err != nil {
		t.Fatalf("check protocol: %v", err)
	}
	if protocol.Allowed || protocol.Code != "egress_denied_protocol" {
		t.Fatalf("expected protocol denial, got %+v", protocol)
	}

	port, err := service.checkEgressDestination(context.Background(), "workflow-safe", "https://example.com:8443", fixedEgressResolver("93.184.216.34"))
	if err != nil {
		t.Fatalf("check port: %v", err)
	}
	if port.Allowed || port.Code != "egress_denied_port" {
		t.Fatalf("expected port denial, got %+v", port)
	}
}

func TestCheckEgressDestinationReportsDNSFailure(t *testing.T) {
	service := NewService(config.FromEnv())

	decision, err := service.checkEgressDestination(context.Background(), "workflow-safe", "https://missing.internal", func(context.Context, string) ([]netip.Addr, error) {
		return nil, errors.New("no such host")
	})
	if err != nil {
		t.Fatalf("check DNS failure: %v", err)
	}
	if decision.Allowed || decision.Code != "egress_denied_dns_resolution" {
		t.Fatalf("expected DNS denial, got %+v", decision)
	}
}

func fixedEgressResolver(values ...string) egressResolver {
	return func(context.Context, string) ([]netip.Addr, error) {
		return parseTestAddrs(values), nil
	}
}

func resolverByHost(values map[string][]string) egressResolver {
	return func(_ context.Context, host string) ([]netip.Addr, error) {
		return parseTestAddrs(values[host]), nil
	}
}

func parseTestAddrs(values []string) []netip.Addr {
	addrs := make([]netip.Addr, 0, len(values))
	for _, value := range values {
		addrs = append(addrs, netip.MustParseAddr(value))
	}
	return addrs
}
