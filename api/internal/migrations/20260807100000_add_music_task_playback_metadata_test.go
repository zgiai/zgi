package migrations

import (
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
)

func TestAddMusicTaskPlaybackMetadataMigration(t *testing.T) {
	db, mock := openMigrationMockDB(t)
	mock.ExpectExec("(?s).*ALTER TABLE public.music_generation_tasks.*ADD COLUMN IF NOT EXISTS title.*ADD COLUMN IF NOT EXISTS style_tags jsonb.*ADD COLUMN IF NOT EXISTS duration_ms.*ADD COLUMN IF NOT EXISTS waveform_peaks jsonb.*").
		WillReturnResult(sqlmock.NewResult(0, 0))

	builder := mschema.New(db)
	if err := upAddMusicTaskPlaybackMetadata(builder); err != nil {
		t.Fatal(err)
	}

	statements := strings.Join(builder.Statements(), "\n")
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS title varchar(255) NOT NULL DEFAULT ''",
		"ADD COLUMN IF NOT EXISTS style_tags jsonb NOT NULL DEFAULT '[]'::jsonb",
		"ADD COLUMN IF NOT EXISTS duration_ms bigint NOT NULL DEFAULT 0",
		"ADD COLUMN IF NOT EXISTS waveform_peaks jsonb NOT NULL DEFAULT '[]'::jsonb",
	} {
		if !strings.Contains(statements, want) {
			t.Fatalf("music playback metadata migration missing %q:\n%s", want, statements)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
