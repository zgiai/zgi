package consolecred

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
)

func TestTransportCredentialsDisabledUsesLegacyTransport(t *testing.T) {
	previous := appconfig.GlobalConfig
	defer func() { appconfig.GlobalConfig = previous }()
	appconfig.GlobalConfig = &appconfig.Config{}

	credentials, err := TransportCredentials("console.example:50052")
	if err != nil || credentials == nil {
		t.Fatalf("TransportCredentials() = (%v, %v)", credentials, err)
	}
}

func TestTransportCredentialsEnabledLoadsCustomCA(t *testing.T) {
	previous := appconfig.GlobalConfig
	defer func() { appconfig.GlobalConfig = previous }()
	caPath := writeTestCA(t)
	appconfig.GlobalConfig = &appconfig.Config{
		Console: appconfig.ConsoleConfig{
			GRPCTLS: appconfig.ConsoleGRPCTLSConfig{Enabled: true, CAFile: caPath},
		},
	}

	credentials, err := TransportCredentials("console.example:50052")
	if err != nil || credentials == nil {
		t.Fatalf("TransportCredentials() = (%v, %v)", credentials, err)
	}
}

func writeTestCA(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/ca.pem"
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
