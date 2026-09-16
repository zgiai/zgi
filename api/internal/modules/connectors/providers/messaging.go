package providers

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func (entry registration) safeMessagingEvidence(value *core.ProviderEvidence, input CredentialInput) *core.ProviderEvidence {
	e := value.Messaging
	d := entry.definition
	if e == nil || e.Version != 1 || e.Provider != d.ID || !e.CredentialVerified || e.Identity != catalog.CredentialIdentity(d, input.Values) {
		return nil
	}
	result := &core.ProviderEvidence{ScopeStatus: "unknown", EventConfiguration: "unsupported", GrantedScopes: []string{}, Messaging: &core.MessagingEvidence{Version: 1, Provider: d.ID, Subject: d.Operations[0].CredentialSubject, Identity: entry.redactText(e.Identity, input), CredentialVerified: true, Operations: []core.OperationEvidence{}}}
	seen := map[string]bool{}
	denied := map[string]bool{}
	for _, op := range e.Operations {
		if op.Availability == "denied" || op.Permission == "denied" {
			denied[op.ID] = true
		}
	}
	for _, op := range d.Operations {
		for _, observed := range e.Operations {
			if observed.ID != op.ID || seen[op.ID] {
				continue
			}
			availability := observed.Availability
			if denied[op.ID] {
				availability = "denied"
			}
			if availability != "verified" && availability != "denied" && availability != "conditional" && availability != "context_required" {
				continue
			}
			if availability == "context_required" && (d.ID == "dingtalk" || op.Effect != catalog.OperationEffectExternalSend) {
				continue
			}
			// Conditional app-token admission is restricted to the frozen DingTalk
			// v3 contract. Other providers need a successful business read.
			wecomRead := d.ID == "wecom" && d.PluginVersion == "1.1.0" && catalog.IsWeComResourceRead(op.ID)
			if availability == "conditional" && !wecomRead && (d.ID != "dingtalk" || d.Setup.VerificationPolicy != "credential_and_conditional_operations") {
				continue
			}
			permission := "unknown"
			if availability == "denied" {
				permission = "denied"
			}
			result.Messaging.Operations = append(result.Messaging.Operations, core.OperationEvidence{ID: op.ID, Availability: availability, Permission: permission, Resource: "evaluated_at_execution"})
			seen[op.ID] = true
		}
	}
	return result
}
