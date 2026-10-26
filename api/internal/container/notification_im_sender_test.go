package container

import (
	"errors"
	"testing"
)

func TestIMDeliveryOutcomeReadsSkippedRecipients(t *testing.T) {
	if err := imDeliveryOutcome(map[string]any{"status": "accepted"}); err != nil {
		t.Fatalf("an accepted send = %v", err)
	}
	if err := imDeliveryOutcome(nil); err != nil {
		t.Fatalf("a provider without a report = %v", err)
	}
	if err := imDeliveryOutcome(map[string]any{"status": "partial", "rate_limited_user_ids": []string{"u1"}}); !errors.Is(err, errIMRecipientRateLimited) {
		t.Fatalf("a rate limited recipient = %v", err)
	}
	if err := imDeliveryOutcome(map[string]any{"status": "partial", "invalid_user_ids": []any{"u1"}, "rate_limited_user_ids": []any{}}); !errors.Is(err, errIMRecipientRejected) {
		t.Fatalf("an invalid recipient = %v", err)
	}
}
