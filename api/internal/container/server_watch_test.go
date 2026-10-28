package container

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/automation/serverwatch"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

type watchBindings struct {
	connectorbindings.Repository
	options connectorbindings.ListOptions
	items   []connectorcore.TargetBinding
}

func (b *watchBindings) ListBindingsByTarget(_ context.Context, _, _ uuid.UUID, _ connectorcore.TargetType, _ string, options connectorbindings.ListOptions) ([]connectorcore.TargetBinding, error) {
	b.options = options
	return b.items, nil
}

type watchEmployees struct{ published uuid.UUID }

func (e watchEmployees) Get(context.Context, digitalhuman.Scope, uuid.UUID) (*digitalhuman.Detail, error) {
	return &digitalhuman.Detail{Definition: &digitalhuman.Definition{Status: digitalhuman.StatusActive, PublishedVersionID: &e.published}}, nil
}

type watchConnections struct {
	connection connectorcore.PersonalConnection
}

func (c watchConnections) GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.PersonalConnection, error) {
	return c.connection, nil
}

func TestServerWatchProbeNeedsABindingOfThePublishedVersion(t *testing.T) {
	owner, connection, published := uuid.New(), uuid.New(), uuid.New()
	target := serverwatch.Target{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerAccountID: owner, DigitalHumanID: uuid.New(), ConnectionID: connection}
	bindings := &watchBindings{items: []connectorcore.TargetBinding{{
		ConnectionID: connection, OwnerAccountID: owner, Status: connectorcore.BindingStatusActive, ConnectorID: "serverops_ssh",
	}}}
	probe := &ServerWatchProbe{
		bindings: bindings, employees: watchEmployees{published: published},
		connections: watchConnections{connection: connectorcore.PersonalConnection{ID: connection, OwnerAccountID: owner, ConnectorID: "serverops_ssh", State: connectorcore.ReadinessReady}},
	}
	if err := probe.Usable(t.Context(), target); err != nil {
		t.Fatalf("Usable() error = %v", err)
	}
	if bindings.options.Scope != connectorcore.ScopePublished || bindings.options.VersionID != published.String() {
		t.Fatalf("bindings listed with %+v, want the published version", bindings.options)
	}

	bindings.items[0].ConnectionID = uuid.New()
	if err := probe.Usable(t.Context(), target); !errors.Is(err, serverwatch.ErrNotUsable) {
		t.Fatalf("Usable() without a binding on the connection = %v", err)
	}
}
