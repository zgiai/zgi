package runtimeworkspace

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	credentialReplayEnvelopePrefix = "v1."
	idempotencyKeyMinLength        = 16
	idempotencyKeyMaxLength        = 200
)

var ErrCredentialReplayUnavailable = errors.New("runtime workspace credential replay protection is unavailable")

type CredentialReplayAAD struct {
	OrganizationID     uuid.UUID
	RuntimeWorkspaceID uuid.UUID
	OperationID        uuid.UUID
	CredentialID       uuid.UUID
	RequestFingerprint string
}

type CredentialReplayProtector interface {
	IdempotencyKeyHash(runtimeWorkspaceID, accountID uuid.UUID, rawKey string) (string, error)
	RequestFingerprint(runtimeWorkspaceID, accountID uuid.UUID, name string, expiresAt *time.Time, limits ...string) string
	Seal(secret string, aad CredentialReplayAAD) (string, error)
	Open(envelope string, aad CredentialReplayAAD) (string, error)
}

type credentialReplayProtector struct {
	aead    cipher.AEAD
	hmacKey [sha256.Size]byte
}

func NewCredentialReplayProtector(masterKey string) (CredentialReplayProtector, error) {
	if len(masterKey) != 32 {
		return nil, ErrCredentialReplayUnavailable
	}
	encryptionKey := deriveReplayKey(masterKey, "zgi/runtime-workspace/credential-replay/aes-256-gcm/v1")
	defer zeroReplayBytes(encryptionKey)
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("initialize runtime credential replay cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize runtime credential replay envelope: %w", err)
	}
	hmacKey := deriveReplayKey(masterKey, "zgi/runtime-workspace/credential-replay/hmac-sha256/v1")
	defer zeroReplayBytes(hmacKey)
	protector := &credentialReplayProtector{aead: aead}
	copy(protector.hmacKey[:], hmacKey)
	return protector, nil
}

func deriveReplayKey(masterKey, purpose string) []byte {
	mac := hmac.New(sha256.New, []byte(masterKey))
	_, _ = mac.Write([]byte(purpose))
	return mac.Sum(nil)
}

func (protector *credentialReplayProtector) IdempotencyKeyHash(runtimeWorkspaceID, accountID uuid.UUID, rawKey string) (string, error) {
	rawKey = strings.TrimSpace(rawKey)
	if protector == nil || runtimeWorkspaceID == uuid.Nil || accountID == uuid.Nil || len(rawKey) < idempotencyKeyMinLength || len(rawKey) > idempotencyKeyMaxLength {
		return "", ErrInvalidCredential
	}
	return protector.digest("idempotency-key", runtimeWorkspaceID.String(), accountID.String(), rawKey), nil
}

func (protector *credentialReplayProtector) RequestFingerprint(runtimeWorkspaceID, accountID uuid.UUID, name string, expiresAt *time.Time, limits ...string) string {
	expiration := ""
	if expiresAt != nil {
		expiration = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	parts := append([]string{"request-fingerprint", runtimeWorkspaceID.String(), accountID.String(), strings.TrimSpace(name), expiration}, limits...)
	return protector.digest(parts...)
}

func (protector *credentialReplayProtector) digest(parts ...string) string {
	mac := hmac.New(sha256.New, protector.hmacKey[:])
	for _, part := range parts {
		writeReplayPart(mac, part)
	}
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func writeReplayPart(writer hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = writer.Write(length[:])
	_, _ = writer.Write([]byte(value))
}

func (protector *credentialReplayProtector) Seal(secret string, aad CredentialReplayAAD) (string, error) {
	if protector == nil || protector.aead == nil || strings.TrimSpace(secret) == "" || !validCredentialReplayAAD(aad) {
		return "", ErrCredentialReplayUnavailable
	}
	plaintext := []byte(secret)
	defer zeroReplayBytes(plaintext)
	nonce := make([]byte, protector.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate runtime credential replay nonce: %w", err)
	}
	sealed := protector.aead.Seal(nil, nonce, plaintext, credentialReplayAADBytes(aad))
	payload := append(nonce, sealed...)
	return credentialReplayEnvelopePrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func (protector *credentialReplayProtector) Open(envelope string, aad CredentialReplayAAD) (string, error) {
	if protector == nil || protector.aead == nil || !validCredentialReplayAAD(aad) || !strings.HasPrefix(envelope, credentialReplayEnvelopePrefix) {
		return "", ErrCredentialReplayUnavailable
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(envelope, credentialReplayEnvelopePrefix))
	if err != nil || len(payload) <= protector.aead.NonceSize() {
		return "", ErrCredentialReplayUnavailable
	}
	defer zeroReplayBytes(payload)
	plaintext, err := protector.aead.Open(nil, payload[:protector.aead.NonceSize()], payload[protector.aead.NonceSize():], credentialReplayAADBytes(aad))
	if err != nil {
		return "", ErrCredentialReplayUnavailable
	}
	defer zeroReplayBytes(plaintext)
	return string(plaintext), nil
}

func validCredentialReplayAAD(aad CredentialReplayAAD) bool {
	return aad.OrganizationID != uuid.Nil && aad.RuntimeWorkspaceID != uuid.Nil && aad.OperationID != uuid.Nil && aad.CredentialID != uuid.Nil && strings.TrimSpace(aad.RequestFingerprint) != ""
}

func credentialReplayAADBytes(aad CredentialReplayAAD) []byte {
	return []byte(fmt.Sprintf("zgi/runtime-workspace/credential-replay/v1\norganization=%s\nruntime_workspace=%s\noperation=%s\ncredential=%s\nfingerprint=%s", aad.OrganizationID, aad.RuntimeWorkspaceID, aad.OperationID, aad.CredentialID, aad.RequestFingerprint))
}

func zeroReplayBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
