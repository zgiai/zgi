package util

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// GenerateKeyPair generates a new RSA key pair and returns the public key in PEM format
func GenerateKeyPair(tenantID string) (string, error) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("failed to generate private key: %w", err)
	}

	// Convert private key to PEM format
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	privateKeyBytes := pem.EncodeToMemory(privateKeyPEM)

	// Convert public key to PEM format
	publicKeyPEM := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: x509.MarshalPKCS1PublicKey(&privateKey.PublicKey),
	}
	publicKeyBytes := pem.EncodeToMemory(publicKeyPEM)

	// Save private key to file
	privKeyDir := filepath.Join("privkeys", tenantID)
	if err := os.MkdirAll(privKeyDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create private key directory: %w", err)
	}

	privKeyPath := filepath.Join(privKeyDir, "private.pem")
	if err := os.WriteFile(privKeyPath, privateKeyBytes, 0600); err != nil {
		return "", fmt.Errorf("failed to save private key: %w", err)
	}

	return string(publicKeyBytes), nil
}
