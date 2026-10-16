package toolusage

import (
	"context"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
)

func TestPostgresSessionReuseAcrossChannels(t *testing.T) {
	dsn := os.Getenv("ZGI_TOOL_USAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("local PostgreSQL DSN required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal("PostgreSQL connection failed")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	tx := db.Begin()
	defer tx.Rollback()
	r := NewRepository(tx)
	org := uuid.New()
	session := uuid.NewString()
	var firstID uuid.UUID
	for i := 0; i < 2; i++ {
		identity := Identity{OrganizationID: org, RequestID: uuid.NewString(), AttemptID: uuid.NewString(), Lane: ownedLane, Provider: "t3_fixture_provider", ChannelID: uuid.New()}
		o := NewObserver(identity, nil)
		o.Observe([]byte(`{"id":"resp_pg","output":[{"id":"ci_pg","type":"code_interpreter_call","status":"completed","container_id":"` + session + `"}]}`))
		events, err := o.Events(true, false)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := r.Record(context.Background(), &events[0])
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstID = stored.ID
		} else if stored.ID != firstID {
			t.Fatal("same session created another fact")
		}
	}
	var count int64
	if err := tx.Model(&Event{}).Where("organization_id = ?", org).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count %d, error %v", count, err)
	}
}
