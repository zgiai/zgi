package connections

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"sort"
	"strings"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const (
	idempotencyKeyMinLength = 16
	idempotencyKeyMaxLength = 200
	credentialValueMaxBytes = 16 * 1024
)

// CommandHasher produces only keyed, non-reversible digests. The derived key
// is purpose-separated from the credential-encryption key even when both are
// configured from the same deployment key material.
type CommandHasher struct {
	key [sha256.Size]byte
}

type CommandFingerprintInput struct {
	ConnectorID           string
	ReconnectConnectionID uuid.UUID
	Target                *connectorcore.TargetRef
	Values                map[string]string `json:"-"`
	MailUsage             string
	DriverID              string
	AuthMethodID          string
}

func NewCommandHasher(masterKey string) (*CommandHasher, error) {
	if len(masterKey) != 32 {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	deriver := hmac.New(sha256.New, []byte(masterKey))
	_, _ = deriver.Write([]byte("zgi/connectors/idempotency-hmac/v1"))
	derived := deriver.Sum(nil)
	hasher := &CommandHasher{}
	copy(hasher.key[:], derived)
	clear(derived)
	return hasher, nil
}

func (hasher *CommandHasher) IdempotencyKeyHash(organizationID, ownerAccountID uuid.UUID, rawKey string) (string, error) {
	rawKey = strings.TrimSpace(rawKey)
	if hasher == nil || organizationID == uuid.Nil || ownerAccountID == uuid.Nil || len(rawKey) < idempotencyKeyMinLength || len(rawKey) > idempotencyKeyMaxLength {
		return "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	mac := hmac.New(sha256.New, hasher.key[:])
	writePart(mac, "idempotency-key")
	writePart(mac, organizationID.String())
	writePart(mac, ownerAccountID.String())
	writePart(mac, rawKey)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func (hasher *CommandHasher) RequestFingerprint(input CommandFingerprintInput) (string, error) {
	if hasher == nil {
		return "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	connectorID, err := connectorcore.NormalizeIdentifier(input.ConnectorID)
	if err != nil || len(input.Values) == 0 || len(input.Values) > 16 {
		return "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	if input.Target != nil {
		if err := input.Target.Validate(); err != nil {
			return "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
		}
	}
	keys := make([]string, 0, len(input.Values))
	for rawName, value := range input.Values {
		name, normalizeErr := connectorcore.NormalizeIdentifier(rawName)
		if normalizeErr != nil || name.String() != rawName || strings.TrimSpace(value) == "" || len(value) > credentialValueMaxBytes {
			return "", connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
		}
		keys = append(keys, rawName)
	}
	sort.Strings(keys)

	mac := hmac.New(sha256.New, hasher.key[:])
	writePart(mac, "request-fingerprint")
	if input.ConnectorID == "gmail" {
		writePart(mac, "mail-v5")
		writePart(mac, input.MailUsage)
		writePart(mac, input.DriverID)
		writePart(mac, input.AuthMethodID)
	}
	if input.ConnectorID == "qqmail" || input.ConnectorID == "netease163" {
		writePart(mac, "mail-v6")
		writePart(mac, input.MailUsage)
		writePart(mac, input.DriverID)
		writePart(mac, input.AuthMethodID)
	}
	writePart(mac, connectorID.String())
	writePart(mac, input.ReconnectConnectionID.String())
	if input.Target == nil {
		writePart(mac, "")
		writePart(mac, "")
		writePart(mac, "")
	} else {
		writePart(mac, string(input.Target.Type))
		writePart(mac, strings.TrimSpace(input.Target.ID))
		writePart(mac, strings.TrimSpace(input.Target.BindingInstanceID))
	}
	for _, key := range keys {
		writePart(mac, key)
		writePart(mac, input.Values[key])
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func writePart(target hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = target.Write(length[:])
	_, _ = target.Write([]byte(value))
}
