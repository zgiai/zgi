package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeGrantBindsOrganizationAndPackage(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "SKILL.md")
	if err := os.WriteFile(file, []byte("synthetic package"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := SkillPackageSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	grant := ScriptRuntimeGrant{OrganizationID: "org-a", PackageSHA256: digest, DependencyProfile: "example-runtime", ProfileVersion: "1", ArtifactChecksum: "sha256:" + strings.Repeat("a", 64), MaxArtifactCount: 20, MaxArtifactBytes: 1024, MaxInputFileBytes: 1024}
	policy := filepath.Join(t.TempDir(), "grants.json")
	raw, _ := json.Marshal([]ScriptRuntimeGrant{grant})
	if err := os.WriteFile(policy, raw, 0600); err != nil {
		t.Fatal(err)
	}
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: "http://127.0.0.1:1", RuntimeGrantsPath: policy})
	match, _, err := runner.runtimeGrant(root, "org-a")
	if err != nil || match == nil {
		t.Fatalf("authorized grant: %v", err)
	}
	for _, org := range []string{"org-b", ""} {
		match, _, err = runner.runtimeGrant(root, org)
		if err != nil || match != nil {
			t.Fatalf("foreign grant accepted: %s", org)
		}
	}
	if err := os.WriteFile(file, []byte("modified package"), 0600); err != nil {
		t.Fatal(err)
	}
	match, _, err = runner.runtimeGrant(root, "org-a")
	if err != nil || match != nil {
		t.Fatal("modified package retained grant")
	}
}

func TestRuntimeGrantFailsClosed(t *testing.T) {
	runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: "http://127.0.0.1:1", RuntimeGrantsPath: filepath.Join(t.TempDir(), "missing.json")})
	if _, _, err := runner.runtimeGrant(t.TempDir(), "org-a"); err == nil {
		t.Fatal("missing configured policy accepted")
	}
	root := t.TempDir()
	if err := os.Symlink("/etc/hosts", filepath.Join(root, "script")); err != nil {
		t.Fatal(err)
	}
	if _, err := SkillPackageSHA256(root); err == nil {
		t.Fatal("symlink package accepted")
	}
}

// The HTTP server is a test double; this checks authorization wiring and cleanup,
// not live sandbox correctness.
func TestRuntimeGrantVerifiesCreatedEnvironmentBeforeUpload(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(fmt.Sprint(mismatch), func(t *testing.T) {
			root := writeTestScriptSkill(t)
			if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(`{"entrypoint":"scripts/run.py","dependency_profile":"workflow-safe"}`), 0600); err != nil {
				t.Fatal(err)
			}
			digest, err := SkillPackageSHA256(root)
			if err != nil {
				t.Fatal(err)
			}
			fake := newFakeSandboxServer(t)
			defer fake.server.Close()
			checksum := "sha256:" + strings.Repeat("a", 64)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/sbx_test" {
					version := "1"
					if mismatch {
						version = "2"
					}
					writeSandboxOK(t, w, map[string]interface{}{"dependency_profile_version": version, "dependency_artifact_checksum": checksum})
					return
				}
				fake.server.Config.Handler.ServeHTTP(w, r)
			}))
			defer server.Close()
			runner := NewSandboxScriptRunner(SandboxScriptRunnerConfig{Endpoint: server.URL})
			runner.runtimeGrants = []ScriptRuntimeGrant{{OrganizationID: "org-a", PackageSHA256: digest, DependencyProfile: "workflow-safe", ProfileVersion: "1", ArtifactChecksum: checksum, MaxArtifactCount: 10, MaxArtifactBytes: maxSkillScriptArtifactBytes, MaxInputFileBytes: maxSkillScriptInputFileBytes}}
			doc := scriptSkillDocument(root)
			doc.Metadata.Source = SkillSourceCustom
			result, err := runner.RunSkillScript(context.Background(), doc, map[string]interface{}{}, ExecutionContext{OrganizationID: "org-a"}, "grant-test")
			if mismatch {
				if err == nil || fake.uploadArchiveRequests != 0 {
					t.Fatalf("mismatched environment used: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if result.Trace.Result["package_sha256"] != digest {
					t.Fatal("missing package execution identity")
				}
			}
			if fake.deleted != 1 {
				t.Fatal("sandbox was not cleaned up")
			}
		})
	}
}
