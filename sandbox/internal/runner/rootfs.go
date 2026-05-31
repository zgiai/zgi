package runner

import (
	"crypto/sha256"
	"encoding/hex"
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

type ErrUnsafeDependencyProfileName struct {
	Profile string
}

func (e ErrUnsafeDependencyProfileName) Error() string {
	return fmt.Sprintf("dependency profile name is not safe: %s", e.Profile)
}

func (s rootFSSelector) resolve(dependencyProfile string, dependencyArtifactChecksum string) (string, error) {
	defaultRootFS := strings.TrimSpace(s.defaultRootFS)
	if defaultRootFS == "" {
		return "", errors.New("default rootfs is required")
	}
	profileDir := strings.TrimSpace(s.dependencyRootFSDir)
	runtimeKey := dependencyArtifactRuntimeKey(dependencyArtifactChecksum)
	artifactKey := runtimeKey
	if runtimeKey == "" {
		runtimeKey = strings.TrimSpace(dependencyProfile)
	}
	if profileDir == "" || runtimeKey == "" {
		return defaultRootFS, nil
	}
	if !safeDependencyProfileName(runtimeKey) {
		return "", fmt.Errorf("dependency profile rootfs selection failed: %w", ErrUnsafeDependencyProfileName{Profile: runtimeKey})
	}
	root := filepath.Join(profileDir, runtimeKey)
	if err := validateRuntimeRootFS(root); err != nil {
		if artifactKey != "" {
			fallback, fallbackErr := s.resolveDependencyProfileRootFS(profileDir, dependencyProfile)
			if fallbackErr == nil {
				return fallback, nil
			}
		}
		return "", fmt.Errorf("dependency profile rootfs %q is not usable: %w", runtimeKey, err)
	}
	return root, nil
}

func (s rootFSSelector) resolveDependencyProfileRootFS(profileDir string, dependencyProfile string) (string, error) {
	runtimeKey := strings.TrimSpace(dependencyProfile)
	if runtimeKey == "" {
		return "", errors.New("dependency profile is required")
	}
	if !safeDependencyProfileName(runtimeKey) {
		return "", fmt.Errorf("dependency profile rootfs selection failed: %w", ErrUnsafeDependencyProfileName{Profile: runtimeKey})
	}
	root := filepath.Join(profileDir, runtimeKey)
	if err := validateRuntimeRootFS(root); err != nil {
		return "", fmt.Errorf("dependency profile rootfs %q is not usable: %w", runtimeKey, err)
	}
	return root, nil
}

func dependencyArtifactRuntimeKey(checksum string) string {
	checksum = strings.ToLower(strings.TrimSpace(checksum))
	if checksum == "" {
		return ""
	}
	if safeDependencyProfileName(checksum) {
		return checksum
	}
	if strings.HasPrefix(checksum, "sha256:") {
		value := strings.TrimPrefix(checksum, "sha256:")
		if isLowerHex(value, 64) {
			return "sha256-" + value
		}
	}
	if strings.HasPrefix(checksum, "sha256-") {
		value := strings.TrimPrefix(checksum, "sha256-")
		if isLowerHex(value, 64) {
			return "sha256-" + value
		}
	}
	sum := sha256.Sum256([]byte(checksum))
	return "artifact-" + hex.EncodeToString(sum[:])
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, char := range value {
		if char >= '0' && char <= '9' || char >= 'a' && char <= 'f' {
			continue
		}
		return false
	}
	return true
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
