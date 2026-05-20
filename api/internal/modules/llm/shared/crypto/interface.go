package crypto

// CryptoService defines the interface for encryption/decryption operations
type CryptoService interface {
	// Encrypt encrypts plaintext and returns ciphertext
	Encrypt(plaintext string) (string, error)

	// Decrypt decrypts ciphertext and returns plaintext
	Decrypt(ciphertext string) (string, error)

	// Hash returns a SHA-256 hash of the input (for lookup purposes)
	Hash(input string) string
}
