package runtimeagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

const testPythonMetadata = sessionHomeDirectory + "/.venv/lib/python3.13/site-packages/ujson-5.10.0.dist-info/METADATA"
const testNodeMetadata = "node_modules/bufferutil/package.json"

func writeLanguageFixture(t *testing.T, b *Backend, name, raw string) {
	t.Helper()
	if err := b.workspace.MkdirAll(path.Dir(name), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := b.workspace.WriteFile(name, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLanguageInventoryUsesMetadataWithoutExecutingCode(t *testing.T) {
	b := packageTestBackend(t)
	writeLanguageFixture(t, b, testPythonMetadata, "Metadata-Version: 2.4\r\nName: ujson\r\nVersion: 5.10.0\r\n\r\nVersion: ignored-description\r\n")
	writeLanguageFixture(t, b, testNodeMetadata, `{"name":"bufferutil","version":"4.0.9","scripts":{"install":"touch must-not-run"}}`)
	writeLanguageFixture(t, b, "node_modules/foo/node_modules/bufferutil/package.json", `{"name":"bufferutil","version":"4.0.8"}`)
	writeLanguageFixture(t, b, "node_modules/foo/package.json", `{"name":"foo","version":"1.0.0"}`)
	writeLanguageFixture(t, b, "node_modules/@scope/example/package.json", `{"name":"@scope/example","version":"1.0.0-beta.2"}`)
	writeLanguageFixture(t, b, "node_modules/JSONStream/package.json", `{"name":"JSONStream","version":"1.3.5"}`)
	writeLanguageFixture(t, b, "package.json", `{"not":"an installed package"}`)
	writeLanguageFixture(t, b, sessionHomeDirectory+"/.venv/lib/python3.13/site-packages/evil.pth", "import os; os.system('touch must-not-run')")
	got, err := b.installedLanguagePackages(t.Context())
	if err != nil || len(got) != 6 {
		t.Fatalf("inventory=%+v err=%v", got, err)
	}
	if _, err := b.workspace.Stat("must-not-run"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("user code executed: %v", err)
	}
	if err := b.captureLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	ledger, exists, err := b.readLanguageLedger()
	if err != nil || !exists || len(ledger.Packages) != 6 || ledger.Version != agentexecution.LanguagePackageLedgerVersion {
		t.Fatalf("ledger=%+v exists=%v err=%v", ledger, exists, err)
	}
}

func TestLanguageCheckpointRoundTripAndRetryAfterRepair(t *testing.T) {
	source := packageTestBackend(t)
	source.dependencyInstallation = true
	writeLanguageFixture(t, source, testPythonMetadata, "Name: ujson\nVersion: 5.10.0\n")
	writeLanguageFixture(t, source, testNodeMetadata, `{"name":"bufferutil","version":"4.0.9"}`)
	stream, err := source.ExportWorkspace(t.Context(), agentexecution.Allocation{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(stream)
	_ = stream.Close()
	if err != nil {
		t.Fatal(err)
	}
	destination, err := New(Options{WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = destination.Close() })
	if err := destination.ImportWorkspace(t.Context(), agentexecution.Allocation{}, bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	// A changed transitive version blocks both execution verification and a new
	// checkpoint; neither operation can bless an incomplete retained environment.
	writeLanguageFixture(t, destination, testNodeMetadata, `{"name":"bufferutil","version":"4.1.0"}`)
	for range 2 {
		if err := destination.verifyLanguageLedger(t.Context()); err == nil || destination.languageLedgerVerified {
			t.Fatalf("changed version accepted: %v", err)
		}
		if err := destination.captureLanguageLedger(t.Context()); err == nil {
			t.Fatal("checkpoint hid corrupt restore")
		}
	}
	writeLanguageFixture(t, destination, testNodeMetadata, `{"name":"bufferutil","version":"4.0.9"}`)
	if err := destination.verifyLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Intentional subsequent installs/removals are recorded at the next checkpoint.
	if err := destination.workspace.Remove(testNodeMetadata); err != nil {
		t.Fatal(err)
	}
	if err := destination.workspace.Remove(path.Dir(testNodeMetadata)); err != nil {
		t.Fatal(err)
	}
	if err := destination.captureLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	ledger, _, err := destination.readLanguageLedger()
	if err != nil || len(ledger.Packages) != 1 {
		t.Fatalf("updated ledger=%+v err=%v", ledger, err)
	}
}

func TestLanguageLedgerLegacyAndMalformedRecords(t *testing.T) {
	b := packageTestBackend(t)
	if err := b.verifyLanguageLedger(t.Context()); err != nil {
		t.Fatalf("legacy checkpoint: %v", err)
	}
	b.languageLedgerVerified = false
	valid := languagePackageLedger{Version: agentexecution.LanguagePackageLedgerVersion, Environment: b.packageLedgerEnvironment(), Packages: []languagePackage{{Language: "python", Path: testPythonMetadata, Name: "ujson", Version: "5.10.0"}}}
	for _, tc := range []struct {
		name   string
		mutate func(*languagePackageLedger)
	}{
		{"version", func(l *languagePackageLedger) { l.Version++ }},
		{"profile", func(l *languagePackageLedger) { l.Environment.Profile = "other" }},
		{"source", func(l *languagePackageLedger) { l.Environment.Sources = "other" }},
		{"architecture", func(l *languagePackageLedger) { l.Environment.Architecture = "other" }},
		{"escape", func(l *languagePackageLedger) { l.Packages[0].Path = "../outside" }},
		{"language", func(l *languagePackageLedger) { l.Packages[0].Language = "bash" }},
		{"version-url", func(l *languagePackageLedger) { l.Packages[0].Version = "https://example.invalid" }},
		{"name", func(l *languagePackageLedger) { l.Packages[0].Name = "--config" }},
		{"duplicate", func(l *languagePackageLedger) { l.Packages = append(l.Packages, l.Packages[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(valid)
			var ledger languagePackageLedger
			if err := json.Unmarshal(raw, &ledger); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&ledger)
			raw, _ = json.Marshal(ledger)
			writeLanguageFixture(t, b, languageLedgerName, string(raw))
			if err := b.verifyLanguageLedger(t.Context()); err == nil || b.languageLedgerVerified {
				t.Fatalf("invalid ledger accepted: %v", err)
			}
		})
	}
	for _, raw := range []string{`null`, `[]`, `{"version":1,"extra":true}`, `{} {}`} {
		writeLanguageFixture(t, b, languageLedgerName, raw)
		if _, _, err := b.readLanguageLedger(); err == nil {
			t.Errorf("malformed ledger accepted: %s", raw)
		}
	}
}

func TestLanguageMetadataRejectsInvalidAndExcessiveInput(t *testing.T) {
	for _, raw := range []string{`{}`, `{"name":"bufferutil","version":"1;id"}`, `{"name":"../escape","version":"1"}`, strings.Repeat("x", maxPackageMetadataBytes+1)} {
		b := packageTestBackend(t)
		writeLanguageFixture(t, b, testNodeMetadata, raw)
		if _, err := b.installedLanguagePackages(t.Context()); err == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
	for _, raw := range []string{"Name: ujson\nName: another\nVersion: 1\n", "Name: ujson\n", "Name: ujson\nVersion: 1\nVersion: 2\n"} {
		b := packageTestBackend(t)
		writeLanguageFixture(t, b, testPythonMetadata, raw)
		if _, err := b.installedLanguagePackages(t.Context()); err == nil {
			t.Fatalf("invalid Python metadata accepted: %q", raw)
		}
	}
}

func TestLanguageRecoveryDiagnosticDoesNotLeakMetadata(t *testing.T) {
	err := &languageRecoveryError{cause: errors.New("private path token")}
	status, code, message := runtimeAgentErrorResponse(err)
	if status != http.StatusServiceUnavailable || code != "language_package_recovery_failed" || message != languageRecoveryMessage || strings.Contains(message, "token") {
		t.Fatalf("unsafe diagnostic: %d %s %s", status, code, message)
	}
	if clientErr := runtimeAgentClientError(code); !errors.Is(clientErr, agentexecution.ErrBackendUnavailable) || !strings.Contains(clientErr.Error(), "no automatic reinstall") {
		t.Fatalf("lost diagnostic: %v", clientErr)
	}
}

func TestLanguageLedgerConcurrentVerificationAndCancellation(t *testing.T) {
	b := packageTestBackend(t)
	writeLanguageFixture(t, b, testPythonMetadata, "Name: ujson\nVersion: 5.10.0\n")
	if err := b.captureLanguageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	b.languageLedgerVerified = false
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := b.verifyLanguageLedger(ctx); !errors.Is(err, context.Canceled) || b.languageLedgerVerified {
		t.Fatalf("cancellation accepted: %v", err)
	}
	var group sync.WaitGroup
	for range 10 {
		group.Go(func() {
			if err := b.verifyLanguageLedger(t.Context()); err != nil {
				t.Errorf("concurrent verify: %v", err)
			}
		})
	}
	group.Wait()
	if !b.languageLedgerVerified {
		t.Fatal("verification not committed")
	}
	capabilities, err := b.Capabilities(t.Context(), agentexecution.Allocation{})
	if err != nil || capabilities.LanguagePackageLedgerVersion != 0 {
		t.Fatalf("legacy capabilities=%+v err=%v", capabilities, err)
	}
	b.dependencyInstallation = true
	capabilities, err = b.Capabilities(t.Context(), agentexecution.Allocation{})
	if err != nil || capabilities.LanguagePackageLedgerVersion != agentexecution.LanguagePackageLedgerVersion {
		t.Fatalf("installation capabilities=%+v err=%v", capabilities, err)
	}
}
