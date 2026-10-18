package digitalhuman

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestWebAppAnonymousSessionIsSignedAndBoundToWebApp(t *testing.T) {
	signer, err := NewWebAppSessionSigner("test-secret-that-is-not-committed")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	signer.now = func() time.Time { return now }
	webAppID := uuid.New()
	token, issued, err := signer.Issue(webAppID)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := signer.Parse(token, webAppID)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SubjectID != issued.SubjectID || parsed.WebAppID != webAppID || !parsed.ExpiresAt.Equal(now.Add(webAppSessionTTL)) {
		t.Fatalf("parsed session = %#v, issued = %#v", parsed, issued)
	}
	if _, err := signer.Parse(token, uuid.New()); !errors.Is(err, ErrInvalidWebAppSession) {
		t.Fatalf("cross-webapp parse error = %v", err)
	}
}

func TestWebAppAnonymousSessionRejectsTampering(t *testing.T) {
	signer, _ := NewWebAppSessionSigner("test-secret-that-is-not-committed")
	webAppID := uuid.New()
	token, _, _ := signer.Issue(webAppID)
	if _, err := signer.Parse(token+"tampered", webAppID); !errors.Is(err, ErrInvalidWebAppSession) {
		t.Fatalf("tampered parse error = %v", err)
	}
}
