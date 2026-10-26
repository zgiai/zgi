package migrations

import (
	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"strings"
	"testing"
)

func TestConnectorSocialOAuthMigrationSchemaAndRollback(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	for range 6 {
		mock.ExpectExec("(?s).*").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	b := mschema.New(db)
	if e := upCreateConnectorSocialOAuth(b); e != nil {
		t.Fatal(e)
	}
	sql := strings.Join(b.Statements(), "\n")
	for _, want := range []string{`"connector_social_oauth_flows"`, `"connector_social_oauth_grants"`, `"state_hash" varchar(64) NOT NULL`, `"envelope" text NOT NULL`, `"credential_version" integer NOT NULL`, `"credential_digest" varchar(64) NOT NULL`, `CREATE UNIQUE INDEX "idx_social_oauth_state"`, `CREATE UNIQUE INDEX "idx_social_oauth_grant_connection"`} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %s in %s", want, sql)
		}
	}
	for _, forbidden := range []string{`"access_token"`, `"refresh_token"`, `"code_verifier"`} {
		if strings.Contains(sql, forbidden) {
			t.Fatal("plaintext secret column", forbidden)
		}
	}
	mock.ExpectExec(`DROP TABLE IF EXISTS "public"\."connector_social_oauth_grants"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DROP TABLE IF EXISTS "public"\."connector_social_oauth_flows"`).WillReturnResult(sqlmock.NewResult(0, 0))
	if e := downCreateConnectorSocialOAuth(mschema.New(db).AllowDestructive()); e != nil {
		t.Fatal(e)
	}
	if e := mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}
