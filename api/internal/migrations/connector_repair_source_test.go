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
		name, source string
		fails        bool
	}{
		{"trigger definition", function, false},
		{"rewrite before trigger", "UPDATE public.claims SET state='denied';" + function, true},
		{"rewrite after trigger", function + "UPDATE public.claims SET state='denied';", true},
		{"anonymous rewrite", "DO $$ BEGIN UPDATE public.claims SET state='denied'; END $$;", true},
		{"delete in trigger", strings.Replace(function, "UPDATE public.claims SET state=NEW.status", "DELETE FROM public.claims", 1), true},
		{"non trigger function", strings.Replace(function, "RETURNS trigger", "RETURNS void", 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "migration.go")
			if err := os.WriteFile(file, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			if err := checkMigrationSourceSafety([]string{file}); (err != nil) != tc.fails {
				t.Fatalf("safety failure=%v want=%v: %v", err != nil, tc.fails, err)
			}
		})
	}
}
