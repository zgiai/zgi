package audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"strings"
)

// DeriveHMACKey derives a purpose-separated audit key without retaining or
// persisting the configured encryption key.
func DeriveHMACKey(masterKey string) []byte {
	if strings.TrimSpace(masterKey) == "" {
		return nil
	}
	mac := hmac.New(sha256.New, []byte(masterKey))
	_, _ = mac.Write([]byte("zgi/integration-audit/v1"))
	return mac.Sum(nil)
}
