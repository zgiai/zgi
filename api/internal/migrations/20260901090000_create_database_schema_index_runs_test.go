package migrations

import (
	"errors"
	"os"
	"testing"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDatabaseSchemaIndexRunsMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_SCHEMA_INDEX_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	rollback := errors.New("rollback migration fixture")
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`).Error; err != nil {
			return err
		}
		if err := upCreateText2SQLSchemaIndexes(mschema.New(tx)); err != nil {
			return err
		}
		if err := upDatabaseSchemaIndexSettings(mschema.New(tx)); err != nil {
			return err
		}
		if err := upDatabaseSchemaIndexRuns(mschema.New(tx)); err != nil {
			return err
		}
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND ((table_name='database_schema_index_settings' AND column_name IN ('pending_run_id','pending_trigger')) OR (table_name='database_schema_index_runs' AND column_name IN ('trigger','status','tables','target_ids')))`).Scan(&count).Error; err != nil {
			return err
		}
		if count != 6 {
			return errors.New("missing schema index run columns")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
