package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCustomScriptDiscoveryDoesNotChangeSystemDiscovery(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "core/helper.py"), []byte("VALUE = 30\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hasLocationScripts(skillLocation{Root: root, Source: SkillSourceCustom}) {
		t.Fatal("custom core module was not discovered")
	}
	if hasLocationScripts(skillLocation{Root: root, Source: SkillSourceSystem}) {
		t.Fatal("system discovery must keep the scripts directory contract")
	}
}

func TestCustomScriptDiscoveryDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	module := filepath.Join(other, "helper.py")
	if err := os.WriteFile(module, []byte("VALUE = 30\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(module, filepath.Join(root, "helper.py")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(other, filepath.Join(root, "core")); err != nil {
		t.Fatal(err)
	}
	if hasCustomScripts(root) {
		t.Fatal("symlinked module must not grant session script discovery")
	}
	if _, _, err := BuildPortableSkillArchive(root); err == nil {
		t.Fatal("symlinked package must fail archive validation")
	}
}

func TestCustomScriptDiscoveryKeepsInstructionOnlySkill(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "usage.md"), []byte("Use Python to analyze input.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasCustomScripts(root) {
		t.Fatal("instruction text is not an executable module")
	}
}
