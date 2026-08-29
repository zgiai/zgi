package integrations

import "github.com/zgiai/zgi/api/internal/modules/integrations/credentials"

// CredentialAAD is kept at the integration boundary for compatibility while
// the encryption implementation lives in the dependency-free credentials
// package.
type CredentialAAD = credentials.CredentialAAD

type CredentialCipher = credentials.CredentialCipher

func NewCredentialCipher(masterKey string) (CredentialCipher, error) {
	return credentials.NewCredentialCipher(masterKey)
}

func NewCredentialKeyring(activeKeyID string, masterKeys map[string]string) (CredentialCipher, error) {
	return credentials.NewCredentialKeyring(activeKeyID, masterKeys)
}

func destroyCredentialMap(values map[string]string) {
	credentials.DestroyMap(values)
}

func zeroBytes(value []byte) {
	credentials.ZeroBytes(value)
}
