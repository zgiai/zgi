package connectorfinalize

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
)

// The standalone historical converter keeps its own copy of the compatible
// checkpoint history. Every ID it accepts must be accepted here as well, so a
// database the API upgrade chain stops at stays convertible.
func TestHistoricalConverterCompatibleMigrationsMatchFinalizer(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "tools", "connector-migrate", "checkpoint.go"))
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)var compatibleCheckpointMigrations = \[\]string\{(.*?)\}`).FindSubmatch(source)
	if block == nil {
		t.Fatal("converter compatible checkpoint list not found")
	}
	ids := regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(block[1], -1)
	if len(ids) == 0 {
		t.Fatal("converter compatible checkpoint list is empty")
	}
	for _, id := range ids {
		if !CompatibleCheckpointMigration(string(id[1])) {
			t.Fatalf("converter accepts %s but the finalizer does not", id[1])
		}
	}
	for _, id := range []string{"20260903120000_add_chat_runtime_external_user", "20260903183000_add_video_runtime_task_poster_url"} {
		if !regexp.MustCompile(regexp.QuoteMeta(`"` + id + `"`)).Match(block[1]) {
			t.Fatalf("finalizer accepts %s but the converter does not", id)
		}
	}
}
