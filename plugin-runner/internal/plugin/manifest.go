package plugin

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// Language defines the runtime language a plugin uses.
type Language string

const (
	LanguagePython Language = "python"
	LanguageNode   Language = "node"
)

// Runner captures executable details for a plugin.
type Runner struct {
	Language   Language `json:"language"`
	Entrypoint string   `json:"entrypoint"`
}

// Requirements capture resource expectations and dependency hints.
type Requirements struct {
	CPU      string   `json:"cpu,omitempty"`
	Memory   string   `json:"memory,omitempty"`
	Packages []string `json:"packages,omitempty"`
	OS       []string `json:"os,omitempty"`
	Arch     []string `json:"arch,omitempty"`
}

// Capabilities enumerate the features a plugin claims to offer.
type Capabilities struct {
	Tools       []string `json:"tools,omitempty"`
	Models      []string `json:"models,omitempty"`
	Datasources []string `json:"datasources,omitempty"`
	Triggers    []string `json:"triggers,omitempty"`
	Endpoints   []string `json:"endpoints,omitempty"`
}

// Manifest stores the metadata required to start a plugin.
type Manifest struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`

	Description string   `json:"description,omitempty"`
	Author      string   `json:"author,omitempty"`
	Tags        []string `json:"tags,omitempty"`

	Runner       Runner       `json:"runner"`
	Requirements Requirements `json:"requirements,omitempty"`
	Capabilities Capabilities `json:"capabilities,omitempty"`
	Permissions  []string     `json:"permissions,omitempty"`
	Patches      []string     `json:"patches,omitempty"`

	MinimumPlatformVersion string `json:"minimum_platform_version,omitempty"`
	Signature              string `json:"signature,omitempty"`
	SignatureKeyID         string `json:"signature_key_id,omitempty"`

	// Marketplace IDs (set when installed from marketplace)
	MarketplacePluginID  string `json:"marketplace_plugin_id,omitempty"`
	MarketplaceVersionID string `json:"marketplace_version_id,omitempty"`
}

// Validate ensures the manifest contains enough information for the executor.
func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("plugin name is required")
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("plugin version is required")
	}
	if strings.TrimSpace(m.Runner.Entrypoint) == "" {
		return fmt.Errorf("plugin entrypoint is required")
	}

	switch normalizeLanguage(m.Runner.Language) {
	case LanguagePython:
		// OK for current runtime
	case LanguageNode:
		// Supported in extended runtime
	default:
		return fmt.Errorf("language %q is not supported yet", m.Runner.Language)
	}

	if err := validateStrings("tags", m.Tags); err != nil {
		return err
	}
	if err := validateStrings("permissions", m.Permissions); err != nil {
		return err
	}
	if err := validateStrings("requirements.packages", m.Requirements.Packages); err != nil {
		return err
	}
	if err := validateOSArch(m.Requirements.OS, m.Requirements.Arch); err != nil {
		return err
	}
	if err := validateCapabilities(m.Capabilities); err != nil {
		return err
	}

	if err := validatePermissions(m.Permissions); err != nil {
		return err
	}

	return nil
}

// CanonicalString builds a deterministic JSON string for signing.
func (m Manifest) CanonicalString() string {
	type canonical struct {
		ID                     string       `json:"id,omitempty"`
		Name                   string       `json:"name"`
		Version                string       `json:"version"`
		Description            string       `json:"description,omitempty"`
		Author                 string       `json:"author,omitempty"`
		Tags                   []string     `json:"tags,omitempty"`
		Runner                 Runner       `json:"runner"`
		Requirements           Requirements `json:"requirements,omitempty"`
		Capabilities           Capabilities `json:"capabilities,omitempty"`
		Permissions            []string     `json:"permissions,omitempty"`
		Patches                []string     `json:"patches,omitempty"`
		MinimumPlatformVersion string       `json:"minimum_platform_version,omitempty"`
		SignatureKeyID         string       `json:"signature_key_id,omitempty"`
	}
	body := canonical{
		ID:                     m.ID,
		Name:                   m.Name,
		Version:                m.Version,
		Description:            m.Description,
		Author:                 m.Author,
		Tags:                   m.Tags,
		Runner:                 m.Runner,
		Requirements:           m.Requirements,
		Capabilities:           m.Capabilities,
		Permissions:            m.Permissions,
		Patches:                m.Patches,
		MinimumPlatformVersion: m.MinimumPlatformVersion,
		SignatureKeyID:         m.SignatureKeyID,
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// ComputeSignatureDigest returns SHA-256 digest of canonical string.
func (m Manifest) ComputeSignatureDigest() []byte {
	sum := sha256.Sum256([]byte(m.CanonicalString()))
	return sum[:]
}

// SetSignature sets signature as base64.
func (m *Manifest) SetSignature(sig []byte) {
	m.Signature = base64.StdEncoding.EncodeToString(sig)
}

func normalizeLanguage(lang Language) Language {
	switch strings.ToLower(string(lang)) {
	case "py", "python":
		return LanguagePython
	case "node", "nodejs", "javascript", "js":
		return LanguageNode
	default:
		return lang
	}
}

func validateStrings(field string, values []string) error {
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s contains empty value", field)
		}
	}
	return nil
}

func validateOSArch(osList, archList []string) error {
	allowedOS := map[string]struct{}{
		"linux":   {},
		"darwin":  {},
		"windows": {},
	}
	allowedArch := map[string]struct{}{
		"amd64": {},
		"arm64": {},
	}

	for _, os := range osList {
		if _, ok := allowedOS[strings.ToLower(os)]; !ok {
			return fmt.Errorf("os %q is not supported", os)
		}
	}
	for _, arch := range archList {
		if _, ok := allowedArch[strings.ToLower(arch)]; !ok {
			return fmt.Errorf("arch %q is not supported", arch)
		}
	}
	return nil
}

func validateCapabilities(c Capabilities) error {
	whitelist := map[string][]string{
		"capabilities.tools":       c.Tools,
		"capabilities.models":      c.Models,
		"capabilities.datasources": c.Datasources,
		"capabilities.triggers":    c.Triggers,
		"capabilities.endpoints":   c.Endpoints,
	}
	for field, vals := range whitelist {
		if err := validateStrings(field, vals); err != nil {
			return err
		}
	}
	return nil
}

func validatePermissions(perms []string) error {
	if len(perms) == 0 {
		return nil
	}
	allowed := map[string]struct{}{
		"network":      {},
		"filesystem":   {},
		"external_api": {},
		"process":      {},
	}
	for _, p := range perms {
		val := strings.TrimSpace(p)
		if val == "" {
			return fmt.Errorf("permissions contains empty value")
		}
		if _, ok := allowed[strings.ToLower(val)]; !ok {
			return fmt.Errorf("permission %q is not allowed", p)
		}
	}
	return nil
}
