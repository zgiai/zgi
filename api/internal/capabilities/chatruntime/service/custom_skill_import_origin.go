package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// customSkillOriginManifestKey is where a custom skill's manifest records the catalog its
// package was installed from. A package a member uploaded has no such entry.
const customSkillOriginManifestKey = "origin"

// SkillPackageOrigin names the catalog entry a skill package was installed from.
type SkillPackageOrigin struct {
	// Source is the catalog, for example "zgi_market".
	Source string
	// Reference identifies the entry within the catalog, such as its slug.
	Reference string
	// Version is the label of the installed version.
	Version string
	// PackageSHA256 is the digest of the package as the catalog served it.
	PackageSHA256 string
}

func (o SkillPackageOrigin) manifest() map[string]interface{} {
	return map[string]interface{}{
		"source": o.Source, "reference": o.Reference, "version": o.Version, "package_sha256": o.PackageSHA256,
	}
}

// InstalledSkillPackage is a custom skill together with the catalog entry it came from.
type InstalledSkillPackage struct {
	SkillID string
	Name    string
	Origin  SkillPackageOrigin
}

func customSkillOrigin(skill *runtimemodel.CustomSkill) (SkillPackageOrigin, bool) {
	if skill == nil {
		return SkillPackageOrigin{}, false
	}
	raw, ok := skill.Manifest[customSkillOriginManifestKey].(map[string]interface{})
	if !ok {
		return SkillPackageOrigin{}, false
	}
	text := func(key string) string {
		value, _ := raw[key].(string)
		return value
	}
	origin := SkillPackageOrigin{
		Source: text("source"), Reference: text("reference"), Version: text("version"), PackageSHA256: text("package_sha256"),
	}
	return origin, origin.Source != "" && origin.Reference != ""
}

// ImportCustomSkillPackage imports a package a catalog served, through the same preview and
// confirmation as an uploaded one. The preview is returned alone when the package cannot be
// imported, or when it would replace a skill that did not come from the same catalog entry and
// overwrite is not set; a skill installed from that entry before is replaced without asking.
func (s *service) ImportCustomSkillPackage(
	ctx context.Context, scope Scope, data []byte, filename string, origin SkillPackageOrigin, overwrite bool,
) (*SkillImportPreview, *skills.SkillDiscoveryMetadata, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, nil, err
	}
	if origin.Source == "" || origin.Reference == "" {
		return nil, nil, fmt.Errorf("%w: skill package origin is required", ErrInvalidInput)
	}
	if len(data) == 0 || len(data) > customSkillMaxPackageSize {
		return nil, nil, fmt.Errorf("%w: skill package is empty or too large", ErrInvalidInput)
	}
	if strings.ToLower(filepath.Ext(filename)) != ".zip" {
		return nil, nil, fmt.Errorf("%w: skill package must be a zip file", ErrInvalidInput)
	}
	preview, err := s.previewCustomSkillPackage(ctx, scope, data, filename)
	if err != nil {
		return nil, nil, err
	}
	if !preview.CanImport {
		return preview, nil, nil
	}
	if preview.WillOverwrite && !overwrite {
		existing, err := s.existingCustomSkill(ctx, scope.OrganizationID, preview.Skill.ID)
		if err != nil {
			_ = s.CancelCustomSkillImportPreview(ctx, scope, preview.ImportID)
			return nil, nil, err
		}
		if installed, ok := customSkillOrigin(existing); !ok || installed.Source != origin.Source || installed.Reference != origin.Reference {
			_ = s.CancelCustomSkillImportPreview(ctx, scope, preview.ImportID)
			return preview, nil, nil
		}
	}
	metadata, err := s.confirmCustomSkillImport(ctx, scope, preview.ImportID, true, &origin)
	if err != nil {
		_ = s.CancelCustomSkillImportPreview(ctx, scope, preview.ImportID)
		return nil, nil, err
	}
	return preview, metadata, nil
}

// ListInstalledSkillPackages returns the organization's custom skills that were installed from
// the given catalog.
func (s *service) ListInstalledSkillPackages(ctx context.Context, scope Scope, source string) ([]InstalledSkillPackage, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	if s.repos == nil || s.repos.CustomSkill == nil {
		return nil, fmt.Errorf("custom skill repository is not configured")
	}
	records, err := s.repos.CustomSkill.ListByOrganization(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	installed := make([]InstalledSkillPackage, 0, len(records))
	for _, record := range records {
		if origin, ok := customSkillOrigin(record); ok && origin.Source == source {
			installed = append(installed, InstalledSkillPackage{SkillID: record.SkillID, Name: record.Name, Origin: origin})
		}
	}
	return installed, nil
}
