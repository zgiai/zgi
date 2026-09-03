package connections

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const removalImpactTokenTTL = 10 * time.Minute

type ImpactTokenCodec struct {
	key [sha256.Size]byte
}

type removalImpactClaims struct {
	Version            int                       `json:"v"`
	OrganizationID     uuid.UUID                 `json:"organization_id"`
	OwnerAccountID     uuid.UUID                 `json:"owner_account_id"`
	ConnectionID       uuid.UUID                 `json:"connection_id"`
	ConnectionRevision int                       `json:"connection_revision"`
	Bindings           []BindingRevisionSnapshot `json:"bindings"`
	ExpiresAtUnix      int64                     `json:"expires_at"`
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
	snapshot := append([]BindingRevisionSnapshot(nil), bindings...)
	sortBindingSnapshots(snapshot)
	for index, item := range snapshot {
		if item.ID == uuid.Nil || item.Revision < 1 || (index > 0 && snapshot[index-1].ID == item.ID) {
			return "", time.Time{}, connectorcore.NewError(connectorcore.ErrorCodeInvalidInput, false, nil)
		}
	}
	expiresAt := now.UTC().Add(removalImpactTokenTTL)
	claims := removalImpactClaims{
		Version: 1, OrganizationID: organizationID, OwnerAccountID: ownerAccountID,
		ConnectionID: connectionID, ConnectionRevision: connectionRevision,
		Bindings: snapshot, ExpiresAtUnix: expiresAt.Unix(),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, connectorcore.NewError(connectorcore.ErrorCodeInternal, false, nil)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := codec.sign(encoded)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature), expiresAt, nil
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
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || len(token) > 4096 {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, codec.sign(parts[0])) {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	var claims removalImpactClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	expected := append([]BindingRevisionSnapshot(nil), bindings...)
	sortBindingSnapshots(expected)
	actual := append([]BindingRevisionSnapshot(nil), claims.Bindings...)
	sortBindingSnapshots(actual)
	if claims.Version != 1 || claims.OrganizationID != organizationID || claims.OwnerAccountID != ownerAccountID ||
		claims.ConnectionID != connectionID || claims.ConnectionRevision != connectionRevision ||
		claims.ExpiresAtUnix <= now.UTC().Unix() || !equalBindingSnapshots(expected, actual) {
		return connectorcore.NewError(connectorcore.ErrorCodeConflict, false, nil)
	}
	return nil
}

func (codec *ImpactTokenCodec) sign(payload string) []byte {
	mac := hmac.New(sha256.New, codec.key[:])
	_, _ = mac.Write([]byte(payload))
	return mac.Sum(nil)
}
