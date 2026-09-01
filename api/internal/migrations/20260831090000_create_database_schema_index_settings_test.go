package migrations

import (
	"errors"
	"os"
	"testing"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDatabaseSchemaIndexMigrationPostgres(t *testing.T) {
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
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND ((table_name='text2sql_schema_indexes' AND column_name='build_attempts') OR (table_name='database_schema_index_settings' AND column_name IN ('requested_by','workspace_id','target_ids')))`).Scan(&count).Error; err != nil {
			return err
		}
		if count != 4 {
			return errors.New("missing schema index migration columns")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
