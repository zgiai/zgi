package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	appConfig "github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/pkg/security"
)

var (
	apiKeyEncrypter     *security.AESGCMEncrypter
	apiKeyEncrypterOnce sync.Once
	apiKeyEncrypterErr  error
)

// GetAPIKeyEncrypter returns a singleton instance of the API key encrypter
func GetAPIKeyEncrypter() (*security.AESGCMEncrypter, error) {
	apiKeyEncrypterOnce.Do(func() {
		if appConfig.GlobalConfig == nil {
			apiKeyEncrypterErr = fmt.Errorf("global config not initialized")
			return
		}

		encryptionKey := appConfig.GlobalConfig.Encryption.APIKeyEncryptionKey
		if encryptionKey == "" {
			apiKeyEncrypterErr = fmt.Errorf("API_KEY_ENCRYPTION_KEY not configured")
			return
		}

		apiKeyEncrypter, apiKeyEncrypterErr = security.NewAESGCMEncrypter(encryptionKey)
	})

	if apiKeyEncrypterErr != nil {
		return nil, apiKeyEncrypterErr
	}

	return apiKeyEncrypter, nil
}

// EncryptAPIKey encrypts an API key using AES-256-GCM
func EncryptAPIKey(apiKey string) (string, error) {
	if apiKey == "" {
		return "", nil
	}

	encrypter, err := GetAPIKeyEncrypter()
	if err != nil {
		return "", fmt.Errorf("failed to get encrypter: %w", err)
	}

	encrypted, err := encrypter.Encrypt(apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt API key: %w", err)
	}

	return encrypted, nil
}

// DecryptAPIKey decrypts an API key using AES-256-GCM
func DecryptAPIKey(encryptedKey string) (string, error) {
	if encryptedKey == "" {
		return "", nil
	}

	encrypter, err := GetAPIKeyEncrypter()
	if err != nil {
		return "", fmt.Errorf("failed to get encrypter: %w", err)
	}

	decrypted, err := encrypter.Decrypt(encryptedKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt API key: %w", err)
	}

	return decrypted, nil
}

// ObfuscateAPIKey returns a partially masked version of the API key for display
// Example: "sk-1234567890abcdef" -> "sk-12**************ef"
func ObfuscateAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}

	if len(apiKey) <= 8 {
		return "****"
	}

	// Show first 5 and last 2 characters
	prefix := apiKey[:5]
	suffix := apiKey[len(apiKey)-2:]
	stars := "**************"

	return prefix + stars + suffix
}

// HashAPIKey generates a SHA-256 hash of the API key for database indexing
func HashAPIKey(apiKey string) string {
	if apiKey == "" {
		return ""
	}

	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}
