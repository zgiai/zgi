package connections

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"testing"
)

func TestMessagingReconnectRejectsDifferentIdentityBeforeVerification(t *testing.T) {
	service, repo, _, verifier, _, _ := newMaintenanceFixture(t)
	d, _ := catalog.MessagingDefinition("dingtalk")
	service.catalog, _ = catalog.New(d)
	existing := maintenanceConnection(service.now(), uuid.New(), uuid.New())
	existing.ConnectorID = "dingtalk"
	identity := "dingtalk:app:original"
	existing.ExternalAccountID = &identity
	repo.connections[existing.ID] = existing
	values := map[string]string{"client_id": "another", "client_secret": "private-secret"}
	_, err := service.Reconnect(context.Background(), ReconnectCommand{OrganizationID: existing.OrganizationID, OwnerAccountID: existing.OwnerAccountID, ConnectionID: existing.ID, Values: values, IdempotencyKey: uuid.NewString()})
	if core.ErrorCodeOf(err) != core.ErrorCodeConflict || verifier.calls != 0 || repo.connections[existing.ID].EncryptedCredentials != existing.EncryptedCredentials || len(values) != 0 {
		t.Fatal("reauthorization changed identity")
	}
}
