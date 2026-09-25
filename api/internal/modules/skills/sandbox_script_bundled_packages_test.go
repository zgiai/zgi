package skills

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPrepareSkillScriptManifestValidatesBundledPackages(t *testing.T) {
	t.Run("python package", func(t *testing.T) {
		root := writeBundledPackageSkill(t, "python3", "scripts/run.py", "zgi_local", "scripts/zgi_local")
		if err := os.WriteFile(filepath.Join(root, "scripts", "zgi_local", "__init__.py"), []byte("VALUE = 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		prepared, err := prepareSkillScriptManifest(root, 30, false)
		if err != nil {
			t.Fatalf("prepareSkillScriptManifest() error = %v", err)
		}
		if got := prepared.Manifest.BundledPackages; len(got) != 1 || got[0].Name != "zgi_local" || got[0].Path != "scripts/zgi_local" {
			t.Fatalf("BundledPackages = %#v", got)
		}
	})

	t.Run("node package", func(t *testing.T) {
		root := writeBundledPackageSkill(t, "nodejs", "scripts/run.js", "zgi-local", "scripts/zgi-local")
		if err := os.WriteFile(filepath.Join(root, "scripts", "zgi-local", "index.js"), []byte("module.exports = {};\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := prepareSkillScriptManifest(root, 30, false); err != nil {
			t.Fatalf("prepareSkillScriptManifest() error = %v", err)
		}
	})

	tests := []struct {
		name        string
		packageName string
		packagePath string
		prepare     func(t *testing.T, root string)
		wantError   string
	}{
		{name: "missing name", packagePath: "scripts/zgi_local", wantError: "name is required"},
		{name: "path escape", packageName: "zgi_local", packagePath: "../zgi_local", wantError: "path must be under scripts"},
		{name: "scripts subtree escape", packageName: "zgi_local", packagePath: "scripts/../zgi_local", wantError: "path must be under scripts"},
		{
			name:        "missing path",
			packageName: "zgi_local",
			packagePath: "scripts/zgi_local",
			prepare: func(t *testing.T, root string) {
				if err := os.RemoveAll(filepath.Join(root, "scripts", "zgi_local")); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "path is missing",
		},
		{
			name:        "not importable from entrypoint",
			packageName: "zgi_local",
			packagePath: "scripts/lib/zgi_local",
			prepare: func(t *testing.T, root string) {
				if err := os.MkdirAll(filepath.Join(root, "scripts", "lib", "zgi_local"), 0o700); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "must resolve from the entrypoint directory",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeBundledPackageSkill(t, "python3", "scripts/run.py", test.packageName, test.packagePath)
			if test.prepare != nil {
				test.prepare(t, root)
			}
			_, err := prepareSkillScriptManifest(root, 30, false)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("prepareSkillScriptManifest() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestPrepareSkillScriptManifestRejectsDuplicateBundledPackages(t *testing.T) {
	root := writeBundledPackageSkill(t, "python3", "scripts/run.py", "zgi_local", "scripts/zgi_local")
	if err := os.WriteFile(filepath.Join(root, "scripts", "zgi_local", "__init__.py"), []byte("VALUE = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"entrypoint":"scripts/run.py",
		"language":"python3",
		"bundled_packages":[
			{"name":"zgi_local","path":"scripts/zgi_local"},
			{"name":"ZGI_LOCAL","path":"scripts/other"}
		]
	}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := prepareSkillScriptManifest(root, 30, false)
	if err == nil || !strings.Contains(err.Error(), "name is duplicated") {
		t.Fatalf("prepareSkillScriptManifest() error = %v, want duplicate name error", err)
	}
}

func TestPrepareSkillScriptManifestRejectsDuplicateBundledPackagePaths(t *testing.T) {
	root := writeBundledPackageSkill(t, "python3", "scripts/run.py", "zgi_local", "scripts/zgi_local")
	manifest := `{
		"entrypoint":"scripts/run.py",
		"language":"python3",
		"bundled_packages":[
			{"name":"zgi_local","path":"scripts/zgi_local"},
			{"name":"zgi_other","path":"scripts/zgi_local"}
		]
	}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := prepareSkillScriptManifest(root, 30, false)
	if err == nil || !strings.Contains(err.Error(), "path is duplicated") {
		t.Fatalf("prepareSkillScriptManifest() error = %v, want duplicate path error", err)
	}
}

func TestPrepareSkillScriptManifestValidatesEntrypointLanguageAndInputFiles(t *testing.T) {
	t.Run("entrypoint language mismatch", func(t *testing.T) {
		root := writeBundledPackageSkill(t, "nodejs", "scripts/run.py", "zgi-local", "scripts/zgi-local")
		_, err := prepareSkillScriptManifest(root, 30, false)
		if err == nil || !strings.Contains(err.Error(), "nodejs entrypoint must use .js") {
			t.Fatalf("prepareSkillScriptManifest() error = %v, want entrypoint language error", err)
		}
	})

	tests := []struct {
		name      string
		items     string
		wantError string
	}{
		{
			name:      "duplicate name",
			items:     `[{"name":"source","argument":"source_id"},{"name":"source","argument":"other_id"}]`,
			wantError: "input file name is duplicated",
		},
		{
			name:      "duplicate argument",
			items:     `[{"name":"source","argument":"file_id"},{"name":"attachment","argument":"file_id"}]`,
			wantError: "input file argument is duplicated",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := writeBundledPackageSkill(t, "python3", "scripts/run.py", "zgi_local", "scripts/zgi_local")
			manifest := `{"entrypoint":"scripts/run.py","language":"python3","input_files":` + test.items + `}`
			if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(manifest), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := prepareSkillScriptManifest(root, 30, false)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("prepareSkillScriptManifest() error = %v, want %q", err, test.wantError)
			}
		})
	}
}

func TestPrepareSkillScriptManifestRejectsBundledPackageSymlink(t *testing.T) {
	root := writeBundledPackageSkill(t, "python3", "scripts/run.py", "zgi_local", "scripts/zgi_local")
	target := filepath.Join(root, "actual")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "scripts", "zgi_local")
	if err := os.RemoveAll(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	_, err := prepareSkillScriptManifest(root, 30, false)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("prepareSkillScriptManifest() error = %v, want symlink error", err)
	}
}

func TestValidateCustomSkillScriptPackagePreflightsManifest(t *testing.T) {
	root := writeBundledPackageSkill(t, "python3", "scripts/run.py", "missing_local", "scripts/missing_local")
	if err := os.RemoveAll(filepath.Join(root, "scripts", "missing_local")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(`---
name: bundled-preflight
description: Verify script package preflight.
runtime_type: prompt
---
Run the script.
`), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ValidateCustomSkillScriptPackage(root, 30)
	if err == nil || !strings.Contains(err.Error(), "bundled package missing_local") {
		t.Fatalf("ValidateCustomSkillScriptPackage() error = %v, want bundled package validation error", err)
	}
}

func writeBundledPackageSkill(t *testing.T, language string, entrypoint string, packageName string, packagePath string) string {
	t.Helper()
	root := t.TempDir()
	entrypointPath := filepath.Join(root, filepath.FromSlash(entrypoint))
	if err := os.MkdirAll(filepath.Dir(entrypointPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypointPath, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if packagePath != "" && !strings.Contains(packagePath, "..") {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(packagePath)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	manifest := `{
		"entrypoint":` + quoteJSON(entrypoint) + `,
		"language":` + quoteJSON(language) + `,
		"bundled_packages":[{"name":` + quoteJSON(packageName) + `,"path":` + quoteJSON(packagePath) + `}]
	}`
	if err := os.WriteFile(filepath.Join(root, "skill.manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func quoteJSON(value string) string {
	return strconv.Quote(value)
}
