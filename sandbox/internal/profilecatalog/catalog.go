package profilecatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

type Profile struct {
	Name               string    `json:"name"`
	Version            string    `json:"version"`
	Status             string    `json:"status"`
	Enabled            bool      `json:"enabled"`
	OwnerScope         string    `json:"owner_scope"`
	Languages          []string  `json:"languages"`
	BaseRuntime        string    `json:"base_runtime"`
	Checksum           string    `json:"checksum"`
	EstimatedSizeBytes int64     `json:"estimated_size_bytes"`
	Description        string    `json:"description"`
	Packages           []Package `json:"packages"`
	RequiredFiles      []string  `json:"required_files"`
}

type Package struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

func Load(source fs.FS) ([]Profile, error) {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, fmt.Errorf("read profile source root: %w", err)
	}
	profiles := make([]Profile, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		profile, err := loadProfile(source, name)
		if err != nil {
			return nil, err
		}
		if seen[profile.Name] {
			return nil, fmt.Errorf("duplicate profile source: %s", profile.Name)
		}
		seen[profile.Name] = true
		profiles = append(profiles, profile)
	}
	if len(profiles) == 0 {
		return nil, errors.New("profile source catalog is empty")
	}
	slices.SortFunc(profiles, func(left Profile, right Profile) int {
		return strings.Compare(left.Name, right.Name)
	})
	return profiles, nil
}

func loadProfile(source fs.FS, dir string) (Profile, error) {
	raw, err := fs.ReadFile(source, filepath.ToSlash(filepath.Join(dir, "manifest.json")))
	if err != nil {
		return Profile{}, fmt.Errorf("read profile manifest %s: %w", dir, err)
	}
	var profile Profile
	if err := json.Unmarshal(raw, &profile); err != nil {
		return Profile{}, fmt.Errorf("parse profile manifest %s: %w", dir, err)
	}
	if err := Validate(source, dir, profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func Validate(source fs.FS, dir string, profile Profile) error {
	profile.Name = strings.TrimSpace(profile.Name)
	if !safeName(profile.Name) {
		return errors.New("profile name must contain only lowercase letters, numbers, and hyphens")
	}
	if dir != "" && profile.Name != dir {
		return fmt.Errorf("profile directory %s does not match manifest name %s", dir, profile.Name)
	}
	if !pinnedVersion(profile.Version) {
		return fmt.Errorf("profile %s version must be pinned", profile.Name)
	}
	if !validStatus(profile.Status) {
		return fmt.Errorf("profile %s status is not supported: %s", profile.Name, profile.Status)
	}
	if profile.Status != "ready" && profile.Enabled {
		return fmt.Errorf("profile %s must not be enabled while status is %s", profile.Name, profile.Status)
	}
	if strings.TrimSpace(profile.OwnerScope) == "" {
		return fmt.Errorf("profile %s owner_scope is required", profile.Name)
	}
	if len(profile.Languages) == 0 {
		return fmt.Errorf("profile %s must declare at least one language", profile.Name)
	}
	for _, language := range profile.Languages {
		if !supportedLanguage(language) {
			return fmt.Errorf("profile %s language is not supported: %s", profile.Name, language)
		}
	}
	if strings.TrimSpace(profile.BaseRuntime) == "" {
		return fmt.Errorf("profile %s base_runtime is required", profile.Name)
	}
	if strings.TrimSpace(profile.Checksum) == "" {
		return fmt.Errorf("profile %s checksum is required", profile.Name)
	}
	if profile.EstimatedSizeBytes <= 0 {
		return fmt.Errorf("profile %s estimated_size_bytes must be positive", profile.Name)
	}
	if err := validateRequiredFiles(source, dir, profile); err != nil {
		return err
	}
	for _, item := range profile.Packages {
		if err := validatePackage(profile.Name, item); err != nil {
			return err
		}
	}
	return nil
}

func validateRequiredFiles(source fs.FS, dir string, profile Profile) error {
	if len(profile.RequiredFiles) == 0 {
		return fmt.Errorf("profile %s required_files must not be empty", profile.Name)
	}
	seen := map[string]bool{}
	for _, raw := range profile.RequiredFiles {
		name := filepath.ToSlash(strings.TrimSpace(raw))
		if unsafeRelativePath(name) {
			return fmt.Errorf("profile %s required file path is unsafe: %s", profile.Name, name)
		}
		if seen[name] {
			return fmt.Errorf("profile %s has duplicate required file: %s", profile.Name, name)
		}
		seen[name] = true
		info, err := fs.Stat(source, filepath.ToSlash(filepath.Join(dir, name)))
		if err != nil {
			return fmt.Errorf("profile %s required file is missing: %s", profile.Name, name)
		}
		if info.IsDir() {
			return fmt.Errorf("profile %s required file is a directory: %s", profile.Name, name)
		}
	}
	return nil
}

func validatePackage(profileName string, item Package) error {
	ecosystem := strings.TrimSpace(item.Ecosystem)
	if !supportedEcosystem(ecosystem) {
		return fmt.Errorf("profile %s package ecosystem is not supported: %s", profileName, ecosystem)
	}
	if strings.TrimSpace(item.Name) == "" {
		return fmt.Errorf("profile %s package name is required", profileName)
	}
	if !pinnedVersion(item.Version) && strings.TrimSpace(item.Version) != "managed" {
		return fmt.Errorf("profile %s package %s version must be pinned", profileName, item.Name)
	}
	if strings.Contains(item.Name, "://") || strings.HasPrefix(item.Name, ".") || strings.HasPrefix(item.Name, "/") {
		return fmt.Errorf("profile %s package %s must not use a path or URL", profileName, item.Name)
	}
	return nil
}

func safeName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func pinnedVersion(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "latest" && !strings.Contains(value, "*")
}

func validStatus(value string) bool {
	switch strings.TrimSpace(value) {
	case "draft", "building", "ready", "failed", "disabled":
		return true
	default:
		return false
	}
}

func supportedLanguage(value string) bool {
	switch strings.TrimSpace(value) {
	case "python3", "nodejs":
		return true
	default:
		return false
	}
}

func supportedEcosystem(value string) bool {
	switch strings.TrimSpace(value) {
	case "python3", "nodejs", "system":
		return true
	default:
		return false
	}
}

func unsafeRelativePath(value string) bool {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(value)))
	return clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/")
}
