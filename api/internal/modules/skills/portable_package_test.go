package skills

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPortableSkillArchiveUsesStablePackageDigest(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"SKILL.md":          "---\nname: portable-example\ndescription: Example.\n---\nRun the script.\n",
		"scripts/report.py": "print('ok')\n",
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wantDigest, err := SkillPackageSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	archive, digest, err := BuildPortableSkillArchive(root)
	if err != nil {
		t.Fatal(err)
	}
	if digest != wantDigest {
		t.Fatalf("archive digest = %q, want %q", digest, wantDigest)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 2 || reader.File[0].Name != "SKILL.md" || reader.File[1].Name != "scripts/report.py" {
		t.Fatalf("archive members = %#v, want SKILL.md and scripts/report.py", reader.File)
	}
	workspacePath, err := PortableSkillWorkspacePath("portable-example", digest)
	if err != nil || workspacePath != ".zgi/skills/portable-example/"+digest {
		t.Fatalf("workspace path = %q, error = %v", workspacePath, err)
	}
}

func TestPortableSkillWorkspacePathRejectsInvalidIdentity(t *testing.T) {
	for _, id := range []string{"../other", "bad/skill", "has spaces"} {
		if _, err := PortableSkillWorkspacePath(id, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
			t.Fatalf("invalid Skill ID %q was accepted", id)
		}
	}
	if _, err := PortableSkillWorkspacePath("valid-skill", "not-a-digest"); err == nil {
		t.Fatal("invalid digest was accepted")
	}
}
