package credentials

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestStorePurposeAndIdentityIsolation(t *testing.T) {
	store, err := NewStore("key.1", map[string]string{"key.1": strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	subject := Subject{Purpose: ConnectionKey, OrganizationID: uuid.New(), ID: uuid.New(), ConnectorID: "github", Version: 1}
	secret := []byte(`{"token":"test-only-secret"}`)
	first, err := store.Protect(subject, secret)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Protect(subject, secret)
	if err != nil || first == second || strings.Contains(first, "test-only-secret") {
		t.Fatal("nonces must be random and ciphertext must hide the secret")
	}
	opened, err := store.Open(subject, first)
	if err != nil || !bytes.Equal(opened, secret) {
		t.Fatalf("round trip: %v", err)
	}
	clear(opened)
	variants := []Subject{subject, subject, subject, subject, subject}
	variants[0].Purpose = ConfirmationContinuation
	variants[1].OrganizationID = uuid.New()
	variants[2].ID = uuid.New()
	variants[3].ConnectorID = "web-search"
	variants[4].Version++
	for _, changed := range variants {
		if _, err := store.Open(changed, first); err == nil {
			t.Fatal("accepted another authenticated subject")
		}
	}
	for _, invalid := range []string{"v1.abc", "v2.key.abc", first[:len(first)-3], "c1.missing.AAAA"} {
		if _, err := store.Open(subject, invalid); err == nil {
			t.Fatal("accepted invalid envelope")
		}
	}
}

func TestStoreKeyRotationAndConcurrentUse(t *testing.T) {
	old, _ := NewStore("old", map[string]string{"old": strings.Repeat("a", 32)})
	next, _ := NewStore("new", map[string]string{"old": strings.Repeat("a", 32), "new": strings.Repeat("b", 32)})
	subject := Subject{Purpose: ConfirmationContinuation, OrganizationID: uuid.New(), ID: uuid.New(), ConnectorID: "github", Version: 2}
	envelope, err := old.Protect(subject, []byte("frozen-request"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := next.Open(subject, envelope); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 24 {
		group.Go(func() {
			sealed, err := next.Protect(subject, []byte("native"))
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := next.Open(subject, sealed); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
}

func TestStoreRejectsInvalidConfigurationAndInputs(t *testing.T) {
	for _, keys := range []map[string]string{nil, {"missing": strings.Repeat("a", 32)}, {"active": "short"}, {"active": strings.Repeat("a", 32), " active": strings.Repeat("b", 32)}} {
		if _, err := NewStore("active", keys); err == nil {
			t.Fatal("accepted invalid keyring")
		}
	}
	store, _ := NewStore("active", map[string]string{"active": strings.Repeat("a", 32)})
	if _, err := store.Protect(Subject{}, []byte("secret")); err == nil {
		t.Fatal("accepted empty subject")
	}
	subject := Subject{Purpose: ConnectionKey, OrganizationID: uuid.New(), ID: uuid.New(), ConnectorID: "github", Version: 1}
	if _, err := store.Protect(subject, make([]byte, MaxPayloadBytes+1)); err == nil {
		t.Fatal("accepted oversized payload")
	}
}
