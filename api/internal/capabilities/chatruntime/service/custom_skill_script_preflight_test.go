package service

import (
	"archive/zip"
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm"
)

func TestPreviewImportCustomSkillPreflightsScriptManifest(t *testing.T) {
	svc := newScriptPreflightService(t)
	scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New(), SkipAccessCheck: true}
	preview, err := svc.PreviewImportCustomSkill(context.Background(), scope, scriptPreflightFileHeader(t, map[string]string{
		"SKILL.md": `---
name: invalid-bundled-package
description: Verify bundled package import validation.
runtime_type: prompt
---
Run the script.
`,
		"scripts/run.py": "print('{}')\n",
		"skill.manifest.json": `{
  "entrypoint":"scripts/run.py",
  "language":"python3",
  "bundled_packages":[{"name":"missing_local","path":"scripts/missing_local"}]
}`,
	}))
	if err != nil {
		t.Fatalf("PreviewImportCustomSkill() error = %v", err)
	}
	if preview.CanImport || preview.ImportID != "" || len(preview.ValidationErrors) != 1 {
		t.Fatalf("preview = %#v, want rejected validation preview", preview)
	}
	if !strings.Contains(preview.ValidationErrors[0], "bundled package missing_local") {
		t.Fatalf("ValidationErrors = %#v, want bundled package error", preview.ValidationErrors)
	}
}

func TestPreviewImportCustomSkillKeepsLegacyScriptPackageCompatible(t *testing.T) {
	svc := newScriptPreflightService(t)
	scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New(), SkipAccessCheck: true}
	preview, err := svc.PreviewImportCustomSkill(context.Background(), scope, scriptPreflightFileHeader(t, map[string]string{
		"SKILL.md": `---
name: legacy-script-package
description: Verify manifest-free script compatibility.
runtime_type: prompt
---
Run the script.
`,
		"scripts/run.py": "print('{}')\n",
	}))
	if err != nil {
		t.Fatalf("PreviewImportCustomSkill() error = %v", err)
	}
	if !preview.CanImport || preview.ImportID == "" || len(preview.ValidationErrors) != 0 {
		t.Fatalf("preview = %#v, want importable legacy package", preview)
	}
}

func TestConfirmCustomSkillImportRepeatsScriptPreflight(t *testing.T) {
	svc := newScriptPreflightService(t)
	scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New(), SkipAccessCheck: true}
	preview, err := svc.PreviewImportCustomSkill(context.Background(), scope, scriptPreflightFileHeader(t, map[string]string{
		"SKILL.md": `---
name: confirm-script-preflight
description: Verify confirmation repeats script package validation.
runtime_type: prompt
---
Run the script.
`,
		"scripts/run.py":                "from zgi_local import run\nprint(run())\n",
		"scripts/zgi_local/__init__.py": "def run(): return {}\n",
		"skill.manifest.json": `{
  "entrypoint":"scripts/run.py",
  "language":"python3",
  "bundled_packages":[{"name":"zgi_local","path":"scripts/zgi_local"}]
}`,
	}))
	if err != nil || !preview.CanImport {
		t.Fatalf("PreviewImportCustomSkill() preview = %#v, error = %v", preview, err)
	}
	stored, err := svc.customSkillStorage.LoadPreview(context.Background(), scope.OrganizationID, preview.ImportID)
	if err != nil {
		t.Fatalf("LoadPreview() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(stored.Root, "scripts", "zgi_local")); err != nil {
		t.Fatalf("remove bundled package: %v", err)
	}

	_, err = svc.ConfirmCustomSkillImport(context.Background(), scope, preview.ImportID, false)
	if err == nil || !strings.Contains(err.Error(), "bundled package zgi_local") {
		t.Fatalf("ConfirmCustomSkillImport() error = %v, want repeated bundled package validation", err)
	}
}

func newScriptPreflightService(t *testing.T) *service {
	t.Helper()
	return &service{
		skillRuntime:       skills.NewRuntimeWithCatalog(nil, nil, t.TempDir()),
		customSkillStorage: newFilesystemCustomSkillStorage(t.TempDir()),
		repos: &repository.Repositories{
			CustomSkill: scriptPreflightCustomSkillRepository{},
		},
	}
}

type scriptPreflightCustomSkillRepository struct{}

func (scriptPreflightCustomSkillRepository) ListByOrganization(context.Context, uuid.UUID) ([]*runtimemodel.CustomSkill, error) {
	return nil, nil
}

func (scriptPreflightCustomSkillRepository) ListManageableByOrganization(context.Context, uuid.UUID) ([]*runtimemodel.CustomSkill, error) {
	return nil, nil
}

func (scriptPreflightCustomSkillRepository) GetBySkillID(context.Context, uuid.UUID, string) (*runtimemodel.CustomSkill, error) {
	return nil, gorm.ErrRecordNotFound
}

func (scriptPreflightCustomSkillRepository) Upsert(context.Context, *runtimemodel.CustomSkill) error {
	return nil
}

func (scriptPreflightCustomSkillRepository) DeleteBySkillID(context.Context, uuid.UUID, string) error {
	return nil
}

func scriptPreflightFileHeader(t *testing.T, files map[string]string) *multipart.FileHeader {
	t.Helper()
	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	for name, content := range files {
		entry, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("zip Create(%q) error = %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("zip Write(%q) error = %v", name, err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}

	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	part, err := multipartWriter.CreateFormFile("file", "skill.zip")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(archive.Bytes()); err != nil {
		t.Fatalf("multipart Write() error = %v", err)
	}
	if err := multipartWriter.Close(); err != nil {
		t.Fatalf("multipart Close() error = %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, "/skills/import/preview", &body)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	if err := request.ParseMultipartForm(int64(body.Len()) + 1024); err != nil {
		t.Fatalf("ParseMultipartForm() error = %v", err)
	}
	return request.MultipartForm.File["file"][0]
}
