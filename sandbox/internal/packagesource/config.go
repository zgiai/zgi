// Package packagesource defines operator-managed sources shared by runtimes.
package packagesource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi-sandbox/internal/egressproxy"
)

// Config supplies one authoritative source per ecosystem, never from a Job.
// URLs cannot carry credentials; redirect/CDN hosts need separate approval.
type Config struct {
	PyPI        string `json:"pypi,omitempty"`
	NPM         string `json:"npm,omitempty"`
	APT         string `json:"apt,omitempty"`
	APTSecurity string `json:"apt_security,omitempty"`
}

// Bootstrap is delivered through authenticated envd before releasing the
// template startup token; restored template processes do not inherit envd env.
type Bootstrap struct {
	PlatformCLI     bool           `json:"platform_cli,omitempty"`
	Runtime         *RuntimeConfig `json:"runtime,omitempty"`
	Sources         Config         `json:"sources"`
	AllowedHosts    []string       `json:"allowed_hosts"`
	ProfileIdentity string         `json:"profile_identity"`
	Check           bool           `json:"check"`
}

// RuntimeConfig is Worker-owned startup policy. E2B template environment can
// shadow create-time variables, so a new template must consume this before
// advertising capabilities. The optional field preserves older payloads.
type RuntimeConfig struct {
	NetworkMode               string `json:"network_mode"`
	EgressMode                string `json:"egress_mode"`
	DependencyInstallation    bool   `json:"dependency_installation"`
	PackageNetworkMode        string `json:"package_network_mode"`
	DependencyPreparation     bool   `json:"dependency_preparation"`
	DependencyPreparationMode string `json:"dependency_preparation_mode"`
	JobUID                    uint32 `json:"job_uid"`
	JobGID                    uint32 `json:"job_gid"`
}

func (c Config) Digest() string {
	digest := sha256.Sum256([]byte(c.Resolved().JSON()))
	return hex.EncodeToString(digest[:])
}

func (c Config) Resolved() Config {
	if c.PyPI == "" {
		c.PyPI = "https://pypi.org/simple"
	}
	if c.NPM == "" {
		c.NPM = "https://registry.npmjs.org/"
	}
	if c.APT == "" {
		c.APT = "https://deb.debian.org/debian"
	}
	if c.APTSecurity == "" {
		c.APTSecurity = "https://security.debian.org/debian-security"
	}
	return c
}

func (c Config) values() []string { return []string{c.PyPI, c.NPM, c.APT, c.APTSecurity} }

// Validate checks syntax and optional provider policy without network access.
func (c Config) Validate(hosts []string) error {
	for _, value := range c.values() {
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || (u.Port() != "" && u.Port() != "443") || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || strings.ContainsAny(value, " \t\r\n\"\\%") || net.ParseIP(u.Hostname()) != nil {
			return errors.New("package source must be a credential-free HTTPS hostname URL")
		}
		if hosts != nil && !(egressproxy.Policy{AllowedHosts: hosts}).Allows(u.Hostname()) {
			return fmt.Errorf("package source host %q is not approved", u.Hostname())
		}
	}
	return nil
}

func (c Config) Hosts() []string {
	var hosts []string
	seen := make(map[string]bool)
	for _, value := range c.values() {
		if value != "" {
			u, _ := url.Parse(value)
			if u != nil {
				host := strings.ToLower(u.Hostname())
				if !seen[host] {
					hosts = append(hosts, host)
					seen[host] = true
				}
			}
		}
	}
	return hosts
}

func (c Config) JSON() string { data, _ := json.Marshal(c); return string(data) }

// WriteAPTConfig runs before accepting Jobs in a fresh root-owned directory.
// It preserves signature verification and the fixed Bookworm suites.
func (c Config) WriteAPTConfig(directory string) (string, error) {
	if err := c.Validate(nil); err != nil {
		return "", err
	}
	if err := os.Mkdir(directory, 0o755); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	c = c.Resolved()
	sources := fmt.Sprintf("Types: deb\nURIs: %s\nSuites: bookworm bookworm-updates\nComponents: main\nSigned-By: /usr/share/keyrings/debian-archive-keyring.gpg\n\nTypes: deb\nURIs: %s\nSuites: bookworm-security\nComponents: main\nSigned-By: /usr/share/keyrings/debian-archive-keyring.gpg\n", c.APT, c.APTSecurity)
	configPath := filepath.Join(directory, "apt.conf")
	config := fmt.Sprintf("Dir::Etc::sourcelist %q;\nDir::Etc::sourceparts \"-\";\n", filepath.Join(directory, "sources.sources"))
	for name, content := range map[string]string{"sources.sources": sources, "apt.conf": config} {
		if err := root.WriteFile(name, []byte(content), 0o644); err != nil {
			return "", err
		}
	}
	return configPath, nil
}

// Check probes bounded metadata through the caller's enforced HTTPS proxy.
// Opt-in probes must not prevent ordinary offline execution by default.
func (c Config) Check(ctx context.Context, client *http.Client) error {
	c = c.Resolved()
	for _, target := range []string{c.PyPI, strings.TrimRight(c.NPM, "/") + "/-/ping", strings.TrimRight(c.APT, "/") + "/dists/bookworm/Release", strings.TrimRight(c.APTSecurity, "/") + "/dists/bookworm-security/Release"} {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("package source probe failed: %w", err)
		}
		_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		closeErr := response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("package source probe HTTP %d", response.StatusCode)
		}
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
	}
	return nil
}
