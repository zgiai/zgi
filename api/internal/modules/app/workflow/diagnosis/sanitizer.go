package diagnosis

import (
	"strings"
)

var sensitiveKeys = []string{
	"api_key",
	"password",
	"secret",
	"token",
	"authorization",
	"access_token",
	"refresh_token",
}

// SanitizeInputSnapshot processes a map to mask sensitive information
func SanitizeInputSnapshot(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	sanitized := make(map[string]any)
	for k, v := range input {
		if isSensitiveKey(k) {
			sanitized[k] = "***"
		} else {
			// handle nested maps if present
			if nestedMap, ok := v.(map[string]any); ok {
				sanitized[k] = SanitizeInputSnapshot(nestedMap)
			} else {
				sanitized[k] = v
			}
		}
	}
	return sanitized
}

// isSensitiveKey checks if a string indicates a sensitive credential
func isSensitiveKey(k string) bool {
	lowerK := strings.ToLower(k)
	for _, maskKey := range sensitiveKeys {
		if strings.Contains(lowerK, maskKey) {
			return true
		}
	}
	return false
}
