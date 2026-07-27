package integrations

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	credentialEnvelopePrefix       = "v2."
	legacyCredentialEnvelopePrefix = "v1."
	defaultCredentialKeyID         = "default"
)

var credentialKeyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// CredentialAAD binds an encrypted credential envelope to exactly one
// organization connection and credential revision.
type CredentialAAD struct {
	OrganizationID    uuid.UUID
	ConnectionID      uuid.UUID
	IntegrationID     string
	CredentialVersion int
}

type CredentialCipher interface {
	EncryptCredentials(credentials map[string]string, aad CredentialAAD) (string, error)
	DecryptCredentials(envelope string, aad CredentialAAD) (map[string]string, error)
}

type credentialKeyring struct {
	activeKeyID string
	aeads       map[string]cipher.AEAD
}

// NewCredentialCipher derives a purpose-separated AES-256 key from
// API_KEY_ENCRYPTION_KEY. The master key is not retained after construction.
func NewCredentialCipher(masterKey string) (CredentialCipher, error) {
	return NewCredentialKeyring(defaultCredentialKeyID, map[string]string{defaultCredentialKeyID: masterKey})
}

// NewCredentialKeyring creates a multi-key credential cipher. New envelopes
// use activeKeyID while older v2 and legacy v1 envelopes remain readable as
// long as their key stays in the ring.
func NewCredentialKeyring(activeKeyID string, masterKeys map[string]string) (CredentialCipher, error) {
	activeKeyID = strings.TrimSpace(activeKeyID)
	if !credentialKeyIDPattern.MatchString(activeKeyID) {
		return nil, fmt.Errorf("integration credential active key id is invalid")
	}
	if len(masterKeys) == 0 || len(masterKeys) > 32 {
		return nil, fmt.Errorf("integration credential keyring is invalid")
	}
	aeads := make(map[string]cipher.AEAD, len(masterKeys))
	for rawID, masterKey := range masterKeys {
		keyID := strings.TrimSpace(rawID)
		if !credentialKeyIDPattern.MatchString(keyID) || len(masterKey) != 32 {
			return nil, fmt.Errorf("integration credential keyring entry is invalid")
		}
		if _, duplicated := aeads[keyID]; duplicated {
			return nil, fmt.Errorf("integration credential keyring contains a duplicate key id")
		}
		aead, err := newCredentialAEAD(masterKey)
		if err != nil {
			return nil, err
		}
		aeads[keyID] = aead
	}
	if _, exists := aeads[activeKeyID]; !exists {
		return nil, fmt.Errorf("integration credential active key is unavailable")
	}
	return &credentialKeyring{activeKeyID: activeKeyID, aeads: aeads}, nil
}

func newCredentialAEAD(masterKey string) (cipher.AEAD, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("integration credential master key must be exactly 32 bytes")
	}
	mac := hmac.New(sha256.New, []byte(masterKey))
	_, _ = mac.Write([]byte("zgi/integration-credentials/aes-256-gcm/v1"))
	derivedKey := mac.Sum(nil)
	defer zeroBytes(derivedKey)
	block, err := aes.NewCipher(derivedKey)
	if err != nil {
		return nil, fmt.Errorf("initialize integration credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize integration credential envelope: %w", err)
	}
	return aead, nil
}

func (c *credentialKeyring) EncryptCredentials(credentials map[string]string, aad CredentialAAD) (string, error) {
	if c == nil || c.aeads == nil {
		return "", fmt.Errorf("integration credential cipher is unavailable")
	}
	aead := c.aeads[c.activeKeyID]
	if aead == nil {
		return "", fmt.Errorf("integration credential active key is unavailable")
	}
	if err := validateCredentialAAD(aad); err != nil {
		return "", err
	}
	if err := validateCredentialPayload(credentials); err != nil {
		return "", err
	}
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return "", fmt.Errorf("encode integration credentials")
	}
	defer zeroBytes(plaintext)
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate integration credential nonce: %w", err)
	}
	sealed := aead.Seal(nil, nonce, plaintext, credentialAADBytesV2(aad, c.activeKeyID))
	envelopeBytes := make([]byte, 0, len(nonce)+len(sealed))
	envelopeBytes = append(envelopeBytes, nonce...)
	envelopeBytes = append(envelopeBytes, sealed...)
	return credentialEnvelopePrefix + c.activeKeyID + "." + base64.RawURLEncoding.EncodeToString(envelopeBytes), nil
}

func (c *credentialKeyring) DecryptCredentials(envelope string, aad CredentialAAD) (map[string]string, error) {
	if c == nil || c.aeads == nil {
		return nil, fmt.Errorf("integration credential cipher is unavailable")
	}
	if err := validateCredentialAAD(aad); err != nil {
		return nil, err
	}
	if strings.HasPrefix(envelope, credentialEnvelopePrefix) {
		encoded := strings.TrimPrefix(envelope, credentialEnvelopePrefix)
		separator := strings.LastIndexByte(encoded, '.')
		if separator <= 0 || separator == len(encoded)-1 {
			return nil, fmt.Errorf("invalid integration credential envelope")
		}
		keyID, payload := encoded[:separator], encoded[separator+1:]
		if !credentialKeyIDPattern.MatchString(keyID) {
			return nil, fmt.Errorf("invalid integration credential envelope")
		}
		aead := c.aeads[keyID]
		if aead == nil {
			return nil, fmt.Errorf("integration credential envelope key is unavailable")
		}
		return decryptCredentialPayload(aead, payload, credentialAADBytesV2(aad, keyID))
	}
	if !strings.HasPrefix(envelope, legacyCredentialEnvelopePrefix) {
		return nil, fmt.Errorf("unsupported integration credential envelope")
	}
	encoded := strings.TrimPrefix(envelope, legacyCredentialEnvelopePrefix)
	keyIDs := make([]string, 0, len(c.aeads))
	keyIDs = append(keyIDs, c.activeKeyID)
	for keyID := range c.aeads {
		if keyID != c.activeKeyID {
			keyIDs = append(keyIDs, keyID)
		}
	}
	sort.Strings(keyIDs[1:])
	for _, keyID := range keyIDs {
		credentials, err := decryptCredentialPayload(c.aeads[keyID], encoded, credentialAADBytes(aad))
		if err == nil {
			return credentials, nil
		}
	}
	return nil, fmt.Errorf("decrypt integration credentials")
}

func decryptCredentialPayload(aead cipher.AEAD, encoded string, additionalData []byte) (map[string]string, error) {
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode integration credential envelope")
	}
	defer zeroBytes(payload)
	if aead == nil || len(payload) <= aead.NonceSize() {
		return nil, fmt.Errorf("invalid integration credential envelope")
	}
	nonce := payload[:aead.NonceSize()]
	sealed := payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, sealed, additionalData)
	if err != nil {
		return nil, fmt.Errorf("decrypt integration credentials")
	}
	defer zeroBytes(plaintext)
	var credentials map[string]string
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, fmt.Errorf("decode integration credentials")
	}
	if err := validateCredentialPayload(credentials); err != nil {
		destroyCredentialMap(credentials)
		return nil, err
	}
	return credentials, nil
}

func validateCredentialAAD(aad CredentialAAD) error {
	if aad.OrganizationID == uuid.Nil || aad.ConnectionID == uuid.Nil || strings.TrimSpace(aad.IntegrationID) == "" || aad.CredentialVersion < 1 {
		return fmt.Errorf("invalid integration credential binding")
	}
	return nil
}

func credentialAADBytes(aad CredentialAAD) []byte {
	return []byte(fmt.Sprintf(
		"zgi/integration-credentials/v1\norganization=%s\nconnection=%s\nintegration=%s\nversion=%d",
		aad.OrganizationID.String(),
		aad.ConnectionID.String(),
		strings.ToLower(strings.TrimSpace(aad.IntegrationID)),
		aad.CredentialVersion,
	))
}

func credentialAADBytesV2(aad CredentialAAD, keyID string) []byte {
	return []byte(fmt.Sprintf(
		"zgi/integration-credentials/v2\nkey=%s\norganization=%s\nconnection=%s\nintegration=%s\nversion=%d",
		strings.TrimSpace(keyID),
		aad.OrganizationID.String(),
		aad.ConnectionID.String(),
		strings.ToLower(strings.TrimSpace(aad.IntegrationID)),
		aad.CredentialVersion,
	))
}

func validateCredentialPayload(credentials map[string]string) error {
	if len(credentials) == 0 || len(credentials) > 16 {
		return fmt.Errorf("integration credentials are required")
	}
	for key, value := range credentials {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > 64 || strings.TrimSpace(value) == "" || len(value) > 16*1024 {
			return fmt.Errorf("integration credentials are invalid")
		}
	}
	return nil
}

func destroyCredentialMap(credentials map[string]string) {
	for key := range credentials {
		credentials[key] = ""
		delete(credentials, key)
	}
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
