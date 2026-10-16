package connectorfinalize

import (
	"errors"
	"slices"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestNativeInventoryFollowsCreatingMigration(t *testing.T) {
	for _, test := range []struct {
		name                        string
		applied, present, wantError bool
	}{
		{"before_creating_migration", false, false, false},
		{"after_creating_migration", true, true, false},
		{"applied_but_missing_table", true, false, true},
		{"unmarked_table", false, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			conn, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("SELECT").WithArgs(pendingSetupsMigrationID).
				WillReturnRows(sqlmock.NewRows([]string{"applied", "present"}).AddRow(test.applied, test.present))
			tables, err := nativeTablesAtCheckpoint(db)
			if (err != nil) != test.wantError {
				t.Fatalf("unexpected result: %v", err)
			}
			if !test.wantError {
				if slices.Contains(tables, "connector_pending_setups") != test.applied {
					t.Fatal("wrong pending-setups inventory")
				}
				for _, table := range NativeTables() {
					if table != "connector_pending_setups" && !slices.Contains(tables, table) {
						t.Fatalf("lost required native table %s", table)
					}
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNativeInventoryRejectsCheckpointReadFailure(t *testing.T) {
	conn, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("checkpoint read failed")
	mock.ExpectQuery("SELECT").WithArgs(pendingSetupsMigrationID).WillReturnError(injected)
	if _, err := nativeTablesAtCheckpoint(db); !errors.Is(err, injected) {
		t.Fatalf("checkpoint query failure was not preserved: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
