package execution

import (
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// SMTP can wait up to ten minutes for the final DATA acknowledgement. Its
// dispatch lease must cover that same provider deadline, otherwise recovery
// could race a send which is still legitimately awaiting acknowledgement.
const gmailSendInvocationTimeout = 12 * time.Minute

func (service *Service) invocationTimeout(connection core.PersonalConnection, operationID core.Identifier) time.Duration {
	if connection.ConnectorID == "wecom" && connection.ProviderID == "wecom" && connection.DriverID == "wecom-http-v1" && connection.AuthMethodID == "wecom.credentials" && operationID == "wecom.documents.get" && service.providerTimeout == defaultProviderInvocationTimeout {
		return 150 * time.Second
	}
	_, mail := catalog.MatchMailProfile(connection.ConnectorID.String(), connection.ProviderID.String(), connection.DriverID.String(), connection.AuthMethodID.String())
	if mail && operationID.String() == connection.ConnectorID.String()+".message.send" && service.providerTimeout == defaultProviderInvocationTimeout {
		return gmailSendInvocationTimeout
	}
	return service.providerTimeout
}
