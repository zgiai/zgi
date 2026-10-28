package container

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/automation/serverwatch"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type watchAccess struct{ allowed bool }

func (a watchAccess) OwnerHasAccess(context.Context, serverwatch.Target) (bool, error) {
	return a.allowed, nil
}

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
		access:   watchAccess{allowed: true},
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

func TestServerWatchProbeStopsWhenTheOwnerLostAccess(t *testing.T) {
	bindings := &watchBindings{}
	probe := &ServerWatchProbe{access: watchAccess{allowed: false}, bindings: bindings, employees: watchEmployees{published: uuid.New()}, connections: watchConnections{}}
	if err := probe.Usable(t.Context(), serverwatch.Target{OwnerAccountID: uuid.New()}); !errors.Is(err, serverwatch.ErrNotUsable) {
		t.Fatalf("Usable() = %v, want ErrNotUsable", err)
	}
	if bindings.options.Scope != "" {
		t.Fatal("bindings were read for an owner without access")
	}
}

func TestServerWatchOwnerAccessFollowsMembership(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:server-watch-access-%s?mode=memory&cache=shared", uuid.NewString())), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE accounts (id TEXT PRIMARY KEY, status TEXT, deleted_at DATETIME)",
		"CREATE TABLE workspace_members (account_id TEXT, workspace_id TEXT)",
		"CREATE TABLE workspaces (id TEXT PRIMARY KEY, organization_id TEXT, status TEXT)",
		"CREATE TABLE members (organization_id TEXT, account_id TEXT, status TEXT)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	target := serverwatch.Target{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), OwnerAccountID: uuid.New()}
	for _, statement := range []string{
		fmt.Sprintf("INSERT INTO accounts VALUES ('%s', 'active', NULL)", target.OwnerAccountID),
		fmt.Sprintf("INSERT INTO workspace_members VALUES ('%s', '%s')", target.OwnerAccountID, target.WorkspaceID),
		fmt.Sprintf("INSERT INTO workspaces VALUES ('%s', '%s', 'normal')", target.WorkspaceID, target.OrganizationID),
		fmt.Sprintf("INSERT INTO members VALUES ('%s', '%s', 'active')", target.OrganizationID, target.OwnerAccountID),
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	access := gormServerWatchOwnerAccess{db: db}
	check := func(want bool, step string) {
		t.Helper()
		got, err := access.OwnerHasAccess(t.Context(), target)
		if err != nil || got != want {
			t.Fatalf("%s: OwnerHasAccess() = %v, %v, want %v", step, got, err, want)
		}
	}
	check(true, "active member")
	db.Exec("UPDATE members SET status = 'inactive'")
	check(false, "disabled member")
	db.Exec("UPDATE members SET status = 'active'")
	db.Exec("DELETE FROM workspace_members")
	check(false, "removed from the workspace")
	db.Exec(fmt.Sprintf("INSERT INTO workspace_members VALUES ('%s', '%s')", target.OwnerAccountID, target.WorkspaceID))
	db.Exec("UPDATE accounts SET deleted_at = CURRENT_TIMESTAMP")
	check(false, "deleted account")
}
