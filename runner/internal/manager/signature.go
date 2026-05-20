package manager

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"plugin_runner/internal/plugin"
)

// SignatureVerifier verifies manifest signatures.
type SignatureVerifier interface {
	Verify(manifest plugin.Manifest) error
}

// RSASignatureVerifier verifies manifest signatures using RSA and SHA-256.
type RSASignatureVerifier struct {
	publicKey *rsa.PublicKey
}

func NewRSAVerifierFromPEM(pemData []byte) (*RSASignatureVerifier, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("invalid public key pem")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not RSA")
	}
	return &RSASignatureVerifier{publicKey: rsaPub}, nil
}

func (v *RSASignatureVerifier) Verify(manifest plugin.Manifest) error {
	if v == nil || v.publicKey == nil {
		return fmt.Errorf("verifier not initialized")
	}
	if manifest.Signature == "" {
		return fmt.Errorf("missing signature")
	}
	digest := sha256.Sum256([]byte(manifest.CanonicalString()))
	sig, err := decodeBase64(manifest.Signature)
	if err != nil {
		return err
	}
	return rsa.VerifyPKCS1v15(v.publicKey, crypto.SHA256, digest[:], sig)
}

func decodeBase64(s string) ([]byte, error) {
	data, err := decodeStdOrURLBase64(s)
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	return data, nil
}

func decodeStdOrURLBase64(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
