package digitalhuman

import "testing"

func TestChannelResponsePolicyDefaultsPreserveExistingBehavior(t *testing.T) {
	for _, encoded := range [][]byte{nil, []byte(`{}`), []byte(`{"manual":true}`)} {
		policy := channelResponsePolicyFromTriggerPolicy(encoded)
		if !policy.IntermediateMessagesEnabled || !policy.AcknowledgementEnabled {
			t.Fatalf("policy for %s = %#v, want both enabled", encoded, policy)
		}
	}
}

func TestChannelResponsePolicySwitchesAreIndependent(t *testing.T) {
	policy := channelResponsePolicyFromTriggerPolicy([]byte(`{
		"manual": true,
		"channel_response": {
			"intermediate_messages_enabled": false,
			"acknowledgement_enabled": true
		}
	}`))
	if policy.IntermediateMessagesEnabled || !policy.AcknowledgementEnabled {
		t.Fatalf("policy = %#v, want intermediate disabled and acknowledgement enabled", policy)
	}

	policy = channelResponsePolicyFromTriggerPolicy([]byte(`{
		"channel_response": {
			"intermediate_messages_enabled": true,
			"acknowledgement_enabled": false
		}
	}`))
	if !policy.IntermediateMessagesEnabled || policy.AcknowledgementEnabled {
		t.Fatalf("policy = %#v, want intermediate enabled and acknowledgement disabled", policy)
	}
}
