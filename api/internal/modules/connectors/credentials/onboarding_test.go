package credentials

import (
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestOnboardingDeviceUsesDistinctPurposeWithoutChangingC1(t *testing.T) {
	store, err := NewStore("test", map[string]string{"test": strings.Repeat("x", 32)})
	if err != nil {
		t.Fatal(err)
	}
	subject := Subject{Purpose: OnboardingDevice, OrganizationID: uuid.New(), ID: uuid.New(), ConnectorID: "feishu", Version: 1}
	envelope, err := store.Protect(subject, []byte(`{"device_code":"fixture-device"}`))
	if err != nil || !strings.HasPrefix(envelope, "c1.") {
		t.Fatal("device encryption failed")
	}
	if _, err = store.Open(subject, envelope); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []Purpose{ConnectionKey, ConfirmationContinuation} {
		other := subject
		other.Purpose = purpose
		if _, err = store.Open(other, envelope); err == nil {
			t.Fatal("cross-purpose decryption")
		}
	}
	other := subject
	other.Version++
	if _, err = store.Open(other, envelope); err == nil {
		t.Fatal("old generation reused")
	}
}
