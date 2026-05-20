package security

import (
	"strings"
	"testing"
)

func TestNewAESGCMEncrypter(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "valid 32-byte key",
			key:     "12345678901234567890123456789012",
			wantErr: false,
		},
		{
			name:    "empty key",
			key:     "",
			wantErr: true,
		},
		{
			name:    "too short key",
			key:     "short",
			wantErr: true,
		},
		{
			name:    "too long key",
			key:     "123456789012345678901234567890123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAESGCMEncrypter(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAESGCMEncrypter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAESGCMEncrypter_EncryptDecrypt(t *testing.T) {
	key := "12345678901234567890123456789012" // 32 bytes
	encrypter, err := NewAESGCMEncrypter(key)
	if err != nil {
		t.Fatalf("Failed to create encrypter: %v", err)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{
			name:      "simple text",
			plaintext: "hello world",
		},
		{
			name:      "API key format",
			plaintext: "sk-1234567890abcdef",
		},
		{
			name:      "long text",
			plaintext: strings.Repeat("a", 1000),
		},
		{
			name:      "empty string",
			plaintext: "",
		},
		{
			name:      "special characters",
			plaintext: "!@#$%^&*()_+-=[]{}|;:',.<>?/~`",
		},
		{
			name:      "unicode characters",
			plaintext: "你好世界 🌍",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encrypt
			encrypted, err := encrypter.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			// Verify encrypted is different from plaintext (unless empty)
			if tt.plaintext != "" && encrypted == tt.plaintext {
				t.Error("Encrypted text should be different from plaintext")
			}

			// Decrypt
			decrypted, err := encrypter.Decrypt(encrypted)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			// Verify decrypted matches original
			if decrypted != tt.plaintext {
				t.Errorf("Decrypted text = %v, want %v", decrypted, tt.plaintext)
			}
		})
	}
}

func TestAESGCMEncrypter_EncryptProducesDifferentCiphertext(t *testing.T) {
	key := "12345678901234567890123456789012"
	encrypter, err := NewAESGCMEncrypter(key)
	if err != nil {
		t.Fatalf("Failed to create encrypter: %v", err)
	}

	plaintext := "test message"

	// Encrypt the same plaintext twice
	encrypted1, err := encrypter.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("First encryption failed: %v", err)
	}

	encrypted2, err := encrypter.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Second encryption failed: %v", err)
	}

	// Ciphertexts should be different due to random nonce
	if encrypted1 == encrypted2 {
		t.Error("Two encryptions of the same plaintext should produce different ciphertexts")
	}

	// But both should decrypt to the same plaintext
	decrypted1, _ := encrypter.Decrypt(encrypted1)
	decrypted2, _ := encrypter.Decrypt(encrypted2)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("Both ciphertexts should decrypt to the original plaintext")
	}
}

func TestAESGCMEncrypter_DecryptWithWrongKey(t *testing.T) {
	key1 := "12345678901234567890123456789012"
	key2 := "abcdefghijklmnopqrstuvwxyz123456"

	encrypter1, _ := NewAESGCMEncrypter(key1)
	encrypter2, _ := NewAESGCMEncrypter(key2)

	plaintext := "secret message"

	// Encrypt with key1
	encrypted, err := encrypter1.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	// Try to decrypt with key2 (should fail)
	_, err = encrypter2.Decrypt(encrypted)
	if err == nil {
		t.Error("Decryption with wrong key should fail")
	}
}

func TestAESGCMEncrypter_DecryptInvalidData(t *testing.T) {
	key := "12345678901234567890123456789012"
	encrypter, _ := NewAESGCMEncrypter(key)

	tests := []struct {
		name       string
		ciphertext string
	}{
		{
			name:       "invalid base64",
			ciphertext: "not-valid-base64!@#$",
		},
		{
			name:       "too short data",
			ciphertext: "YWJj", // "abc" in base64, too short for nonce
		},
		{
			name:       "corrupted data",
			ciphertext: "YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXo=", // valid base64 but invalid ciphertext
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encrypter.Decrypt(tt.ciphertext)
			if err == nil {
				t.Error("Decryption of invalid data should fail")
			}
		})
	}
}

func TestAESGCMEncrypter_EmptyString(t *testing.T) {
	key := "12345678901234567890123456789012"
	encrypter, _ := NewAESGCMEncrypter(key)

	// Encrypt empty string
	encrypted, err := encrypter.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	if encrypted != "" {
		t.Error("Encrypting empty string should return empty string")
	}

	// Decrypt empty string
	decrypted, err := encrypter.Decrypt("")
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}

	if decrypted != "" {
		t.Error("Decrypting empty string should return empty string")
	}
}

func TestAESGCMEncrypter_EncryptIfNotEmpty(t *testing.T) {
	key := "12345678901234567890123456789012"
	encrypter, _ := NewAESGCMEncrypter(key)

	// Test with non-empty string
	plaintext := "test"
	encrypted, err := encrypter.EncryptIfNotEmpty(plaintext)
	if err != nil {
		t.Fatalf("EncryptIfNotEmpty() error = %v", err)
	}
	if encrypted == "" {
		t.Error("EncryptIfNotEmpty() should return non-empty string for non-empty input")
	}

	// Test with empty string
	encrypted, err = encrypter.EncryptIfNotEmpty("")
	if err != nil {
		t.Fatalf("EncryptIfNotEmpty() error = %v", err)
	}
	if encrypted != "" {
		t.Error("EncryptIfNotEmpty() should return empty string for empty input")
	}
}

func TestAESGCMEncrypter_DecryptIfNotEmpty(t *testing.T) {
	key := "12345678901234567890123456789012"
	encrypter, _ := NewAESGCMEncrypter(key)

	plaintext := "test"
	encrypted, _ := encrypter.Encrypt(plaintext)

	// Test with non-empty string
	decrypted, err := encrypter.DecryptIfNotEmpty(encrypted)
	if err != nil {
		t.Fatalf("DecryptIfNotEmpty() error = %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("DecryptIfNotEmpty() = %v, want %v", decrypted, plaintext)
	}

	// Test with empty string
	decrypted, err = encrypter.DecryptIfNotEmpty("")
	if err != nil {
		t.Fatalf("DecryptIfNotEmpty() error = %v", err)
	}
	if decrypted != "" {
		t.Error("DecryptIfNotEmpty() should return empty string for empty input")
	}
}

// Benchmark tests
func BenchmarkAESGCMEncrypter_Encrypt(b *testing.B) {
	key := "12345678901234567890123456789012"
	encrypter, _ := NewAESGCMEncrypter(key)
	plaintext := "sk-1234567890abcdef1234567890abcdef"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encrypter.Encrypt(plaintext)
	}
}

func BenchmarkAESGCMEncrypter_Decrypt(b *testing.B) {
	key := "12345678901234567890123456789012"
	encrypter, _ := NewAESGCMEncrypter(key)
	plaintext := "sk-1234567890abcdef1234567890abcdef"
	encrypted, _ := encrypter.Encrypt(plaintext)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encrypter.Decrypt(encrypted)
	}
}
