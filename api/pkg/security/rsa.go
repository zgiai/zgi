package security

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/pkg/redis"
	"github.com/zgiai/zgi/api/pkg/storage"
)

var prefixHybrid = []byte("HYBRID:")

// GenerateKeyPair generates a new RSA key pair and returns the public key in PEM format
// This function already exists in user_rsa.go, so we don't need to reimplement it here

// encrypt encrypts text using RSA public key with hybrid encryption (RSA+AES)
func Encrypt(text string, publicKey interface{}) ([]byte, error) {
	var pubKey *rsa.PublicKey

	// Handle different types of publicKey input
	switch pk := publicKey.(type) {
	case string:
		// Parse PEM encoded public key
		block, _ := pem.Decode([]byte(pk))
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM block containing public key")
		}

		var err error
		pubKey, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			// Try parsing as PKIX public key
			parsedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse public key: %w", err)
			}

			var ok bool
			pubKey, ok = parsedKey.(*rsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("not an RSA public key")
			}
		}
	case *rsa.PublicKey:
		pubKey = pk
	case []byte:
		// Parse PEM encoded public key from bytes
		block, _ := pem.Decode(pk)
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM block containing public key")
		}

		var err error
		pubKey, err = x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			// Try parsing as PKIX public key
			parsedKey, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse public key: %w", err)
			}

			var ok bool
			pubKey, ok = parsedKey.(*rsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("not an RSA public key")
			}
		}
	default:
		return nil, fmt.Errorf("unsupported public key type")
	}

	// Generate a random AES key
	aesKey := make([]byte, 16) // AES-128
	if _, err := rand.Read(aesKey); err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the text with AES-GCM
	ciphertext := gcm.Seal(nil, nonce, []byte(text), nil)

	// Encrypt the AES key with RSA
	encAesKey, err := rsa.EncryptPKCS1v15(rand.Reader, pubKey, aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt AES key: %w", err)
	}

	// Combine encrypted AES key, nonce and ciphertext
	encryptedData := append(encAesKey, nonce...)
	encryptedData = append(encryptedData, ciphertext...)

	// Add prefix
	result := append(prefixHybrid, encryptedData...)

	return result, nil
}

// getDecryptDecoding gets the decrypt decoding information from storage with Redis caching
func GetDecryptDecoding(tenantID string) (*rsa.PrivateKey, error) {
	filepath := fmt.Sprintf("privkeys/%s/private.pem", tenantID)

	// Create cache key
	hasher := sha256.New()
	hasher.Write([]byte(filepath))
	cacheKey := fmt.Sprintf("tenant_privkey:%x", hasher.Sum(nil))

	// Try to get private key from Redis cache
	ctx := context.Background()
	privateKeyPEM, err := redis.RedisClient.Get(ctx, cacheKey).Result()
	if err != nil {
		// Private key not in cache, load from storage
		storageClient := storage.GetStorage()
		privateKeyBytes, err := storageClient.Load(filepath)
		if err != nil {
			return nil, &PrivkeyNotFoundError{fmt.Sprintf("Private key not found, tenant_id: %s", tenantID)}
		}

		privateKeyPEM = string(privateKeyBytes)

		// Cache the private key in Redis for 120 seconds
		err = redis.RedisClient.SetEx(ctx, cacheKey, privateKeyPEM, 120*time.Second).Err()
		if err != nil {
			// Log error but don't fail the operation
			// In a real implementation, you might want to use a logger here
		}
	}

	// Parse the private key
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block containing private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, nil
}

// decryptTokenWithDecoding decrypts an encrypted text with provided RSA private key
func DecryptTokenWithDecoding(encryptedText []byte, privateKey *rsa.PrivateKey) (string, error) {
	// Check if encrypted text starts with hybrid prefix
	if bytes.HasPrefix(encryptedText, prefixHybrid) {
		encryptedText = encryptedText[len(prefixHybrid):]
	}

	// For hybrid encryption, we expect:
	// 1. Encrypted AES key (rsaKey.Size() bytes)
	// 2. Nonce (12 bytes for GCM)
	// 3. Ciphertext (remaining bytes)

	if len(encryptedText) <= privateKey.Size() {
		return "", fmt.Errorf("encrypted text too short")
	}

	// Extract encrypted AES key
	encAesKey := encryptedText[:privateKey.Size()]

	// Decrypt the AES key with RSA
	aesKey, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, encAesKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt AES key: %w", err)
	}

	// Create AES cipher
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Check minimum length
	nonceSize := gcm.NonceSize()
	if len(encryptedText) <= privateKey.Size()+nonceSize {
		return "", fmt.Errorf("encrypted text too short")
	}

	// Extract nonce and ciphertext
	nonce := encryptedText[privateKey.Size() : privateKey.Size()+nonceSize]
	ciphertext := encryptedText[privateKey.Size()+nonceSize:]

	// Decrypt the ciphertext
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt ciphertext: %w", err)
	}

	return string(plaintext), nil
}

// Decrypt decrypts an encrypted text using tenant ID to retrieve the private key
func Decrypt(encryptedText []byte, tenantID string) (string, error) {
	privateKey, err := GetDecryptDecoding(tenantID)
	if err != nil {
		return "", err
	}

	return DecryptTokenWithDecoding(encryptedText, privateKey)
}

// PrivkeyNotFoundError represents an error when private key is not found
type PrivkeyNotFoundError struct {
	message string
}

func (e *PrivkeyNotFoundError) Error() string {
	return e.message
}
