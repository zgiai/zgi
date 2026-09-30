package connections

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// Derive the maintenance verification plan from the enforced operation set,
// not the informational evidence supplied for display.
func mailUsage(c core.PersonalConnection) string {
	if !isMailConnection(c) {
		return ""
	}
	read, send := false, false
	for _, op := range c.UsableOperationIDs {
		switch op.String() {
		case c.ConnectorID.String() + ".folder.list", c.ConnectorID.String() + ".message.list", c.ConnectorID.String() + ".message.get":
			read = true
		case c.ConnectorID.String() + ".message.send":
			send = true
		}
	}
	if read && send {
		return "read_send"
	}
	if read {
		return "read"
	}
	if send {
		return "send"
	}
	return ""
}
func mailOperationIDs(c core.PersonalConnection) []string {
	return catalog.MailOperationsFor(c.ConnectorID.String(), mailUsage(c))
}

func validateMailVerification(d catalog.Definition, usage, login string, v CandidateVerificationResult) error {
	if !d.IsMailCredentials() {
		return nil
	}
	wanted := catalog.MailOperationsFor(d.ID, usage)
	seen := map[string]bool{}
	for _, id := range v.UsableOperationIDs {
		if seen[id] {
			return core.NewError(core.ErrorCodeInvalidInput, false, nil)
		}
		seen[id] = true
	}
	if len(wanted) == 0 || len(seen) != len(wanted) || v.ExternalAccountID != login {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	for _, id := range wanted {
		if !seen[id] {
			return core.NewError(core.ErrorCodeInvalidInput, false, nil)
		}
	}
	if v.Evidence == nil || v.Evidence.Mail == nil || v.Evidence.Mail.Provider != d.ID || v.Evidence.Mail.Usage != usage || !v.Evidence.Mail.CredentialVerified || v.Evidence.Mail.AuthenticatedLogin != login {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return nil
}

func isMailConnection(c core.PersonalConnection) bool {
	_, ok := catalog.MatchMailProfile(c.ConnectorID.String(), c.ProviderID.String(), c.DriverID.String(), c.AuthMethodID.String())
	return ok
}
