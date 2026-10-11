package checkpointbackup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

// A real S3-compatible endpoint and pre-created private bucket are required.
// Unique prefixes ensure this test never overwrites another run's objects.
func TestBackupRealS3IndependentRestore(t *testing.T) {
	endpoint, bucket := os.Getenv("ZGI_CHECKPOINT_S3_TEST_ENDPOINT"), os.Getenv("ZGI_CHECKPOINT_S3_TEST_BUCKET")
	if endpoint == "" || bucket == "" {
		t.Skip("configure a real private S3 checkpoint fixture")
	}
	m, _, content := fixture(t)
	prefix := "backup-contract-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	newStore := func(suffix string) *workspacecheckpoint.S3Store {
		t.Helper()
		store, err := workspacecheckpoint.NewS3Store(t.Context(), workspacecheckpoint.S3Config{Endpoint: endpoint, Bucket: bucket, Region: "us-east-1", Prefix: prefix + "/" + suffix, UsePathStyle: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := store.Delete(ctx, m.Bindings[0].CheckpointKey); err != nil {
				t.Errorf("remove own backup fixture object: %v", err)
			}
		})
		return store
	}
	source, target := newStore("source"), newStore("target")
	if err := source.Save(t.Context(), m.Bindings[0].CheckpointKey, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "backup")
	if _, err := Export(t.Context(), directory, m, source); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyTo(t.Context(), directory, target); err != nil {
		t.Fatal(err)
	}
	reader, err := target.Open(t.Context(), m.Bindings[0].CheckpointKey)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || !bytes.Equal(content, actual) {
		t.Fatalf("S3 recovery=%q read=%v close=%v", actual, err, closeErr)
	}
	if err := source.Delete(t.Context(), m.Bindings[0].CheckpointKey); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(t.Context(), directory); err != nil {
		t.Fatalf("source loss invalidated independent backup: %v", err)
	}
	failed := filepath.Join(t.TempDir(), "missing-source")
	if _, err := Export(t.Context(), failed, m, source); err == nil {
		t.Fatal("missing S3 source falsely backed up")
	}
	if _, err := os.Stat(filepath.Join(failed, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("incomplete S3 export published manifest: %v", err)
	}
}
