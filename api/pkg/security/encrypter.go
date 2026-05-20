package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Encrypter handles RSA encryption/decryption for credentials
type Encrypter struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	cache      *redis.Client
}

// NewEncrypter creates a new encrypter with RSA key pair
func NewEncrypter(cache *redis.Client) (*Encrypter, error) {
	// Generate RSA key pair (2048 bits)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	return &Encrypter{
		privateKey: privateKey,
		publicKey:  &privateKey.PublicKey,
		cache:      cache,
	}, nil
}

// NewEncrypterWithKeys creates encrypter with existing keys
func NewEncrypterWithKeys(privateKeyPEM, publicKeyPEM string, cache *redis.Client) (*Encrypter, error) {
	// Parse private key
	privateBlock, _ := pem.Decode([]byte(privateKeyPEM))
	if privateBlock == nil {
		return nil, fmt.Errorf("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(privateBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Parse public key
	publicBlock, _ := pem.Decode([]byte(publicKeyPEM))
	if publicBlock == nil {
		return nil, fmt.Errorf("failed to decode public key PEM")
	}

	publicKey, err := x509.ParsePKCS1PublicKey(publicBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return &Encrypter{
		privateKey: privateKey,
		publicKey:  publicKey,
		cache:      cache,
	}, nil
}

// Encrypt encrypts data using RSA public key
func (e *Encrypter) Encrypt(data interface{}) (string, error) {
	// Convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("failed to marshal data: %w", err)
	}

	// Encrypt using RSA-OAEP
	encryptedData, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, e.publicKey, jsonData, nil)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt data: %w", err)
	}

	// Encode to base64
	return base64.StdEncoding.EncodeToString(encryptedData), nil
}

// Decrypt decrypts data using RSA private key
func (e *Encrypter) Decrypt(encryptedData string) (map[string]interface{}, error) {
	// Decode from base64
	data, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	// Decrypt using RSA-OAEP
	decryptedData, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, e.privateKey, data, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	// Parse JSON
	var result map[string]interface{}
	if err := json.Unmarshal(decryptedData, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal decrypted data: %w", err)
	}

	return result, nil
}

// GetFromCache retrieves decrypted credentials from cache
func (e *Encrypter) GetFromCache(ctx context.Context, key string) (map[string]interface{}, error) {
	if e.cache == nil {
		return nil, fmt.Errorf("cache not available")
	}

	cachedData, err := e.cache.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(cachedData), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached data: %w", err)
	}

	return result, nil
}

// SetToCache stores decrypted credentials in cache with TTL
func (e *Encrypter) SetToCache(ctx context.Context, key string, data map[string]interface{}, ttl time.Duration) error {
	if e.cache == nil {
		return fmt.Errorf("cache not available")
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	return e.cache.Set(ctx, key, jsonData, ttl).Err()
}

// GetPublicKeyPEM returns the public key in PEM format
func (e *Encrypter) GetPublicKeyPEM() (string, error) {
	// Fix: x509.MarshalPKCS1PublicKey returns only 1 value, not 2
	publicKeyBytes := x509.MarshalPKCS1PublicKey(e.publicKey)

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM), nil
}

// GetPrivateKeyPEM returns the private key in PEM format
func (e *Encrypter) GetPrivateKeyPEM() (string, error) {
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(e.privateKey)

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return string(privateKeyPEM), nil
}
