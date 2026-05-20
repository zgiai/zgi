package crypto

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/pkg/security"
)

// aesGCMService implements CryptoService using AES-GCM encryption
type aesGCMService struct {
	encrypter *security.AESGCMEncrypter
}

// NewAESGCMService creates a new AES-GCM crypto service
func NewAESGCMService(key string) (CryptoService, error) {
	encrypter, err := security.NewAESGCMEncrypter(key)
	if err != nil {
		return nil, err
	}
	return &aesGCMService{encrypter: encrypter}, nil
}

// DefaultCryptoService creates a CryptoService using AES-GCM encryption
// It reads the encryption key from the loaded .env configuration or uses a default.
func DefaultCryptoService() (CryptoService, error) {
	key := config.Current().LLM.EncryptionKey
	if key == "" {
		// Default 32-byte key for development (MUST be changed in production)
		key = "zgi-llm-default-encryption-key!!"
	}
	return NewAESGCMService(key)
}

// Encrypt encrypts plaintext and returns ciphertext
func (s *aesGCMService) Encrypt(plaintext string) (string, error) {
	return s.encrypter.Encrypt(plaintext)
}

// Decrypt decrypts ciphertext and returns plaintext
func (s *aesGCMService) Decrypt(ciphertext string) (string, error) {
	return s.encrypter.Decrypt(ciphertext)
}

// Hash returns a SHA-256 hash of the input
func (s *aesGCMService) Hash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}
