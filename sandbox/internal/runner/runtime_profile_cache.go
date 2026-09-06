package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// seedDependencyProfileRuntimeCache copies trusted, build-time generated cache
// entries into the isolated workspace. Scientific runtimes such as Matplotlib
// need a writable cache, but sharing that writable directory between tenants
// would permit cache poisoning. A small per-session copy gives each execution
// fast startup without weakening the profile's read-only trust boundary.
func seedDependencyProfileRuntimeCache(profileDir string, workDir string) error {
	if profileDir == "" || workDir == "" {
		return nil
	}
	source := filepath.Join(profileDir, "runtime-cache", "matplotlib")
	if info, err := os.Stat(source); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect dependency profile runtime cache: %w", err)
	} else if !info.IsDir() {
		return errors.New("dependency profile matplotlib runtime cache must be a directory")
	}

	cacheRoot := filepath.Join(workDir, ".zgi-runtime-cache")
	destination := filepath.Join(cacheRoot, "matplotlib")
	if _, err := os.Stat(destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect workspace runtime cache: %w", err)
	}
	if err := os.MkdirAll(cacheRoot, 0o777); err != nil {
		return fmt.Errorf("prepare workspace runtime cache: %w", err)
	}
	temporary, err := os.MkdirTemp(cacheRoot, ".matplotlib-seed-")
	if err != nil {
		return fmt.Errorf("prepare workspace matplotlib cache: %w", err)
	}
	defer os.RemoveAll(temporary)
	if err := copyTrustedRuntimeCache(source, temporary); err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0o777); err != nil {
		return fmt.Errorf("make workspace matplotlib cache writable: %w", err)
	}
	if err := os.Rename(temporary, destination); err != nil {
		if _, statErr := os.Stat(destination); statErr == nil {
			return nil
		}
		return fmt.Errorf("activate workspace matplotlib cache: %w", err)
	}
	return nil
}

func copyTrustedRuntimeCache(source string, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(destination, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("dependency profile runtime cache contains unsupported entry: %s", relative)
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0o777)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
