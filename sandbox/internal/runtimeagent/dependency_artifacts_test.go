package runtimeagent

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func dependencyFixture(t *testing.T) *Backend {
	t.Helper()
	b := packageTestBackend(t)
	writeLanguageFixture(t, b, testPythonMetadata, "Name: ujson\nVersion: 5.10.0\n")
	writeLanguageFixture(t, b, path.Dir(testPythonMetadata)+"/WHEEL", "Tag: cp313-cp313-linux_x86_64\n")
	writeLanguageFixture(t, b, path.Dir(path.Dir(testPythonMetadata))+"/ujson.so", "native-python-payload")
	writeLanguageFixture(t, b, testNodeMetadata, `{"name":"bufferutil","version":"4.0.9","scripts":{"install":"touch must-not-run"}}`)
	writeLanguageFixture(t, b, "node_modules/bufferutil/build/Release/bufferutil.node", "native-node-payload")
	writeLanguageFixture(t, b, sessionHomeDirectory+"/.venv/bin/python", "copied-interpreter")
	if err := b.captureLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDependencyArtifactsRestoreMissingNativeFilesAndEntireTrees(t *testing.T) {
	b := dependencyFixture(t)
	ledger, _, err := b.readLanguageLedger()
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Artifacts) != 2 {
		t.Fatalf("artifacts=%+v", ledger.Artifacts)
	}
	for _, artifact := range ledger.Artifacts {
		before, err := b.dependencyArchive(t.Context(), artifact.Target, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if before != artifact {
			t.Fatalf("fingerprint=%+v want=%+v", before, artifact)
		}
	}
	if err := b.workspace.Remove(sessionHomeDirectory + "/.venv/lib/python3.13/site-packages/ujson.so"); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.RemoveAll("node_modules"); err != nil {
		t.Fatal(err)
	}
	b.languageLedgerVerified = false
	if err := b.verifyLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		sessionHomeDirectory + "/.venv/lib/python3.13/site-packages/ujson.so": "native-python-payload",
		"node_modules/bufferutil/build/Release/bufferutil.node":               "native-node-payload",
	} {
		raw, err := b.workspace.ReadFile(name)
		if err != nil || string(raw) != want {
			t.Fatalf("restored %s=%q err=%v", name, raw, err)
		}
	}
	if _, err := b.workspace.Stat("must-not-run"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("installation hook executed")
	}
	// Matching compute needs no artifact read, network, installer or rewrite.
	for _, artifact := range ledger.Artifacts {
		if err := b.workspace.Remove(artifact.name()); err != nil {
			t.Fatal(err)
		}
	}
	b.languageLedgerVerified = false
	if err := b.verifyLanguageLedger(t.Context()); err != nil {
		t.Fatalf("matching tree required backup: %v", err)
	}
}

func TestDependencyArtifactsValidateAllBeforeReplacingAnyTree(t *testing.T) {
	b := dependencyFixture(t)
	ledger, _, err := b.readLanguageLedger()
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range ledger.Artifacts {
		if err := b.workspace.RemoveAll(artifact.Target); err != nil {
			t.Fatal(err)
		}
	}
	bad := ledger.Artifacts[len(ledger.Artifacts)-1]
	good, err := b.workspace.ReadFile(bad.name())
	if err != nil {
		t.Fatal(err)
	}
	wrong := bytes.Clone(good)
	wrong[len(wrong)/2] ^= 0xff
	if err := b.workspace.WriteFile(bad.name(), wrong, 0o600); err != nil {
		t.Fatal(err)
	}
	b.languageLedgerVerified = false
	if err := b.verifyLanguageLedger(t.Context()); err == nil {
		t.Fatal("corrupt artifact accepted")
	}
	for _, artifact := range ledger.Artifacts {
		if _, err := b.workspace.Stat(artifact.Target); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("live tree changed before all artifacts validated: %s %v", artifact.Target, err)
		}
	}
	if err := b.workspace.WriteFile(bad.name(), good, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.verifyLanguageLedger(t.Context()); err != nil {
		t.Fatalf("artifact repair retry: %v", err)
	}
	entries, err := fs.ReadDir(b.workspace.FS(), dependencyArtifactDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "restore-") {
			t.Errorf("temporary tree leaked: %s", entry.Name())
		}
	}
}

func TestDependencyRecoveryAfterInterruptedDirectorySwap(t *testing.T) {
	for _, swapped := range []bool{false, true} {
		t.Run(fmt.Sprint(swapped), func(t *testing.T) {
			b := dependencyFixture(t)
			ledger, _, err := b.readLanguageLedger()
			if err != nil {
				t.Fatal(err)
			}
			artifact := ledger.Artifacts[1]
			stage, backup := dependencyRecoveryNames(artifact)
			if err := b.workspace.Rename(artifact.Target, backup); err != nil {
				t.Fatal(err)
			}
			writeLanguageFixture(t, b, stage+"/partial", "interrupted import")
			if swapped {
				if err := b.workspace.Rename(backup, artifact.Target); err != nil {
					t.Fatal(err)
				}
				writeLanguageFixture(t, b, backup+"/old", "old installation")
			}
			// Another session-owned-looking tree is not selected by this ledger.
			writeLanguageFixture(t, b, dependencyArtifactDirectory+"/restore-unrelated.stage/business", "keep")
			b.languageLedgerVerified = false
			if err := b.verifyLanguageLedger(t.Context()); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{stage, backup} {
				if _, err := b.workspace.Lstat(name); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("interrupted state retained: %s %v", name, err)
				}
			}
			if _, err := b.workspace.Stat(dependencyArtifactDirectory + "/restore-unrelated.stage/business"); err != nil {
				t.Fatalf("unrelated directory removed: %v", err)
			}
		})
	}
}

func TestDependencyArtifactCoverageAndUntrustedTargets(t *testing.T) {
	b := dependencyFixture(t)
	valid, _, err := b.readLanguageLedger()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*languagePackageLedger)
	}{
		{"escape", func(l *languagePackageLedger) { l.Artifacts[0].Target = "../node_modules" }},
		{"arbitrary-directory", func(l *languagePackageLedger) { l.Artifacts[0].Target = "business-files" }},
		{"hash-path", func(l *languagePackageLedger) { l.Artifacts[0].SHA256 = "../../outside" }},
		{"oversize", func(l *languagePackageLedger) { l.Artifacts[0].Size = maxDependencyArtifactBytes + 1 }},
		{"missing", func(l *languagePackageLedger) { l.Artifacts = l.Artifacts[:1] }},
		{"duplicate", func(l *languagePackageLedger) { l.Artifacts[1] = l.Artifacts[0] }},
		{"uncached-duplicate", func(l *languagePackageLedger) { l.UncachedTargets = []string{l.Artifacts[0].Target} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := valid
			l.Artifacts = slices.Clone(valid.Artifacts)
			tc.mutate(&l)
			if err := b.validateDependencyArtifacts(l); err == nil {
				t.Fatal("invalid artifact references accepted")
			}
		})
	}
	if !slices.Equal(dependencyTargets([]languagePackage{{Language: "python", Path: testPythonMetadata}, {Language: "node", Path: sessionHomeDirectory + "/.venv/node_modules/foo/package.json"}}), []string{sessionHomeDirectory + "/.venv"}) {
		t.Fatal("overlapping dependency targets not coalesced")
	}
	limited := artifactWriter{writer: io.Discard, remaining: 1}
	if _, err := limited.Write([]byte("ab")); !errors.Is(err, agentexecution.ErrOutputLimit) {
		t.Fatalf("unbounded artifact writer: %v", err)
	}
}

func TestDependencyArtifactArchiveRejectsMaliciousEntries(t *testing.T) {
	for _, header := range []tar.Header{
		{Name: "../outside", Typeflag: tar.TypeReg, Size: 1},
		{Name: "/outside", Typeflag: tar.TypeReg, Size: 1},
		{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../outside"},
		{Name: "fifo", Typeflag: tar.TypeFifo},
		{Name: "bomb", Typeflag: tar.TypeReg, Size: maxWorkspaceArchiveBytes + 1},
	} {
		t.Run(header.Name, func(t *testing.T) {
			b := packageTestBackend(t)
			var raw bytes.Buffer
			gz := gzip.NewWriter(&raw)
			tw := tar.NewWriter(gz)
			if err := tw.WriteHeader(&header); err != nil {
				t.Fatal(err)
			}
			if header.Size == 1 {
				if _, err := tw.Write([]byte("x")); err != nil {
					t.Fatal(err)
				}
			}
			_ = tw.Close()
			_ = gz.Close()
			hash := sha256.Sum256(raw.Bytes())
			artifact := dependencyArtifact{Target: "node_modules", SHA256: hex.EncodeToString(hash[:]), Size: int64(raw.Len())}
			writeLanguageFixture(t, b, artifact.name(), raw.String())
			if _, err := b.stageDependencyArtifact(t.Context(), artifact); err == nil {
				t.Fatal("malicious archive accepted")
			}
			if _, err := b.workspace.Stat("outside"); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("archive escaped staging")
			}
		})
	}
}

func TestDependencyCaptureDeduplicationGCAndCancellation(t *testing.T) {
	b := dependencyFixture(t)
	before, _, err := b.readLanguageLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.captureLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	after, _, err := b.readLanguageLedger()
	if err != nil || !slices.Equal(before.Artifacts, after.Artifacts) {
		t.Fatalf("unchanged artifacts not reused: %+v %v", after.Artifacts, err)
	}
	writeLanguageFixture(t, b, testNodeMetadata, `{"name":"bufferutil","version":"4.1.0"}`)
	if err := b.captureLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	current, _, err := b.readLanguageLedger()
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range before.Artifacts {
		if !slices.Contains(current.Artifacts, old) {
			if _, err := b.workspace.Stat(old.name()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("superseded artifact retained: %v", err)
			}
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := b.dependencyArchive(ctx, "node_modules", io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled archive accepted: %v", err)
	}
}
