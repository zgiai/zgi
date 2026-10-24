package digitalhuman

// ChannelResponsePolicy is the conversational feedback a channel message
// gets: an acknowledgement once it is accepted and intermediate progress
// while it runs. Both are always on now; the policy stays in the execution
// snapshot so the revisions of persisted runs are unchanged. Final answers
// and generated files use the durable delivery outbox instead.
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

func channelResponsePolicyPointer(policy ChannelResponsePolicy) *ChannelResponsePolicy {
	result := policy
	return &result
}
