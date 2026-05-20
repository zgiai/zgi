package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
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
	aichatmodel "github.com/zgiai/ginext/internal/modules/aichat/model"
	"github.com/zgiai/ginext/internal/modules/aichat/repository"
	"github.com/zgiai/ginext/internal/modules/skills"
	"github.com/zgiai/ginext/pkg/logger"
	"gorm.io/gorm"
)

const (
	customSkillStorageRoot    = "storage/aichat/skills"
	customSkillMaxPackageSize = 20 * 1024 * 1024
	customSkillMaxFileSize    = 5 * 1024 * 1024
	customSkillMaxFileCount   = 200
)

type extractedSkillPackage struct {
	Root      string
	FileCount int
	TotalSize int64
	Files     []string
}

func (s *service) ImportCustomSkill(ctx context.Context, scope Scope, fileHeader *multipart.FileHeader) (*skills.SkillDiscoveryMetadata, error) {
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
	data, err := readUploadedSkillPackage(fileHeader)
	if err != nil {
		return nil, err
	}
	importID := uuid.New().String()
	tempRoot := filepath.Join(customSkillStorageRoot, scope.OrganizationID.String(), ".imports", importID)
	if err := os.RemoveAll(tempRoot); err != nil {
		return nil, fmt.Errorf("failed to prepare custom skill import directory: %w", err)
	}
	extracted, err := extractSkillZip(data, tempRoot)
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, err
	}
	doc, err := skills.LoadCustomSkillDocument(extracted.Root)
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if _, err := s.skillRuntime.GetSkillMetadata(ctx, doc.Metadata.ID); err == nil {
		_ = os.RemoveAll(tempRoot)
		return nil, fmt.Errorf("%w: custom skill id conflicts with a system skill", ErrInvalidInput)
	}
	finalRoot := filepath.Join(customSkillStorageRoot, scope.OrganizationID.String(), doc.Metadata.ID, "current")
	published, err := replaceCustomSkillDirectory(extracted.Root, finalRoot)
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, err
	}
	record := customSkillRecordFromDocument(scope, doc, finalRoot, extracted)
	if err := s.repos.CustomSkill.Upsert(ctx, record); err != nil {
		published.rollback()
		return nil, err
	}
	published.cleanup()
	metadata, err := s.skillRuntime.GetSkillMetadataWithCustom(ctx, doc.Metadata.ID, []skills.CustomSkillCatalogEntry{{
		SkillID: doc.Metadata.ID,
		Root:    finalRoot,
	}})
	if err != nil {
		return nil, err
	}
	metadata.Enabled = s.isOrganizationSkillEnabled(ctx, scope.OrganizationID, metadata.ID)
	return metadata, nil
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
		if err := os.RemoveAll(record.StoragePath); err != nil {
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
	}
	if fileCount == 0 {
		return nil, fmt.Errorf("%w: skill package is empty", ErrInvalidInput)
	}
	sort.Strings(files)
	return &extractedSkillPackage{Root: targetRoot, FileCount: fileCount, TotalSize: totalSize, Files: files}, nil
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
