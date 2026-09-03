package audit

import (
	"bytes"
	"testing"
)

func TestDeriveHMACKeyIsPurposeSeparatedAndDeterministic(t *testing.T) {
	if key := DeriveHMACKey("   "); key != nil {
		t.Fatalf("empty master key derived %d bytes", len(key))
	}
	first := DeriveHMACKey("12345678901234567890123456789012")
	second := DeriveHMACKey("12345678901234567890123456789012")
	other := DeriveHMACKey("abcdefghijklmnopqrstuvwxzy123456")
	if len(first) != 32 || !bytes.Equal(first, second) || bytes.Equal(first, other) {
		t.Fatal("audit HMAC key derivation is not stable and key-specific")
	}
}
