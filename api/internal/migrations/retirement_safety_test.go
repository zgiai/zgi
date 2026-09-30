package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetirementChecksDoNotPermitNewOrModifiedDestructiveMigrations(t *testing.T) {
	if err := checkRetirementSourceSafety(); err != nil {
		t.Fatal(err)
	}
	registeredIDs := map[string]bool{}
	for _, migration := range allMigrations() {
		registeredIDs[migration.ID] = true
	}
	for id := range historicalMigrationSourceDigests {
		if !registeredIDs[id] {
			t.Fatal("source exception is not a registered public migration", id)
		}
	}
	dir := t.TempDir()
	newFile := filepath.Join(dir, "20990101000000_unreviewed.go")
	if err := os.WriteFile(newFile, []byte("package migrations\n// DROP TABLE accounts"), 0600); err != nil {
		t.Fatal(err)
	}
	if checkMigrationSourceSafety([]string{newFile}) == nil {
		t.Fatal("ordinary migration gained destructive capability")
	}
	for id := range historicalMigrationSourceDigests {
		file := filepath.Join(dir, id+".go")
		if err := os.WriteFile(file, []byte("package migrations\n// changed historical source"), 0600); err != nil {
			t.Fatal(err)
		}
		if checkMigrationSourceSafety([]string{file}) == nil {
			t.Fatal("modified historical migration accepted")
		}
		break
	}
	if migrationSourceDigest([]byte("one\r\ntwo\r\n")) != migrationSourceDigest([]byte("one\ntwo\n")) {
		t.Fatal("line endings changed historical identity")
	}
	if strings.TrimSpace(retirementSchemaSourceDigest) == "" {
		t.Fatal("retirement executor has no explicit check")
	}
}
