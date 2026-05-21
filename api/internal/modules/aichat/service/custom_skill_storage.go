package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	customSkillImportTTL        = 30 * time.Minute
	customSkillPreviewStateFile = ".preview-state"
)

type customSkillStorage interface {
	SavePreviewPackage(ctx context.Context, organizationID uuid.UUID, importID string, data []byte) (*storedSkillPreview, error)
	LoadPreview(ctx context.Context, organizationID uuid.UUID, importID string) (*storedSkillPreview, error)
	PublishPreview(ctx context.Context, preview *storedSkillPreview, skillID string) (*publishedCustomSkillDirectory, string, error)
	DeleteSkill(ctx context.Context, storagePath string) error
	CleanupExpiredPreviews(ctx context.Context, organizationID uuid.UUID, now time.Time)
}

type filesystemCustomSkillStorage struct {
	root string
}

type storedSkillPreview struct {
	ImportID  string               `json:"import_id"`
	Root      string               `json:"root"`
	FileCount int                  `json:"file_count"`
	TotalSize int64                `json:"total_size"`
	Files     []extractedSkillFile `json:"files"`
	ExpiresAt time.Time            `json:"expires_at"`
}

func newFilesystemCustomSkillStorage(root string) customSkillStorage {
	return &filesystemCustomSkillStorage{root: strings.TrimSpace(root)}
}

func (s *filesystemCustomSkillStorage) SavePreviewPackage(ctx context.Context, organizationID uuid.UUID, importID string, data []byte) (*storedSkillPreview, error) {
	_ = ctx
	s.CleanupExpiredPreviews(ctx, organizationID, time.Now())
	tempRoot := s.previewRoot(organizationID, importID)
	if err := os.RemoveAll(tempRoot); err != nil {
		return nil, fmt.Errorf("failed to prepare custom skill import directory: %w", err)
	}
	extracted, err := extractSkillZip(data, tempRoot)
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, err
	}
	preview := &storedSkillPreview{
		ImportID:  importID,
		Root:      extracted.Root,
		FileCount: extracted.FileCount,
		TotalSize: extracted.TotalSize,
		Files:     append([]extractedSkillFile(nil), extracted.FileDetails...),
		ExpiresAt: time.Now().Add(customSkillImportTTL),
	}
	if err := s.writePreviewState(preview); err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, err
	}
	return preview, nil
}

func (s *filesystemCustomSkillStorage) LoadPreview(ctx context.Context, organizationID uuid.UUID, importID string) (*storedSkillPreview, error) {
	_ = ctx
	if _, err := uuid.Parse(strings.TrimSpace(importID)); err != nil {
		return nil, fmt.Errorf("%w: invalid import id", ErrInvalidInput)
	}
	root := s.previewRoot(organizationID, importID)
	raw, err := os.ReadFile(filepath.Join(root, customSkillPreviewStateFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: skill import preview not found", ErrNotFound)
		}
		return nil, fmt.Errorf("failed to load custom skill import preview: %w", err)
	}
	var preview storedSkillPreview
	if err := json.Unmarshal(raw, &preview); err != nil {
		return nil, fmt.Errorf("failed to parse custom skill import preview: %w", err)
	}
	if preview.ImportID != strings.TrimSpace(importID) || !sameCleanPath(preview.Root, root) {
		return nil, fmt.Errorf("%w: invalid skill import preview", ErrInvalidInput)
	}
	if !preview.ExpiresAt.IsZero() && time.Now().After(preview.ExpiresAt) {
		_ = os.RemoveAll(root)
		return nil, fmt.Errorf("%w: skill import preview expired", ErrInvalidInput)
	}
	if _, err := os.Stat(preview.Root); err != nil {
		return nil, fmt.Errorf("%w: skill import preview not found", ErrNotFound)
	}
	return &preview, nil
}

func (s *filesystemCustomSkillStorage) PublishPreview(ctx context.Context, preview *storedSkillPreview, skillID string) (*publishedCustomSkillDirectory, string, error) {
	_ = ctx
	if preview == nil {
		return nil, "", fmt.Errorf("%w: skill import preview is required", ErrInvalidInput)
	}
	finalRoot := filepath.Join(s.organizationRootFromPreview(preview), strings.ToLower(strings.TrimSpace(skillID)), "current")
	_ = os.Remove(filepath.Join(preview.Root, customSkillPreviewStateFile))
	published, err := replaceCustomSkillDirectory(preview.Root, finalRoot)
	if err != nil {
		return nil, "", err
	}
	_ = os.RemoveAll(preview.Root)
	return published, finalRoot, nil
}

func (s *filesystemCustomSkillStorage) DeleteSkill(ctx context.Context, storagePath string) error {
	_ = ctx
	if strings.TrimSpace(storagePath) == "" {
		return nil
	}
	return os.RemoveAll(storagePath)
}

func (s *filesystemCustomSkillStorage) CleanupExpiredPreviews(ctx context.Context, organizationID uuid.UUID, now time.Time) {
	_ = ctx
	importsRoot := filepath.Join(s.organizationRoot(organizationID), ".imports")
	entries, err := os.ReadDir(importsRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		root := filepath.Join(importsRoot, entry.Name())
		raw, err := os.ReadFile(filepath.Join(root, customSkillPreviewStateFile))
		if err != nil {
			continue
		}
		var preview storedSkillPreview
		if err := json.Unmarshal(raw, &preview); err != nil {
			continue
		}
		if !preview.ExpiresAt.IsZero() && now.After(preview.ExpiresAt) {
			_ = os.RemoveAll(root)
		}
	}
}

func (s *filesystemCustomSkillStorage) previewRoot(organizationID uuid.UUID, importID string) string {
	return filepath.Join(s.organizationRoot(organizationID), ".imports", strings.TrimSpace(importID))
}

func (s *filesystemCustomSkillStorage) organizationRoot(organizationID uuid.UUID) string {
	return filepath.Join(s.root, organizationID.String())
}

func (s *filesystemCustomSkillStorage) organizationRootFromPreview(preview *storedSkillPreview) string {
	return filepath.Dir(filepath.Dir(preview.Root))
}

func (s *filesystemCustomSkillStorage) writePreviewState(preview *storedSkillPreview) error {
	data, err := json.Marshal(preview)
	if err != nil {
		return fmt.Errorf("failed to serialize custom skill import preview: %w", err)
	}
	if err := os.WriteFile(filepath.Join(preview.Root, customSkillPreviewStateFile), data, 0o644); err != nil {
		return fmt.Errorf("failed to save custom skill import preview: %w", err)
	}
	return nil
}

func sameCleanPath(left string, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return filepath.Clean(left) == filepath.Clean(right)
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}
