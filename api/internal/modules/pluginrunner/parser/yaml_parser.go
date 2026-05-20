package parser

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PluginManifest represents the manifest.yaml structure
type PluginManifest struct {
	Name        string            `yaml:"name" json:"name"`
	Author      string            `yaml:"author" json:"author"`
	Version     string            `yaml:"version" json:"version"`
	Label       map[string]string `yaml:"label" json:"label"`
	Description map[string]string `yaml:"description" json:"description"`
	Icon        string            `yaml:"icon" json:"icon,omitempty"`
	Tags        []string          `yaml:"tags" json:"tags,omitempty"`
	Plugins     struct {
		Tools []string `yaml:"tools" json:"tools"`
	} `yaml:"plugins" json:"plugins"`
	Meta struct {
		Version string `yaml:"version"`
		Runner  struct {
			Language   string `yaml:"language"`
			Entrypoint string `yaml:"entrypoint"`
			Version    string `yaml:"version"`
		} `yaml:"runner"`
	} `yaml:"meta"`
}

// ProviderDefinition represents provider/*.yaml structure
type ProviderDefinition struct {
	Identity struct {
		Name        string            `yaml:"name" json:"name"`
		Author      string            `yaml:"author" json:"author,omitempty"`
		Label       map[string]string `yaml:"label" json:"label"`
		Description map[string]string `yaml:"description" json:"description"`
		Icon        string            `yaml:"icon" json:"icon,omitempty"`
		Tags        []string          `yaml:"tags" json:"tags,omitempty"`
	} `yaml:"identity" json:"identity"`
	Tools           []string         `yaml:"tools" json:"tools"`
	ExecutionPolicy *ExecutionPolicy `yaml:"execution_policy" json:"execution_policy,omitempty"`
}

// ExecutionPolicy defines provider-level execution behavior.
type ExecutionPolicy struct {
	WaitMode              string `yaml:"wait_mode" json:"wait_mode,omitempty"`
	StreamMode            string `yaml:"stream_mode" json:"stream_mode,omitempty"`
	SessionPolicy         string `yaml:"session_policy" json:"session_policy,omitempty"`
	SessionIdleTTLSeconds int    `yaml:"session_idle_ttl_seconds" json:"session_idle_ttl_seconds,omitempty"`
	SerializeInvocations  *bool  `yaml:"serialize_invocations" json:"serialize_invocations,omitempty"`
}

// ToolDefinition represents tools/*.yaml structure
type ToolDefinition struct {
	Identity struct {
		Name   string            `yaml:"name" json:"name"`
		Author string            `yaml:"author" json:"author,omitempty"`
		Label  map[string]string `yaml:"label" json:"label"`
	} `yaml:"identity" json:"identity"`
	Description struct {
		Human map[string]string `yaml:"human" json:"human"`
		LLM   string            `yaml:"llm" json:"llm"`
	} `yaml:"description" json:"description"`
	Parameters []ParameterDefinition `yaml:"parameters" json:"parameters"`
}

// ParameterDefinition represents a tool parameter
type ParameterDefinition struct {
	Name             string            `yaml:"name" json:"name"`
	Type             string            `yaml:"type" json:"type"`
	Required         bool              `yaml:"required" json:"required"`
	Form             string            `yaml:"form" json:"form"`
	Label            map[string]string `yaml:"label" json:"label"`
	HumanDescription map[string]string `yaml:"human_description" json:"human_description,omitempty"`
	Default          interface{}       `yaml:"default" json:"default,omitempty"`
	Options          []Option          `yaml:"options" json:"options,omitempty"`
}

// ConfigurationDefinition represents a provider configuration parameter
type ConfigurationDefinition struct {
	Name             string            `yaml:"-" json:"name"`
	Type             string            `yaml:"type" json:"type"`
	Required         bool              `yaml:"required" json:"required"`
	Label            map[string]string `yaml:"label" json:"label"`
	Help             map[string]string `yaml:"help" json:"help,omitempty"`
	Placeholder      map[string]string `yaml:"placeholder" json:"placeholder,omitempty"`
	Default          interface{}       `yaml:"default" json:"default,omitempty"`
	Options          []Option          `yaml:"options" json:"options,omitempty"`
	HumanDescription map[string]string `yaml:"human_description" json:"human_description,omitempty"`
}

// Option represents a parameter option
type Option struct {
	Value interface{}       `yaml:"value" json:"value"`
	Label map[string]string `yaml:"label" json:"label"`
}

// PluginDeclaration is the combined declaration structure to store in database
type PluginDeclaration struct {
	Provider ProviderInfo `json:"provider"`
	Tools    []ToolInfo   `json:"tools"`
}

// ProviderInfo represents provider info in declaration
type ProviderInfo struct {
	Name            string            `json:"name"`
	Author          string            `json:"author,omitempty"`
	Label           map[string]string `json:"label"`
	Description     map[string]string `json:"description"`
	Icon            string            `json:"icon,omitempty"`
	Tags            []string          `json:"tags,omitempty"`
	ExecutionPolicy *ExecutionPolicy  `json:"execution_policy,omitempty"`
}

// ToolInfo represents tool info in declaration
type ToolInfo struct {
	Name           string                    `json:"name"`
	Label          map[string]string         `json:"label"`
	Description    ToolDescription           `json:"description"`
	Parameters     []ParameterDefinition     `json:"parameters"`
	Configurations []ConfigurationDefinition `json:"configurations,omitempty"`
}

// ToolDescription represents tool description
type ToolDescription struct {
	Human map[string]string `json:"human"`
	LLM   string            `json:"llm"`
}

// RunnerManifest contains info needed by Plugin Runner
type RunnerManifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Entrypoint  string   `json:"entrypoint"`
	Language    string   `json:"language"`
}

// ParseResult contains both declaration (for database) and manifest (for runner) from single ZIP parse
type ParseResult struct {
	Declaration *PluginDeclaration // For database storage
	Manifest    *RunnerManifest    // For Plugin Runner
}

// ParsePluginDirectory parses plugin YAML files from a directory
func ParsePluginDirectory(pluginDir string) (*PluginDeclaration, error) {
	// Read manifest.yaml
	manifestPath := filepath.Join(pluginDir, "manifest.yaml")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest.yaml: %w", err)
	}

	var manifest PluginManifest
	if err := yaml.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest.yaml: %w", err)
	}

	// Parse provider files
	var declaration PluginDeclaration
	for _, providerPath := range manifest.Plugins.Tools {
		fullPath := filepath.Join(pluginDir, providerPath)
		providerDecl, configDefs, err := parseProviderFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse provider %s: %w", providerPath, err)
		}

		// Set provider info (use first provider)
		if declaration.Provider.Name == "" {
			declaration.Provider = ProviderInfo{
				Name:            providerDecl.Identity.Name,
				Author:          providerDecl.Identity.Author,
				Label:           providerDecl.Identity.Label,
				Description:     providerDecl.Identity.Description,
				Icon:            providerDecl.Identity.Icon,
				Tags:            providerDecl.Identity.Tags,
				ExecutionPolicy: providerDecl.ExecutionPolicy,
			}
		}

		// Parse tools referenced by this provider
		for _, toolPath := range providerDecl.Tools {
			toolFullPath := filepath.Join(pluginDir, toolPath)
			toolDef, err := parseTool(toolFullPath)
			if err != nil {
				return nil, fmt.Errorf("failed to parse tool %s: %w", toolPath, err)
			}

			declaration.Tools = append(declaration.Tools, ToolInfo{
				Name:  toolDef.Identity.Name,
				Label: toolDef.Identity.Label,
				Description: ToolDescription{
					Human: toolDef.Description.Human,
					LLM:   toolDef.Description.LLM,
				},
				Parameters:     toolDef.Parameters,
				Configurations: configDefs,
			})
		}
	}

	return &declaration, nil
}

// parseProvider parses a provider YAML file
func parseProviderFile(providerPath string) (*ProviderDefinition, []ConfigurationDefinition, error) {
	data, err := os.ReadFile(providerPath)
	if err != nil {
		return nil, nil, err
	}

	return parseProviderData(data)
}

func parseProviderData(data []byte) (*ProviderDefinition, []ConfigurationDefinition, error) {
	var provider ProviderDefinition
	if err := yaml.Unmarshal(data, &provider); err != nil {
		return nil, nil, err
	}

	configDefs, err := parseProviderConfigurations(data)
	if err != nil {
		return nil, nil, err
	}

	return &provider, configDefs, nil
}

// parseTool parses a tool YAML file
func parseTool(toolPath string) (*ToolDefinition, error) {
	data, err := os.ReadFile(toolPath)
	if err != nil {
		return nil, err
	}

	var tool ToolDefinition
	if err := yaml.Unmarshal(data, &tool); err != nil {
		return nil, err
	}

	return &tool, nil
}

// ToJSON converts declaration to JSON bytes for database storage
func (d *PluginDeclaration) ToJSON() ([]byte, error) {
	return json.Marshal(d)
}

// ParsePluginFromZipFull parses plugin YAML files directly from ZIP bytes
// Returns both declaration (for database) and manifest (for Plugin Runner)
// This is the optimized single-parse function
func ParsePluginFromZipFull(zipData []byte) (*ParseResult, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("failed to open zip archive: %w", err)
	}

	// Build a map of file paths to their content
	files := make(map[string][]byte)
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		// Normalize path (remove leading ./ or /)
		name := strings.TrimPrefix(f.Name, "./")
		name = strings.TrimPrefix(name, "/")
		files[name] = content
	}

	// Find and parse manifest.yaml
	manifestData, ok := files["manifest.yaml"]
	if !ok {
		// Try with subdirectory (some ZIPs have a root folder)
		for path, content := range files {
			if strings.HasSuffix(path, "/manifest.yaml") || strings.HasSuffix(path, "manifest.yaml") {
				manifestData = content
				break
			}
		}
		if manifestData == nil {
			return nil, fmt.Errorf("manifest.yaml not found in zip")
		}
	}

	var manifest PluginManifest
	if err := yaml.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest.yaml: %w", err)
	}

	// Build RunnerManifest from parsed manifest
	description := ""
	if desc, ok := manifest.Description["en_US"]; ok {
		description = desc
	} else if desc, ok := manifest.Description["zh_Hans"]; ok {
		description = desc
	}

	// Use version from meta if available, otherwise from top level
	version := manifest.Version
	if manifest.Meta.Version != "" {
		version = manifest.Meta.Version
	}

	runnerManifest := &RunnerManifest{
		Name:        manifest.Name,
		Version:     version,
		Author:      manifest.Author,
		Description: description,
		Tags:        manifest.Tags,
		Language:    manifest.Meta.Runner.Language,
		Entrypoint:  manifest.Meta.Runner.Entrypoint,
	}

	// Parse provider and tool files
	var declaration PluginDeclaration
	for _, providerPath := range manifest.Plugins.Tools {
		providerData := findFileInZip(files, providerPath)
		if providerData == nil {
			return nil, fmt.Errorf("provider file not found: %s", providerPath)
		}

		provider, configDefs, err := parseProviderData(providerData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse provider %s: %w", providerPath, err)
		}

		// Set provider info (use first provider)
		if declaration.Provider.Name == "" {
			declaration.Provider = ProviderInfo{
				Name:            provider.Identity.Name,
				Author:          provider.Identity.Author,
				Label:           provider.Identity.Label,
				Description:     provider.Identity.Description,
				Icon:            provider.Identity.Icon,
				Tags:            provider.Identity.Tags,
				ExecutionPolicy: provider.ExecutionPolicy,
			}
		}

		// Parse tools referenced by this provider
		for _, toolPath := range provider.Tools {
			toolData := findFileInZip(files, toolPath)
			if toolData == nil {
				return nil, fmt.Errorf("tool file not found: %s", toolPath)
			}

			var tool ToolDefinition
			if err := yaml.Unmarshal(toolData, &tool); err != nil {
				return nil, fmt.Errorf("failed to parse tool %s: %w", toolPath, err)
			}

			declaration.Tools = append(declaration.Tools, ToolInfo{
				Name:  tool.Identity.Name,
				Label: tool.Identity.Label,
				Description: ToolDescription{
					Human: tool.Description.Human,
					LLM:   tool.Description.LLM,
				},
				Parameters:     tool.Parameters,
				Configurations: configDefs,
			})
		}
	}

	return &ParseResult{
		Declaration: &declaration,
		Manifest:    runnerManifest,
	}, nil
}

// ParsePluginFromZip parses plugin YAML files directly from ZIP bytes
// Returns only declaration (for backward compatibility)
func ParsePluginFromZip(zipData []byte) (*PluginDeclaration, error) {
	result, err := ParsePluginFromZipFull(zipData)
	if err != nil {
		return nil, err
	}
	return result.Declaration, nil
}

func parseProviderConfigurations(data []byte) ([]ConfigurationDefinition, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}

	if len(node.Content) == 0 {
		return nil, nil
	}

	root := node.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil
	}

	for i := 0; i < len(root.Content); i += 2 {
		keyNode := root.Content[i]
		valueNode := root.Content[i+1]
		if keyNode.Value != "credentials_for_provider" {
			continue
		}
		if valueNode.Kind != yaml.MappingNode {
			return nil, nil
		}

		configs := make([]ConfigurationDefinition, 0, len(valueNode.Content)/2)
		for j := 0; j < len(valueNode.Content); j += 2 {
			nameNode := valueNode.Content[j]
			configNode := valueNode.Content[j+1]
			if nameNode.Value == "" {
				continue
			}

			var config ConfigurationDefinition
			if err := configNode.Decode(&config); err != nil {
				return nil, fmt.Errorf("failed to parse credentials_for_provider %s: %w", nameNode.Value, err)
			}
			config.Name = nameNode.Value
			configs = append(configs, config)
		}
		return configs, nil
	}

	return nil, nil
}

// findFileInZip finds a file in the zip files map, handling different path formats
func findFileInZip(files map[string][]byte, targetPath string) []byte {
	// Try exact match first
	if data, ok := files[targetPath]; ok {
		return data
	}

	// Try with normalized path
	normalized := strings.TrimPrefix(targetPath, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	if data, ok := files[normalized]; ok {
		return data
	}

	// Try finding in subdirectory (ZIP might have root folder)
	for path, content := range files {
		if strings.HasSuffix(path, "/"+normalized) || strings.HasSuffix(path, normalized) {
			return content
		}
	}

	return nil
}
