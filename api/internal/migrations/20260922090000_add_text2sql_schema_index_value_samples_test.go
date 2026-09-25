package migrations

import (
	"errors"
	"os"
	"testing"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestText2SQLSchemaIndexValueSamplesMigrationPostgres(t *testing.T) {
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
		if err := tx.Exec(`CREATE TABLE public.text2sql_schema_indexes (id integer PRIMARY KEY)`).Error; err != nil {
			return err
		}
		if err := upAddText2SQLSchemaIndexValueSamples(mschema.New(tx)); err != nil {
			return err
		}
		var count int64
		if err := tx.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='text2sql_schema_indexes' AND column_name='value_samples'`).Scan(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errors.New("missing value_samples column")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
}
