package service

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/uuid"
)

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
