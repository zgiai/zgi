package shortlink

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

const (
	tokenLength            = 8
	tokenAlphabet          = "23456789abcdefghkmnpqrstuvwxyz"
	tokenCreateMaxAttempts = 5
)

var newToken = generateToken

func generateToken() (string, error) {
	var builder strings.Builder
	builder.Grow(tokenLength)
	max := big.NewInt(int64(len(tokenAlphabet)))
	for builder.Len() < tokenLength {
		index, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate short link token: %w", err)
		}
		builder.WriteByte(tokenAlphabet[index.Int64()])
	}
	return builder.String(), nil
}

func normalizeToken(token string) string {
	return strings.ToLower(strings.TrimSpace(token))
}

func isValidToken(token string) bool {
	token = normalizeToken(token)
	if len(token) != tokenLength {
		return false
	}
	for _, char := range token {
		if !strings.ContainsRune(tokenAlphabet, char) {
			return false
		}
	}
	return true
}
