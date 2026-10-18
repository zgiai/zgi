//go:build linux

package runtimeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zgiai/zgi-sandbox/internal/agentexecution"
)

func TestParseSystemPackages(t *testing.T) {
	packages, err := parseSystemPackages("zlib1g:amd64\tamd64\t1:1.2.13.dfsg-1~deb12u1\tinstalled\nfonts-test\tall\t2.0+dfsg-1\tinstalled\nremoved\tamd64\t1.0\tconfig-files\n")
	if err != nil || len(packages) != 2 || packages[0].Name != "fonts-test" || packages[1].Version != "1:1.2.13.dfsg-1~deb12u1" {
		t.Fatalf("packages=%+v error=%v", packages, err)
	}
	for _, raw := range []string{
		"tree\tamd64\t1.0\n",
		"../tree\tamd64\t1.0\tinstalled\n",
		"tree:arm64\tamd64\t1.0\tinstalled\n",
		"tree\t--evil\t1.0\tinstalled\n",
		"tree\tamd64\t1.0;id\tinstalled\n",
		"tree\tamd64\t1.0\tinstalled\ntree\tamd64\t2.0\tinstalled\n",
	} {
		if _, err := parseSystemPackages(raw); err == nil {
			t.Errorf("unsafe query accepted: %q", raw)
		}
	}
	var excessive strings.Builder
	for i := range maxSystemPackages + 1 {
		excessive.WriteString("pkg" + strconv.Itoa(i) + "\tamd64\t1\tinstalled\n")
	}
	if _, err := parseSystemPackages(excessive.String()); !errors.Is(err, agentexecution.ErrOutputLimit) {
		t.Fatalf("excessive query accepted: %v", err)
	}
}

func writePackageInventoryFixture(t *testing.T, backend *Backend, raw string) string {
	t.Helper()
	inventory := filepath.Join(t.TempDir(), "inventory")
	if err := os.WriteFile(inventory, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat '" + inventory + "'\n"
	if err := os.WriteFile(backend.packageQueryPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return inventory
}

func TestPackageLedgerUpdatesResolvedVersionsAndSeparatesDownloads(t *testing.T) {
	backend := packageTestBackend(t)
	inventory := writePackageInventoryFixture(t, backend, "tree\tamd64\t2.1.0-1\tinstalled\nlibtest\tamd64\t1:3.0-1\tinstalled\n")
	args := []string{"install", "-y", "tree"}
	if err := backend.recordPackageLedger(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inventory, []byte("tree\tamd64\t2.2.0-1\tinstalled\nlibtest\tamd64\t1:3.1-1\tinstalled\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backend.recordPackageLedger(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	// A download must not query or replace the installed package state.
	if err := os.Remove(backend.packageQueryPath); err != nil {
		t.Fatal(err)
	}
	if err := backend.recordPackageLedger(t.Context(), []string{"download", "ffmpeg"}); err != nil {
		t.Fatal(err)
	}
	ledger, err := backend.readPackageLedger()
	if err != nil || ledger.Version != packageLedgerVersion || len(ledger.Entries) != 2 || len(ledger.Packages) != 2 || ledger.Packages[1].Version != "2.2.0-1" {
		t.Fatalf("ledger=%+v error=%v", ledger, err)
	}
}

func TestPackageLedgerRejectsUntrustedLocksBeforeExecution(t *testing.T) {
	backend := packageTestBackend(t)
	valid := packageLedger{
		Version: packageLedgerVersion, Environment: backend.packageLedgerEnvironment(),
		Packages: []systemPackage{{Name: "tree", Architecture: "amd64", Version: "1:2.1.0-1~deb12u1"}},
		Entries:  []packageLedgerEntry{{Args: []string{"install", "tree"}}},
	}
	for _, tc := range []struct {
		name   string
		mutate func(*packageLedger)
	}{
		{"unknown-version", func(l *packageLedger) { l.Version = 3 }},
		{"profile", func(l *packageLedger) { l.Environment.Profile = "another-profile" }},
		{"source", func(l *packageLedger) { l.Environment.Sources = "another-source" }},
		{"architecture", func(l *packageLedger) { l.Environment.Architecture = "other" }},
		{"command", func(l *packageLedger) { l.Entries[0].Args = []string{"install", "-o", "Dir=/"} }},
		{"name", func(l *packageLedger) { l.Packages[0].Name = "--config" }},
		{"url-version", func(l *packageLedger) { l.Packages[0].Version = "https://invalid/pkg" }},
		{"shell-version", func(l *packageLedger) { l.Packages[0].Version = "1;id" }},
		{"architecture-option", func(l *packageLedger) { l.Packages[0].Architecture = "--option" }},
		{"duplicate", func(l *packageLedger) { l.Packages = append(l.Packages, l.Packages[0]) }},
		{"missing-lock", func(l *packageLedger) { l.Packages = nil }},
		{"unversioned-lock", func(l *packageLedger) { l.Version = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(valid)
			var ledger packageLedger
			if err := json.Unmarshal(raw, &ledger); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&ledger)
			raw, _ = json.Marshal(ledger)
			if err := backend.workspace.WriteFile(packageLedgerName, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := backend.replayPackageLedger(t.Context()); err == nil || backend.packageReplayComplete {
				t.Fatalf("unsafe ledger accepted: %+v error=%v", ledger, err)
			}
		})
	}
	for _, raw := range []string{`{"entries":[],"extra":"untrusted"}`, `{"entries":[]} {"entries":[]}`, `[]`, `null`} {
		if err := backend.workspace.WriteFile(packageLedgerName, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := backend.readPackageLedger(); err == nil {
			t.Errorf("invalid ledger accepted: %s", raw)
		}
	}
}

func TestLockedPackageReplayPinsDependenciesAndVerifies(t *testing.T) {
	backend := packageTestBackend(t)
	expected := "tree\tamd64\t2.1.0-1\tinstalled\nlibtest\tamd64\t1:3.0-1\tinstalled\n"
	inventory := writePackageInventoryFixture(t, backend, expected)
	if err := backend.recordPackageLedger(t.Context(), []string{"install", "tree"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inventory, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	log := filepath.Join(root, "commands")
	helper := filepath.Join(root, "apt-fixture")
	correct := filepath.Join(root, "expected")
	if err := os.WriteFile(correct, []byte(expected), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\ncp '" + correct + "' '" + inventory + "'\n"
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", helper)
	if err := backend.replayPackageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	commands, err := os.ReadFile(log)
	want := "install -y --no-install-recommends --allow-downgrades --no-remove libtest:amd64=1:3.0-1 tree:amd64=2.1.0-1\n"
	if err != nil || string(commands) != want {
		t.Fatalf("commands=%q error=%v want=%q", commands, err, want)
	}
	backend.packageReplayComplete = false // Another resume of already retained compute.
	if err := backend.replayPackageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	commandsAgain, _ := os.ReadFile(log)
	if string(commandsAgain) != want {
		t.Fatalf("reinstalled matching packages: %q", commandsAgain)
	}
}

func TestLockedReplayFailureAndVersionMismatchRemainRetryable(t *testing.T) {
	for _, wrongVersion := range []bool{false, true} {
		t.Run(map[bool]string{false: "version-unavailable", true: "post-install-mismatch"}[wrongVersion], func(t *testing.T) {
			backend := packageTestBackend(t)
			inventory := writePackageInventoryFixture(t, backend, "tree\tamd64\t2.1.0-1\tinstalled\n")
			if err := backend.recordPackageLedger(t.Context(), []string{"install", "tree"}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(inventory, []byte("tree\tamd64\t2.2.0-1\tinstalled\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			helper := filepath.Join(t.TempDir(), "apt-fixture")
			failure := "#!/bin/sh\necho 'Version unavailable' >&2\nexit 100\n"
			if wrongVersion {
				failure = "#!/bin/sh\nexit 0\n"
			}
			if err := os.WriteFile(helper, []byte(failure), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", helper)
			for range 2 {
				if err := backend.replayPackageLedger(t.Context()); err == nil || backend.packageReplayComplete {
					t.Fatalf("failed replay accepted: %v", err)
				}
			}
			if err := os.WriteFile(inventory, []byte("tree\tamd64\t2.1.0-1\tinstalled\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := backend.replayPackageLedger(t.Context()); err != nil {
				t.Fatalf("retry after recovery: %v", err)
			}
		})
	}
}

func TestLegacyPackageReplayCapturesActualVersions(t *testing.T) {
	backend := packageTestBackend(t)
	if err := backend.workspace.WriteFile(packageLedgerName, []byte(`{"entries":[{"args":["install","tree"]},{"args":["download","ffmpeg"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(t.TempDir(), "apt-fixture")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ntest \"$1\" = install\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", helper)
	if err := backend.replayPackageLedger(t.Context()); err != nil {
		t.Fatal(err)
	}
	ledger, err := backend.readPackageLedger()
	if err != nil || ledger.Version != packageLedgerVersion || len(ledger.Packages) != 2 {
		t.Fatalf("migrated ledger=%+v error=%v", ledger, err)
	}
}

func TestPackageQueryFailureAndBoundsDoNotRecordLocks(t *testing.T) {
	backend := packageTestBackend(t)
	if err := os.WriteFile(backend.packageQueryPath, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := backend.recordPackageLedger(t.Context(), []string{"install", "tree"}); err == nil {
		t.Fatal("query failure accepted")
	}
	if _, err := backend.workspace.Lstat(packageLedgerName); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed query recorded ledger: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := backend.installedSystemPackages(ctx); err == nil {
		t.Fatal("canceled query accepted")
	}
	ledger := packageLedger{Version: packageLedgerVersion, Environment: backend.packageLedgerEnvironment(), Packages: make([]systemPackage, maxSystemPackages+1)}
	if err := backend.validatePackageLedger(ledger); !errors.Is(err, agentexecution.ErrOutputLimit) {
		t.Fatalf("unbounded package inventory accepted: %v", err)
	}
}

func TestInvalidLedgerStopsAPTBeforeMutation(t *testing.T) {
	backend := packageTestBackend(t)
	backend.dependencyInstallation, backend.networkMode = true, "package-install"
	if err := backend.workspace.WriteFile(packageLedgerName, []byte(`{"entries":[{"args":["upgrade"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	marker := filepath.Join(root, "mutated")
	helper := filepath.Join(root, "apt-fixture")
	if err := os.WriteFile(helper, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", helper)
	if _, err := backend.InstallSystemPackages(t.Context(), []string{"install", "tree"}); err == nil {
		t.Fatal("installation accepted invalid retained ledger")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("APT ran before ledger validation: %v", err)
	}
}

func TestDownloadOnlyLedgerDoesNotInstallOnReplay(t *testing.T) {
	backend := packageTestBackend(t)
	if err := os.Remove(backend.packageQueryPath); err != nil {
		t.Fatal(err)
	}
	if err := backend.recordPackageLedger(t.Context(), []string{"download", "ffmpeg"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZGI_RUNTIME_AGENT_APT_PATH", filepath.Join(t.TempDir(), "must-not-run"))
	if err := backend.replayPackageLedger(t.Context()); err != nil {
		t.Fatalf("download replay attempted installation or query: %v", err)
	}
}

func TestVirtualPackageRecordsItsInstalledProviders(t *testing.T) {
	backend := packageTestBackend(t)
	// APT resolves virtual packages; the dpkg inventory only contains providers.
	if err := backend.recordPackageLedger(t.Context(), []string{"install", "virtual-provider"}); err != nil {
		t.Fatal(err)
	}
	ledger, err := backend.readPackageLedger()
	if err != nil || len(ledger.Packages) != 2 || ledger.Packages[0].Name != "poppler-utils" {
		t.Fatalf("provider inventory=%+v error=%v", ledger.Packages, err)
	}
}

func TestPackageRecoveryDiagnosticSurvivesTransportWithoutLeakingCause(t *testing.T) {
	err := fmt.Errorf("restore ledger: %w", &packageRecoveryError{cause: errors.New("fixture private path or credential")})
	status, code, message := runtimeAgentErrorResponse(err)
	if status != http.StatusServiceUnavailable || code != "package_recovery_failed" || message != packageRecoveryMessage || strings.Contains(message, "credential") {
		t.Fatalf("diagnostic: status=%d code=%s message=%s", status, code, message)
	}
	clientErr := runtimeAgentClientError(code)
	if !errors.Is(clientErr, agentexecution.ErrBackendUnavailable) || !strings.Contains(clientErr.Error(), "no latest-version fallback") {
		t.Fatalf("client diagnostic lost: %v", clientErr)
	}
}
