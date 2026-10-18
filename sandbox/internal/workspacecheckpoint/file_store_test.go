package workspacecheckpoint

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const testKey = "1b35ad14ef39430cc10379d63d1b8f370c27a997ce4cf02f31ccaaa02530fe10"

func TestFileStorePublishesAndVerifiesCheckpoint(t *testing.T) {
	directory := t.TempDir()
	store, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	content := []byte("workspace archive bytes")
	if err := store.Save(context.Background(), testKey, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	reader, err := store.Open(context.Background(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("checkpoint = %q, %v", got, err)
	}
	if err := store.Save(context.Background(), testKey, strings.NewReader("replacement")); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(directory, testKey+".tar"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte("X"), 0); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	reader, err = store.Open(context.Background(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(reader)
	_ = reader.Close()
	if !errors.Is(err, agentexecution.ErrBackendUnavailable) {
		t.Fatalf("corruption error = %v", err)
	}
	if err := store.Delete(context.Background(), testKey); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), testKey); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("deleted checkpoint error = %v", err)
	}
}

func TestFileStoreConcurrentPublishHasOneValidArchive(t *testing.T) {
	directory := t.TempDir()
	first, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var group sync.WaitGroup
	for _, item := range []struct {
		store *FileStore
		value string
	}{{first, "first"}, {second, "second"}} {
		group.Add(1)
		go func() {
			defer group.Done()
			if err := item.store.Save(context.Background(), testKey, strings.NewReader(item.value)); err != nil {
				t.Errorf("publish: %v", err)
			}
		}()
	}
	group.Wait()
	reader, err := first.Open(context.Background(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	value, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || (string(value) != "first" && string(value) != "second") {
		t.Fatalf("published content = %q, %v", value, err)
	}
}

func TestFileStoreRejectsUnsafeKeysAndIncompleteSource(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, key := range []string{"../escape", "/tmp/file", "short"} {
		if err := store.Save(context.Background(), key, strings.NewReader("x")); !errors.Is(err, agentexecution.ErrInvalidRequest) {
			t.Fatalf("key %q error = %v", key, err)
		}
	}
	if err := store.Save(context.Background(), testKey, errorReader{}); err == nil {
		t.Fatal("incomplete source was published")
	}
	if _, err := store.Open(context.Background(), testKey); !errors.Is(err, agentexecution.ErrNotFound) {
		t.Fatalf("incomplete checkpoint error = %v", err)
	}
}

func TestFileStoreSecuresCheckpointDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("checkpoint directory mode = %v", info.Mode())
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("source failed") }
