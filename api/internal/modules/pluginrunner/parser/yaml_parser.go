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
	Name        string        `yaml:"name" json:"name"`
	Author      string        `yaml:"author" json:"author"`
	Version     string        `yaml:"version" json:"version"`
	Label       LocalizedText `yaml:"label" json:"label"`
	Description LocalizedText `yaml:"description" json:"description"`
	Icon        string        `yaml:"icon" json:"icon,omitempty"`
	Tags        []string      `yaml:"tags" json:"tags,omitempty"`
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
		Name        string        `yaml:"name" json:"name"`
		Author      string        `yaml:"author" json:"author,omitempty"`
		Label       LocalizedText `yaml:"label" json:"label"`
		Description LocalizedText `yaml:"description" json:"description"`
		Icon        string        `yaml:"icon" json:"icon,omitempty"`
		Tags        []string      `yaml:"tags" json:"tags,omitempty"`
	} `yaml:"identity" json:"identity"`
	Tools           []ProviderTool   `yaml:"tools" json:"tools"`
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
		Name   string        `yaml:"name" json:"name"`
		Author string        `yaml:"author" json:"author,omitempty"`
		Label  LocalizedText `yaml:"label" json:"label"`
	} `yaml:"identity" json:"identity"`
	Description struct {
		Human LocalizedText `yaml:"human" json:"human"`
		LLM   string        `yaml:"llm" json:"llm"`
	} `yaml:"description" json:"description"`
	Parameters []ParameterDefinition `yaml:"parameters" json:"parameters"`
}

// ParameterDefinition represents a tool parameter
type ParameterDefinition struct {
	Name             string        `yaml:"name" json:"name"`
	Type             string        `yaml:"type" json:"type"`
	Required         bool          `yaml:"required" json:"required"`
	Form             string        `yaml:"form" json:"form"`
	Label            LocalizedText `yaml:"label" json:"label"`
	HumanDescription LocalizedText `yaml:"human_description" json:"human_description,omitempty"`
	Default          interface{}   `yaml:"default" json:"default,omitempty"`
	Options          []Option      `yaml:"options" json:"options,omitempty"`
}

// ConfigurationDefinition represents a provider configuration parameter
type ConfigurationDefinition struct {
	Name             string        `yaml:"-" json:"name"`
	Type             string        `yaml:"type" json:"type"`
	Required         bool          `yaml:"required" json:"required"`
	Label            LocalizedText `yaml:"label" json:"label"`
	Help             LocalizedText `yaml:"help" json:"help,omitempty"`
	Placeholder      LocalizedText `yaml:"placeholder" json:"placeholder,omitempty"`
	Default          interface{}   `yaml:"default" json:"default,omitempty"`
	Options          []Option      `yaml:"options" json:"options,omitempty"`
	HumanDescription LocalizedText `yaml:"human_description" json:"human_description,omitempty"`
}

// Option represents a parameter option
type Option struct {
	Value interface{}   `yaml:"value" json:"value"`
	Label LocalizedText `yaml:"label" json:"label"`
}

// LocalizedText accepts both old scalar strings and Dify-style localized maps.
type LocalizedText map[string]string

func (t *LocalizedText) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if strings.TrimSpace(value.Value) == "" {
			*t = nil
			return nil
		}
		*t = LocalizedText{"en_US": value.Value}
		return nil
	case yaml.MappingNode:
		next := make(LocalizedText, len(value.Content)/2)
		for i := 0; i < len(value.Content); i += 2 {
			key := strings.TrimSpace(value.Content[i].Value)
			if key == "" {
				continue
			}
			var val string
			if err := value.Content[i+1].Decode(&val); err != nil {
				return err
			}
			next[key] = val
		}
		*t = next
		return nil
	case yaml.SequenceNode:
		return fmt.Errorf("localized text must be a string or map")
	default:
		*t = nil
		return nil
	}
}

// ProviderTool accepts either a referenced tool YAML path or an inline tool definition.
type ProviderTool struct {
	Path       string
	InlineTool *ToolDefinition
}

func (t *ProviderTool) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		t.Path = strings.TrimSpace(value.Value)
		return nil
	case yaml.MappingNode:
		tool, err := decodeInlineTool(value)
		if err != nil {
			return err
		}
		t.InlineTool = tool
		return nil
	default:
		return fmt.Errorf("tool entry must be a path or object")
	}
}

// PluginDeclaration is the combined declaration structure to store in database
type PluginDeclaration struct {
	Provider ProviderInfo `json:"provider"`
	Tools    []ToolInfo   `json:"tools"`
}

// ProviderInfo represents provider info in declaration
type ProviderInfo struct {
	Name            string           `json:"name"`
	Author          string           `json:"author,omitempty"`
	Label           LocalizedText    `json:"label"`
	Description     LocalizedText    `json:"description"`
	Icon            string           `json:"icon,omitempty"`
	Tags            []string         `json:"tags,omitempty"`
	ExecutionPolicy *ExecutionPolicy `json:"execution_policy,omitempty"`
}

// ToolInfo represents tool info in declaration
type ToolInfo struct {
	Name           string                    `json:"name"`
	Label          LocalizedText             `json:"label"`
	Description    ToolDescription           `json:"description"`
	Parameters     []ParameterDefinition     `json:"parameters"`
	Configurations []ConfigurationDefinition `json:"configurations,omitempty"`
}

// ToolDescription represents tool description
type ToolDescription struct {
	Human LocalizedText `json:"human"`
	LLM   string        `json:"llm"`
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

		// Parse referenced tools or inline tools from this provider.
		for _, toolRef := range providerDecl.Tools {
			toolDef := toolRef.InlineTool
			if toolDef == nil {
				toolFullPath := filepath.Join(pluginDir, toolRef.Path)
				var err error
				toolDef, err = parseTool(toolFullPath)
				if err != nil {
					return nil, fmt.Errorf("failed to parse tool %s: %w", toolRef.Path, err)
				}
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

	return parseToolData(data)
}

func parseToolData(data []byte) (*ToolDefinition, error) {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	if len(node.Content) == 0 {
		return nil, fmt.Errorf("tool YAML is empty")
	}

	if mappingHasKey(node.Content[0], "identity") {
		var tool ToolDefinition
		if err := node.Content[0].Decode(&tool); err != nil {
			return nil, err
		}
		return &tool, nil
	}

	return decodeInlineTool(node.Content[0])
}

func decodeInlineTool(node *yaml.Node) (*ToolDefinition, error) {
	type inlineTool struct {
		Name           string                    `yaml:"name"`
		Author         string                    `yaml:"author"`
		Label          LocalizedText             `yaml:"label"`
		Description    LocalizedText             `yaml:"description"`
		LLMDescription string                    `yaml:"llm_description"`
		Parameters     []ParameterDefinition     `yaml:"parameters"`
		Configurations []ConfigurationDefinition `yaml:"configurations"`
	}

	var raw inlineTool
	if err := node.Decode(&raw); err != nil {
		return nil, err
	}

	var tool ToolDefinition
	tool.Identity.Name = raw.Name
	tool.Identity.Author = raw.Author
	tool.Identity.Label = raw.Label
	tool.Description.Human = raw.Description
	tool.Description.LLM = raw.LLMDescription
	if tool.Description.LLM == "" {
		tool.Description.LLM = preferredLocalizedText(raw.Description)
	}
	tool.Parameters = raw.Parameters

	return &tool, nil
}

func mappingHasKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

func preferredLocalizedText(text LocalizedText) string {
	for _, key := range []string{"en_US", "en-US", "zh_Hans", "zh-Hans"} {
		if value := strings.TrimSpace(text[key]); value != "" {
			return value
		}
	}
	for _, value := range text {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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

		// Parse referenced tools or inline tools from this provider.
		for _, toolRef := range provider.Tools {
			tool := toolRef.InlineTool
			if tool == nil {
				toolData := findFileInZip(files, toolRef.Path)
				if toolData == nil {
					return nil, fmt.Errorf("tool file not found: %s", toolRef.Path)
				}

				var err error
				tool, err = parseToolData(toolData)
				if err != nil {
					return nil, fmt.Errorf("failed to parse tool %s: %w", toolRef.Path, err)
				}
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
