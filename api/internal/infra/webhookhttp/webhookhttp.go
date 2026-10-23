// Package webhookhttp sends webhooks to customer URLs: an HTTP client that
// reaches only public addresses and a request signature receivers verify.
package webhookhttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"time"
)

// Timeout bounds one webhook request.
const Timeout = 10 * time.Second

// Sign signs a webhook body: HMAC-SHA256 over "<timestamp>.<body>", hex
// encoded. Receivers recompute it with their secret, compare it to the
// X-ZGI-Signature header ("sha256=<hex>"), and reject old timestamps.
func Sign(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// PublicAddr reports whether an address is a public unicast address a
// webhook may reach.
func PublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// Client dials only public addresses resolved at connect time, so a webhook
// cannot reach internal services, and follows no redirects. allowPrivate is
// for local development only.
func Client(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Client{
		Timeout:       Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
				host, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
				if err != nil {
					return nil, err
				}
				var lastErr error = errors.New("webhook host has no public address")
				for _, addr := range addrs {
					if !allowPrivate && !PublicAddr(addr) {
						continue
					}
					conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.Unmap().String(), port))
					if err == nil {
						return conn, nil
					}
					lastErr = err
				}
				return nil, lastErr
			},
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: Timeout,
			DisableKeepAlives:     true,
		},
	}
}
