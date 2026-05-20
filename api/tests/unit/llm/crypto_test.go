package llm_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared/crypto"
)

func TestAESGCMService_EncryptDecrypt(t *testing.T) {
	// Use a valid 32-byte key for AES-256
	key := "12345678901234567890123456789012"

	svc, err := crypto.NewAESGCMService(key)
	require.NoError(t, err)
	require.NotNil(t, svc)

	t.Run("encrypt and decrypt plaintext", func(t *testing.T) {
		plaintext := "sk-test-api-key-12345"

		encrypted, err := svc.Encrypt(plaintext)
		assert.NoError(t, err)
		assert.NotEmpty(t, encrypted)
		assert.NotEqual(t, plaintext, encrypted)

		decrypted, err := svc.Decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})

	t.Run("encrypt empty string returns empty", func(t *testing.T) {
		encrypted, err := svc.Encrypt("")
		assert.NoError(t, err)
		assert.Empty(t, encrypted)
	})

	t.Run("decrypt empty string returns empty", func(t *testing.T) {
		decrypted, err := svc.Decrypt("")
		assert.NoError(t, err)
		assert.Empty(t, decrypted)
	})

	t.Run("same plaintext produces different ciphertext", func(t *testing.T) {
		plaintext := "same-api-key"

		encrypted1, err := svc.Encrypt(plaintext)
		assert.NoError(t, err)

		encrypted2, err := svc.Encrypt(plaintext)
		assert.NoError(t, err)

		// Due to random nonce, ciphertexts should be different
		assert.NotEqual(t, encrypted1, encrypted2)

		// But both should decrypt to the same plaintext
		decrypted1, _ := svc.Decrypt(encrypted1)
		decrypted2, _ := svc.Decrypt(encrypted2)
		assert.Equal(t, decrypted1, decrypted2)
	})

	t.Run("decrypt invalid ciphertext returns error", func(t *testing.T) {
		_, err := svc.Decrypt("invalid-base64-ciphertext!!!")
		assert.Error(t, err)
	})
}

func TestAESGCMService_Hash(t *testing.T) {
	key := "12345678901234567890123456789012"
	svc, err := crypto.NewAESGCMService(key)
	require.NoError(t, err)

	t.Run("hash produces consistent output", func(t *testing.T) {
		input := "test-input"
		hash1 := svc.Hash(input)
		hash2 := svc.Hash(input)

		assert.Equal(t, hash1, hash2)
		assert.Len(t, hash1, 64) // SHA-256 produces 64 hex characters
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		hash1 := svc.Hash("input1")
		hash2 := svc.Hash("input2")

		assert.NotEqual(t, hash1, hash2)
	})
}

func TestNewAESGCMService_InvalidKey(t *testing.T) {
	t.Run("empty key returns error", func(t *testing.T) {
		_, err := crypto.NewAESGCMService("")
		assert.Error(t, err)
	})

	t.Run("short key returns error", func(t *testing.T) {
		_, err := crypto.NewAESGCMService("short-key")
		assert.Error(t, err)
	})

	t.Run("valid 32-byte key succeeds", func(t *testing.T) {
		svc, err := crypto.NewAESGCMService("12345678901234567890123456789012")
		assert.NoError(t, err)
		assert.NotNil(t, svc)
	})
}

func TestDefaultCryptoService(t *testing.T) {
	t.Run("creates service with default key", func(t *testing.T) {
		svc, err := crypto.DefaultCryptoService()
		assert.NoError(t, err)
		assert.NotNil(t, svc)

		// Test basic encrypt/decrypt
		plaintext := "test-api-key"
		encrypted, err := svc.Encrypt(plaintext)
		assert.NoError(t, err)

		decrypted, err := svc.Decrypt(encrypted)
		assert.NoError(t, err)
		assert.Equal(t, plaintext, decrypted)
	})
}
