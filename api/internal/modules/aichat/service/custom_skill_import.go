package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/aichat/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

const (
	customSkillStorageRoot    = "storage/aichat/skills"
	customSkillMaxPackageSize = 20 * 1024 * 1024
	customSkillMaxFileSize    = 5 * 1024 * 1024
	customSkillMaxFileCount   = 200
)

type extractedSkillFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type extractedSkillPackage struct {
	Root        string
	FileCount   int
	TotalSize   int64
	Files       []string
	FileDetails []extractedSkillFile
}

func (s *service) PreviewImportCustomSkill(ctx context.Context, scope Scope, fileHeader *multipart.FileHeader) (*SkillImportPreview, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	if fileHeader == nil {
		return nil, fmt.Errorf("%w: skill package is required", ErrInvalidInput)
	}
	if s.skillRuntime == nil {
		return nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if s.repos == nil || s.repos.CustomSkill == nil {
		return nil, fmt.Errorf("custom skill repository is not configured")
	}
	if s.customSkillStorage == nil {
		return nil, fmt.Errorf("custom skill storage is not configured")
	}
	data, err := readUploadedSkillPackage(fileHeader)
	if err != nil {
		return nil, err
	}
	importID := uuid.New().String()
	preview, err := s.customSkillStorage.SavePreviewPackage(ctx, scope.OrganizationID, importID, data)
	if err != nil {
		return nil, err
	}
	result := skillImportPreviewFromStored(preview)
	doc, err := skills.LoadCustomSkillDocument(preview.Root)
	if err != nil {
		_ = s.customSkillStorage.DeleteSkill(ctx, preview.Root)
		result.ImportID = ""
		result.ExpiresAt = time.Time{}
		result.ValidationErrors = []string{err.Error()}
		result.CanImport = false
		return result, nil
	}
	if s.customSkillIDConflictsWithSystem(ctx, doc.Metadata.ID) {
		_ = s.customSkillStorage.DeleteSkill(ctx, preview.Root)
		result.ImportID = ""
		result.ExpiresAt = time.Time{}
		result.Skill = skillDiscoveryMetadataPtr(doc)
		result.ValidationErrors = []string{"custom skill id conflicts with a system skill"}
		result.CanImport = false
		return result, nil
	}
	result.Skill = skillDiscoveryMetadataPtr(doc)
	result.References = skillReferencePaths(doc)
	result.HasScripts = doc.Metadata.HasScripts
	result.ScriptsSupported = doc.Metadata.ScriptsSupported
	if existing, err := s.existingCustomSkill(ctx, scope.OrganizationID, doc.Metadata.ID); err != nil {
		_ = s.customSkillStorage.DeleteSkill(ctx, preview.Root)
		return nil, err
	} else if existing != nil {
		result.WillOverwrite = true
		result.ExistingSkill = existingSkillPreview(existing)
	}
	if doc.Metadata.HasScripts && !doc.Metadata.ScriptsSupported {
		result.Warnings = append(result.Warnings, "scripts are present but are not supported for custom skills")
	}
	result.CanImport = true
	return result, nil
}

func (s *service) ConfirmCustomSkillImport(ctx context.Context, scope Scope, importID string, overwriteConfirmed bool) (*skills.SkillDiscoveryMetadata, error) {
	return s.confirmCustomSkillImport(ctx, scope, importID, overwriteConfirmed)
}

func (s *service) confirmCustomSkillImport(ctx context.Context, scope Scope, importID string, overwriteConfirmed bool) (*skills.SkillDiscoveryMetadata, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	if s.skillRuntime == nil {
		return nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if s.repos == nil || s.repos.CustomSkill == nil {
		return nil, fmt.Errorf("custom skill repository is not configured")
	}
	if s.customSkillStorage == nil {
		return nil, fmt.Errorf("custom skill storage is not configured")
	}
	preview, err := s.customSkillStorage.LoadPreview(ctx, scope.OrganizationID, importID)
	if err != nil {
		return nil, err
	}
	doc, err := skills.LoadCustomSkillDocument(preview.Root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if s.customSkillIDConflictsWithSystem(ctx, doc.Metadata.ID) {
		return nil, fmt.Errorf("%w: custom skill id conflicts with a system skill", ErrInvalidInput)
	}
	existing, err := s.existingCustomSkill(ctx, scope.OrganizationID, doc.Metadata.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil && !overwriteConfirmed {
		return nil, fmt.Errorf("%w: custom skill already exists; confirm overwrite before importing", ErrInvalidInput)
	}
	published, finalRoot, err := s.customSkillStorage.PublishPreview(ctx, preview, doc.Metadata.ID)
	if err != nil {
		return nil, err
	}
	extracted := extractedSkillPackageFromPreview(preview)
	record := customSkillRecordFromDocument(scope, doc, finalRoot, extracted)
	if err := s.repos.CustomSkill.Upsert(ctx, record); err != nil {
		published.rollback()
		return nil, err
	}
	published.cleanup()
	metadata := skillDiscoveryMetadataPtr(doc)
	metadata.Enabled = s.isOrganizationSkillEnabled(ctx, scope.OrganizationID, metadata.ID)
	return metadata, nil
}

func (s *service) existingCustomSkill(ctx context.Context, organizationID uuid.UUID, skillID string) (*aichatmodel.CustomSkill, error) {
	if s.repos == nil || s.repos.CustomSkill == nil {
		return nil, fmt.Errorf("custom skill repository is not configured")
	}
	existing, err := s.repos.CustomSkill.GetBySkillID(ctx, organizationID, skillID)
	if err == nil {
		return existing, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(mapRepoError(err), ErrNotFound) {
		return nil, nil
	}
	return nil, err
}

func (s *service) CancelCustomSkillImportPreview(ctx context.Context, scope Scope, importID string) error {
	if err := s.ensureMember(ctx, scope); err != nil {
		return err
	}
	if s.customSkillStorage == nil {
		return fmt.Errorf("custom skill storage is not configured")
	}
	return s.customSkillStorage.DeletePreview(ctx, scope.OrganizationID, importID)
}

func (s *service) CleanupExpiredCustomSkillImportPreviews(ctx context.Context) error {
	if s.customSkillStorage == nil {
		return nil
	}
	s.customSkillStorage.CleanupExpiredPreviews(ctx, time.Now())
	return nil
}

func (s *service) customSkillIDConflictsWithSystem(ctx context.Context, skillID string) bool {
	_ = ctx
	if s.skillRuntime == nil {
		return false
	}
	return s.skillRuntime.SystemSkillExists(skillID)
}

func (s *service) DeleteSkill(ctx context.Context, scope Scope, skillID string) error {
	if err := s.ensureMember(ctx, scope); err != nil {
		return err
	}
	id := strings.ToLower(strings.TrimSpace(skillID))
	if id == "" {
		return fmt.Errorf("%w: skill id is required", ErrInvalidInput)
	}
	if s.skillRuntime != nil {
		if metadata, err := s.skillRuntime.GetSkillMetadata(ctx, id); err == nil && metadata.Source == skills.SkillSourceSystem {
			return fmt.Errorf("%w: system skill cannot be deleted", ErrInvalidInput)
		}
	}
	if s.repos == nil || s.repos.CustomSkill == nil || s.repos.SkillConfig == nil || s.repos.DB == nil {
		return fmt.Errorf("custom skill repository is not configured")
	}
	record, err := s.repos.CustomSkill.GetBySkillID(ctx, scope.OrganizationID, id)
	if err != nil {
		return mapRepoError(err)
	}
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepos := repository.NewRepositories(tx)
		if err := txRepos.CustomSkill.DeleteBySkillID(ctx, scope.OrganizationID, id); err != nil {
			return mapRepoError(err)
		}
		if err := txRepos.SkillConfig.DeleteByOrganizationAndSkill(ctx, scope.OrganizationID, id); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if strings.TrimSpace(record.StoragePath) != "" {
		if err := s.customSkillStorage.DeleteSkill(ctx, record.StoragePath); err != nil {
			logger.WarnContext(ctx, "failed to remove custom skill directory", "skill_id", id, "path", record.StoragePath, err)
		}
	}
	return nil
}

func readUploadedSkillPackage(fileHeader *multipart.FileHeader) ([]byte, error) {
	if fileHeader.Size > customSkillMaxPackageSize {
		return nil, fmt.Errorf("%w: skill package is too large", ErrInvalidInput)
	}
	if strings.ToLower(filepath.Ext(fileHeader.Filename)) != ".zip" {
		return nil, fmt.Errorf("%w: skill package must be a zip file", ErrInvalidInput)
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open skill package: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, customSkillMaxPackageSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read skill package: %w", err)
	}
	if len(data) > customSkillMaxPackageSize {
		return nil, fmt.Errorf("%w: skill package is too large", ErrInvalidInput)
	}
	return data, nil
}

func extractSkillZip(data []byte, targetRoot string) (*extractedSkillPackage, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid skill package zip", ErrInvalidInput)
	}
	prefix, err := detectSkillZipRoot(reader.File)
	if err != nil {
		return nil, err
	}
	var fileCount int
	var totalSize int64
	files := make([]string, 0, len(reader.File))
	fileDetails := make([]extractedSkillFile, 0, len(reader.File))
	for _, file := range reader.File {
		clean, ok := cleanZipPath(file.Name)
		if !ok {
			return nil, fmt.Errorf("%w: invalid path in skill package", ErrInvalidInput)
		}
		if prefix != "" {
			if clean != strings.TrimSuffix(prefix, "/") && !strings.HasPrefix(clean, prefix) {
				continue
			}
			clean = strings.TrimPrefix(clean, prefix)
		}
		if clean == "" || clean == "." {
			continue
		}
		if file.FileInfo().Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: symlinks are not allowed in skill packages", ErrInvalidInput)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		fileCount++
		if fileCount > customSkillMaxFileCount {
			return nil, fmt.Errorf("%w: skill package contains too many files", ErrInvalidInput)
		}
		if file.UncompressedSize64 > customSkillMaxFileSize {
			return nil, fmt.Errorf("%w: skill package file is too large", ErrInvalidInput)
		}
		totalSize += int64(file.UncompressedSize64)
		if totalSize > customSkillMaxPackageSize {
			return nil, fmt.Errorf("%w: skill package expanded size is too large", ErrInvalidInput)
		}
		if err := extractZipFile(file, targetRoot, clean); err != nil {
			return nil, err
		}
		files = append(files, clean)
		fileDetails = append(fileDetails, extractedSkillFile{Path: clean, Size: int64(file.UncompressedSize64)})
	}
	if fileCount == 0 {
		return nil, fmt.Errorf("%w: skill package is empty", ErrInvalidInput)
	}
	sort.Strings(files)
	sort.Slice(fileDetails, func(i, j int) bool { return fileDetails[i].Path < fileDetails[j].Path })
	return &extractedSkillPackage{Root: targetRoot, FileCount: fileCount, TotalSize: totalSize, Files: files, FileDetails: fileDetails}, nil
}

func detectSkillZipRoot(files []*zip.File) (string, error) {
	rootSkill := false
	topLevel := map[string]struct{}{}
	for _, file := range files {
		clean, ok := cleanZipPath(file.Name)
		if !ok {
			return "", fmt.Errorf("%w: invalid path in skill package", ErrInvalidInput)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if clean == "SKILL.md" {
			rootSkill = true
			continue
		}
		parts := strings.Split(clean, "/")
		if len(parts) >= 2 && parts[1] == "SKILL.md" {
			topLevel[parts[0]] = struct{}{}
		}
	}
	if rootSkill {
		return "", nil
	}
	if len(topLevel) != 1 {
		return "", fmt.Errorf("%w: skill package must contain SKILL.md at root or inside one top-level directory", ErrInvalidInput)
	}
	for dir := range topLevel {
		return dir + "/", nil
	}
	return "", fmt.Errorf("%w: skill package must contain SKILL.md", ErrInvalidInput)
}

func cleanZipPath(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name == "" || strings.Contains(name, "\\") {
		return "", false
	}
	clean := path.Clean(name)
	if clean == "." || strings.HasPrefix(clean, "../") || clean == ".." || path.IsAbs(clean) {
		return "", false
	}
	return clean, true
}

func extractZipFile(file *zip.File, root string, relativePath string) error {
	destination := filepath.Join(root, filepath.FromSlash(relativePath))
	rel, err := filepath.Rel(root, destination)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return fmt.Errorf("%w: invalid path in skill package", ErrInvalidInput)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("failed to create skill package directory: %w", err)
	}
	source, err := file.Open()
	if err != nil {
		return fmt.Errorf("failed to open skill package file: %w", err)
	}
	defer source.Close()
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to create skill package file: %w", err)
	}
	defer target.Close()
	if _, err := io.Copy(target, io.LimitReader(source, customSkillMaxFileSize+1)); err != nil {
		return fmt.Errorf("failed to extract skill package file: %w", err)
	}
	return nil
}

type publishedCustomSkillDirectory struct {
	rollback func()
	cleanup  func()
}

func replaceCustomSkillDirectory(sourceRoot string, finalRoot string) (*publishedCustomSkillDirectory, error) {
	parent := filepath.Dir(finalRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create custom skill directory: %w", err)
	}
	backup := finalRoot + ".backup-" + uuid.New().String()
	hasExisting := false
	if _, err := os.Stat(finalRoot); err == nil {
		hasExisting = true
		if err := os.Rename(finalRoot, backup); err != nil {
			return nil, fmt.Errorf("failed to backup existing custom skill: %w", err)
		}
	}
	if err := os.Rename(sourceRoot, finalRoot); err != nil {
		if hasExisting {
			_ = os.Rename(backup, finalRoot)
		}
		return nil, fmt.Errorf("failed to publish custom skill: %w", err)
	}
	published := &publishedCustomSkillDirectory{
		rollback: func() {
			_ = os.RemoveAll(finalRoot)
			if hasExisting {
				_ = os.Rename(backup, finalRoot)
			}
		},
		cleanup: func() {
			if hasExisting {
				_ = os.RemoveAll(backup)
			}
		},
	}
	return published, nil
}

func customSkillRecordFromDocument(scope Scope, doc skills.SkillDocument, storagePath string, extracted *extractedSkillPackage) *aichatmodel.CustomSkill {
	return &aichatmodel.CustomSkill{
		ID:             uuid.New(),
		OrganizationID: scope.OrganizationID,
		SkillID:        doc.Metadata.ID,
		Name:           doc.Metadata.Name,
		Description:    doc.Metadata.Description,
		WhenToUse:      doc.Metadata.WhenToUse,
		RuntimeType:    skills.SkillRuntimeTypePrompt,
		Display:        skillDisplayMap(doc.Metadata.Display),
		StoragePath:    storagePath,
		Manifest:       customSkillManifest(doc, extracted),
		Status:         aichatmodel.CustomSkillStatusActive,
		CreatedBy:      scope.AccountID,
	}
}

func customSkillManifest(doc skills.SkillDocument, extracted *extractedSkillPackage) map[string]interface{} {
	references := make([]string, 0, len(doc.Metadata.References))
	for _, ref := range doc.Metadata.References {
		references = append(references, ref.Path)
	}
	manifest := map[string]interface{}{
		"file_count":        0,
		"total_size":        int64(0),
		"files":             []string{},
		"references":        references,
		"has_scripts":       doc.Metadata.HasScripts,
		"scripts_supported": false,
		"imported_at":       time.Now().Unix(),
	}
	if extracted != nil {
		manifest["file_count"] = extracted.FileCount
		manifest["total_size"] = extracted.TotalSize
		manifest["files"] = append([]string(nil), extracted.Files...)
	}
	return manifest
}

func skillImportPreviewFromStored(preview *storedSkillPreview) *SkillImportPreview {
	if preview == nil {
		return &SkillImportPreview{Files: []SkillImportPreviewFile{}, References: []string{}, Warnings: []string{}, ValidationErrors: []string{}}
	}
	files := make([]SkillImportPreviewFile, 0, len(preview.Files))
	for _, file := range preview.Files {
		files = append(files, SkillImportPreviewFile{Path: file.Path, Size: file.Size})
	}
	return &SkillImportPreview{
		ImportID:         preview.ImportID,
		ExpiresAt:        preview.ExpiresAt,
		FileCount:        preview.FileCount,
		TotalSize:        preview.TotalSize,
		Files:            files,
		References:       []string{},
		Warnings:         []string{},
		ValidationErrors: []string{},
		CanImport:        false,
	}
}

func existingSkillPreview(skill *aichatmodel.CustomSkill) *ExistingSkill {
	if skill == nil {
		return nil
	}
	return &ExistingSkill{
		SkillID:   strings.ToLower(strings.TrimSpace(skill.SkillID)),
		Name:      strings.TrimSpace(skill.Name),
		UpdatedAt: skill.UpdatedAt,
	}
}

func extractedSkillPackageFromPreview(preview *storedSkillPreview) *extractedSkillPackage {
	if preview == nil {
		return nil
	}
	files := make([]string, 0, len(preview.Files))
	for _, file := range preview.Files {
		files = append(files, file.Path)
	}
	sort.Strings(files)
	return &extractedSkillPackage{
		Root:        preview.Root,
		FileCount:   preview.FileCount,
		TotalSize:   preview.TotalSize,
		Files:       files,
		FileDetails: append([]extractedSkillFile(nil), preview.Files...),
	}
}

func skillDiscoveryMetadataPtr(doc skills.SkillDocument) *skills.SkillDiscoveryMetadata {
	metadata := skills.SkillDiscoveryMetadata{
		ID:               doc.Metadata.ID,
		Source:           doc.Metadata.Source,
		Name:             doc.Metadata.Name,
		Description:      doc.Metadata.Description,
		WhenToUse:        doc.Metadata.WhenToUse,
		Display:          doc.Metadata.Display,
		RuntimeType:      doc.Metadata.RuntimeType,
		HasTools:         len(doc.Tools) > 0,
		HasReferences:    len(doc.Metadata.References) > 0,
		HasScripts:       doc.Metadata.HasScripts,
		ScriptsSupported: doc.Metadata.ScriptsSupported,
		MaxCallsPerTurn:  doc.Metadata.MaxCallsPerTurn,
		TimeoutSeconds:   doc.Metadata.TimeoutSeconds,
		Status:           skills.SkillStatusActive,
	}
	return &metadata
}

func skillReferencePaths(doc skills.SkillDocument) []string {
	references := make([]string, 0, len(doc.Metadata.References))
	for _, ref := range doc.Metadata.References {
		references = append(references, ref.Path)
	}
	sort.Strings(references)
	return references
}

func previewValidationErrors(preview *SkillImportPreview) []string {
	if preview == nil || len(preview.ValidationErrors) == 0 {
		return []string{"skill package cannot be imported"}
	}
	return preview.ValidationErrors
}

func skillDisplayMap(display skills.SkillDisplayMetadata) map[string]interface{} {
	data, err := json.Marshal(display)
	if err != nil {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{}
	}
	if out == nil {
		return map[string]interface{}{}
	}
	return out
}
