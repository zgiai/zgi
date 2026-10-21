package service

import (
	"archive/zip"
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm"
)

// memoryCustomSkillRepository keeps custom skills of one organization in memory.
type memoryCustomSkillRepository struct {
	mu     sync.Mutex
	skills map[string]*runtimemodel.CustomSkill
}

func (r *memoryCustomSkillRepository) ListByOrganization(context.Context, uuid.UUID) ([]*runtimemodel.CustomSkill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	listed := make([]*runtimemodel.CustomSkill, 0, len(r.skills))
	for _, skill := range r.skills {
		listed = append(listed, skill)
	}
	return listed, nil
}

func (r *memoryCustomSkillRepository) ListManageableByOrganization(ctx context.Context, id uuid.UUID) ([]*runtimemodel.CustomSkill, error) {
	return r.ListByOrganization(ctx, id)
}

func (r *memoryCustomSkillRepository) GetBySkillID(_ context.Context, _ uuid.UUID, skillID string) (*runtimemodel.CustomSkill, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if skill, ok := r.skills[skillID]; ok {
		return skill, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *memoryCustomSkillRepository) Upsert(_ context.Context, skill *runtimemodel.CustomSkill) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[skill.SkillID] = skill
	return nil
}

func (r *memoryCustomSkillRepository) DeleteBySkillID(_ context.Context, _ uuid.UUID, skillID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.skills, skillID)
	return nil
}

func skillArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("zip Create(%q) error = %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("zip Write(%q) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip Close() error = %v", err)
	}
	return archive.Bytes()
}

func TestImportCustomSkillPackageRecordsItsOriginAndGuardsOtherSkills(t *testing.T) {
	store := &memoryCustomSkillRepository{skills: map[string]*runtimemodel.CustomSkill{}}
	svc := &service{
		skillRuntime:       skills.NewRuntimeWithCatalog(nil, nil, t.TempDir()),
		customSkillStorage: newFilesystemCustomSkillStorage(t.TempDir()),
		repos:              &repository.Repositories{CustomSkill: store},
	}
	scope := Scope{OrganizationID: uuid.New(), AccountID: uuid.New(), SkipAccessCheck: true}
	ctx := context.Background()
	portable := func(body string) []byte {
		return skillArchive(t, map[string]string{
			"csv-summary/SKILL.md":             "---\ndescription: Summarize CSV input with a portable Python script.\n---\n" + body + "\n",
			"csv-summary/scripts/summarize.py": "print('summary')\n",
		})
	}
	origin := SkillPackageOrigin{Source: "zgi_market", Reference: "csv-summary-listing", Version: "1.0.0", PackageSHA256: "aa"}

	// A package of a catalog goes through the same preview and confirmation as an upload.
	preview, installed, err := svc.ImportCustomSkillPackage(ctx, scope, portable("v1"), "csv-summary.zip", origin, false)
	if err != nil || installed == nil || installed.ID != "csv-summary" || !preview.CanImport {
		t.Fatalf("first install = %#v, %#v, %v; want csv-summary installed", preview, installed, err)
	}
	listed, err := svc.ListInstalledSkillPackages(ctx, scope, "zgi_market")
	if err != nil || len(listed) != 1 || listed[0].SkillID != "csv-summary" || listed[0].Origin != origin {
		t.Fatalf("installed packages = %#v, %v; want the one with its origin", listed, err)
	}
	if other, _ := svc.ListInstalledSkillPackages(ctx, scope, "another_catalog"); len(other) != 0 {
		t.Fatalf("another catalog lists %#v, want nothing", other)
	}

	// A new version of the same catalog entry replaces it without asking.
	origin.Version = "1.1.0"
	if _, installed, err = svc.ImportCustomSkillPackage(ctx, scope, portable("v2"), "csv-summary.zip", origin, false); err != nil || installed == nil {
		t.Fatalf("update = %#v, %v; want the skill replaced", installed, err)
	}
	if listed, _ = svc.ListInstalledSkillPackages(ctx, scope, "zgi_market"); len(listed) != 1 || listed[0].Origin.Version != "1.1.0" {
		t.Fatalf("installed after update = %#v, want version 1.1.0", listed)
	}

	// Another catalog entry with the same skill name does not replace it unless told to.
	stranger := SkillPackageOrigin{Source: "zgi_market", Reference: "another-listing", Version: "9"}
	preview, installed, err = svc.ImportCustomSkillPackage(ctx, scope, portable("other"), "csv-summary.zip", stranger, false)
	if err != nil || installed != nil || preview == nil || !preview.WillOverwrite {
		t.Fatalf("conflicting install = %#v, %#v, %v; want only the preview of an overwrite", preview, installed, err)
	}
	if listed, _ = svc.ListInstalledSkillPackages(ctx, scope, "zgi_market"); listed[0].Origin.Reference != "csv-summary-listing" {
		t.Fatalf("the installed skill changed owner: %#v", listed)
	}
	if _, installed, err = svc.ImportCustomSkillPackage(ctx, scope, portable("other"), "csv-summary.zip", stranger, true); err != nil || installed == nil {
		t.Fatalf("confirmed overwrite = %#v, %v; want it installed", installed, err)
	}

	// A skill a member uploaded is never replaced silently either.
	delete(store.skills["csv-summary"].Manifest, customSkillOriginManifestKey)
	if _, installed, _ = svc.ImportCustomSkillPackage(ctx, scope, portable("again"), "csv-summary.zip", origin, false); installed != nil {
		t.Fatalf("an uploaded skill was replaced without confirmation")
	}

	// A package that is not a skill is refused by the checks of the import.
	if _, installed, err = svc.ImportCustomSkillPackage(ctx, scope, skillArchive(t, map[string]string{"x/readme.txt": "no skill"}), "x.zip", stranger, false); err == nil || installed != nil {
		t.Fatalf("invalid package = %#v, %v; want a refusal", installed, err)
	}
	for name, attempt := range map[string]func() error{
		"no origin": func() error {
			_, _, err := svc.ImportCustomSkillPackage(ctx, scope, portable("v"), "a.zip", SkillPackageOrigin{}, false)
			return err
		},
		"not a zip": func() error {
			_, _, err := svc.ImportCustomSkillPackage(ctx, scope, portable("v"), "a.tar", origin, false)
			return err
		},
		"empty": func() error {
			_, _, err := svc.ImportCustomSkillPackage(ctx, scope, nil, "a.zip", origin, false)
			return err
		},
	} {
		if err := attempt(); err == nil {
			t.Fatalf("%s: want a refusal", name)
		}
	}
}
