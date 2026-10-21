// Package egressproxy implements the deliberately small protocol used to
// bridge HTTPS CONNECT requests out of a network-isolated runtime. The runtime
// never gets a host network namespace: it talks to the trusted worker over a
// host-visible Unix socket and the worker applies the allowlist before dialing.
package egressproxy

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	FrameTypeConnect = "connect"
	FrameTypeResult  = "connect_result"
	MaxFrameBytes    = 64 * 1024
	MaxTunnelBytes   = 64 * 1024 * 1024
)

type Frame struct {
	Type   string `json:"type"`
	Target string `json:"target,omitempty"`
	OK     bool   `json:"ok,omitempty"`
	Error  string `json:"error,omitempty"`
}

func WriteFrame(w io.Writer, frame Frame) error {
	payload, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > MaxFrameBytes {
		return fmt.Errorf("egress frame exceeds %d bytes", MaxFrameBytes)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, payload)
}

func writeAll(w io.Writer, value []byte) error {
	for len(value) > 0 {
		written, err := w.Write(value)
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		value = value[written:]
	}
	return nil
}

func ReadFrame(r io.Reader) (Frame, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > MaxFrameBytes {
		return Frame{}, errors.New("invalid egress frame size")
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Frame{}, err
	}
	var frame Frame
	if err := json.Unmarshal(payload, &frame); err != nil {
		return Frame{}, fmt.Errorf("decode egress frame: %w", err)
	}
	return frame, nil
}

// Policy is intentionally HTTPS-only. Hosts can be exact names or a single
// left-most wildcard such as *.pythonhosted.org. IP literals are never
// accepted, which also makes DNS rebinding checks explicit.
type Policy struct {
	AllowedHosts []string
}

func (p Policy) Validate() error {
	if len(p.AllowedHosts) == 0 {
		return errors.New("controlled egress requires at least one allowed host")
	}
	seen := make(map[string]struct{}, len(p.AllowedHosts))
	for _, raw := range p.AllowedHosts {
		host := strings.ToLower(strings.TrimSpace(raw))
		if host == "" || net.ParseIP(strings.TrimPrefix(host, "*.")) != nil || strings.Contains(host, "/") || strings.Contains(host, ":") {
			return fmt.Errorf("invalid egress host %q", raw)
		}
		if strings.HasPrefix(host, "*.") {
			if len(host) <= 2 || strings.Contains(host[2:], "*") {
				return fmt.Errorf("invalid egress wildcard %q", raw)
			}
		} else if strings.Contains(host, "*") {
			return fmt.Errorf("invalid egress wildcard %q", raw)
		}
		candidate := strings.TrimPrefix(host, "*.")
		if len(candidate) > 253 || strings.HasPrefix(candidate, ".") || strings.HasSuffix(candidate, ".") {
			return fmt.Errorf("invalid egress hostname %q", raw)
		}
		for _, label := range strings.Split(candidate, ".") {
			if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return fmt.Errorf("invalid egress hostname %q", raw)
			}
			for _, character := range label {
				if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
					return fmt.Errorf("invalid egress hostname %q", raw)
				}
			}
		}
		if _, ok := seen[host]; ok {
			return fmt.Errorf("duplicate egress host %q", raw)
		}
		seen[host] = struct{}{}
	}
	return nil
}

func (p Policy) Allows(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, raw := range p.AllowedHosts {
		allowed := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
		if host == allowed {
			return true
		}
		if strings.HasPrefix(allowed, "*.") {
			base := strings.TrimPrefix(allowed, "*.")
			if strings.HasSuffix(host, "."+base) && strings.Count(host, ".") == strings.Count(base, ".")+1 {
				return true
			}
		}
	}
	return false
}

func (p Policy) ResolveAndDial(ctx context.Context, target string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return nil, errors.New("egress target must be host:port")
	}
	if !p.Allows(host) {
		return nil, errors.New("egress host is not allowlisted")
	}
	if port != "443" {
		return nil, errors.New("egress only permits HTTPS port 443")
	}
	ipAddresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ipAddresses) == 0 {
		return nil, errors.New("egress host could not be resolved")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var lastErr error
	for _, address := range ipAddresses {
		if blockedAddress(address.IP) {
			lastErr = errors.New("egress destination resolves to an IPv6, private, or special address")
			continue
		}
		connection, dialErr := dialer.DialContext(ctx, "tcp", net.JoinHostPort(address.IP.String(), port))
		if dialErr != nil {
			lastErr = dialErr
			continue
		}
		return connection, nil
	}
	if lastErr == nil {
		lastErr = errors.New("egress connection failed")
	}
	return nil, lastErr
}

func blockedAddress(address net.IP) bool {
	return address.To4() == nil || address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsUnspecified() || address.IsMulticast()
}

// ServeConnection handles one host-side bridge connection. A runtime proxy
// connection is deliberately one-shot so a failed tunnel cannot be reused.
func ServeConnection(ctx context.Context, connection net.Conn, policy Policy) error {
	defer connection.Close()
	closed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.Close()
		case <-closed:
		}
	}()
	defer close(closed)
	// Bridges are preconnected and queued by the Runtime Agent. An idle
	// deadline here would close queued bridges during a long compilation,
	// leaving the next installer with stale connections. Lifecycle cancellation
	// still closes idle bridges; bound the handshake after a request arrives.
	frame, err := ReadFrame(connection)
	if err != nil {
		return err
	}
	_ = connection.SetDeadline(time.Now().Add(30 * time.Second))
	if frame.Type != FrameTypeConnect {
		return errors.New("unexpected egress frame type")
	}
	remote, err := policy.ResolveAndDial(ctx, frame.Target)
	if err != nil {
		_ = WriteFrame(connection, Frame{Type: FrameTypeResult, Error: err.Error()})
		return err
	}
	defer remote.Close()
	if err := WriteFrame(connection, Frame{Type: FrameTypeResult, OK: true}); err != nil {
		return err
	}
	_ = connection.SetDeadline(time.Time{})
	var clientHello [3]byte
	if _, err := io.ReadFull(connection, clientHello[:]); err != nil {
		return err
	}
	if clientHello[0] != 0x16 || clientHello[1] != 0x03 || clientHello[2] < 0x01 || clientHello[2] > 0x04 {
		return errors.New("egress tunnel did not start with a TLS handshake")
	}
	if err := writeAll(remote, clientHello[:]); err != nil {
		return err
	}
	return tunnel(ctx, connection, remote)
}

func tunnel(ctx context.Context, left, right net.Conn) error {
	result := make(chan error, 2)
	copySide := func(destination net.Conn, source net.Conn) {
		_, err := io.CopyN(destination, source, MaxTunnelBytes)
		result <- err
	}
	go copySide(left, right)
	go copySide(right, left)
	select {
	case err := <-result:
		_ = left.Close()
		_ = right.Close()
		return err
	case <-ctx.Done():
		_ = left.Close()
		_ = right.Close()
		return ctx.Err()
	}
}

func ValidateTargetURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Hostname() == "" {
		return "", errors.New("egress URL must be an HTTPS URL without credentials")
	}
	port := parsed.Port()
	if port == "" {
		port = strconv.Itoa(443)
	}
	if port != "443" {
		return "", errors.New("egress URL must use port 443")
	}
	return net.JoinHostPort(parsed.Hostname(), port), nil
}
