package skills

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Snapshot into a private directory before authorization so hashing, manifest
// validation, dependency builds, and uploads all consume identical package bytes.
// Root confines reads even if an imported directory changes during the copy.
func snapshotSkillPackage(source string) (string, error) {
	root, err := os.OpenRoot(source)
	if err != nil {
		return "", err
	}
	defer root.Close()
	target, err := os.MkdirTemp("", "zgi-skill-snapshot-")
	if err != nil {
		return "", err
	}
	var total int64
	count := 0
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill package contains a symlink")
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(target, filepath.FromSlash(name)), 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("skill package contains a nonregular file")
		}
		count++
		if count > ScriptPackageMaxFiles {
			return fmt.Errorf("skill package exceeds snapshot file limit")
		}
		in, err := root.Open(name)
		if err != nil {
			return err
		}
		defer in.Close()
		info, err := in.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("skill package contains a nonregular file")
		}
		if info.Size() > ScriptPackageMaxFileBytes {
			return fmt.Errorf("skill package file exceeds snapshot size limit")
		}
		out, err := os.OpenFile(filepath.Join(target, filepath.FromSlash(name)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(in, ScriptPackageMaxBytes-total+1))
		closeErr := out.Close()
		total += n
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if total > ScriptPackageMaxBytes {
			return fmt.Errorf("skill package exceeds snapshot size limit")
		}
		return nil
	})
	if err != nil {
		os.RemoveAll(target)
		return "", err
	}
	return target, nil
}

// validateSkillPackageUploadSize applies the package size limit to the bytes
// that will actually be uploaded. A normalized manifest replaces the source
// manifest in the archive and can be larger after defaults are populated.
func validateSkillPackageUploadSize(root string, manifestRaw []byte) error {
	var total int64
	manifestReplaced := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill package contains a symlink")
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("skill package contains a nonregular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		size := info.Size()
		if len(manifestRaw) > 0 && filepath.ToSlash(relative) == "skill.manifest.json" {
			size = int64(len(manifestRaw))
			manifestReplaced = true
		}
		total += size
		if total > ScriptPackageMaxBytes {
			return fmt.Errorf("skill package exceeds upload size limit of %d bytes", ScriptPackageMaxBytes)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(manifestRaw) > 0 && !manifestReplaced {
		total += int64(len(manifestRaw))
	}
	if total > ScriptPackageMaxBytes {
		return fmt.Errorf("skill package exceeds upload size limit of %d bytes", ScriptPackageMaxBytes)
	}
	return nil
}
