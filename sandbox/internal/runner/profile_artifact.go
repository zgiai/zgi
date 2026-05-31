package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const secureProfileBasePath = "/opt/zgi/profiles"

type dependencyProfileActivation struct {
	RootFS              string
	ProfileName         string
	ProfileHostDir      string
	ProfileContainerDir string
	ProfileChecksum     string
	ProfileSizeBytes    int64
	ProfileEnv          map[string]string
}

type builtProfileManifest struct {
	Name    string               `json:"name"`
	Version string               `json:"version"`
	Build   profileBuildMetadata `json:"build"`
}

type profileBuildMetadata struct {
	Checksum           string `json:"checksum"`
	SizeBytes          int64  `json:"size_bytes"`
	VerificationPassed bool   `json:"verification_passed"`
}

func resolveDependencyProfileActivation(defaultRootFS string, dependencyRootFSDir string, dependencyProfile string) (dependencyProfileActivation, error) {
	rootfs, err := rootFSSelector{
		defaultRootFS:       defaultRootFS,
		dependencyRootFSDir: dependencyRootFSDir,
	}.resolve(dependencyProfile)
	if err != nil {
		return dependencyProfileActivation{}, err
	}

	profile := strings.TrimSpace(dependencyProfile)
	activation := dependencyProfileActivation{RootFS: rootfs}
	if profile == "" {
		return activation, nil
	}
	if !safeDependencyProfileName(profile) {
		return dependencyProfileActivation{}, ErrUnsafeDependencyProfileName{Profile: profile}
	}
	profileEnv, err := secureDependencyProfileEnv(profile)
	if err != nil {
		return dependencyProfileActivation{}, err
	}
	activation.ProfileName = profile
	activation.ProfileContainerDir = secureProfileBasePath + "/" + profile
	activation.ProfileEnv = profileEnv
	if strings.TrimSpace(dependencyRootFSDir) == "" {
		return activation, nil
	}

	profileDir := filepath.Join(rootfs, strings.TrimPrefix(secureProfileBasePath, "/"), profile)
	manifest, err := validateBuiltProfileArtifact(profileDir, profile)
	if err != nil {
		return dependencyProfileActivation{}, err
	}
	activation.ProfileHostDir = profileDir
	activation.ProfileChecksum = manifest.Build.Checksum
	activation.ProfileSizeBytes = manifest.Build.SizeBytes
	return activation, nil
}

func validateBuiltProfileArtifact(profileDir string, expectedProfile string) (builtProfileManifest, error) {
	if err := validateProfileDir(profileDir); err != nil {
		return builtProfileManifest{}, fmt.Errorf("dependency profile artifact is not usable: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(profileDir, "manifest.json"))
	if err != nil {
		return builtProfileManifest{}, fmt.Errorf("read dependency profile artifact manifest: %w", err)
	}
	var manifest builtProfileManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return builtProfileManifest{}, fmt.Errorf("parse dependency profile artifact manifest: %w", err)
	}
	if manifest.Name != expectedProfile {
		return builtProfileManifest{}, fmt.Errorf("dependency profile artifact name %q does not match selected profile %q", manifest.Name, expectedProfile)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return builtProfileManifest{}, errors.New("dependency profile artifact version is required")
	}
	if strings.TrimSpace(manifest.Build.Checksum) == "" {
		return builtProfileManifest{}, errors.New("dependency profile artifact checksum is required")
	}
	if manifest.Build.SizeBytes <= 0 {
		return builtProfileManifest{}, errors.New("dependency profile artifact size_bytes must be positive")
	}
	if !manifest.Build.VerificationPassed {
		return builtProfileManifest{}, errors.New("dependency profile artifact verification has not passed")
	}
	checksum, size, err := checksumProfileArtifactDir(profileDir)
	if err != nil {
		return builtProfileManifest{}, err
	}
	if checksum != manifest.Build.Checksum {
		return builtProfileManifest{}, fmt.Errorf("dependency profile artifact checksum mismatch: manifest=%s actual=%s", manifest.Build.Checksum, checksum)
	}
	if size != manifest.Build.SizeBytes {
		return builtProfileManifest{}, fmt.Errorf("dependency profile artifact size mismatch: manifest=%d actual=%d", manifest.Build.SizeBytes, size)
	}
	return manifest, nil
}

func validateProfileDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("profile directory must not be a symlink")
	}
	if !info.IsDir() {
		return errors.New("profile directory must be a directory")
	}
	if info.Mode().Perm()&0o002 != 0 {
		return errors.New("profile directory must not be world-writable")
	}
	return nil
}

func checksumProfileArtifactDir(root string) (string, int64, error) {
	var files []string
	var size int64
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return validateProfileChildDir(root, path)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("dependency profile artifact contains symlink: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if filepath.ToSlash(rel) == "manifest.json" {
			return nil
		}
		files = append(files, rel)
		size += info.Size()
		return nil
	}); err != nil {
		return "", 0, err
	}
	slices.Sort(files)
	hash := sha256.New()
	for _, rel := range files {
		hash.Write([]byte(filepath.ToSlash(rel)))
		hash.Write([]byte{0})
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", 0, err
		}
		hash.Write(raw)
		hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), size, nil
}

func validateProfileChildDir(root string, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("dependency profile artifact contains symlink directory: %s", path)
	}
	if path == root {
		return nil
	}
	if info.Mode().Perm()&0o002 != 0 {
		rel, _ := filepath.Rel(root, path)
		return fmt.Errorf("dependency profile artifact directory is world-writable: %s", filepath.ToSlash(rel))
	}
	return nil
}
