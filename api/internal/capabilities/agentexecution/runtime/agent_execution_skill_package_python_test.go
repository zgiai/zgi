package executionruntime

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestAgentSkillPackageUnpackProgram(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is required for the runtime unpack contract")
	}
	if info, err := os.Stat("/workspace"); err != nil || !info.IsDir() {
		t.Skip("a writable /workspace is required for the runtime unpack contract")
	}
	root, err := os.MkdirTemp("/workspace", "skill-unpack-test-")
	if err != nil {
		t.Skipf("a writable /workspace is required: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	source := testPortableSkillPackage(t)
	archive, digest, err := skills.BuildPortableSkillArchive(source)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(root, "incoming.zip")
	targetPath := filepath.Join(root, "staged")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	checksum := sha256.Sum256(archive)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "python3", "-c", agentSkillPackageUnpackProgram, archivePath, targetPath, hex.EncodeToString(checksum[:]), digest, "1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("unpack valid Skill package: %v, output = %s", err, output)
	}
	content, err := os.ReadFile(filepath.Join(targetPath, "scripts", "task.py"))
	if err != nil || string(content) != "print('done')\n" {
		t.Fatalf("unpacked script = %q, error = %v", content, err)
	}
	marker, err := os.ReadFile(filepath.Join(targetPath, ".zgi-ready"))
	if err != nil || strings.TrimSpace(string(marker)) != digest {
		t.Fatalf("completion marker = %q, error = %v", marker, err)
	}
	archivePath = filepath.Join(root, "chunked.zip")
	targetPath = filepath.Join(root, "chunked-staged")
	cut := len(archive) / 2
	for index, data := range [][]byte{archive[:cut], archive[cut:]} {
		if err := os.WriteFile(archivePath+".part."+strconv.Itoa(index), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command = exec.CommandContext(ctx, "python3", "-c", agentSkillPackageUnpackProgram, archivePath, targetPath, hex.EncodeToString(checksum[:]), digest, "2")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("unpack chunked Skill package: %v, output = %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(targetPath, "scripts", "task.py")); err != nil {
		t.Fatalf("chunked package script missing: %v", err)
	}

	var malicious bytes.Buffer
	writer := zip.NewWriter(&malicious)
	entry, err := writer.Create("../outside.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("escape")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archivePath = filepath.Join(root, "malicious.zip")
	if err := os.WriteFile(archivePath, malicious.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	checksum = sha256.Sum256(malicious.Bytes())
	command = exec.CommandContext(ctx, "python3", "-c", agentSkillPackageUnpackProgram, archivePath, filepath.Join(root, "bad-target"), hex.EncodeToString(checksum[:]), digest, "1")
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "invalid package member") {
		t.Fatalf("malicious archive error = %v, output = %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(root, "outside.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive traversal wrote outside target: %v", err)
	}
}
