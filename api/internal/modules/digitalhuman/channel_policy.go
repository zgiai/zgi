package digitalhuman

import (
	"encoding/json"
)

// ChannelResponsePolicy controls best-effort conversational feedback for
// external chat channels. Final answers and generated files are intentionally
// outside this policy because they use the durable delivery outbox.
type ChannelResponsePolicy struct {
	IntermediateMessagesEnabled bool `json:"intermediate_messages_enabled"`
	AcknowledgementEnabled      bool `json:"acknowledgement_enabled"`
}

func defaultChannelResponsePolicy() ChannelResponsePolicy {
	return ChannelResponsePolicy{
		IntermediateMessagesEnabled: true,
		AcknowledgementEnabled:      true,
	}
}

func channelResponsePolicyFromTriggerPolicy(encoded []byte) ChannelResponsePolicy {
	policy := defaultChannelResponsePolicy()
	if len(encoded) == 0 {
		return policy
	}
	var trigger struct {
		ChannelResponse *struct {
			IntermediateMessagesEnabled *bool `json:"intermediate_messages_enabled"`
			AcknowledgementEnabled      *bool `json:"acknowledgement_enabled"`
		} `json:"channel_response"`
	}
	if json.Unmarshal(encoded, &trigger) != nil || trigger.ChannelResponse == nil {
		return policy
	}
	if trigger.ChannelResponse.IntermediateMessagesEnabled != nil {
		policy.IntermediateMessagesEnabled = *trigger.ChannelResponse.IntermediateMessagesEnabled
	}
	if trigger.ChannelResponse.AcknowledgementEnabled != nil {
		policy.AcknowledgementEnabled = *trigger.ChannelResponse.AcknowledgementEnabled
	}
	return policy
}

func channelResponsePolicyFromRun(run *Run) ChannelResponsePolicy {
	if run == nil || len(run.ExecutionSnapshot) == 0 {
		return defaultChannelResponsePolicy()
	}
	var snapshot struct {
		ChannelResponsePolicy *ChannelResponsePolicy `json:"channel_response_policy"`
	}
	if json.Unmarshal(run.ExecutionSnapshot, &snapshot) != nil || snapshot.ChannelResponsePolicy == nil {
		return defaultChannelResponsePolicy()
	}
	return *snapshot.ChannelResponsePolicy
}

func channelResponsePolicyPointer(policy ChannelResponsePolicy) *ChannelResponsePolicy {
	result := policy
	return &result
}
