package integrations

import (
	"crypto/hmac"
	"crypto/sha256"
	"strings"
)

// DeriveAuditHMACKey derives a purpose-separated key without retaining or
// persisting the configured encryption key in the integrations module.
func DeriveAuditHMACKey(masterKey string) []byte {
	if strings.TrimSpace(masterKey) == "" {
		return nil
	}
	mac := hmac.New(sha256.New, []byte(masterKey))
	_, _ = mac.Write([]byte("zgi/integration-audit/v1"))
	return mac.Sum(nil)
}
