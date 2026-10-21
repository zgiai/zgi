package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectorRepairMigrationSourceSafety(t *testing.T) {
	dir, err := migrationsDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkMigrationSourceSafety([]string{filepath.Join(dir, migration202609070512481192ID+".go")}); err != nil {
		t.Fatal(err)
	}
	function := "CREATE OR REPLACE FUNCTION public.advance_state() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN UPDATE public.claims SET state=NEW.status; RETURN NEW; END $$;"
	for _, tc := range []struct {
		name, source   string
		fails, dataFix bool
	}{
		{"schema-only index", "CREATE INDEX claims_state_idx ON public.claims(state);", false, false},
		{"trigger inside reviewed DataFix", function, false, true},
		// New migrations must put DML in an explicit DataFix, even in a trigger body.
		{"trigger definition outside DataFix", function, true, false},
		{"rewrite before trigger", "UPDATE public.claims SET state='denied';" + function, true, false},
		{"rewrite after trigger", function + "UPDATE public.claims SET state='denied';", true, false},
		{"anonymous rewrite", "DO $$ BEGIN UPDATE public.claims SET state='denied'; END $$;", true, false},
		{"delete in trigger", strings.Replace(function, "UPDATE public.claims SET state=NEW.status", "DELETE FROM public.claims", 1), true, false},
		{"non trigger function", strings.Replace(function, "RETURNS trigger", "RETURNS void", 1), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "migration.go")
			// The scanner consumes Go migration source, then checks its SQL literals.
			imports := "import mschema \"github.com/zgiai/zgi/api/internal/migrations/schema\"\n"
			body := "return schema.Raw(`" + tc.source + "`)"
			if tc.dataFix {
				imports += "import \"gorm.io/gorm\"\n"
				body = "return schema.DataFix(\"reviewed trigger projection\", func(db *gorm.DB) error { return db.Exec(`" + tc.source + "`).Error })"
			}
			source := "package migrations\n" + imports + "func init() { registerSchemaMigration(\"test\", upTest, nil) }\nfunc upTest(schema *mschema.Builder) error { " + body + " }\n"
			if err := os.WriteFile(file, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			if err := checkMigrationSourceSafety([]string{file}); (err != nil) != tc.fails {
				t.Fatalf("safety failure=%v want=%v: %v", err != nil, tc.fails, err)
			}
		})
	}
}
