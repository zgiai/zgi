package providers

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func (entry registration) safeMailEvidence(value *core.ProviderEvidence, input CredentialInput) *core.ProviderEvidence {
	if value == nil || value.Mail == nil {
		return nil
	}
	m := value.Mail
	if m.Version != 1 || !m.CredentialVerified || m.Provider != entry.definition.ID || m.AuthenticatedLogin != input.Values["email"] || m.IdentityKind != "authenticated_login" || m.Usage != input.MailUsage || m.CheckedAt.IsZero() {
		return nil
	}
	result := &core.MailEvidence{Version: 1, Provider: entry.definition.ID, IdentityKind: "authenticated_login", AuthenticatedLogin: input.Values["email"], Usage: input.MailUsage, ScopeGranularity: "not_available", IMAPStatus: "not_requested", SMTPStatus: "not_requested", CheckedAt: m.CheckedAt, Operations: []core.OperationEvidence{}}
	read := input.MailUsage == "read" || input.MailUsage == "read_send"
	send := input.MailUsage == "send" || input.MailUsage == "read_send"
	if read {
		result.IMAPStatus = m.IMAPStatus
		if m.IMAPStatus != "verified" {
			return nil
		}
	}
	if send {
		result.SMTPStatus = m.SMTPStatus
		if m.SMTPStatus != "verified" {
			return nil
		}
	}
	result.CredentialVerified = true
	for _, id := range catalog.MailOperationsFor(entry.definition.ID, input.MailUsage) {
		availability := "verified"
		if id == entry.definition.ID+".message.send" {
			availability = "conditional"
		}
		result.Operations = append(result.Operations, core.OperationEvidence{ID: id, Availability: availability, Permission: "not_scoped", Resource: "evaluated_at_execution"})
	}
	return &core.ProviderEvidence{Mail: result, GrantedScopes: []string{}, ScopeStatus: "not_available"}
}
