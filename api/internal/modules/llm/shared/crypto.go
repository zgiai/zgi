package shared

import (
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/security"
)

// CryptoService defines the interface for encryption/decryption operations
type CryptoService interface {
	Encrypt(plaintext string) (string, error)
	Decrypt(ciphertext string) (string, error)
}

// DefaultCryptoService creates a CryptoService using AES-GCM encryption
// It reads the encryption key from the loaded .env configuration or uses a default.
func DefaultCryptoService() (CryptoService, error) {
	key := config.Current().LLM.EncryptionKey
	if key == "" {
		// Default 32-byte key for development (MUST be changed in production)
		key = "zgi-llm-default-encryption-key!!"
	}
	return security.NewAESGCMEncrypter(key)
}
