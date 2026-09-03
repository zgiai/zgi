// Package credentials owns the Connector secret envelope. It does not accept
// any predecessor format; data conversion is an offline administrative task.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const MaxPayloadBytes = 256 * 1024

type Purpose string

const (
	ConnectionKey            Purpose = "connection-key"
	ConfirmationContinuation Purpose = "confirmation-continuation"
	OnboardingDevice         Purpose = "onboarding-device"
	MessagingState           Purpose = "messaging-state"
	MessagingContext         Purpose = "messaging-context"
)

var (
	ErrConfiguration   = errors.New("connector secret store configuration is invalid")
	ErrEnvelope        = errors.New("connector secret envelope is unavailable or invalid")
	keyIDPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	connectorIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

type Subject struct {
	Purpose        Purpose
	OrganizationID uuid.UUID
	ID             uuid.UUID
	ConnectorID    string
	Version        int
}

// Store is immutable after construction and safe for concurrent use. It retains
// derived AEAD keys, never the supplied master strings or caller's keyring map.
type Store struct {
	active string
	keys   map[string]cipher.AEAD
}

func NewStore(active string, masters map[string]string) (*Store, error) {
	if !keyIDPattern.MatchString(active) || len(masters) == 0 || len(masters) > 32 {
		return nil, ErrConfiguration
	}
	store := &Store{active: active, keys: make(map[string]cipher.AEAD, len(masters))}
	for id, master := range masters {
		if !keyIDPattern.MatchString(id) || len(master) != 32 {
			return nil, ErrConfiguration
		}
		material := []byte(master)
		key, err := hkdf.Key(sha256.New, material, []byte("zgi/connector-secrets/c1"), "aes-256-gcm", 32)
		clear(material)
		if err != nil {
			return nil, ErrConfiguration
		}
		block, err := aes.NewCipher(key)
		clear(key)
		if err != nil {
			return nil, ErrConfiguration
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, ErrConfiguration
		}
		store.keys[id] = aead
	}
	if store.keys[active] == nil {
		return nil, ErrConfiguration
	}
	return store, nil
}

func (subject Subject) aad(keyID string) ([]byte, error) {
	if subject.Purpose != ConnectionKey && subject.Purpose != ConfirmationContinuation && subject.Purpose != OnboardingDevice && subject.Purpose != MessagingState && subject.Purpose != MessagingContext || subject.OrganizationID == uuid.Nil || subject.ID == uuid.Nil || !connectorIDPattern.MatchString(subject.ConnectorID) || subject.Version < 1 {
		return nil, ErrEnvelope
	}
	return json.Marshal([]any{"zgi/connector-secrets", 1, string(subject.Purpose), keyID, subject.OrganizationID.String(), subject.ID.String(), subject.ConnectorID, subject.Version})
}

func (store *Store) Protect(subject Subject, plaintext []byte) (string, error) {
	if store == nil || store.keys[store.active] == nil || len(plaintext) == 0 || len(plaintext) > MaxPayloadBytes {
		return "", ErrEnvelope
	}
	aad, err := subject.aad(store.active)
	if err != nil {
		return "", err
	}
	aead := store.keys[store.active]
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", ErrEnvelope
	}
	payload := aead.Seal(nonce, nonce, plaintext, aad)
	// The key identifier is separately encoded so dots inside a key ID cannot
	// make the envelope ambiguous. IDs are authenticated as well as selected.
	return "c1." + base64.RawURLEncoding.EncodeToString([]byte(store.active)) + "." + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (store *Store) Open(subject Subject, envelope string) ([]byte, error) {
	if store == nil || len(envelope) > 2*MaxPayloadBytes {
		return nil, ErrEnvelope
	}
	parts := strings.Split(envelope, ".")
	if len(parts) != 3 || parts[0] != "c1" {
		return nil, ErrEnvelope
	}
	id, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrEnvelope
	}
	keyID := string(id)
	aead := store.keys[keyID]
	if aead == nil {
		return nil, ErrEnvelope
	}
	aad, err := subject.aad(keyID)
	if err != nil {
		return nil, err
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(payload) < aead.NonceSize()+aead.Overhead()+1 || len(payload) > aead.NonceSize()+aead.Overhead()+MaxPayloadBytes {
		return nil, ErrEnvelope
	}
	plaintext, err := aead.Open(nil, payload[:aead.NonceSize()], payload[aead.NonceSize():], aad)
	if err != nil {
		return nil, ErrEnvelope
	}
	return plaintext, nil
}
