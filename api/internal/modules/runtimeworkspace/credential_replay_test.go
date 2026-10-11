package runtimeworkspace

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCredentialReplayProtectorRoundTripsBoundSecret(t *testing.T) {
	protector, err := NewCredentialReplayProtector("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	aad := CredentialReplayAAD{
		OrganizationID: uuid.New(), RuntimeWorkspaceID: uuid.New(), OperationID: uuid.New(),
		CredentialID: uuid.New(), RequestFingerprint: "fingerprint",
	}
	envelope, err := protector.Seal("zgi_secret", aad)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := protector.Open(envelope, aad)
	if err != nil || secret != "zgi_secret" {
		t.Fatalf("opened secret = %q, %v", secret, err)
	}
	aad.CredentialID = uuid.New()
	if _, err := protector.Open(envelope, aad); err == nil {
		t.Fatal("credential replay envelope opened with different AAD")
	}
}

func TestCredentialReplayProtectorScopesIdempotencyAndFingerprint(t *testing.T) {
	protector, err := NewCredentialReplayProtector("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, accountID := uuid.New(), uuid.New()
	first, err := protector.IdempotencyKeyHash(workspaceID, accountID, "request-key-0001")
	if err != nil {
		t.Fatal(err)
	}
	repeated, _ := protector.IdempotencyKeyHash(workspaceID, accountID, "request-key-0001")
	otherWorkspace, _ := protector.IdempotencyKeyHash(uuid.New(), accountID, "request-key-0001")
	if first != repeated || first == otherWorkspace {
		t.Fatalf("idempotency hashes = %q/%q/%q", first, repeated, otherWorkspace)
	}
	expiresAt := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	fingerprint := protector.RequestFingerprint(workspaceID, accountID, "production", &expiresAt)
	if fingerprint == protector.RequestFingerprint(workspaceID, accountID, "staging", &expiresAt) {
		t.Fatal("request fingerprint ignored credential name")
	}
}
