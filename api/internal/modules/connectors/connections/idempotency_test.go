package connections

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestCommandHasherProducesStableScopedIdempotencyHash(t *testing.T) {
	hasher, err := NewCommandHasher("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewCommandHasher() error = %v", err)
	}
	organizationID, ownerID := uuid.New(), uuid.New()

	first, err := hasher.IdempotencyKeyHash(organizationID, ownerID, strings.Repeat("a", 16))
	if err != nil {
		t.Fatalf("IdempotencyKeyHash() error = %v", err)
	}
	second, _ := hasher.IdempotencyKeyHash(organizationID, ownerID, strings.Repeat("a", 16))
	otherOwner, _ := hasher.IdempotencyKeyHash(organizationID, uuid.New(), strings.Repeat("a", 16))
	if first == "" || first != second || first == otherOwner || strings.Contains(first, strings.Repeat("a", 16)) {
		t.Fatalf("idempotency hashes first=%q second=%q other=%q", first, second, otherOwner)
	}
}

func TestCommandHasherFingerprintIsOrderIndependentAndFreezesSecretValues(t *testing.T) {
	hasher, err := NewCommandHasher("12345678901234567890123456789012")
	if err != nil {
		t.Fatalf("NewCommandHasher() error = %v", err)
	}
	target := &connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: uuid.NewString()}
	base := CommandFingerprintInput{
		ConnectorID: "github", Target: target,
		Values: map[string]string{"token": "github_pat_secret", "region": "cn"},
	}
	reordered := CommandFingerprintInput{
		ConnectorID: "github", Target: target,
		Values: map[string]string{"region": "cn", "token": "github_pat_secret"},
	}

	first, err := hasher.RequestFingerprint(base)
	if err != nil {
		t.Fatalf("RequestFingerprint() error = %v", err)
	}
	second, _ := hasher.RequestFingerprint(reordered)
	changedValue := reordered
	changedValue.Values = map[string]string{"region": "cn", "token": "github_pat_different"}
	third, _ := hasher.RequestFingerprint(changedValue)
	if first == "" || first != second || first == third {
		t.Fatalf("fingerprints first=%q second=%q changed=%q", first, second, third)
	}
	if strings.Contains(first, "github_pat_secret") {
		t.Fatal("request fingerprint contains submitted Key")
	}
}

func TestCommandHasherRejectsInvalidInputsWithoutEchoingSecret(t *testing.T) {
	if _, err := NewCommandHasher("short"); err == nil {
		t.Fatal("NewCommandHasher() accepted short key")
	}
	hasher, _ := NewCommandHasher("12345678901234567890123456789012")
	const secret = "github_pat_never_echo"
	_, err := hasher.RequestFingerprint(CommandFingerprintInput{
		ConnectorID: "GitHub with spaces",
		Values:      map[string]string{"token": secret},
	})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("RequestFingerprint() error = %v", err)
	}
}
