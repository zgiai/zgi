package providers

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// safeServerOpsEvidence keeps only the non-secret SSH evidence an approval
// preview needs. The privilege is taken from the validated credential values,
// never from the driver, and evidence whose identity is missing or carries a
// different privilege is dropped, so a preview can never claim a privilege the
// stored credential does not have. The identity is redacted exactly like the
// connection's external account id, so the two always agree.
func (entry registration) safeServerOpsEvidence(value *core.ProviderEvidence, input CredentialInput) *core.ProviderEvidence {
	if value == nil || value.ServerOps == nil || value.ServerOps.Version != 1 || value.ServerOps.Identity == "" {
		return nil
	}
	privilege := input.Values["privilege"]
	if value.ServerOps.Privilege != privilege || (privilege != "" && privilege != catalog.ServerOpsSSHPrivilegeSudo) {
		return nil
	}
	return &core.ProviderEvidence{
		GrantedScopes: []string{},
		ServerOps: &core.ServerOpsEvidence{
			Version: 1, Identity: entry.redactText(value.ServerOps.Identity, input), Privilege: privilege,
		},
	}
}
