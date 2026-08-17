package integrations

import "testing"

func TestActionDisabledErrorExposesSafePolicyRecovery(t *testing.T) {
	err := NewErrorWithReason(
		ErrorCodeDisabled,
		"action_disabled_by_policy",
		"internal policy message",
		nil,
	)
	recovery := err.(*Error).PublicErrorRecovery()
	if recovery["reason_code"] != "action_disabled_by_policy" ||
		recovery["recovery_kind"] != "action_policy" ||
		recovery["provider_request_sent"] != false ||
		recovery["recoverable"] != false ||
		recovery["recovery_action"] != "enable_action_in_connection_center" {
		t.Fatalf("recovery = %#v", recovery)
	}
	for _, forbidden := range []string{"connection_id", "credentials", "internal policy message"} {
		if _, exists := recovery[forbidden]; exists {
			t.Fatalf("recovery exposes %q: %#v", forbidden, recovery)
		}
	}
}
