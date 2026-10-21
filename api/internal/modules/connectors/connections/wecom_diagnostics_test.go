package connections

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestDiagnosticsDoNotAcceptNewWeComOperations(t *testing.T) {
	s, repo, _, verifier, _, _ := newMaintenanceFixture(t)
	definition := catalog.WeComReadDefinition()
	s.catalog, _ = catalog.New(definition)
	c := maintenanceConnection(s.now(), uuid.New(), uuid.New())
	c.ConnectorID, c.ProviderID, c.DriverID, c.AuthMethodID = "wecom", "wecom", "wecom-http-v1", "wecom.credentials"
	identity := "wecom:bot:fixture"
	c.ExternalAccountID = &identity
	c.UsableOperationIDs = []core.Identifier{"wecom.identity.get", "wecom.sessions.list", "wecom.messages.send"}
	c.UnavailableOperations = map[core.Identifier]string{}
	c.Constraint = core.OwnerConstraint(c.OwnerAccountID, c.UsableOperationIDs)
	repo.connections[c.ID] = c
	verifier.result.ExternalAccountID = identity
	verifier.result.UsableOperationIDs = []string{}
	for _, op := range definition.Operations {
		verifier.result.UsableOperationIDs = append(verifier.result.UsableOperationIDs, op.ID)
	}
	for i := 0; i < 2; i++ {
		result, err := s.Diagnose(context.Background(), c.OrganizationID, c.OwnerAccountID, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.UsableOperationIDs) != 3 || slices.Contains(result.UsableOperationIDs, "wecom.documents.search") || result.UnavailableOperations["wecom.documents.search"] != "" {
			t.Fatal("diagnostics expanded accepted operations")
		}
	}
}
