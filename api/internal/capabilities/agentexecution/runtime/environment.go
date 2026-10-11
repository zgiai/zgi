package executionruntime

import (
	"net"
	"net/url"
	"os"
	"strings"
)

func agentExecutionEnvBool(name string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func validAgentRuntimeGatewayURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	if u.Scheme != "http" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		// HTTP is only allowed to loopback/private addresses. Public runtime
		// gateways must use TLS so the deployment-controlled API key is not
		// sent over a plaintext public connection.
		return ip.IsLoopback() || ip.IsPrivate()
	}
	// Local container/service DNS names are deployment-owned endpoints. A
	// public hostname still needs HTTPS; unqualified names and explicit
	// private suffixes cover Docker/Kubernetes service discovery without
	// turning arbitrary public HTTP URLs into an approved gateway.
	return !strings.Contains(host, ".") || strings.HasSuffix(strings.ToLower(host), ".local") || strings.HasSuffix(strings.ToLower(host), ".internal")
}
