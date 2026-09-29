package skills

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ZIP headers can make a valid 64 MiB package slightly larger in transit.
const portableSkillArchiveMaxBytes = ScriptPackageMaxBytes + (1 << 20)

// PortableSkillWorkspacePath returns the relative workspace directory for a
// content-addressed custom Skill package. It does not expose a host path.
func PortableSkillWorkspacePath(skillID, digest string) (string, error) {
	skillID = normalizeSkillID(skillID)
	if !isValidSkillName(skillID) || len(digest) != 64 {
		return "", fmt.Errorf("invalid skill package identity")
	}
	for _, ch := range digest {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			return "", fmt.Errorf("invalid skill package digest")
		}
	}
	return path.Join(".zgi", "skills", skillID, digest), nil
}

// BuildPortableSkillArchive snapshots a package before hashing and packaging
// so both values describe the same validated bytes. The caller owns the ZIP.
func BuildPortableSkillArchive(root string) ([]byte, string, error) {
	snapshot, err := snapshotSkillPackage(root)
	if err != nil {
		return nil, "", fmt.Errorf("snapshot skill package: %w", err)
	}
	defer os.RemoveAll(snapshot)
	digest, err := SkillPackageSHA256(snapshot)
	if err != nil {
		return nil, "", fmt.Errorf("hash skill package snapshot: %w", err)
	}
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	err = fs.WalkDir(os.DirFS(snapshot), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("skill package contains a nonregular file")
		}
		file, err := os.Open(filepath.Join(snapshot, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		output, err := writer.Create(filepath.ToSlash(name))
		if err != nil {
			file.Close()
			return err
		}
		_, copyErr := io.Copy(output, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if archive.Len() > portableSkillArchiveMaxBytes {
			return fmt.Errorf("skill package archive exceeds %d bytes", portableSkillArchiveMaxBytes)
		}
		return nil
	})
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, "", fmt.Errorf("archive skill package: %w", err)
	}
	if archive.Len() > portableSkillArchiveMaxBytes {
		return nil, "", fmt.Errorf("skill package archive exceeds %d bytes", portableSkillArchiveMaxBytes)
	}
	return archive.Bytes(), digest, nil
}
