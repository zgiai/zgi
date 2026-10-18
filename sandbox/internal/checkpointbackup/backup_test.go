package checkpointbackup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
	"github.com/zgiai/zgi-sandbox/internal/workspacecheckpoint"
)

func fixture(t *testing.T) (Manifest, *workspacecheckpoint.FileStore, []byte) {
	t.Helper()
	key := strings.Repeat("a", 64)
	digest := "sha256:" + strings.Repeat("b", 64)
	b := agentexecution.Binding{ID: "binding", OrganizationID: "org", SubjectID: "user", AgentID: "agent", ConversationID: "conversation", ProfileID: "profile", WorkspaceID: "workspace", Backend: "fake", IdempotencyKey: "request", RequestHash: "hash", Generation: 1, Status: agentexecution.BindingStatusSuspended, CheckpointKey: key, RetentionExpiresAt: time.Now().Add(time.Hour)}
	m := Manifest{Version: Version, Backend: "fake", CreatedAt: time.Now(), ConfigurationDigest: digest, SourceStorageDigest: digest, Profiles: map[string]string{"profile": digest}, Bindings: []agentexecution.Binding{b}}
	store, err := workspacecheckpoint.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	content := []byte("private workspace and installed dependency bytes")
	if err := store.Save(t.Context(), key, bytes.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	return m, store, content
}

func TestBackupVerifiedRoundTripAndDestinationConflict(t *testing.T) {
	m, source, content := fixture(t)
	directory := filepath.Join(t.TempDir(), "set")
	completed, err := Export(t.Context(), directory, m, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(completed.Objects) != 1 || completed.Objects[0].Size != int64(len(content)) {
		t.Fatalf("objects=%+v", completed.Objects)
	}
	if _, err := Verify(t.Context(), directory); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(t.Context(), directory, m, source); err == nil {
		t.Fatal("existing backup directory overwritten")
	}
	target, err := workspacecheckpoint.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	if _, err := CopyTo(t.Context(), directory, target); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyTo(t.Context(), directory, target); err != nil {
		t.Fatalf("object retry: %v", err)
	}
	reader, err := target.Open(t.Context(), m.Bindings[0].CheckpointKey)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := io.ReadAll(reader)
	err = errors.Join(err, reader.Close())
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("restored=%q error=%v", actual, err)
	}
	conflict, err := workspacecheckpoint.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer conflict.Close()
	if err := conflict.Save(t.Context(), m.Bindings[0].CheckpointKey, strings.NewReader("different bytes")); err != nil {
		t.Fatal(err)
	}
	if _, err := CopyTo(t.Context(), directory, conflict); err == nil {
		t.Fatal("unverified idempotent destination accepted")
	}
	if err := os.WriteFile(filepath.Join(directory, "checkpoints", m.Bindings[0].CheckpointKey+".tar"), bytes.Repeat([]byte("X"), len(content)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(t.Context(), directory); err == nil {
		t.Fatal("corrupt backup accepted")
	}
}

func TestIncompleteBackupNeverPublishesManifest(t *testing.T) {
	m, source, _ := fixture(t)
	for _, test := range []struct {
		name   string
		source agentexecution.WorkspaceCheckpointStore
	}{
		{"source-missing", missingStore{}}, {"upload-interrupted", interruptedStore{WorkspaceCheckpointStore: source}},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "incomplete")
			if _, err := Export(t.Context(), directory, m, test.source); err == nil {
				t.Fatal("incomplete export succeeded")
			}
			if _, err := os.Stat(filepath.Join(directory, "manifest.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("ready manifest exists: %v", err)
			}
			if _, err := Verify(t.Context(), directory); err == nil {
				t.Fatal("incomplete backup verified")
			}
		})
	}
}

func TestBackupManifestRejectsUnsafeRecoveryRecords(t *testing.T) {
	m, source, _ := fixture(t)
	directory := filepath.Join(t.TempDir(), "set")
	m, err := Export(t.Context(), directory, m, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*Manifest)
	}{
		{"active", func(m *Manifest) { m.Bindings[0].Status = agentexecution.BindingStatusActive }},
		{"provider-ref", func(m *Manifest) { m.Bindings[0].BackendRef = "source-environment" }},
		{"expired", func(m *Manifest) { m.Bindings[0].RetentionExpiresAt = time.Now().Add(-time.Minute) }},
		{"profile-missing", func(m *Manifest) { m.Profiles = nil }},
		{"object-missing", func(m *Manifest) { m.Objects = nil }},
		{"escaping-key", func(m *Manifest) { m.Objects[0].Key = "../private" }},
		{"duplicate-binding", func(m *Manifest) { m.Bindings = append(m.Bindings, m.Bindings[0]) }},
		{"unknown-version", func(m *Manifest) { m.Version++ }},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := m
			copy.Bindings = append([]agentexecution.Binding(nil), m.Bindings...)
			copy.Objects = append([]Object(nil), m.Objects...)
			test.change(&copy)
			if err := copy.Validate(time.Now()); err == nil {
				t.Fatal("unsafe manifest accepted")
			}
		})
	}
}

type missingStore struct {
	agentexecution.WorkspaceCheckpointStore
}

func (missingStore) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, agentexecution.ErrNotFound
}

type interruptedStore struct {
	agentexecution.WorkspaceCheckpointStore
}

func (interruptedStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(errorReader{}), nil
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
