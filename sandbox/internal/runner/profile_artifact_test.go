package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDependencyProfileActivationValidatesBuiltArtifact(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")
	profileRoot := testRootFSDir(t, "profile-root")
	profileDir := filepath.Join(profileRoot, "opt", "zgi", "profiles", "skill-office")
	writeBuiltProfileArtifact(t, profileDir, "skill-office", map[string]string{
		"venv/bin/python":       "python",
		"node_modules/pkg.json": "{}",
	})
	dependencyRoot := t.TempDir()
	if err := os.Rename(profileRoot, filepath.Join(dependencyRoot, "skill-office")); err != nil {
		t.Fatalf("move profile rootfs: %v", err)
	}

	activation, err := resolveDependencyProfileActivation(defaultRoot, dependencyRoot, "skill-office", "")
	if err != nil {
		t.Fatalf("resolve activation: %v", err)
	}
	if activation.RootFS != filepath.Join(dependencyRoot, "skill-office") {
		t.Fatalf("expected profile rootfs, got %+v", activation)
	}
	if activation.ProfileHostDir != filepath.Join(dependencyRoot, "skill-office", "opt", "zgi", "profiles", "skill-office") {
		t.Fatalf("expected profile host dir, got %+v", activation)
	}
	if activation.ProfileContainerDir != "/opt/zgi/profiles/skill-office" {
		t.Fatalf("expected profile container dir, got %+v", activation)
	}
	if activation.ProfileChecksum == "" || activation.ProfileSizeBytes <= 0 {
		t.Fatalf("expected checksum and size, got %+v", activation)
	}
	if activation.ProfileEnv["NODE_PATH"] != "/opt/zgi/profiles/skill-office/node_modules" {
		t.Fatalf("expected profile env, got %+v", activation.ProfileEnv)
	}
}

func TestResolveDependencyProfileActivationRejectsMissingArtifact(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")
	dependencyRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(dependencyRoot, "skill-office"), 0o755); err != nil {
		t.Fatalf("create profile rootfs: %v", err)
	}

	_, err := resolveDependencyProfileActivation(defaultRoot, dependencyRoot, "skill-office", "")
	if err == nil || !strings.Contains(err.Error(), "artifact") {
		t.Fatalf("expected missing artifact rejection, got %v", err)
	}
}

func TestResolveDependencyProfileActivationKeepsProfileEnvWithoutProfileRootFS(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")

	activation, err := resolveDependencyProfileActivation(defaultRoot, "", "skill-office", "")
	if err != nil {
		t.Fatalf("resolve activation: %v", err)
	}
	if activation.RootFS != defaultRoot {
		t.Fatalf("expected default rootfs, got %+v", activation)
	}
	if activation.ProfileHostDir != "" || activation.ProfileChecksum != "" {
		t.Fatalf("expected no artifact metadata without profile rootfs, got %+v", activation)
	}
	if activation.ProfileEnv["NODE_PATH"] != "/opt/zgi/profiles/skill-office/node_modules" {
		t.Fatalf("expected profile env to be preserved, got %+v", activation.ProfileEnv)
	}
}

func TestResolveDependencyProfileActivationUsesReusableArtifactChecksum(t *testing.T) {
	defaultRoot := testRootFSDir(t, "default")
	artifactRoot := testRootFSDir(t, "artifact-root")
	profileDir := filepath.Join(artifactRoot, "opt", "zgi", "profiles", "skill-office")
	writeBuiltProfileArtifact(t, profileDir, "skill-office", map[string]string{
		"venv/bin/python": "python",
	})
	manifest, err := validateBuiltProfileArtifact(profileDir, "skill-office")
	if err != nil {
		t.Fatalf("validate artifact: %v", err)
	}
	dependencyRoot := t.TempDir()
	artifactKey := dependencyArtifactRuntimeKey(manifest.Build.Checksum)
	if err := os.Rename(artifactRoot, filepath.Join(dependencyRoot, artifactKey)); err != nil {
		t.Fatalf("move artifact rootfs: %v", err)
	}

	activation, err := resolveDependencyProfileActivation(defaultRoot, dependencyRoot, "team-data", manifest.Build.Checksum)
	if err != nil {
		t.Fatalf("resolve activation: %v", err)
	}
	if activation.RootFS != filepath.Join(dependencyRoot, artifactKey) {
		t.Fatalf("expected artifact rootfs, got %+v", activation)
	}
	if activation.ProfileHostDir != filepath.Join(dependencyRoot, artifactKey, "opt", "zgi", "profiles", "skill-office") {
		t.Fatalf("expected reusable artifact host dir, got %+v", activation)
	}
	if activation.ProfileContainerDir != "/opt/zgi/profiles/team-data" {
		t.Fatalf("expected selected profile container alias, got %+v", activation)
	}
	if activation.ProfileEnv["NODE_PATH"] != "/opt/zgi/profiles/team-data/node_modules" {
		t.Fatalf("expected selected profile env alias, got %+v", activation.ProfileEnv)
	}
	if activation.ProfileChecksum != manifest.Build.Checksum {
		t.Fatalf("expected artifact checksum, got %+v", activation)
	}
}

func TestValidateBuiltProfileArtifactRejectsMismatchedName(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profile")
	writeBuiltProfileArtifact(t, profileDir, "other-profile", map[string]string{"data.txt": "ok"})

	_, err := validateBuiltProfileArtifact(profileDir, "skill-office")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatched name rejection, got %v", err)
	}
}

func TestValidateBuiltProfileArtifactRejectsChecksumMismatch(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profile")
	writeBuiltProfileArtifact(t, profileDir, "skill-office", map[string]string{"data.txt": "ok"})
	if err := os.WriteFile(filepath.Join(profileDir, "data.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatalf("mutate artifact: %v", err)
	}

	_, err := validateBuiltProfileArtifact(profileDir, "skill-office")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch rejection, got %v", err)
	}
}

func TestValidateBuiltProfileArtifactRejectsUnverifiedManifest(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profile")
	writeBuiltProfileArtifact(t, profileDir, "skill-office", map[string]string{"data.txt": "ok"})
	raw, err := os.ReadFile(filepath.Join(profileDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest builtProfileManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	manifest.Build.VerificationPassed = false
	raw, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err = validateBuiltProfileArtifact(profileDir, "skill-office")
	if err == nil || !strings.Contains(err.Error(), "verification has not passed") {
		t.Fatalf("expected verification rejection, got %v", err)
	}
}

func TestValidateBuiltProfileArtifactRejectsSymlink(t *testing.T) {
	profileDir := filepath.Join(t.TempDir(), "profile")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(profileDir, "data.txt")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	manifest := builtProfileManifest{
		Name:    "skill-office",
		Version: "2026.05.31",
		Build: profileBuildMetadata{
			Checksum:           "sha256:bad",
			SizeBytes:          1,
			VerificationPassed: true,
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err = validateBuiltProfileArtifact(profileDir, "skill-office")
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestListDependencyProfileArtifactsReadsRootFSArtifacts(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, "skill-office", "opt", "zgi", "profiles", "skill-office")
	writeBuiltProfileArtifact(t, profileDir, "skill-office", map[string]string{"venv/bin/python": "python"})

	artifacts, err := ListDependencyProfileArtifacts(root)
	if err != nil {
		t.Fatalf("list dependency profile artifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("expected one artifact, got %+v", artifacts)
	}
	artifact := artifacts[0]
	if artifact.Name != "skill-office" || artifact.Version != "2026.05.31" || artifact.Checksum == "" || artifact.SizeBytes <= 0 {
		t.Fatalf("unexpected artifact metadata: %+v", artifact)
	}
	if len(artifact.Languages) != 2 || artifact.Languages[0] != "python3" || artifact.Packages[0].Name != "office-tools" {
		t.Fatalf("unexpected profile catalog metadata: %+v", artifact)
	}
}

func writeBuiltProfileArtifact(t *testing.T, profileDir string, profile string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	for name, content := range files {
		path := filepath.Join(profileDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create artifact parent: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write artifact file: %v", err)
		}
	}
	checksum, size, err := checksumProfileArtifactDir(profileDir)
	if err != nil {
		t.Fatalf("checksum artifact: %v", err)
	}
	manifest := builtProfileManifest{
		Name:        profile,
		Version:     "2026.05.31",
		Status:      "disabled",
		Enabled:     false,
		OwnerScope:  "global",
		Languages:   []string{"python3", "nodejs"},
		BaseRuntime: "linux-secure",
		Checksum:    "profile-source:" + profile + ":2026.05.31",
		Description: "Managed document automation profile.",
		Packages: []builtProfilePackage{
			{Ecosystem: "python3", Name: "office-tools", Version: "managed"},
			{Ecosystem: "nodejs", Name: "office-tools", Version: "managed"},
		},
		Build: profileBuildMetadata{
			Checksum:           checksum,
			SizeBytes:          size,
			VerificationPassed: true,
		},
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "manifest.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
