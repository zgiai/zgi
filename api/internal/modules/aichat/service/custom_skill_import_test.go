//go:build legacy_aichat_service
// +build legacy_aichat_service

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
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
	writeSystemSkill(t, catalogDir, "brief-writer")
	runtime := skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir)
	svc := &service{skillRuntime: runtime}

	if !svc.customSkillIDConflictsWithSystem(context.Background(), "brief-writer") {
		t.Fatal("customSkillIDConflictsWithSystem() = false, want true for existing system id")
	}
}

func TestPreviewImportCustomSkillRejectsSystemSkillNameWithFriendlyMessage(t *testing.T) {
	svc, scope, _ := testCustomSkillServiceWithSystemSkill(t, "brief-writer")

	preview, err := svc.PreviewImportCustomSkill(context.Background(), scope, testSkillFileHeader(t, testSkillZip(t, map[string]string{
		"SKILL.md": testCustomSkillMarkdown(),
	})))
	if err != nil {
		t.Fatalf("PreviewImportCustomSkill() error = %v", err)
	}
	if preview.CanImport {
		t.Fatal("PreviewImportCustomSkill().CanImport = true, want false")
	}
	if len(preview.ValidationErrors) != 1 || preview.ValidationErrors[0] != customSkillSystemNameConflictMessage {
		t.Fatalf("ValidationErrors = %#v, want friendly system conflict message", preview.ValidationErrors)
	}
}

func TestConfirmCustomSkillImportRejectsSystemSkillNameWithFriendlyMessage(t *testing.T) {
	svc, scope, _ := testCustomSkillServiceWithSystemSkill(t, "brief-writer")
	importID := uuid.NewString()
	if _, err := svc.customSkillStorage.SavePreviewPackage(context.Background(), scope.OrganizationID, importID, testSkillZip(t, map[string]string{
		"SKILL.md": testCustomSkillMarkdown(),
	})); err != nil {
		t.Fatalf("SavePreviewPackage() error = %v", err)
	}

	_, err := svc.ConfirmCustomSkillImport(context.Background(), scope, importID, false)
	if err == nil || !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), customSkillSystemNameConflictMessage) {
		t.Fatalf("ConfirmCustomSkillImport() error = %v, want friendly system conflict invalid input", err)
	}
}

func TestPreviewImportCustomSkillReturnsOverwriteInfo(t *testing.T) {
	svc, scope, customRepo := testCustomSkillService(t)
	existing := &aichatmodel.CustomSkill{
		OrganizationID: scope.OrganizationID,
		SkillID:        "brief-writer",
		Name:           "Existing Brief Writer",
		Status:         aichatmodel.CustomSkillStatusActive,
		UpdatedAt:      time.Unix(123, 0),
	}
	customRepo.items[existing.SkillID] = existing

	preview, err := svc.PreviewImportCustomSkill(context.Background(), scope, testSkillFileHeader(t, testSkillZip(t, map[string]string{
		"SKILL.md": testCustomSkillMarkdown(),
	})))
	if err != nil {
		t.Fatalf("PreviewImportCustomSkill() error = %v", err)
	}
	if !preview.WillOverwrite {
		t.Fatal("PreviewImportCustomSkill().WillOverwrite = false, want true")
	}
	if preview.ExistingSkill == nil || preview.ExistingSkill.SkillID != "brief-writer" || preview.ExistingSkill.Name != "Existing Brief Writer" {
		t.Fatalf("PreviewImportCustomSkill().ExistingSkill = %#v, want existing skill summary", preview.ExistingSkill)
	}
}

func TestConfirmCustomSkillImportRequiresOverwriteConfirmation(t *testing.T) {
	svc, scope, customRepo := testCustomSkillService(t)
	customRepo.items["brief-writer"] = &aichatmodel.CustomSkill{
		OrganizationID: scope.OrganizationID,
		SkillID:        "brief-writer",
		Name:           "Existing Brief Writer",
		Status:         aichatmodel.CustomSkillStatusActive,
	}
	preview, err := svc.PreviewImportCustomSkill(context.Background(), scope, testSkillFileHeader(t, testSkillZip(t, map[string]string{
		"SKILL.md": testCustomSkillMarkdown(),
	})))
	if err != nil {
		t.Fatalf("PreviewImportCustomSkill() error = %v", err)
	}

	_, err = svc.ConfirmCustomSkillImport(context.Background(), scope, preview.ImportID, false)
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ConfirmCustomSkillImport() error = %v, want invalid input", err)
	}
}

func TestConfirmCustomSkillImportAllowsConfirmedOverwrite(t *testing.T) {
	svc, scope, customRepo := testCustomSkillService(t)
	customRepo.items["brief-writer"] = &aichatmodel.CustomSkill{
		OrganizationID: scope.OrganizationID,
		SkillID:        "brief-writer",
		Name:           "Existing Brief Writer",
		Status:         aichatmodel.CustomSkillStatusActive,
	}
	preview, err := svc.PreviewImportCustomSkill(context.Background(), scope, testSkillFileHeader(t, testSkillZip(t, map[string]string{
		"SKILL.md": strings.Replace(testCustomSkillMarkdown(), "Help draft short writing briefs.", "Updated brief writer.", 1),
	})))
	if err != nil {
		t.Fatalf("PreviewImportCustomSkill() error = %v", err)
	}

	metadata, err := svc.ConfirmCustomSkillImport(context.Background(), scope, preview.ImportID, true)
	if err != nil {
		t.Fatalf("ConfirmCustomSkillImport() error = %v", err)
	}
	if metadata.ID != "brief-writer" {
		t.Fatalf("ConfirmCustomSkillImport().ID = %q, want brief-writer", metadata.ID)
	}
	if got := customRepo.items["brief-writer"].Description; got != "Updated brief writer." {
		t.Fatalf("updated custom skill description = %q, want updated value", got)
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

func testSkillFileHeader(t *testing.T, data []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "skill.zip")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, "/skills/import/preview", &body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(int64(body.Len()) + 1024); err != nil {
		t.Fatalf("ParseMultipartForm() error = %v", err)
	}
	files := request.MultipartForm.File["file"]
	if len(files) != 1 {
		t.Fatalf("multipart files = %d, want 1", len(files))
	}
	return files[0]
}

func testCustomSkillService(t *testing.T) (*service, Scope, *fakeCustomSkillRepository) {
	t.Helper()
	catalogDir := t.TempDir()
	return testCustomSkillServiceWithCatalog(t, catalogDir)
}

func testCustomSkillServiceWithSystemSkill(t *testing.T, skillID string) (*service, Scope, *fakeCustomSkillRepository) {
	t.Helper()
	catalogDir := t.TempDir()
	writeSystemSkill(t, catalogDir, skillID)
	return testCustomSkillServiceWithCatalog(t, catalogDir)
}

func testCustomSkillServiceWithCatalog(t *testing.T, catalogDir string) (*service, Scope, *fakeCustomSkillRepository) {
	t.Helper()
	customRepo := &fakeCustomSkillRepository{items: map[string]*aichatmodel.CustomSkill{}}
	svc := &service{
		repos: &repository.Repositories{
			Access:      fakeAccessRepository{},
			CustomSkill: customRepo,
			SkillConfig: fakeSkillConfigRepository{},
		},
		skillRuntime:       skills.NewRuntimeWithCatalog(tools.NewToolEngine(tools.NewToolManager(nil)), tools.NewToolManager(nil), catalogDir),
		customSkillStorage: newFilesystemCustomSkillStorage(t.TempDir()),
	}
	return svc, Scope{OrganizationID: uuid.New(), AccountID: uuid.New()}, customRepo
}

func writeSystemSkill(t *testing.T, catalogDir string, skillID string) {
	t.Helper()
	skillDir := filepath.Join(catalogDir, skillID)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir system skill: %v", err)
	}
	content := `---
name: ` + skillID + `
description: Built-in system skill.
provider_type: builtin
provider_id: missing
tools:
  - missing_tool
---

# System Skill
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write system skill: %v", err)
	}
}

type fakeAccessRepository struct{}

func (fakeAccessRepository) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}

func (fakeAccessRepository) GetCurrentWorkspaceID(context.Context, uuid.UUID) (*uuid.UUID, error) {
	return nil, nil
}

type fakeCustomSkillRepository struct {
	items map[string]*aichatmodel.CustomSkill
}

func (r *fakeCustomSkillRepository) ListByOrganization(context.Context, uuid.UUID) ([]*aichatmodel.CustomSkill, error) {
	return nil, nil
}

func (r *fakeCustomSkillRepository) ListManageableByOrganization(context.Context, uuid.UUID) ([]*aichatmodel.CustomSkill, error) {
	items := make([]*aichatmodel.CustomSkill, 0, len(r.items))
	for _, item := range r.items {
		items = append(items, item)
	}
	return items, nil
}

func (r *fakeCustomSkillRepository) GetBySkillID(_ context.Context, _ uuid.UUID, skillID string) (*aichatmodel.CustomSkill, error) {
	item, ok := r.items[strings.ToLower(strings.TrimSpace(skillID))]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *fakeCustomSkillRepository) Upsert(_ context.Context, skill *aichatmodel.CustomSkill) error {
	if skill == nil {
		return nil
	}
	r.items[strings.ToLower(strings.TrimSpace(skill.SkillID))] = skill
	return nil
}

func (r *fakeCustomSkillRepository) DeleteBySkillID(_ context.Context, _ uuid.UUID, skillID string) error {
	delete(r.items, strings.ToLower(strings.TrimSpace(skillID)))
	return nil
}

type fakeSkillConfigRepository struct{}

func (fakeSkillConfigRepository) ListByOrganization(context.Context, uuid.UUID) ([]*aichatmodel.OrganizationSkillConfig, error) {
	return nil, nil
}

func (fakeSkillConfigRepository) ReplaceForOrganization(context.Context, uuid.UUID, []*aichatmodel.OrganizationSkillConfig) error {
	return nil
}

func (fakeSkillConfigRepository) DeleteByOrganizationAndSkill(context.Context, uuid.UUID, string) error {
	return nil
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
