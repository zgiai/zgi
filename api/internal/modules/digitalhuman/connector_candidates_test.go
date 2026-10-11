package digitalhuman

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type connectorCandidateCatalogStub struct {
	definitions map[string]catalog.Definition
}

func (stub connectorCandidateCatalogStub) Get(id string) (catalog.Definition, bool) {
	definition, ok := stub.definitions[id]
	return definition, ok
}

const connectorCandidateConnectionSchema = `
CREATE TABLE connector_connections (
	connection_constraint text,
	id text PRIMARY KEY,
	organization_id text NOT NULL,
	owner_account_id text NOT NULL,
	connector_id text NOT NULL,
	provider_id text NOT NULL,
	driver_id text NOT NULL,
	auth_method_id text NOT NULL,
	name text NOT NULL,
	external_account_id text,
	external_display_name text,
	encrypted_credentials text NOT NULL,
	credential_version integer NOT NULL,
	state text NOT NULL,
	usable_operation_ids text NOT NULL,
	unavailable_operations text NOT NULL,
	diagnostic_code text,
	revision integer NOT NULL,
	last_verified_at datetime NOT NULL,
	last_provider_success_at datetime,
	last_provider_failure_at datetime,
	created_at datetime,
	updated_at datetime,
	deleted_at datetime
)`

// newConnectorCandidateFixture seeds the two catalog surfaces the projection
// must distinguish: a Digital Human contract and a chat-only contract.
func newConnectorCandidateFixture(t *testing.T) (*GormRepository, *Service, Scope, catalog.Definition, catalog.Definition) {
	t.Helper()
	service, repo := newTestService(t)
	if err := repo.db.Exec(connectorCandidateConnectionSchema).Error; err != nil {
		t.Fatal(err)
	}
	digitalHumanDefinition := catalog.Definition{
		ID: "test.digitalhuman", Description: "Digital Human test connector",
		Surfaces:   []catalog.Surface{catalog.SurfaceDigitalHuman},
		Operations: []catalog.Operation{{ID: "search"}, {ID: "write"}},
	}
	chatDefinition := catalog.Definition{
		ID: "test.chat", Description: "Chat-only test connector",
		Surfaces:   []catalog.Surface{catalog.SurfaceChat},
		Operations: []catalog.Operation{{ID: "search"}},
	}
	repo.SetConnectorCatalog(connectorCandidateCatalogStub{definitions: map[string]catalog.Definition{
		digitalHumanDefinition.ID: digitalHumanDefinition,
		chatDefinition.ID:         chatDefinition,
	}})
	return repo, service, testScope(), digitalHumanDefinition, chatDefinition
}

func insertConnectorCandidateConnection(
	t *testing.T,
	repo *GormRepository,
	scope Scope,
	definition catalog.Definition,
	owner uuid.UUID,
	name string,
	state core.Readiness,
	usable, allowed []core.Identifier,
) core.PersonalConnection {
	t.Helper()
	now := time.Now().UTC()
	connection := core.PersonalConnection{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, OwnerAccountID: owner,
		ConnectorID: core.Identifier(definition.ID), ProviderID: "test.provider",
		DriverID: "test.driver", AuthMethodID: "test.auth_method", Name: name,
		EncryptedCredentials: "test-envelope-candidate-secret", CredentialVersion: 1,
		State: state, Revision: 1, UsableOperationIDs: usable, UnavailableOperations: map[core.Identifier]string{},
		Constraint: core.OwnerConstraint(owner, allowed), LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := connections.NewGormRepository(repo.db).CreateConnection(context.Background(), connection); err != nil {
		t.Fatal(err)
	}
	return connection
}

func TestConnectorCandidateProjectionIsOwnerScopedAndCredentialFree(t *testing.T) {
	repo, service, scope, digitalHumanDefinition, chatDefinition := newConnectorCandidateFixture(t)
	detail := createPublishableDigitalHuman(t, service, scope, "Connector candidates")
	ownerReady := insertConnectorCandidateConnection(t, repo, scope, digitalHumanDefinition, scope.ActorID, "Owner ready", core.ReadinessReady, []core.Identifier{"search", "write"}, []core.Identifier{"search"})
	insertConnectorCandidateConnection(t, repo, scope, digitalHumanDefinition, scope.ActorID, "Reconnect required", core.ReadinessReconnectRequired, []core.Identifier{"search"}, []core.Identifier{"search"})
	insertConnectorCandidateConnection(t, repo, scope, digitalHumanDefinition, uuid.New(), "Foreign owner", core.ReadinessReady, []core.Identifier{"search"}, []core.Identifier{"search"})
	insertConnectorCandidateConnection(t, repo, scope, digitalHumanDefinition, scope.ActorID, "No usable operation", core.ReadinessReady, []core.Identifier{"search"}, []core.Identifier{"write"})
	insertConnectorCandidateConnection(t, repo, scope, chatDefinition, scope.ActorID, "Chat-only connector", core.ReadinessReady, []core.Identifier{"search"}, []core.Identifier{"search"})

	preCreation, err := repo.ListConnectorCandidates(t.Context(), scope)
	if err != nil {
		t.Fatalf("ListConnectorCandidates() error = %v", err)
	}
	if len(preCreation) != 3 {
		t.Fatalf("pre-creation connector candidates = %#v, want owner-ready, unready, and constrained", preCreation)
	}
	byName := make(map[string]ResourceCandidate, len(preCreation))
	for _, candidate := range preCreation {
		byName[candidate.DisplayName] = candidate
	}
	ready, ok := byName["Owner ready"]
	if !ok {
		t.Fatalf("owner-ready candidate missing: %#v", preCreation)
	}
	if ready.ResourceID != ownerReady.ID.String() || ready.ParentResourceID != digitalHumanDefinition.ID ||
		!ready.Available || ready.Status != string(core.ReadinessReady) {
		t.Fatalf("owner-ready candidate = %#v", ready)
	}
	operations, ok := ready.Metadata["allowed_operation_ids"].([]core.Identifier)
	if !ok || len(operations) != 1 || operations[0] != "search" {
		t.Fatalf("owner-ready operations = %#v, want only the constrained operation", ready.Metadata["allowed_operation_ids"])
	}
	if constrained, ok := byName["No usable operation"]; !ok || constrained.Available || len(constrained.Metadata["allowed_operation_ids"].([]core.Identifier)) != 0 {
		t.Fatalf("constrained candidate = %#v, want present with no allowed operation", constrained)
	}
	if unready, ok := byName["Reconnect required"]; !ok || unready.Available || unready.Status != string(core.ReadinessReconnectRequired) ||
		len(unready.Metadata["allowed_operation_ids"].([]core.Identifier)) != 1 {
		t.Fatalf("unready candidate = %#v, want present and unavailable", unready)
	}
	if _, ok := byName["Foreign owner"]; ok {
		t.Fatal("foreign-owner connection leaked into the caller's candidates")
	}
	if _, ok := byName["Chat-only connector"]; ok {
		t.Fatal("non-Digital-Human connector leaked into the candidates")
	}

	raw, err := json.Marshal(preCreation)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("test-envelope-candidate-secret")) {
		t.Fatalf("candidate projection exposed credential material: %s", raw)
	}

	draft, err := repo.ListResourceCandidates(t.Context(), scope, detail.Definition.ID, ResourceCandidateQuery{Type: resourcebindings.BindingTypeConnector})
	if err != nil {
		t.Fatalf("ListResourceCandidates() error = %v", err)
	}
	if !reflect.DeepEqual(draft, preCreation) {
		t.Fatalf("draft and pre-creation projections diverged: draft=%#v pre-creation=%#v", draft, preCreation)
	}
}
