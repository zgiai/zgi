package connections

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const (
	impactTokenTestMaxLength    = 4096
	impactTokenTestV2MaxLength  = 512
	impactTokenTestDigestDomain = "zgi/connectors/removal-impact-active-bindings/v2\x00"
)

func TestImpactTokenV2IsBoundedAndOmitsBindingDetails(t *testing.T) {
	codec := newImpactTokenTestCodec(t)
	organizationID := uuid.New()
	ownerAccountID := uuid.New()
	connectionID := uuid.New()
	now := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)

	for _, count := range []int{0, 1, 268, 10_000} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			bindings := impactTokenTestSnapshots(count)
			reverseBindingSnapshots(bindings)
			original := append([]BindingRevisionSnapshot(nil), bindings...)

			token, expiresAt, err := codec.Issue(
				organizationID, ownerAccountID, connectionID, 7, bindings, now,
			)
			if err != nil {
				t.Fatalf("Issue() error = %v", err)
			}
			if !slices.Equal(bindings, original) {
				t.Fatal("Issue() mutated the caller's binding order")
			}
			if len(token) > impactTokenTestMaxLength {
				t.Fatalf("Issue() token length = %d, want <= %d", len(token), impactTokenTestMaxLength)
			}
			if len(token) > impactTokenTestV2MaxLength {
				t.Fatalf("Issue() V2 token length = %d, want <= %d", len(token), impactTokenTestV2MaxLength)
			}
			if !expiresAt.Equal(now.Add(removalImpactTokenTTL)) {
				t.Fatalf("Issue() expiresAt = %v, want %v", expiresAt, now.Add(removalImpactTokenTTL))
			}
			if err := codec.Verify(
				token, organizationID, ownerAccountID, connectionID, 7,
				reversedBindingSnapshotCopy(bindings), now,
			); err != nil {
				t.Fatalf("Verify() error = %v", err)
			}

			payload := decodeImpactTokenPayload(t, token)
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatal(err)
			}
			var version int
			if err := json.Unmarshal(fields["v"], &version); err != nil || version != 2 {
				t.Fatalf("claims version = %d, %v; want 2", version, err)
			}
			if _, exists := fields["bindings"]; exists {
				t.Fatal("V2 claims leaked the complete bindings array")
			}
			var bindingCount uint64
			if err := json.Unmarshal(fields["binding_count"], &bindingCount); err != nil || bindingCount != uint64(count) {
				t.Fatalf("binding_count = %d, %v; want %d", bindingCount, err, count)
			}
			var encodedDigest string
			if err := json.Unmarshal(fields["binding_digest"], &encodedDigest); err != nil {
				t.Fatal(err)
			}
			if want := expectedImpactTokenBindingDigest(bindings); encodedDigest != want {
				t.Fatalf("binding_digest = %q, want %q", encodedDigest, want)
			}
			digest, err := base64.RawURLEncoding.DecodeString(encodedDigest)
			if err != nil || len(digest) != 32 {
				t.Fatalf("binding_digest decoded length = %d, %v; want 32", len(digest), err)
			}
			for _, binding := range bindings {
				if bytes.Contains(payload, []byte(binding.ID.String())) {
					t.Fatalf("V2 claims leaked binding UUID %s", binding.ID)
				}
			}
		})
	}
}

func TestImpactTokenBindingDigestGoldenVectors(t *testing.T) {
	first := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	second := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	tests := []struct {
		name     string
		bindings []BindingRevisionSnapshot
		want     string
	}{
		{name: "empty", want: "NzCE2GjIxRlzKfl2mg9vmQzfxmth7q522kY3er8ArWg"},
		{
			name: "sorted UUID and revision encoding",
			bindings: []BindingRevisionSnapshot{
				{ID: second, Revision: 2},
				{ID: first, Revision: 5},
			},
			want: "LrQtPb8ogF7b3iaYxllzBwacUXfnw-BqIK-BWOPqcbY",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			count, digest, err := removalImpactBindingDigest(test.bindings)
			if err != nil {
				t.Fatal(err)
			}
			if count != uint64(len(test.bindings)) {
				t.Fatalf("count = %d, want %d", count, len(test.bindings))
			}
			if got := base64.RawURLEncoding.EncodeToString(digest[:]); got != test.want {
				t.Fatalf("digest = %q, want %q", got, test.want)
			}
		})
	}
}

func TestImpactTokenV2DetectsSnapshotAndIdentityDrift(t *testing.T) {
	codec := newImpactTokenTestCodec(t)
	organizationID := uuid.New()
	ownerAccountID := uuid.New()
	connectionID := uuid.New()
	now := time.Date(2026, time.September, 5, 13, 0, 0, 0, time.UTC)
	bindings := impactTokenTestSnapshots(3)
	token, _, err := codec.Issue(organizationID, ownerAccountID, connectionID, 4, bindings, now)
	if err != nil {
		t.Fatal(err)
	}

	reordered := reversedBindingSnapshotCopy(bindings)
	if err := codec.Verify(token, organizationID, ownerAccountID, connectionID, 4, reordered, now); err != nil {
		t.Fatalf("Verify() reordered snapshot error = %v", err)
	}

	revisionChanged := append([]BindingRevisionSnapshot(nil), bindings...)
	revisionChanged[0].Revision++
	removed := append([]BindingRevisionSnapshot(nil), bindings[:len(bindings)-1]...)
	added := append(append([]BindingRevisionSnapshot(nil), bindings...), impactTokenTestSnapshots(4)[3])
	replaced := append([]BindingRevisionSnapshot(nil), bindings...)
	replaced[0].ID = uuid.New()

	checks := []struct {
		name             string
		organizationID   uuid.UUID
		ownerAccountID   uuid.UUID
		connectionID     uuid.UUID
		connectionRev    int
		bindings         []BindingRevisionSnapshot
		verificationTime time.Time
	}{
		{name: "revision changed", organizationID: organizationID, ownerAccountID: ownerAccountID, connectionID: connectionID, connectionRev: 4, bindings: revisionChanged, verificationTime: now},
		{name: "binding removed", organizationID: organizationID, ownerAccountID: ownerAccountID, connectionID: connectionID, connectionRev: 4, bindings: removed, verificationTime: now},
		{name: "binding added", organizationID: organizationID, ownerAccountID: ownerAccountID, connectionID: connectionID, connectionRev: 4, bindings: added, verificationTime: now},
		{name: "binding replaced", organizationID: organizationID, ownerAccountID: ownerAccountID, connectionID: connectionID, connectionRev: 4, bindings: replaced, verificationTime: now},
		{name: "organization changed", organizationID: uuid.New(), ownerAccountID: ownerAccountID, connectionID: connectionID, connectionRev: 4, bindings: bindings, verificationTime: now},
		{name: "owner changed", organizationID: organizationID, ownerAccountID: uuid.New(), connectionID: connectionID, connectionRev: 4, bindings: bindings, verificationTime: now},
		{name: "connection changed", organizationID: organizationID, ownerAccountID: ownerAccountID, connectionID: uuid.New(), connectionRev: 4, bindings: bindings, verificationTime: now},
		{name: "connection revision changed", organizationID: organizationID, ownerAccountID: ownerAccountID, connectionID: connectionID, connectionRev: 5, bindings: bindings, verificationTime: now},
		{name: "expired", organizationID: organizationID, ownerAccountID: ownerAccountID, connectionID: connectionID, connectionRev: 4, bindings: bindings, verificationTime: now.Add(removalImpactTokenTTL)},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			err := codec.Verify(
				token, check.organizationID, check.ownerAccountID, check.connectionID,
				check.connectionRev, check.bindings, check.verificationTime,
			)
			assertImpactTokenErrorCode(t, err, connectorcore.ErrorCodeConflict)
		})
	}
}

func TestImpactTokenRejectsInvalidSnapshotsAndMalformedTokens(t *testing.T) {
	codec := newImpactTokenTestCodec(t)
	organizationID := uuid.New()
	ownerAccountID := uuid.New()
	connectionID := uuid.New()
	now := time.Date(2026, time.September, 5, 14, 0, 0, 0, time.UTC)
	validBindings := impactTokenTestSnapshots(2)

	invalidSnapshots := []struct {
		name     string
		bindings []BindingRevisionSnapshot
	}{
		{name: "nil id", bindings: []BindingRevisionSnapshot{{ID: uuid.Nil, Revision: 1}}},
		{name: "zero revision", bindings: []BindingRevisionSnapshot{{ID: uuid.New(), Revision: 0}}},
		{name: "duplicate id", bindings: []BindingRevisionSnapshot{{ID: validBindings[0].ID, Revision: 1}, {ID: validBindings[0].ID, Revision: 2}}},
	}
	for _, check := range invalidSnapshots {
		t.Run("issue "+check.name, func(t *testing.T) {
			_, _, err := codec.Issue(organizationID, ownerAccountID, connectionID, 1, check.bindings, now)
			assertImpactTokenErrorCode(t, err, connectorcore.ErrorCodeInvalidInput)
		})
	}

	validToken, _, err := codec.Issue(organizationID, ownerAccountID, connectionID, 1, validBindings, now)
	if err != nil {
		t.Fatal(err)
	}
	validParts := strings.Split(validToken, ".")
	tamperedPayload := validParts[0]
	if tamperedPayload[0] == 'A' {
		tamperedPayload = "B" + tamperedPayload[1:]
	} else {
		tamperedPayload = "A" + tamperedPayload[1:]
	}
	otherCodec, err := NewImpactTokenCodec(strings.Repeat("j", 32))
	if err != nil {
		t.Fatal(err)
	}
	wrongKeyToken, _, err := otherCodec.Issue(organizationID, ownerAccountID, connectionID, 1, validBindings, now)
	if err != nil {
		t.Fatal(err)
	}
	malformed := []string{
		"",
		"one-part",
		"too.many.parts",
		" " + validToken,
		validToken + " ",
		"%%%." + validParts[1],
		validParts[0] + ".%%%",
		validParts[0] + "." + validParts[1][:8] + "\r\n" + validParts[1][8:],
		validParts[0] + "=." + validParts[1],
		validParts[0] + "." + validParts[1] + "=",
		validParts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte("short")),
		tamperedPayload + "." + validParts[1],
		wrongKeyToken,
		strings.Repeat("x", impactTokenTestMaxLength+1),
	}
	for index, token := range malformed {
		t.Run("malformed "+strconv.Itoa(index), func(t *testing.T) {
			err := codec.Verify(token, organizationID, ownerAccountID, connectionID, 1, validBindings, now)
			assertImpactTokenErrorCode(t, err, connectorcore.ErrorCodeConflict)
		})
	}

	badClaims := []map[string]any{
		{
			"v": 3, "organization_id": organizationID, "owner_account_id": ownerAccountID,
			"connection_id": connectionID, "connection_revision": 1,
			"binding_count": 2, "binding_digest": strings.Repeat("a", 43),
			"expires_at": now.Add(removalImpactTokenTTL).Unix(),
		},
		{
			"v": 2, "organization_id": organizationID, "owner_account_id": ownerAccountID,
			"connection_id": connectionID, "connection_revision": 1,
			"binding_count": -1, "binding_digest": strings.Repeat("a", 43),
			"expires_at": now.Add(removalImpactTokenTTL).Unix(),
		},
		{
			"v": 2, "organization_id": organizationID, "owner_account_id": ownerAccountID,
			"connection_id": connectionID, "connection_revision": 1,
			"binding_count": 2, "binding_digest": "short",
			"expires_at": now.Add(removalImpactTokenTTL).Unix(),
		},
	}
	for index, claims := range badClaims {
		t.Run("signed bad claims "+strconv.Itoa(index), func(t *testing.T) {
			token := signImpactTokenTestClaims(t, codec, claims)
			err := codec.Verify(token, organizationID, ownerAccountID, connectionID, 1, validBindings, now)
			assertImpactTokenErrorCode(t, err, connectorcore.ErrorCodeConflict)
		})
	}
	for _, digestSize := range []int{sha256.Size - 1, sha256.Size + 1} {
		t.Run("signed digest size "+strconv.Itoa(digestSize), func(t *testing.T) {
			token := signImpactTokenTestClaims(t, codec, map[string]any{
				"v": 2, "organization_id": organizationID, "owner_account_id": ownerAccountID,
				"connection_id": connectionID, "connection_revision": 1,
				"binding_count":  2,
				"binding_digest": base64.RawURLEncoding.EncodeToString(make([]byte, digestSize)),
				"expires_at":     now.Add(removalImpactTokenTTL).Unix(),
			})
			err := codec.Verify(token, organizationID, ownerAccountID, connectionID, 1, validBindings, now)
			assertImpactTokenErrorCode(t, err, connectorcore.ErrorCodeConflict)
		})
	}

	invalidCurrent := []BindingRevisionSnapshot{{ID: validBindings[0].ID, Revision: 1}, {ID: validBindings[0].ID, Revision: 2}}
	assertImpactTokenErrorCode(
		t,
		codec.Verify(validToken, organizationID, ownerAccountID, connectionID, 1, invalidCurrent, now),
		connectorcore.ErrorCodeConflict,
	)
}

func TestImpactTokenVerifyAcceptsBoundedLegacyV1(t *testing.T) {
	codec := newImpactTokenTestCodec(t)
	organizationID := uuid.New()
	ownerAccountID := uuid.New()
	connectionID := uuid.New()
	now := time.Date(2026, time.September, 5, 15, 0, 0, 0, time.UTC)
	bindings := impactTokenTestSnapshots(3)
	legacy := issueLegacyImpactToken(t, codec, organizationID, ownerAccountID, connectionID, 2, bindings, now)
	if len(legacy) > impactTokenTestMaxLength {
		t.Fatalf("legacy fixture length = %d, want bounded", len(legacy))
	}
	if err := codec.Verify(legacy, organizationID, ownerAccountID, connectionID, 2, reversedBindingSnapshotCopy(bindings), now); err != nil {
		t.Fatalf("Verify() legacy error = %v", err)
	}

	drifted := append([]BindingRevisionSnapshot(nil), bindings...)
	drifted[0].Revision++
	assertImpactTokenErrorCode(
		t,
		codec.Verify(legacy, organizationID, ownerAccountID, connectionID, 2, drifted, now),
		connectorcore.ErrorCodeConflict,
	)
	assertImpactTokenErrorCode(
		t,
		codec.Verify(legacy, organizationID, ownerAccountID, connectionID, 2, bindings, now.Add(removalImpactTokenTTL)),
		connectorcore.ErrorCodeConflict,
	)

	largeBindings := impactTokenTestSnapshots(64)
	largeLegacy := issueLegacyImpactToken(t, codec, organizationID, ownerAccountID, connectionID, 2, largeBindings, now)
	if len(largeLegacy) <= impactTokenTestMaxLength {
		t.Fatalf("large legacy fixture length = %d, want > %d", len(largeLegacy), impactTokenTestMaxLength)
	}
	assertImpactTokenErrorCode(
		t,
		codec.Verify(largeLegacy, organizationID, ownerAccountID, connectionID, 2, largeBindings, now),
		connectorcore.ErrorCodeConflict,
	)
}

func TestMaintenanceHighCardinalityForceRemovalUsesBoundedToken(t *testing.T) {
	service, repository, bindings, _, _, _ := newMaintenanceFixture(t)
	existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
	repository.connections[existing.ID] = existing
	for range 268 {
		bindings.items = append(bindings.items, maintenanceBinding(existing, connectorcore.BindingStatusActive))
	}

	impact, err := service.GetRemovalImpact(context.Background(), existing.OrganizationID, existing.OwnerAccountID, existing.ID)
	if err != nil {
		t.Fatal(err)
	}
	if impact.CanRemove || len(impact.AffectedBindings) != 268 || len(impact.ImpactToken) > impactTokenTestMaxLength {
		t.Fatalf("GetRemovalImpact() canRemove/bindings/tokenLength = %v/%d/%d", impact.CanRemove, len(impact.AffectedBindings), len(impact.ImpactToken))
	}
	if err := service.Remove(context.Background(), RemoveConnectionCommand{
		OrganizationID: existing.OrganizationID,
		OwnerAccountID: existing.OwnerAccountID,
		ConnectionID:   existing.ID,
		Force:          true,
		ImpactToken:    impact.ImpactToken,
	}); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if !repository.removed || len(repository.lastRemovalBindings) != 268 {
		t.Fatalf("repository removal/full snapshot = %v/%d, want true/268", repository.removed, len(repository.lastRemovalBindings))
	}
	for index, binding := range bindings.items {
		if binding.Status != connectorcore.BindingStatusNeedsReplacement || binding.Revision != 2 {
			t.Fatalf("binding[%d] status/revision = %s/%d", index, binding.Status, binding.Revision)
		}
	}
}

type legacyRemovalImpactClaims struct {
	Version            int                       `json:"v"`
	OrganizationID     uuid.UUID                 `json:"organization_id"`
	OwnerAccountID     uuid.UUID                 `json:"owner_account_id"`
	ConnectionID       uuid.UUID                 `json:"connection_id"`
	ConnectionRevision int                       `json:"connection_revision"`
	Bindings           []BindingRevisionSnapshot `json:"bindings"`
	ExpiresAtUnix      int64                     `json:"expires_at"`
}

func newImpactTokenTestCodec(t *testing.T) *ImpactTokenCodec {
	t.Helper()
	codec, err := NewImpactTokenCodec(strings.Repeat("i", 32))
	if err != nil {
		t.Fatal(err)
	}
	return codec
}

func impactTokenTestSnapshots(count int) []BindingRevisionSnapshot {
	result := make([]BindingRevisionSnapshot, 0, count)
	for index := range count {
		id := uuid.UUID{0: 1}
		binary.BigEndian.PutUint64(id[8:], uint64(index+1))
		result = append(result, BindingRevisionSnapshot{ID: id, Revision: index%7 + 1})
	}
	return result
}

func reverseBindingSnapshots(items []BindingRevisionSnapshot) {
	for left, right := 0, len(items)-1; left < right; left, right = left+1, right-1 {
		items[left], items[right] = items[right], items[left]
	}
}

func reversedBindingSnapshotCopy(items []BindingRevisionSnapshot) []BindingRevisionSnapshot {
	result := append([]BindingRevisionSnapshot(nil), items...)
	reverseBindingSnapshots(result)
	return result
}

func decodeImpactTokenPayload(t *testing.T, token string) []byte {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		t.Fatalf("token parts = %d, want 2", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func signImpactTokenTestClaims(t *testing.T, codec *ImpactTokenCodec, claims any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(codec.sign(encoded))
}

func expectedImpactTokenBindingDigest(bindings []BindingRevisionSnapshot) string {
	snapshot := append([]BindingRevisionSnapshot(nil), bindings...)
	slices.SortFunc(snapshot, func(left, right BindingRevisionSnapshot) int {
		return bytes.Compare(left.ID[:], right.ID[:])
	})
	digest := sha256.New()
	_, _ = digest.Write([]byte(impactTokenTestDigestDomain))
	var encodedInteger [8]byte
	binary.BigEndian.PutUint64(encodedInteger[:], uint64(len(snapshot)))
	_, _ = digest.Write(encodedInteger[:])
	for _, item := range snapshot {
		_, _ = digest.Write(item.ID[:])
		binary.BigEndian.PutUint64(encodedInteger[:], uint64(item.Revision))
		_, _ = digest.Write(encodedInteger[:])
	}
	return base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}

func issueLegacyImpactToken(
	t *testing.T,
	codec *ImpactTokenCodec,
	organizationID, ownerAccountID, connectionID uuid.UUID,
	connectionRevision int,
	bindings []BindingRevisionSnapshot,
	now time.Time,
) string {
	t.Helper()
	snapshot := append([]BindingRevisionSnapshot(nil), bindings...)
	sortBindingSnapshots(snapshot)
	return signImpactTokenTestClaims(t, codec, legacyRemovalImpactClaims{
		Version: 1, OrganizationID: organizationID, OwnerAccountID: ownerAccountID,
		ConnectionID: connectionID, ConnectionRevision: connectionRevision,
		Bindings: snapshot, ExpiresAtUnix: now.UTC().Add(removalImpactTokenTTL).Unix(),
	})
}

func assertImpactTokenErrorCode(t *testing.T, err error, want connectorcore.ErrorCode) {
	t.Helper()
	if !connectorcore.IsErrorCode(err, want) {
		t.Fatalf("error = %v (%s), want %s", err, connectorcore.ErrorCodeOf(err), want)
	}
}
