package local

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zgiai/zgi/runner/internal/plugin"
	"github.com/zgiai/zgi/runner/internal/storage"
)

// Store implements storage.Store using the local filesystem.
type Store struct {
	workspaceRoot string
	packageRoot   string
}

func NewStore(workspaceRoot, packageRoot string) *Store {
	return &Store{
		workspaceRoot: workspaceRoot,
		packageRoot:   packageRoot,
	}
}

var _ storage.Store = (*Store)(nil)

func (s *Store) SavePackage(ctx context.Context, manifest plugin.Manifest, pkg []byte) (string, error) {
	targetDir := s.Workspace(manifest)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", fmt.Errorf("create workspace: %w", err)
	}

	if len(pkg) == 0 {
		return targetDir, nil
	}

	readerAt := bytes.NewReader(pkg)
	archive, err := zip.NewReader(readerAt, int64(len(pkg)))
	if err != nil {
		return "", fmt.Errorf("open package: %w", err)
	}

	for _, file := range archive.File {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		if err := s.extractFile(file, targetDir); err != nil {
			return "", err
		}
	}

	return targetDir, nil
}

func (s *Store) Remove(ctx context.Context, manifest plugin.Manifest) error {
	dir := s.Workspace(manifest)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	return os.RemoveAll(dir)
}

func (s *Store) Workspace(manifest plugin.Manifest) string {
	safeName := fmt.Sprintf("%s-%s", manifest.Name, manifest.Version)
	return filepath.Join(s.workspaceRoot, safeName)
}

func (s *Store) extractFile(file *zip.File, targetDir string) error {
	path := filepath.Join(targetDir, file.Name)

	if file.FileInfo().IsDir() {
		return os.MkdirAll(path, file.Mode())
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	rc, err := file.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return err
	}

	return nil
}
