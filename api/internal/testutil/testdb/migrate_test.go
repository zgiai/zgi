package testdb

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type postgresFixture struct {
	ID          string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	SecondaryID string    `gorm:"type:uuid;not null;default:gen_random_uuid()"`
	OccurredAt  time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	CreatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	Identity    string    `gorm:"not null;uniqueIndex"`
}

func TestSQLiteFixtureRetainsDefaultsAndConstraints(t *testing.T) {
	db := openSQLiteFixture(t)
	if err := AutoMigrate(db, &postgresFixture{}); err != nil {
		t.Fatal(err)
	}
	row := postgresFixture{Identity: "unique"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := uuid.Parse(row.ID); err != nil {
		t.Fatalf("generated UUID %q: %v", row.ID, err)
	}
	if _, err := uuid.Parse(row.SecondaryID); err != nil {
		t.Fatalf("generated secondary UUID %q: %v", row.SecondaryID, err)
	}
	if row.OccurredAt.IsZero() || row.CreatedAt.IsZero() {
		t.Fatal("timestamp defaults were not applied")
	}
	var loaded postgresFixture
	if err := db.First(&loaded, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !loaded.OccurredAt.Equal(row.OccurredAt) {
		t.Fatalf("timestamp round trip = %v, want %v", loaded.OccurredAt, row.OccurredAt)
	}
	if err := db.Create(&postgresFixture{Identity: "unique"}).Error; err == nil {
		t.Fatal("fixture lost its unique constraint")
	}
}

func TestSQLiteFixtureDoesNotAlterOtherDatabaseSchema(t *testing.T) {
	first := openSQLiteFixture(t)
	if err := AutoMigrate(first, &postgresFixture{}); err != nil {
		t.Fatal(err)
	}
	second := openSQLiteFixture(t)
	statement := &gorm.Statement{DB: second}
	if err := statement.Parse(&postgresFixture{}); err != nil {
		t.Fatal(err)
	}
	if got := statement.Schema.LookUpField("ID").DefaultValue; got != "uuid_generate_v4()" {
		t.Fatalf("another connection inherited the fixture default: %q", got)
	}
	if got := statement.Schema.LookUpField("OccurredAt").TagSettings["TYPE"]; got != "timestamptz" {
		t.Fatalf("another connection inherited the fixture timestamp type: %q", got)
	}
}

func openSQLiteFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}
