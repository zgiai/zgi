package policy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

type EgressDecision struct {
	Allowed              bool     `json:"allowed"`
	Code                 string   `json:"code"`
	Reason               string   `json:"reason"`
	Policy               string   `json:"policy"`
	Destination          string   `json:"destination"`
	Protocol             string   `json:"protocol"`
	Host                 string   `json:"host"`
	Port                 int      `json:"port"`
	ResolvedIPs          []string `json:"resolved_ips,omitempty"`
	DeniedCIDR           string   `json:"denied_cidr,omitempty"`
	MaxRequestDurationMS int      `json:"max_request_duration_ms"`
}

type egressResolver func(context.Context, string) ([]netip.Addr, error)

func (s *Service) CheckEgressDestination(ctx context.Context, policyName string, destination string) (EgressDecision, error) {
	return s.checkEgressDestination(ctx, policyName, destination, defaultEgressResolver)
}

func (s *Service) checkEgressDestination(ctx context.Context, policyName string, destination string, resolver egressResolver) (EgressDecision, error) {
	profile, ok := s.networkProfile(policyName)
	if !ok {
		return EgressDecision{}, fmt.Errorf("unsupported network policy: %s", policyName)
	}
	decision := EgressDecision{
		Policy:               profile.Name,
		Destination:          strings.TrimSpace(destination),
		MaxRequestDurationMS: profile.MaxRequestDurationMS,
	}
	if !profile.NetworkEnabled {
		return denyEgress(decision, "egress_denied_network_disabled", "network policy does not allow outbound access"), nil
	}

	parsed, err := url.Parse(decision.Destination)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return EgressDecision{}, errors.New("destination must be an absolute URL")
	}
	decision.Protocol = strings.ToLower(parsed.Scheme)
	decision.Host = strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	decision.Port, err = egressPort(parsed)
	if err != nil {
		return EgressDecision{}, err
	}
	if !containsString(profile.AllowedProtocols, decision.Protocol) {
		return denyEgress(decision, "egress_denied_protocol", "protocol is not allowed by network policy"), nil
	}
	if !containsInt(profile.AllowedPorts, decision.Port) {
		return denyEgress(decision, "egress_denied_port", "port is not allowed by network policy"), nil
	}
	if len(profile.AllowedHosts) > 0 && !containsHost(profile.AllowedHosts, decision.Host) {
		return denyEgress(decision, "egress_denied_host", "host is not allowed by network policy"), nil
	}

	addrs, err := resolveEgressHost(ctx, decision.Host, resolver)
	if err != nil {
		return denyEgress(decision, "egress_denied_dns_resolution", err.Error()), nil
	}
	denied, deniedCIDR := deniedEgressAddress(addrs, profile.DeniedCIDRRanges)
	decision.ResolvedIPs = formatEgressAddresses(addrs)
	if denied {
		decision.DeniedCIDR = deniedCIDR
		return denyEgress(decision, "egress_denied_cidr", "destination resolves to a denied network range"), nil
	}

	decision.Allowed = true
	decision.Code = "egress_allowed"
	decision.Reason = "network policy allows destination"
	return decision, nil
}

func (s *Service) networkProfile(policyName string) (NetworkProfile, bool) {
	for _, item := range s.networkProfiles {
		if item.Name == policyName {
			return item, true
		}
	}
	return NetworkProfile{}, false
}

func egressPort(parsed *url.URL) (int, error) {
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port <= 0 || port > 65535 {
			return 0, errors.New("destination port is invalid")
		}
		return port, nil
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http":
		return 80, nil
	case "https":
		return 443, nil
	default:
		return 0, errors.New("destination port is required for this protocol")
	}
}

func resolveEgressHost(ctx context.Context, host string, resolver egressResolver) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr}, nil
	}
	addrs, err := resolver(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("destination host could not be resolved: %w", err)
	}
	if len(addrs) == 0 {
		return nil, errors.New("destination host did not resolve to an IP address")
	}
	return addrs, nil
}

func defaultEgressResolver(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func deniedEgressAddress(addrs []netip.Addr, cidrs []string) (bool, string) {
	for _, raw := range cidrs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if prefix.Contains(addr) {
				return true, prefix.String()
			}
		}
	}
	return false, ""
}

func formatEgressAddresses(addrs []netip.Addr) []string {
	values := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		values = append(values, addr.String())
	}
	return values
}

func denyEgress(decision EgressDecision, code string, reason string) EgressDecision {
	decision.Allowed = false
	decision.Code = code
	decision.Reason = reason
	return decision
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), expected) {
			return true
		}
	}
	return false
}

func containsHost(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(value), "."), expected) {
			return true
		}
	}
	return false
}

func containsInt(values []int, expected int) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
