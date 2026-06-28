package urlguard

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// Resolver is the DNS surface used by the guard. Tests can inject a fake
// resolver; outbound HTTP uses net.DefaultResolver.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type Policy struct {
	AllowPrivate bool
	GuardDNS     bool
	Resolver     Resolver
}

var (
	metadataIPv4        = netip.MustParseAddr("169.254.169.254")
	alibabaMetadataIPv4 = netip.MustParseAddr("100.100.100.200")
	awsMetadataIPv6     = netip.MustParseAddr("fd00:ec2::254")

	blockedPublicBypassPrefixes = []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
		netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
		netip.MustParsePrefix("192.0.2.0/24"),    // documentation
		netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
		netip.MustParsePrefix("198.51.100.0/24"), // documentation
		netip.MustParsePrefix("203.0.113.0/24"),  // documentation
		netip.MustParsePrefix("2001:db8::/32"),   // documentation
		netip.MustParsePrefix("2001::/32"),       // Teredo
		netip.MustParsePrefix("2002::/16"),       // 6to4
		netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use NAT64
		netip.MustParsePrefix("0100::/64"),       // discard-only
		netip.MustParsePrefix("3fff::/20"),       // documentation
		netip.MustParsePrefix("5f00::/16"),       // segment routing SIDs
	}
)

func ValidateBaseURL(ctx context.Context, raw string, policy Policy) error {
	parsed, err := parseBaseURL(raw)
	if err != nil {
		return err
	}
	return ValidateURL(ctx, parsed, policy)
}

func ValidateURL(ctx context.Context, parsed *url.URL, policy Policy) error {
	if parsed == nil {
		return fmt.Errorf("base_url is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("base_url scheme must be http or https")
	}
	if parsed.User != nil {
		return fmt.Errorf("base_url must not contain userinfo")
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("base_url host is required")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("base_url fragment is not allowed")
	}

	if _, err := safeHostAddrs(ctx, parsed.Hostname(), policy, policy.GuardDNS); err != nil {
		return err
	}
	return nil
}

func ResolveSafeHost(ctx context.Context, host string, policy Policy) ([]netip.Addr, error) {
	return safeHostAddrs(ctx, host, policy, true)
}

func parseBaseURL(raw string) (*url.URL, error) {
	baseURL := strings.TrimSpace(raw)
	if baseURL == "" {
		return nil, fmt.Errorf("base_url is required")
	}

	if strings.HasSuffix(baseURL, "#") {
		baseURL = strings.TrimSpace(strings.TrimSuffix(baseURL, "#"))
		if baseURL == "" {
			return nil, fmt.Errorf("base_url before # is required")
		}
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	if !parsed.IsAbs() {
		return nil, fmt.Errorf("base_url must be an absolute URL")
	}
	return parsed, nil
}

func safeHostAddrs(ctx context.Context, host string, policy Policy, resolveDNS bool) ([]netip.Addr, error) {
	normalizedHost := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if normalizedHost == "" {
		return nil, fmt.Errorf("base_url host is required")
	}
	if normalizedHost == "localhost" || strings.HasSuffix(normalizedHost, ".localhost") {
		if policy.AllowPrivate {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return nil, fmt.Errorf("blocked unsafe target %q: localhost is not allowed", host)
	}

	if addr, err := netip.ParseAddr(normalizedHost); err == nil {
		if err := validateAddr(addr, policy); err != nil {
			return nil, fmt.Errorf("blocked unsafe target %q: %w", host, err)
		}
		return []netip.Addr{addr.Unmap()}, nil
	}
	if !resolveDNS {
		return nil, nil
	}

	resolver := policy.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	resolved, err := resolver.LookupIPAddr(ctx, normalizedHost)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("resolve %q: no addresses", host)
	}

	addrs := make([]netip.Addr, 0, len(resolved))
	for _, item := range resolved {
		addr, ok := addrFromIP(item.IP)
		if !ok {
			return nil, fmt.Errorf("resolve %q: invalid IP address", host)
		}
		if err := validateAddr(addr, policy); err != nil {
			return nil, fmt.Errorf("blocked unsafe target %q: %w", host, err)
		}
		addrs = append(addrs, addr.Unmap())
	}
	return addrs, nil
}

func addrFromIP(ip net.IP) (netip.Addr, bool) {
	if ip4 := ip.To4(); ip4 != nil {
		return netip.AddrFromSlice(ip4)
	}
	return netip.AddrFromSlice(ip)
}

func validateAddr(addr netip.Addr, policy Policy) error {
	if !addr.IsValid() {
		return fmt.Errorf("invalid IP address")
	}
	if addr.Is4In6() {
		return fmt.Errorf("IPv4-mapped IPv6 address is not allowed")
	}

	unmapped := addr.Unmap()
	if isMetadataAddr(unmapped) {
		return fmt.Errorf("metadata service address is not allowed")
	}
	if unmapped.IsUnspecified() {
		return fmt.Errorf("unspecified address is not allowed")
	}
	if unmapped.IsMulticast() {
		return fmt.Errorf("multicast address is not allowed")
	}
	if unmapped.IsLinkLocalUnicast() {
		return fmt.Errorf("link-local address is not allowed")
	}
	if unmapped.IsLoopback() {
		if policy.AllowPrivate {
			return nil
		}
		return fmt.Errorf("loopback address is not allowed")
	}
	if unmapped.IsPrivate() {
		if policy.AllowPrivate {
			return nil
		}
		return fmt.Errorf("private address is not allowed")
	}
	if !policy.AllowPrivate && isBlockedPublicBypass(unmapped) {
		return fmt.Errorf("special-use address is not allowed")
	}
	return nil
}

func isMetadataAddr(addr netip.Addr) bool {
	return addr == metadataIPv4 || addr == alibabaMetadataIPv4 || addr == awsMetadataIPv6
}

func isBlockedPublicBypass(addr netip.Addr) bool {
	for _, prefix := range blockedPublicBypassPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}
