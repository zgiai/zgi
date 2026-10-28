package serverops

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"golang.org/x/crypto/ssh"
)

// The connection setup can generate the key pair in the browser (ADR 0030):
// WebCrypto exports an unencrypted PKCS#8 Ed25519 key, wrapped as a PEM
// "PRIVATE KEY" block, and the person installs the public half.
func TestPlatformGeneratedKeyPairIsAccepted(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if err != nil {
		t.Fatalf("ParsePrivateKey() error = %v", err)
	}
	want, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	if string(signer.PublicKey().Marshal()) != string(want.Marshal()) {
		t.Fatal("the parsed key is not the generated one")
	}
}
