package service

import "github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"

// publicExternalActionPayload keeps the authoritative frozen invocation in
// server persistence while returning a detached, bounded argument preview to
// browsers. Call this only at a public response/stream boundary.
func publicExternalActionPayload(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return nil
	}
	sanitized, ok := toolgovernance.SanitizeExternalActionPublicValue(payload).(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return sanitized
}

func publicExternalActionStreamEvent(event StreamEvent) StreamEvent {
	switch event.EventType {
	case streamEventToolGovernanceDecision,
		streamEventSkillCallStart,
		streamEventSkillCallEnd,
		streamEventSkillCallError,
		streamEventMessageEnd:
		event.Payload = publicExternalActionPayload(event.Payload)
	}
	event.hydratePayloadEnvelope()
	return event
}
