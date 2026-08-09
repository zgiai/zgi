package adapter

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsDeterministicRejectionUsesTypedSentinelsOnly(t *testing.T) {
	for _, err := range []error{
		ErrInvalidRequest,
		fmt.Errorf("provider rejected request: %w", ErrContentPolicyViolation),
		ErrCapabilityUnsupported,
	} {
		if !IsDeterministicRejection(err) {
			t.Fatalf("IsDeterministicRejection(%v) = false", err)
		}
	}

	if IsDeterministicRejection(errors.New("upstream operation unsupported during maintenance")) {
		t.Fatal("untyped provider text must not suppress an operational failure")
	}
}
