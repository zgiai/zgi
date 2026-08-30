package migrations

import (
	"testing"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddVendorToLLMModelsMigrationIsCompatibilityMarker(t *testing.T) {
	db, _ := openMigrationMockDB(t)
	builder := mschema.New(db)
	if err := upAddVendorToLLMModels(builder); err != nil {
		t.Fatal(err)
	}
	if statements := builder.Statements(); len(statements) != 0 {
		t.Fatalf("compatibility migration executed schema statements: %v", statements)
	}
}
