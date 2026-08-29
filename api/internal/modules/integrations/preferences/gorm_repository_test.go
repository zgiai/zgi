package preferences

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGormRepositoryListsOnlyTheRequestedScope(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, DriverName: "postgres"}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	organizationID, accountID, workspaceID := uuid.New(), uuid.New(), uuid.New()
	preferenceID, connectionID := uuid.New(), uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "aichat_integration_preferences" WHERE (organization_id = $1 AND account_id = $2) AND workspace_id = $3 ORDER BY integration_id ASC`)).
		WithArgs(organizationID, accountID, workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "account_id", "workspace_id", "integration_id", "selected_connection_ids", "preferred_connection_id", "revision"}).
			AddRow(preferenceID, organizationID, accountID, workspaceID, "github", `["`+connectionID.String()+`"]`, connectionID, 1))

	items, err := NewGormRepository(db).List(context.Background(), organizationID, accountID, &workspaceID)
	if err != nil || len(items) != 1 || items[0].ID != preferenceID || items[0].PreferredConnectionID == nil || *items[0].PreferredConnectionID != connectionID {
		t.Fatalf("List() = %#v, %v", items, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
