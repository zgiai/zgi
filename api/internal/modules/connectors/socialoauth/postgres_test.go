package socialoauth

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestPostgresResolverLocksCurrentConnectionAndGrant(t *testing.T) {
	s, org, owner := fixture(t)
	sqlDB, mock, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	defer sqlDB.Close()
	db, e := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
	if e != nil {
		t.Fatal(e)
	}
	s.db = db
	c := connectionRow{ID: uuid.New(), OrganizationID: org, OwnerAccountID: owner, ConnectorID: "x-social", CredentialVersion: 1, State: "ready"}
	c.EncryptedCredentials, _ = s.vault.Seal(connections.CredentialSealRequest{OrganizationID: org, ConnectionID: c.ID, ConnectorID: c.ConnectorID, CredentialVersion: 1, Values: map[string]string{"token": "current"}})
	g := grantRecord{ID: uuid.New(), OrganizationID: org, ConnectionID: c.ID, ConnectorID: c.ConnectorID, CredentialVersion: 1, CredentialDigest: digest(c.EncryptedCredentials)}
	g.Envelope, _ = s.seal(grantSubject(g), tokenGrant{AccessToken: "current", ExpiresAt: time.Now().Add(time.Hour)})
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "connector_connections" WHERE .* FOR UPDATE`).WithArgs(c.ID, org, c.ConnectorID, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "owner_account_id", "connector_id", "credential_version", "encrypted_credentials", "state"}).AddRow(c.ID, org, owner, c.ConnectorID, 1, c.EncryptedCredentials, "ready"))
	mock.ExpectQuery(`SELECT \* FROM "connector_social_oauth_grants" WHERE .* FOR UPDATE`).WithArgs(c.ID, org, c.ConnectorID, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "connection_id", "connector_id", "credential_version", "credential_digest", "envelope"}).AddRow(g.ID, org, c.ID, c.ConnectorID, 1, g.CredentialDigest, g.Envelope))
	mock.ExpectCommit()
	v, e := s.ResolveCredentials(context.Background(), currentRequest(c))
	if e != nil || v["token"] != "current" {
		t.Fatal(v, e)
	}
	if e = mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
