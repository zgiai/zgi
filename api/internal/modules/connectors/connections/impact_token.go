package connections

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const (
	removalImpactTokenTTL                   = 10 * time.Minute
	removalImpactTokenMaxLength             = 4096
	removalImpactTokenVersionV1             = 1
	removalImpactTokenVersionV2             = 2
	removalImpactActiveBindingsDigestDomain = "zgi/connectors/removal-impact-active-bindings/v2\x00"
)

type ImpactTokenCodec struct {
	key [sha256.Size]byte
}

type removalImpactClaimsVersion struct {
	Version int `json:"v"`
}

type removalImpactClaimsV1 struct {
	Version            int                       `json:"v"`
	OrganizationID     uuid.UUID                 `json:"organization_id"`
	OwnerAccountID     uuid.UUID                 `json:"owner_account_id"`
	ConnectionID       uuid.UUID                 `json:"connection_id"`
	ConnectionRevision int                       `json:"connection_revision"`
	Bindings           []BindingRevisionSnapshot `json:"bindings"`
	ExpiresAtUnix      int64                     `json:"expires_at"`
}

type removalImpactClaimsV2 struct {
	Version            int       `json:"v"`
	OrganizationID     uuid.UUID `json:"organization_id"`
	OwnerAccountID     uuid.UUID `json:"owner_account_id"`
	ConnectionID       uuid.UUID `json:"connection_id"`
	ConnectionRevision int       `json:"connection_revision"`
	BindingCount       uint64    `json:"binding_count"`
	BindingDigest      string    `json:"binding_digest"`
	ExpiresAtUnix      int64     `json:"expires_at"`
}

func NewImpactTokenCodec(masterKey string) (*ImpactTokenCodec, error) {
	if len(masterKey) != 32 {
		return nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	deriver := hmac.New(sha256.New, []byte(masterKey))
	_, _ = deriver.Write([]byte("zgi/connectors/removal-impact-token/v1"))
	derived := deriver.Sum(nil)
	codec := &ImpactTokenCodec{}
	copy(codec.key[:], derived)
	clear(derived)
	return codec, nil
}

func (codec *ImpactTokenCodec) Issue(
	organizationID, ownerAccountID, connectionID uuid.UUID,
	connectionRevision int,
	bindings []BindingRevisionSnapshot,
	now time.Time,
) (string, time.Time, error) {
	if codec == nil || organizationID == uuid.Nil || ownerAccountID == uuid.Nil || connectionID == uuid.Nil ||
		connectionRevision < 1 || now.IsZero() {
		return "", time.Time{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	bindingCount, bindingDigest, err := removalImpactBindingDigest(bindings)
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := now.UTC().Add(removalImpactTokenTTL)
	claims := removalImpactClaimsV2{
		Version: removalImpactTokenVersionV2, OrganizationID: organizationID, OwnerAccountID: ownerAccountID,
		ConnectionID: connectionID, ConnectionRevision: connectionRevision,
		BindingCount: bindingCount, BindingDigest: base64.RawURLEncoding.EncodeToString(bindingDigest[:]),
		ExpiresAtUnix: expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := codec.sign(encoded)
	token := encoded + "." + base64.RawURLEncoding.EncodeToString(signature)
	if len(token) > removalImpactTokenMaxLength {
		return "", time.Time{}, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	return token, expiresAt, nil
}

func (codec *ImpactTokenCodec) Verify(
	token string,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	connectionRevision int,
	bindings []BindingRevisionSnapshot,
	now time.Time,
) error {
	if codec == nil || organizationID == uuid.Nil || ownerAccountID == uuid.Nil || connectionID == uuid.Nil ||
		connectionRevision < 1 || now.IsZero() {
		return connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
	}
	if len(token) > removalImpactTokenMaxLength {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	if token != strings.TrimSpace(token) {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(signature) != sha256.Size ||
		base64.RawURLEncoding.EncodeToString(signature) != parts[1] ||
		!hmac.Equal(signature, codec.sign(parts[0])) {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	payload, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || base64.RawURLEncoding.EncodeToString(payload) != parts[0] {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	var version removalImpactClaimsVersion
	if err := json.Unmarshal(payload, &version); err != nil {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	valid := false
	switch version.Version {
	case removalImpactTokenVersionV1:
		valid = verifyRemovalImpactClaimsV1(
			payload, organizationID, ownerAccountID, connectionID, connectionRevision, bindings, now,
		)
	case removalImpactTokenVersionV2:
		valid = verifyRemovalImpactClaimsV2(
			payload, organizationID, ownerAccountID, connectionID, connectionRevision, bindings, now,
		)
	}
	if !valid {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	return nil
}

func verifyRemovalImpactClaimsV1(
	payload []byte,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	connectionRevision int,
	bindings []BindingRevisionSnapshot,
	now time.Time,
) bool {
	var claims removalImpactClaimsV1
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	expected, err := normalizeRemovalImpactBindings(bindings)
	if err != nil {
		return false
	}
	actual, err := normalizeRemovalImpactBindings(claims.Bindings)
	if err != nil {
		return false
	}
	return claims.Version == removalImpactTokenVersionV1 &&
		claims.OrganizationID == organizationID && claims.OwnerAccountID == ownerAccountID &&
		claims.ConnectionID == connectionID && claims.ConnectionRevision == connectionRevision &&
		claims.ExpiresAtUnix > now.UTC().Unix() && equalBindingSnapshots(expected, actual)
}

func verifyRemovalImpactClaimsV2(
	payload []byte,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	connectionRevision int,
	bindings []BindingRevisionSnapshot,
	now time.Time,
) bool {
	var claims removalImpactClaimsV2
	if err := json.Unmarshal(payload, &claims); err != nil ||
		claims.Version != removalImpactTokenVersionV2 ||
		claims.OrganizationID != organizationID || claims.OwnerAccountID != ownerAccountID ||
		claims.ConnectionID != connectionID || claims.ConnectionRevision != connectionRevision ||
		claims.ExpiresAtUnix <= now.UTC().Unix() {
		return false
	}
	claimedDigest, err := base64.RawURLEncoding.Strict().DecodeString(claims.BindingDigest)
	if err != nil || len(claimedDigest) != sha256.Size {
		return false
	}
	bindingCount, bindingDigest, err := removalImpactBindingDigest(bindings)
	if err != nil {
		return false
	}
	return claims.BindingCount == bindingCount && hmac.Equal(claimedDigest, bindingDigest[:])
}

func removalImpactBindingDigest(bindings []BindingRevisionSnapshot) (uint64, [sha256.Size]byte, error) {
	snapshot, err := normalizeRemovalImpactBindings(bindings)
	if err != nil {
		return 0, [sha256.Size]byte{}, err
	}

	digest := sha256.New()
	_, _ = digest.Write([]byte(removalImpactActiveBindingsDigestDomain))
	var encodedInteger [8]byte
	binary.BigEndian.PutUint64(encodedInteger[:], uint64(len(snapshot)))
	_, _ = digest.Write(encodedInteger[:])
	for _, item := range snapshot {
		_, _ = digest.Write(item.ID[:])
		binary.BigEndian.PutUint64(encodedInteger[:], uint64(item.Revision))
		_, _ = digest.Write(encodedInteger[:])
	}
	var result [sha256.Size]byte
	copy(result[:], digest.Sum(nil))
	return uint64(len(snapshot)), result, nil
}

func normalizeRemovalImpactBindings(bindings []BindingRevisionSnapshot) ([]BindingRevisionSnapshot, error) {
	snapshot := append([]BindingRevisionSnapshot(nil), bindings...)
	sortBindingSnapshots(snapshot)
	for index, item := range snapshot {
		if item.ID == uuid.Nil || item.Revision < 1 || (index > 0 && snapshot[index-1].ID == item.ID) {
			return nil, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
		}
	}
	return snapshot, nil
}

func (codec *ImpactTokenCodec) sign(payload string) []byte {
	mac := hmac.New(sha256.New, codec.key[:])
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}
