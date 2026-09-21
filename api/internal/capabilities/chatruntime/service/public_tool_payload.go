package service

import "github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"

// publicToolPayload keeps the authoritative frozen invocation in
// server persistence while returning a detached, bounded argument preview to
// browsers. Call this only at a public response/stream boundary.
func publicToolPayload(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return nil
	}
	sanitized, ok := toolgovernance.SanitizePublicValue(payload).(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return sanitized
}

func publicToolStreamEvent(event StreamEvent) StreamEvent {
	switch event.EventType {
	case streamEventToolGovernanceDecision,
		streamEventSkillCallStart,
		streamEventSkillCallEnd,
		streamEventSkillCallError,
		streamEventMessageEnd:
		event.Payload = publicToolPayload(event.Payload)
	}
	event.hydratePayloadEnvelope()
	return event
}
