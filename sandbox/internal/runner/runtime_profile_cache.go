package runner

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	runtimeCacheRootName       = ".zgi-runtime-cache"
	matplotlibRuntimeCacheName = "matplotlib"
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

	workspaceRoot, err := os.OpenRoot(workDir)
	if err != nil {
		return fmt.Errorf("open workspace root: %w", err)
	}
	defer func() { _ = workspaceRoot.Close() }()

	cacheRoot, err := openWorkspaceRuntimeCacheRoot(workspaceRoot)
	if err != nil {
		return err
	}
	defer func() { _ = cacheRoot.Close() }()

	if info, err := cacheRoot.Lstat(matplotlibRuntimeCacheName); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("workspace matplotlib runtime cache must not be a symbolic link")
		}
		if !info.IsDir() {
			return errors.New("workspace matplotlib runtime cache must be a directory")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect workspace runtime cache: %w", err)
	}
	temporary := ".matplotlib-seed-" + rand.Text()
	if err := cacheRoot.Mkdir(temporary, 0o700); err != nil {
		return fmt.Errorf("prepare workspace matplotlib cache: %w", err)
	}
	defer func() { _ = cacheRoot.RemoveAll(temporary) }()
	temporaryRoot, err := cacheRoot.OpenRoot(temporary)
	if err != nil {
		return fmt.Errorf("open workspace matplotlib cache: %w", err)
	}
	if err := copyTrustedRuntimeCache(source, temporaryRoot); err != nil {
		_ = temporaryRoot.Close()
		return err
	}
	if err := temporaryRoot.Chmod(".", 0o777); err != nil {
		_ = temporaryRoot.Close()
		return fmt.Errorf("make workspace matplotlib cache writable: %w", err)
	}
	if err := temporaryRoot.Close(); err != nil {
		return fmt.Errorf("close workspace matplotlib cache: %w", err)
	}
	if err := cacheRoot.Rename(temporary, matplotlibRuntimeCacheName); err != nil {
		if info, statErr := cacheRoot.Lstat(matplotlibRuntimeCacheName); statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		return fmt.Errorf("activate workspace matplotlib cache: %w", err)
	}
	return nil
}

func openWorkspaceRuntimeCacheRoot(workspaceRoot *os.Root) (*os.Root, error) {
	info, err := workspaceRoot.Lstat(runtimeCacheRootName)
	if errors.Is(err, os.ErrNotExist) {
		if err := workspaceRoot.Mkdir(runtimeCacheRootName, 0o777); err != nil {
			return nil, fmt.Errorf("prepare workspace runtime cache: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("inspect workspace runtime cache root: %w", err)
	} else {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("workspace runtime cache root must not be a symbolic link")
		}
		if !info.IsDir() {
			return nil, errors.New("workspace runtime cache root must be a directory")
		}
	}
	cacheRoot, err := workspaceRoot.OpenRoot(runtimeCacheRootName)
	if err != nil {
		return nil, fmt.Errorf("open workspace runtime cache root: %w", err)
	}
	return cacheRoot, nil
}

func copyTrustedRuntimeCache(source string, destination *os.Root) error {
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
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return fmt.Errorf("dependency profile runtime cache contains unsupported entry: %s", relative)
		}
		if info.IsDir() {
			return destination.MkdirAll(relative, 0o777)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := destination.OpenFile(relative, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		return errors.Join(copyErr, inputCloseErr, outputCloseErr)
	})
}
