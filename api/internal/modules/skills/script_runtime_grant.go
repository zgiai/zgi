package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ScriptRuntimeGrant is administrator-owned configuration, never package metadata.
// A grant authorizes exactly one organization's package content and runtime.
type ScriptRuntimeGrant struct {
	OrganizationID    string `json:"organization_id"`
	PackageSHA256     string `json:"package_sha256"`
	DependencyProfile string `json:"dependency_profile"`
	ProfileVersion    string `json:"profile_version"`
	ArtifactChecksum  string `json:"artifact_checksum"`
	MaxArtifactCount  int    `json:"max_artifact_count"`
	MaxArtifactBytes  int64  `json:"max_artifact_bytes"`
	MaxInputFileBytes int64  `json:"max_input_file_bytes"`
}

// SkillPackageSHA256 hashes sorted relative names and file content hashes. ZIP
// timestamps and compression do not change the identity of an imported package.
func SkillPackageSHA256(root string) (string, error) {
	var names []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("skill package contains a symlink")
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("skill package contains a nonregular file")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(content)
		fmt.Fprintf(hash, "%s\x00%x\n", name, digest)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func loadScriptRuntimeGrants(path string) ([]ScriptRuntimeGrant, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open script runtime grants: %w", err)
	}
	defer file.Close()
	var grants []ScriptRuntimeGrant
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&grants); err != nil {
		return nil, fmt.Errorf("decode script runtime grants: %w", err)
	}
	if decoder.Decode(new(interface{})) != io.EOF {
		return nil, fmt.Errorf("runtime grants must contain exactly one JSON document")
	}
	seen := map[string]bool{}
	for _, grant := range grants {
		key := grant.OrganizationID + "/" + grant.PackageSHA256
		if seen[key] {
			return nil, fmt.Errorf("duplicate runtime grant organization and package")
		}
		seen[key] = true
		checksum, checksumErr := hex.DecodeString(strings.TrimPrefix(grant.ArtifactChecksum, "sha256:"))
		if checksumErr != nil || len(checksum) != 32 || !strings.HasPrefix(grant.ArtifactChecksum, "sha256:") {
			return nil, fmt.Errorf("runtime grant artifact checksum must be SHA256")
		}
		digest, err := hex.DecodeString(grant.PackageSHA256)
		if err != nil || len(digest) != 32 || grant.OrganizationID == "" || grant.DependencyProfile == "" || grant.ProfileVersion == "" || grant.ArtifactChecksum == "" {
			return nil, fmt.Errorf("runtime grant requires organization, package SHA256, profile, version and artifact checksum")
		}
		if grant.MaxArtifactCount < 1 || grant.MaxArtifactCount > maxManagedSkillScriptArtifactCount || grant.MaxArtifactBytes < 1 || grant.MaxArtifactBytes > maxManagedSkillScriptArtifactBytes || grant.MaxInputFileBytes < 1 || grant.MaxInputFileBytes > maxManagedSkillScriptInputFileBytes {
			return nil, fmt.Errorf("runtime grant quotas exceed server limits")
		}
	}
	return grants, nil
}

func (r *SandboxScriptRunner) runtimeGrant(root, organizationID string) (*ScriptRuntimeGrant, string, error) {
	if r.runtimeGrantsError != nil {
		return nil, "", r.runtimeGrantsError
	}
	digest, err := SkillPackageSHA256(root)
	if err != nil {
		return nil, "", err
	}
	for _, grant := range r.runtimeGrants {
		if organizationID != "" && grant.OrganizationID == organizationID && grant.PackageSHA256 == digest {
			return &grant, digest, nil
		}
	}
	return nil, digest, nil
}
