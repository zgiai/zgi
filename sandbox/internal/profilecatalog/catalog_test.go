package profilecatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRepositoryProfileSources(t *testing.T) {
	profiles, err := Load(os.DirFS("../../profiles"))
	if err != nil {
		t.Fatalf("load repository profile sources: %v", err)
	}
	byName := map[string]Profile{}
	for _, profile := range profiles {
		byName[profile.Name] = profile
	}
	if byName["stdlib"].Status != "ready" || !byName["stdlib"].Enabled {
		t.Fatalf("expected ready stdlib profile, got %+v", byName["stdlib"])
	}
	office := byName["skill-office"]
	if office.Status != "disabled" || office.Enabled {
		t.Fatalf("expected disabled skill-office source profile, got %+v", office)
	}
	if len(office.Packages) == 0 || !containsLanguage(office.Languages, "python3") || !containsLanguage(office.Languages, "nodejs") {
		t.Fatalf("expected skill-office package and language metadata, got %+v", office)
	}
}

func TestLoadRejectsMissingRequiredFile(t *testing.T) {
	root := t.TempDir()
	profileDir := filepath.Join(root, "bad-profile")
	if err := os.Mkdir(profileDir, 0o755); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	manifest := `{
		"name":"bad-profile",
		"version":"2026.05.31",
		"status":"disabled",
		"enabled":false,
		"owner_scope":"global",
		"languages":["python3"],
		"base_runtime":"linux-secure",
		"checksum":"profile-source:bad-profile:2026.05.31",
		"estimated_size_bytes":1,
		"required_files":["manifest.json","missing.lock"]
	}`
	if err := os.WriteFile(filepath.Join(profileDir, "manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := Load(os.DirFS(root))
	if err == nil || !strings.Contains(err.Error(), "required file is missing") {
		t.Fatalf("expected missing required file rejection, got %v", err)
	}
}

func TestValidateRejectsEnabledNonReadyProfile(t *testing.T) {
	profile := Profile{
		Name:               "bad-profile",
		Version:            "2026.05.31",
		Status:             "disabled",
		Enabled:            true,
		OwnerScope:         "global",
		Languages:          []string{"python3"},
		BaseRuntime:        "linux-secure",
		Checksum:           "profile-source:bad-profile:2026.05.31",
		EstimatedSizeBytes: 1,
		RequiredFiles:      []string{"manifest.json"},
	}
	err := Validate(os.DirFS("../../profiles/stdlib"), "", profile)
	if err == nil || !strings.Contains(err.Error(), "must not be enabled") {
		t.Fatalf("expected enabled non-ready rejection, got %v", err)
	}
}

func TestValidateRejectsUnpinnedPackage(t *testing.T) {
	profile := Profile{
		Name:               "bad-profile",
		Version:            "2026.05.31",
		Status:             "ready",
		Enabled:            true,
		OwnerScope:         "global",
		Languages:          []string{"python3"},
		BaseRuntime:        "linux-secure",
		Checksum:           "profile-source:bad-profile:2026.05.31",
		EstimatedSizeBytes: 1,
		RequiredFiles:      []string{"manifest.json"},
		Packages:           []Package{{Ecosystem: "python3", Name: "requests", Version: "latest"}},
	}
	err := Validate(os.DirFS("../../profiles/stdlib"), "", profile)
	if err == nil || !strings.Contains(err.Error(), "version must be pinned") {
		t.Fatalf("expected unpinned package rejection, got %v", err)
	}
}

func containsLanguage(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
