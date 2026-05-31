package runner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type rootFSSelector struct {
	defaultRootFS       string
	dependencyRootFSDir string
}

func (s rootFSSelector) resolve(dependencyProfile string) (string, error) {
	defaultRootFS := strings.TrimSpace(s.defaultRootFS)
	if defaultRootFS == "" {
		return "", errors.New("default rootfs is required")
	}
	profileDir := strings.TrimSpace(s.dependencyRootFSDir)
	profile := strings.TrimSpace(dependencyProfile)
	if profileDir == "" || profile == "" {
		return defaultRootFS, nil
	}
	if !safeDependencyProfileName(profile) {
		return "", fmt.Errorf("dependency profile name is not safe for rootfs selection: %s", profile)
	}
	root := filepath.Join(profileDir, profile)
	if err := validateRuntimeRootFS(root); err != nil {
		return "", fmt.Errorf("dependency profile rootfs %q is not usable: %w", profile, err)
	}
	return root, nil
}

func safeDependencyProfileName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' ||
			char >= '0' && char <= '9' ||
			char == '-' {
			continue
		}
		return false
	}
	return true
}

func validateRuntimeRootFS(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("rootfs must not be a symlink")
	}
	if !info.IsDir() {
		return errors.New("rootfs must be a directory")
	}
	if info.Mode().Perm()&0o002 != 0 {
		return errors.New("rootfs must not be world-writable")
	}
	return nil
}
