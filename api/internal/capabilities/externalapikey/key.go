// Package externalapikey contains transport-neutral helpers for ZGI external API keys.
package externalapikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	defaultPrefix = "zgi_"
	keyBytes      = 32
	prefixLength  = 12
)

var (
	ErrMissingKey      = errors.New("external api key is required")
	ErrInvalidAuth     = errors.New("external api authorization is invalid")
	ErrConflictingKeys = errors.New("external api credentials conflict")
)

// Generate creates a cryptographically random API key, its display prefix, and its hash.
func Generate() (key, displayPrefix, hash string, err error) {
	random := make([]byte, keyBytes)
	if _, err = rand.Read(random); err != nil {
		return "", "", "", err
	}
	key = defaultPrefix + hex.EncodeToString(random)
	displayPrefix = key[:prefixLength]
	return key, displayPrefix, Hash(key), nil
}

// Hash returns the lowercase SHA-256 hex digest stored for one API key.
func Hash(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

// Extract reads a Bearer credential or the compatibility X-API-Key value.
func Extract(authorization, xAPIKey string) (string, error) {
	authorization = strings.TrimSpace(authorization)
	xAPIKey = strings.TrimSpace(xAPIKey)
	if authorization == "" {
		if xAPIKey == "" {
			return "", ErrMissingKey
		}
		return xAPIKey, nil
	}
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || strings.TrimSpace(parts[1]) == "" {
		return "", ErrInvalidAuth
	}
	key := strings.TrimSpace(parts[1])
	if xAPIKey != "" && xAPIKey != key {
		return "", ErrConflictingKeys
	}
	return key, nil
}
