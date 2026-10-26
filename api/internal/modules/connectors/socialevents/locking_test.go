package socialevents

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPostgresReceiveLocksCurrentPageMappings(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer rawDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: rawDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	vault, _ := credentials.NewStore("test", map[string]string{"test": strings.Repeat("k", 32)})
	s, _ := New(db, vault, Options{FacebookAppSecret: "secret", FacebookVerifyToken: "verify"})
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM "connector_connections" WHERE connector_id = \$1 AND external_account_id IN \(\$2\) AND state = \$3 AND deleted_at IS NULL ORDER BY id ASC LIMIT \$4 FOR UPDATE`).WithArgs("facebook-social", "page1", "ready", 101).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectCommit()
	deliver(t, s, `{"object":"page","entry":[{"id":"page1"}]}`)
	mock.ExpectBegin()
	org, owner, conn := uuid.New(), uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT .* FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4 AND deleted_at IS NULL LIMIT \$5 FOR UPDATE`).WithArgs(conn, org, owner, "facebook-social", 1).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	if _, err = s.List(context.Background(), org, owner, conn, 0, 10); err == nil {
		t.Fatal("missing connection accepted")
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
