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
	Name               string                `json:"name"`
	Version            string                `json:"version"`
	Status             string                `json:"status"`
	Enabled            bool                  `json:"enabled"`
	OwnerScope         string                `json:"owner_scope"`
	Languages          []string              `json:"languages"`
	BaseRuntime        string                `json:"base_runtime"`
	Checksum           string                `json:"checksum"`
	EstimatedSizeBytes int64                 `json:"estimated_size_bytes"`
	Description        string                `json:"description"`
	Packages           []builtProfilePackage `json:"packages"`
	Build              profileBuildMetadata  `json:"build"`
}

type builtProfilePackage struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Ecosystem string `json:"ecosystem,omitempty"`
}

type profileBuildMetadata struct {
	Checksum           string `json:"checksum"`
	SizeBytes          int64  `json:"size_bytes"`
	VerificationPassed bool   `json:"verification_passed"`
}

type DependencyProfileArtifact struct {
	Name        string
	Version     string
	OwnerScope  string
	Languages   []string
	Packages    []DependencyProfileArtifactPackage
	BaseRuntime string
	Checksum    string
	SizeBytes   int64
	Description string
}

type DependencyProfileArtifactPackage struct {
	Name      string
	Version   string
	Ecosystem string
}

func ListDependencyProfileArtifacts(dependencyRootFSDir string) ([]DependencyProfileArtifact, error) {
	root := strings.TrimSpace(dependencyRootFSDir)
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dependency profile rootfs directory: %w", err)
	}
	artifacts := make([]DependencyProfileArtifact, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		profile := entry.Name()
		if !safeDependencyProfileName(profile) {
			return nil, ErrUnsafeDependencyProfileName{Profile: profile}
		}
		profileDir := filepath.Join(root, profile, strings.TrimPrefix(secureProfileBasePath, "/"), profile)
		manifest, err := validateBuiltProfileArtifact(profileDir, profile)
		if err != nil {
			return nil, fmt.Errorf("inspect dependency profile artifact %s: %w", profile, err)
		}
		artifacts = append(artifacts, dependencyProfileArtifactFromManifest(manifest))
	}
	return artifacts, nil
}

func dependencyProfileArtifactFromManifest(manifest builtProfileManifest) DependencyProfileArtifact {
	packages := make([]DependencyProfileArtifactPackage, 0, len(manifest.Packages))
	for _, item := range manifest.Packages {
		packages = append(packages, DependencyProfileArtifactPackage{
			Name:      item.Name,
			Version:   item.Version,
			Ecosystem: item.Ecosystem,
		})
	}
	return DependencyProfileArtifact{
		Name:        manifest.Name,
		Version:     manifest.Version,
		OwnerScope:  defaultString(manifest.OwnerScope, "global"),
		Languages:   append([]string(nil), manifest.Languages...),
		Packages:    packages,
		BaseRuntime: defaultString(manifest.BaseRuntime, "linux-secure"),
		Checksum:    manifest.Build.Checksum,
		SizeBytes:   manifest.Build.SizeBytes,
		Description: strings.TrimSpace(manifest.Description),
	}
}

func defaultString(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func resolveDependencyProfileActivation(defaultRootFS string, dependencyRootFSDir string, dependencyProfile string, dependencyArtifactChecksum string) (dependencyProfileActivation, error) {
	rootfs, err := rootFSSelector{
		defaultRootFS:       defaultRootFS,
		dependencyRootFSDir: dependencyRootFSDir,
	}.resolve(dependencyProfile, dependencyArtifactChecksum)
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

	profileDir, manifest, err := findRuntimeProfileArtifact(rootfs, profile, dependencyArtifactChecksum)
	if err != nil {
		return dependencyProfileActivation{}, err
	}
	activation.ProfileHostDir = profileDir
	activation.ProfileChecksum = manifest.Build.Checksum
	activation.ProfileSizeBytes = manifest.Build.SizeBytes
	return activation, nil
}

func findRuntimeProfileArtifact(rootfs string, dependencyProfile string, dependencyArtifactChecksum string) (string, builtProfileManifest, error) {
	profilesRoot := filepath.Join(rootfs, strings.TrimPrefix(secureProfileBasePath, "/"))
	preferred := filepath.Join(profilesRoot, dependencyProfile)
	manifest, err := validateBuiltProfileArtifactForActivation(preferred, dependencyProfile, dependencyArtifactChecksum)
	if err == nil {
		return preferred, manifest, nil
	}
	if strings.TrimSpace(dependencyArtifactChecksum) == "" {
		return "", builtProfileManifest{}, err
	}

	entries, readErr := os.ReadDir(profilesRoot)
	if readErr != nil {
		return "", builtProfileManifest{}, readErr
	}
	for _, entry := range entries {
		if !entry.IsDir() || !safeDependencyProfileName(entry.Name()) {
			continue
		}
		candidate := filepath.Join(profilesRoot, entry.Name())
		manifest, err := validateBuiltProfileArtifactForActivation(candidate, dependencyProfile, dependencyArtifactChecksum)
		if err == nil {
			return candidate, manifest, nil
		}
	}
	return "", builtProfileManifest{}, fmt.Errorf("dependency profile artifact checksum %q is not available for profile %q", dependencyArtifactChecksum, dependencyProfile)
}

func validateBuiltProfileArtifact(profileDir string, expectedProfile string) (builtProfileManifest, error) {
	return validateBuiltProfileArtifactForActivation(profileDir, expectedProfile, "")
}

func validateBuiltProfileArtifactForActivation(profileDir string, expectedProfile string, expectedChecksum string) (builtProfileManifest, error) {
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
	expectedChecksum = strings.TrimSpace(expectedChecksum)
	if manifest.Name != expectedProfile && expectedChecksum == "" {
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
	if expectedChecksum != "" && checksum != expectedChecksum {
		return builtProfileManifest{}, fmt.Errorf("dependency profile artifact checksum %q does not match selected artifact %q", checksum, expectedChecksum)
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
