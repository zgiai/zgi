package service

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestCustomSkillVersionsPreserveUpgradeAndRollback(t *testing.T) {
	storage := &filesystemCustomSkillStorage{root: t.TempDir()}
	org := uuid.New()
	publish := func(content string) (string, string) {
		t.Helper()
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		f, err := w.Create("SKILL.md")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte("---\nname: example\ndescription: synthetic example\n---\n" + content)); err != nil {
			t.Fatal(err)
		}
		if err = w.Close(); err != nil {
			t.Fatal(err)
		}
		preview, err := storage.SavePreviewPackage(context.Background(), org, uuid.NewString(), buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		published, path, err := storage.PublishPreview(context.Background(), preview, "example")
		if err != nil {
			t.Fatal(err)
		}
		published.cleanup()
		hash, err := skills.SkillPackageSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(path) != hash {
			t.Fatal("version path does not match content")
		}
		return path, hash
	}
	first, firstHash := publish("version one")
	second, secondHash := publish("version two")
	if first == second || firstHash == secondHash {
		t.Fatal("upgrade overwrote original version")
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatal("original version not retained")
	}
	rollback, rollbackHash := publish("version one")
	if rollback != first || rollbackHash != firstHash {
		t.Fatal("rollback did not select original content")
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatal("rollback removed newer version")
	}
}
