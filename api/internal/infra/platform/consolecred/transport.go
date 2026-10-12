package consolecred

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strings"

	appconfig "github.com/zgiai/zgi/api/config"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// TransportCredentials centralizes the Console gRPC transport choice. With
// TLS disabled it intentionally returns the legacy insecure credentials.
func TransportCredentials(addr string) (credentials.TransportCredentials, error) {
	cfg := appconfig.Current()
	if cfg == nil || !cfg.Console.GRPCTLS.Enabled {
		return insecure.NewCredentials(), nil
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	caFile := strings.TrimSpace(cfg.Console.GRPCTLS.CAFile)
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read console gRPC TLS CA file %q: %w", caFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificate found in console gRPC TLS CA file %q", caFile)
		}
		tlsConfig.RootCAs = pool
	}

	serverName := strings.TrimSpace(cfg.Console.GRPCTLS.ServerName)
	if serverName == "" {
		serverName = hostFromAddress(addr)
	}
	if serverName != "" {
		tlsConfig.ServerName = serverName
	}
	return credentials.NewTLS(tlsConfig), nil
}

func hostFromAddress(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(strings.TrimSpace(addr), "[]")
}
