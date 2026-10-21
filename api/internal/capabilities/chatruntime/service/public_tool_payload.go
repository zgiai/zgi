package service

import (
	"encoding/json"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

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
	restoreInlineApprovalCredentials(payload, sanitized)
	return sanitized
}

// restoreInlineApprovalCredentials puts approval tokens back into a sanitized
// payload, but only for approval records the viewer is explicitly allowed to
// answer inline. The generic sanitizer redacts every token-shaped field, which
// would otherwise replace the one-time approval token in the final message
// state with a placeholder and make the inline approval card unusable. Records
// that are not inline-approvable (ui_approval_allowed is not true) stay
// redacted.
func restoreInlineApprovalCredentials(original, sanitized interface{}) {
	switch typed := original.(type) {
	case map[string]interface{}:
		target, ok := sanitized.(map[string]interface{})
		if !ok {
			return
		}
		if allowed, _ := typed["ui_approval_allowed"].(bool); allowed {
			if token, _ := typed["approval_token"].(string); token != "" {
				if _, present := target["approval_token"]; present {
					target["approval_token"] = token
				}
			}
			// The approval form is the definition the viewer must fill in. Its
			// field "key" names would otherwise be redacted as key-like values,
			// so the submitted inputs would no longer match the form.
			if form := typed["approval_form"]; form != nil {
				if copied := cloneApprovalForm(form); copied != nil {
					target["approval_form"] = copied
				}
			}
		}
		for key, child := range typed {
			restoreInlineApprovalCredentials(child, target[key])
		}
	case []interface{}:
		target, ok := sanitized.([]interface{})
		if !ok {
			return
		}
		for index, child := range typed {
			if index >= len(target) {
				return
			}
			restoreInlineApprovalCredentials(child, target[index])
		}
	}
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

// cloneApprovalForm copies the form through JSON so it works whatever concrete
// map or struct type the in-memory metadata holds.
func cloneApprovalForm(form interface{}) map[string]interface{} {
	encoded, err := json.Marshal(form)
	if err != nil {
		return nil
	}
	var copied map[string]interface{}
	if json.Unmarshal(encoded, &copied) != nil {
		return nil
	}
	return copied
}
