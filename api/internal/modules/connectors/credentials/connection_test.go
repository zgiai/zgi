package credentials

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const resolverTestMasterKey = "12345678901234567890123456789012"

type personalConnectionReaderStub struct {
	connection connectorcore.PersonalConnection
	err        error
	calls      int
	callOrg    uuid.UUID
	callOwner  uuid.UUID
	callConn   uuid.UUID
}

func (stub *personalConnectionReaderStub) GetConnection(
	_ context.Context,
	organizationID, ownerAccountID, connectionID uuid.UUID,
) (connectorcore.PersonalConnection, error) {
	stub.calls++
	stub.callOrg, stub.callOwner, stub.callConn = organizationID, ownerAccountID, connectionID
	return stub.connection, stub.err
}

type resolverFixture struct {
	resolver *PersonalConnectionResolver
	reader   *personalConnectionReaderStub
	org      uuid.UUID
	owner    uuid.UUID
	conn     uuid.UUID
}

func newResolverFixture(t *testing.T, secret string) *resolverFixture {
	t.Helper()
	store, err := NewStore("test", map[string]string{"test": resolverTestMasterKey})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	org, owner, conn := uuid.New(), uuid.New(), uuid.New()
	envelope, err := store.Seal(connections.CredentialSealRequest{
		OrganizationID: org, ConnectionID: conn, ConnectorID: "feishu", CredentialVersion: 3,
		Values: map[string]string{"app_id": "safe-id", "app_secret": secret},
	})
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}
	connection := connectorcore.PersonalConnection{
		ID: conn, OrganizationID: org, OwnerAccountID: owner,
		ConnectorID: "feishu", ProviderID: "feishu", DriverID: "feishu-open-api", AuthMethodID: "feishu_tenant_app",
		Name: "Feishu", EncryptedCredentials: envelope, CredentialVersion: 3,
		State: connectorcore.ReadinessReady, Revision: 7,
	}
	reader := &personalConnectionReaderStub{connection: connection}
	return &resolverFixture{
		resolver: NewPersonalConnectionResolver(reader, store),
		reader:   reader,
		org:      org,
		owner:    owner,
		conn:     conn,
	}
}

func (fixture *resolverFixture) resolve(ctx context.Context) (connectorcore.PersonalConnection, map[string]string, error) {
	return fixture.resolver.ResolvePersonalConnectionCredentials(ctx, fixture.org, fixture.owner, fixture.conn)
}

func TestPersonalConnectionResolverReadsFullRecordAndDecrypts(t *testing.T) {
	fixture := newResolverFixture(t, "secret-value")
	connection, values, err := fixture.resolve(t.Context())
	if err != nil {
		t.Fatalf("ResolvePersonalConnectionCredentials() error = %v", err)
	}
	if connection.ID != fixture.conn || connection.EncryptedCredentials == "" {
		t.Fatalf("resolved connection = %#v, want full owner-scoped record", connection)
	}
	if values["app_id"] != "safe-id" || values["app_secret"] != "secret-value" {
		t.Fatalf("resolved values = %#v", values)
	}
	if fixture.reader.calls != 1 || fixture.reader.callOrg != fixture.org ||
		fixture.reader.callOwner != fixture.owner || fixture.reader.callConn != fixture.conn {
		t.Fatalf("reader scope = %d/%s/%s/%s", fixture.reader.calls, fixture.reader.callOrg, fixture.reader.callOwner, fixture.reader.callConn)
	}
	for key := range values {
		values[key] = ""
		delete(values, key)
	}
}

func TestPersonalConnectionResolverFailsClosedOnUnusableRecord(t *testing.T) {
	deletedAt := time.Now()
	cases := []struct {
		name   string
		mutate func(*resolverFixture)
	}{
		{"foreign organization", func(f *resolverFixture) { f.reader.connection.OrganizationID = uuid.New() }},
		{"foreign owner", func(f *resolverFixture) { f.reader.connection.OwnerAccountID = uuid.New() }},
		{"foreign connection", func(f *resolverFixture) { f.reader.connection.ID = uuid.New() }},
		{"soft deleted", func(f *resolverFixture) { f.reader.connection.DeletedAt = &deletedAt }},
		{"reconnect required", func(f *resolverFixture) { f.reader.connection.State = connectorcore.ReadinessReconnectRequired }},
		{"unavailable", func(f *resolverFixture) { f.reader.connection.State = connectorcore.ReadinessUnavailable }},
		{"zero revision", func(f *resolverFixture) { f.reader.connection.Revision = 0 }},
		{"zero credential version", func(f *resolverFixture) { f.reader.connection.CredentialVersion = 0 }},
		{"missing encrypted credentials", func(f *resolverFixture) { f.reader.connection.EncryptedCredentials = "" }},
		{"stale credential version", func(f *resolverFixture) { f.reader.connection.CredentialVersion = 4 }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newResolverFixture(t, "resolver-secret")
			testCase.mutate(fixture)
			connection, values, err := fixture.resolve(t.Context())
			if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConnectionNotReady) {
				t.Fatalf("error = %v, want %s", err, connectorcore.ErrorCodeConnectionNotReady)
			}
			if connection.ID != uuid.Nil || values != nil {
				t.Fatalf("failure leaked connection/values: %#v/%#v", connection, values)
			}
		})
	}
}

func TestPersonalConnectionResolverPropagatesReaderFailure(t *testing.T) {
	sentinel := errors.New("repository unavailable")
	fixture := newResolverFixture(t, "resolver-secret")
	fixture.reader.err = sentinel
	connection, values, err := fixture.resolve(t.Context())
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want propagated reader failure", err)
	}
	if connection.ID != uuid.Nil || values != nil {
		t.Fatalf("reader failure leaked connection/values: %#v/%#v", connection, values)
	}
}

func TestPersonalConnectionResolverPropagatesContextCancellation(t *testing.T) {
	fixture := newResolverFixture(t, "resolver-secret")
	fixture.reader.err = context.Canceled
	if _, _, err := fixture.resolve(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestPersonalConnectionResolverRejectsInvalidScope(t *testing.T) {
	fixture := newResolverFixture(t, "resolver-secret")
	for name, ids := range map[string][3]uuid.UUID{
		"nil organization": {uuid.Nil, fixture.owner, fixture.conn},
		"nil owner":        {fixture.org, uuid.Nil, fixture.conn},
		"nil connection":   {fixture.org, fixture.owner, uuid.Nil},
	} {
		t.Run(name, func(t *testing.T) {
			connection, values, err := fixture.resolver.ResolvePersonalConnectionCredentials(t.Context(), ids[0], ids[1], ids[2])
			if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidInput) {
				t.Fatalf("error = %v, want %s", err, connectorcore.ErrorCodeInvalidInput)
			}
			if connection.ID != uuid.Nil || values != nil {
				t.Fatalf("invalid input leaked connection/values: %#v/%#v", connection, values)
			}
		})
	}
	var nilResolver *PersonalConnectionResolver
	if _, _, err := nilResolver.ResolvePersonalConnectionCredentials(t.Context(), fixture.org, fixture.owner, fixture.conn); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidInput) {
		t.Fatalf("nil resolver error = %v, want %s", err, connectorcore.ErrorCodeInvalidInput)
	}
}

func TestPersonalConnectionResolverDoesNotLeakCredentials(t *testing.T) {
	const secret = "super-secret-app-password"
	fixture := newResolverFixture(t, secret)
	// Pass the record gates, then fail inside the keyring so the error path is
	// exercised with a real sealed secret in the fixture.
	fixture.reader.connection.CredentialVersion = 4
	connection, values, err := fixture.resolve(t.Context())
	if err == nil {
		t.Fatal("expected resolution failure")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error text leaked a credential: %v", err)
	}
	if connection.ID != uuid.Nil || values != nil {
		t.Fatalf("failure leaked connection/values: %#v/%#v", connection, values)
	}
}

func TestNewPersonalConnectionResolverRequiresDependencies(t *testing.T) {
	store, err := NewStore("test", map[string]string{"test": resolverTestMasterKey})
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	for name, build := range map[string]func(){
		"nil reader": func() { NewPersonalConnectionResolver(nil, store) },
		"nil store":  func() { NewPersonalConnectionResolver(&personalConnectionReaderStub{}, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected constructor to panic")
				}
			}()
			build()
		})
	}
}

type refreshingConnectionStub struct{ calls int }

func (s *refreshingConnectionStub) ResolveConnectionCredentials(_ context.Context, c connectorcore.PersonalConnection) (map[string]string, error) {
	s.calls++
	return map[string]string{"token": "renewed"}, nil
}
func TestPersonalConnectionResolverAcceptsRenewalBoundary(t *testing.T) {
	f := newResolverFixture(t, "old")
	refresh := &refreshingConnectionStub{}
	resolver := NewPersonalConnectionResolver(f.reader, refresh)
	_, values, err := resolver.ResolvePersonalConnectionCredentials(t.Context(), f.org, f.owner, f.conn)
	if err != nil || refresh.calls != 1 || values["token"] != "renewed" {
		t.Fatal("renewal resolver was bypassed")
	}
}
