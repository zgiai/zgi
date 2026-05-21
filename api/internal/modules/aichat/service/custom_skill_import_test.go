package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestExtractSkillZipRejectsPathTraversal(t *testing.T) {
	data := testSkillZip(t, map[string]string{
		"SKILL.md": "../evil",
		"../x.md":  "bad",
	})

	_, err := extractSkillZip(data, t.TempDir())
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("extractSkillZip() error = %v, want invalid input", err)
	}
}

func TestFilesystemCustomSkillStoragePreviewAndPublish(t *testing.T) {
	root := t.TempDir()
	storage := newFilesystemCustomSkillStorage(root)
	orgID := uuid.New()
	importID := uuid.New().String()
	data := testSkillZip(t, map[string]string{
		"SKILL.md":       testCustomSkillMarkdown(),
		"guide.md":       "reference",
		"scripts/run.py": "print('skip')",
	})

	preview, err := storage.SavePreviewPackage(context.Background(), orgID, importID, data)
	if err != nil {
		t.Fatalf("SavePreviewPackage() error = %v", err)
	}
	if preview.FileCount != 3 || preview.TotalSize == 0 || preview.ExpiresAt.Before(time.Now()) {
		t.Fatalf("preview = %#v, want extracted metadata", preview)
	}
	loaded, err := storage.LoadPreview(context.Background(), orgID, importID)
	if err != nil {
		t.Fatalf("LoadPreview() error = %v", err)
	}
	published, finalRoot, err := storage.PublishPreview(context.Background(), loaded, "brief-writer")
	if err != nil {
		t.Fatalf("PublishPreview() error = %v", err)
	}
	defer published.cleanup()
	if _, err := os.Stat(filepath.Join(finalRoot, "SKILL.md")); err != nil {
		t.Fatalf("published SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(finalRoot, customSkillPreviewStateFile)); !os.IsNotExist(err) {
		t.Fatalf("preview state was published, stat error = %v", err)
	}
}

func TestFilesystemCustomSkillStorageDeletePreviewIsIdempotent(t *testing.T) {
	root := t.TempDir()
	storage := newFilesystemCustomSkillStorage(root)
	orgID := uuid.New()
	importID := uuid.New().String()
	data := testSkillZip(t, map[string]string{
		"SKILL.md": testCustomSkillMarkdown(),
	})

	if _, err := storage.SavePreviewPackage(context.Background(), orgID, importID, data); err != nil {
		t.Fatalf("SavePreviewPackage() error = %v", err)
	}
	if err := storage.DeletePreview(context.Background(), orgID, importID); err != nil {
		t.Fatalf("DeletePreview() error = %v", err)
	}
	if err := storage.DeletePreview(context.Background(), orgID, importID); err != nil {
		t.Fatalf("DeletePreview(second) error = %v", err)
	}
	_, err := storage.LoadPreview(context.Background(), orgID, importID)
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadPreview() error = %v, want not found", err)
	}
}

func TestFilesystemCustomSkillStorageDeletePreviewRejectsInvalidImportID(t *testing.T) {
	storage := newFilesystemCustomSkillStorage(t.TempDir())

	err := storage.DeletePreview(context.Background(), uuid.New(), "../bad")
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("DeletePreview() error = %v, want invalid input", err)
	}
}

func TestFilesystemCustomSkillStorageCleanupExpiredPreviewsScansOrganizations(t *testing.T) {
	root := t.TempDir()
	storage := newFilesystemCustomSkillStorage(root)
	now := time.Now()
	expiredOrgID := uuid.New()
	activeOrgID := uuid.New()
	expiredID := uuid.New().String()
	activeID := uuid.New().String()
	data := testSkillZip(t, map[string]string{
		"SKILL.md": testCustomSkillMarkdown(),
	})

	expiredPreview, err := storage.SavePreviewPackage(context.Background(), expiredOrgID, expiredID, data)
	if err != nil {
		t.Fatalf("SavePreviewPackage(expired) error = %v", err)
	}
	expiredPreview.ExpiresAt = now.Add(-time.Minute)
	if err := storage.(*filesystemCustomSkillStorage).writePreviewState(expiredPreview); err != nil {
		t.Fatalf("write expired preview state: %v", err)
	}
	activePreview, err := storage.SavePreviewPackage(context.Background(), activeOrgID, activeID, data)
	if err != nil {
		t.Fatalf("SavePreviewPackage(active) error = %v", err)
	}
	activePreview.ExpiresAt = now.Add(time.Minute)
	if err := storage.(*filesystemCustomSkillStorage).writePreviewState(activePreview); err != nil {
		t.Fatalf("write active preview state: %v", err)
	}

	storage.CleanupExpiredPreviews(context.Background(), now)

	if _, err := storage.LoadPreview(context.Background(), expiredOrgID, expiredID); err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadPreview(expired) error = %v, want not found", err)
	}
	if _, err := storage.LoadPreview(context.Background(), activeOrgID, activeID); err != nil {
		t.Fatalf("LoadPreview(active) error = %v", err)
	}
}

func TestValidateSkillConfigIDsRejectsInvalidSkill(t *testing.T) {
	_, err := validateSkillConfigIDs([]string{"broken-skill"}, []skills.SkillDiscoveryMetadata{
		{ID: "broken-skill", Status: skills.SkillStatusInvalid},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown skill id broken-skill") {
		t.Fatalf("validateSkillConfigIDs() error = %v, want unknown invalid skill", err)
	}
}

func TestCustomSkillIDConflictsWithBrokenSystemSkill(t *testing.T) {
	catalogDir := t.TempDir()
	skillDir := filepath.Join(catalogDir, "brief-writer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir system skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: brief-writer
description: Broken system skill.
provider_type: builtin
provider_id: missing
tools:
  - missing_tool
---

# Broken
`), 0o644); err != nil {
		t.Fatalf("write system skill: %v", err)
	}
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir)
	svc := &service{skillRuntime: runtime}

	if !svc.customSkillIDConflictsWithSystem(context.Background(), "brief-writer") {
		t.Fatal("customSkillIDConflictsWithSystem() = false, want true for existing system id")
	}
}

func testSkillZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip file: %v", err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatalf("write zip file: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func testCustomSkillMarkdown() string {
	return `---
name: brief-writer
description: Help draft short writing briefs.
---

# Brief Writer

Use the references before drafting a brief.
`
}
