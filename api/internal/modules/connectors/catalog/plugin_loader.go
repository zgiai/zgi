package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	AgentPluginsV1ManifestSchemaID = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	ZGIConnectorExtensionNamespace = "com.zgiai.zgi"
	ZGIMetadataSchemaVersion1      = "1.0.0"
	maxPluginManifestBytes         = 1 << 20
)

var (
	ErrPluginManifestInvalid      = errors.New("invalid Agent Plugin manifest")
	ErrUnsupportedPluginSchema    = errors.New("unsupported Agent Plugin schema")
	ErrConnectorMetadataInvalid   = errors.New("invalid ZGI Connector plugin metadata")
	ErrPluginPackagePathEscape    = errors.New("Agent Plugin package path escapes its root")
	ErrPluginAdmissionUnavailable = errors.New("Connector definition admission is unavailable")
	agentPluginNamePattern        = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
)

// PluginLoadWarning represents one of the two non-fatal plugin.json
// exceptions required by Agent Plugins 1.0.0. No manifest value is retained in
// the warning because plugin packages are not a secret transport.
type PluginLoadWarning struct {
	Code  string
	Field string
}

type PluginComponentFailure struct {
	PluginID       string
	ConnectorIndex int
	Code           string
	Err            error
}

type PluginPackageFailure struct {
	Root string
	Err  error
}

type LoadedPlugin struct {
	PluginID      string
	PluginVersion string
	PluginDigest  string
	Warnings      []PluginLoadWarning
	Definitions   []Definition
	Failures      []PluginComponentFailure
}

type PluginLoadReport struct {
	Plugins     []LoadedPlugin
	Definitions []Definition
	Failures    []PluginPackageFailure
}

type PluginAdmissionReport struct {
	LoadReport          PluginLoadReport
	AdmittedDefinitions []Definition
	Failures            []PluginPackageFailure
}

// PluginLoader implements the Agent Plugins 1.0.0 manifest failure semantics
// locally. It does not retrieve schemas, launch MCP servers, read Keys, or
// create Personal Connections.
type PluginLoader struct {
	manifestSchema *jsonschema.Schema
}

func NewPluginLoader() (*PluginLoader, error) {
	var document interface{}
	if err := json.Unmarshal([]byte(agentPluginsV1ManifestSchema), &document); err != nil {
		return nil, fmt.Errorf("compile Agent Plugins manifest schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(AgentPluginsV1ManifestSchemaID, document); err != nil {
		return nil, fmt.Errorf("compile Agent Plugins manifest schema: %w", err)
	}
	compiled, err := compiler.Compile(AgentPluginsV1ManifestSchemaID)
	if err != nil {
		return nil, fmt.Errorf("compile Agent Plugins manifest schema: %w", err)
	}
	return &PluginLoader{manifestSchema: compiled}, nil
}

func (loader *PluginLoader) LoadDirectory(ctx context.Context, root string) (LoadedPlugin, error) {
	if loader == nil || loader.manifestSchema == nil {
		return LoadedPlugin{}, ErrPluginAdmissionUnavailable
	}
	if err := ctx.Err(); err != nil {
		return LoadedPlugin{}, err
	}
	resolvedRoot, manifestBytes, err := readPluginManifest(root)
	if err != nil {
		return LoadedPlugin{}, err
	}
	manifest, extension, warnings, err := loader.parseManifest(manifestBytes)
	if err != nil {
		return LoadedPlugin{}, err
	}
	digest, err := digestPluginPackage(ctx, resolvedRoot)
	if err != nil {
		return LoadedPlugin{}, err
	}
	loaded := LoadedPlugin{
		PluginID: manifest.Name, PluginVersion: strings.TrimSpace(manifest.Version),
		PluginDigest: digest, Warnings: warnings,
	}
	if extension == nil {
		return loaded, nil
	}
	definitions, failures := parseZGIConnectorExtension(manifest, loaded.PluginDigest, extension)
	loaded.Definitions = definitions
	loaded.Failures = failures
	return cloneLoadedPlugin(loaded), nil
}

// LoadManifestPackage admits a trusted embedded package whose complete package
// consists of one plugin.json file. It follows the same schema and extension
// parsing path as a filesystem package; the deterministic digest represents
// that immutable virtual package and never contains a user credential.
func (loader *PluginLoader) LoadManifestPackage(ctx context.Context, manifestBytes []byte) (LoadedPlugin, error) {
	if loader == nil || loader.manifestSchema == nil {
		return LoadedPlugin{}, ErrPluginAdmissionUnavailable
	}
	if err := ctx.Err(); err != nil {
		return LoadedPlugin{}, err
	}
	if len(manifestBytes) == 0 || len(manifestBytes) > maxPluginManifestBytes {
		return LoadedPlugin{}, fmt.Errorf("%w: plugin.json", ErrPluginManifestInvalid)
	}
	manifest, extension, warnings, err := loader.parseManifest(manifestBytes)
	if err != nil {
		return LoadedPlugin{}, err
	}
	digest := sha256.Sum256(manifestBytes)
	loaded := LoadedPlugin{
		PluginID: manifest.Name, PluginVersion: strings.TrimSpace(manifest.Version),
		PluginDigest: hex.EncodeToString(digest[:]), Warnings: warnings,
	}
	if extension == nil {
		return loaded, nil
	}
	loaded.Definitions, loaded.Failures = parseZGIConnectorExtension(manifest, loaded.PluginDigest, extension)
	return cloneLoadedPlugin(loaded), nil
}

// BuildAgentPluginManifest materializes trusted built-in definitions as a
// standards-conforming Agent Plugins 1.0 package manifest before admission.
// This keeps the compatibility provider adapter outside the canonical catalog:
// the loader remains the only way a built-in capability becomes usable.
func BuildAgentPluginManifest(definitions []Definition) ([]byte, error) {
	if len(definitions) == 0 {
		return nil, ErrInvalidDefinition
	}
	pluginID := definitions[0].PluginID
	pluginVersion := definitions[0].PluginVersion
	extensionVersion := ZGIMetadataSchemaVersion1
	if definitions[0].UsesCredentials() {
		extensionVersion = definitions[0].ExtensionSchemaVersion
	}
	connectors := make([]zgiConnectorMetadata, 0, len(definitions))
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if err := Validate(definition); err != nil {
			return nil, err
		}
		if definition.PluginID != pluginID || definition.PluginVersion != pluginVersion {
			return nil, invalid("plugin_package_identity")
		}
		if definition.ExtensionSchemaVersion != definitions[0].ExtensionSchemaVersion {
			return nil, invalid("plugin_extension_version")
		}
		if _, exists := seen[definition.ID]; exists {
			return nil, ErrDuplicateConnector
		}
		seen[definition.ID] = struct{}{}
		connectors = append(connectors, zgiConnectorMetadata{
			ID: definition.ID, ProviderID: definition.ProviderID, DriverID: definition.DriverID,
			AuthMethodID: definition.AuthMethodID, Name: definition.Name, Description: definition.Description,
			NameI18n: definition.NameI18n.Clone(), DescriptionI18n: definition.DescriptionI18n.Clone(),
			Icon: definition.Icon, Classification: definition.Classification,
			CapabilityTypes: append([]CapabilityType(nil), definition.CapabilityTypes...),
			SetupMethod:     definition.SetupMethod, Ownership: definition.Ownership,
			Surfaces:         append([]Surface(nil), definition.Surfaces...),
			KeyFields:        append([]KeyField(nil), definition.KeyFields...),
			Operations:       cloneDefinition(definition).Operations,
			CredentialFields: cloneCredentialFields(definition.CredentialFields), Setup: cloneSetup(definition.Setup),
		})
	}
	return json.Marshal(map[string]interface{}{
		"$schema": AgentPluginsV1ManifestSchemaID,
		"name":    pluginID,
		"version": pluginVersion,
		"extensions": map[string]interface{}{
			ZGIConnectorExtensionNamespace: map[string]interface{}{
				"schema_version": extensionVersion,
				"connectors":     connectors,
			},
		},
	})
}

// LoadDirectories isolates package and Connector-entry failures. A malformed
// package never removes definitions loaded from independent valid packages.
func (loader *PluginLoader) LoadDirectories(ctx context.Context, roots []string) PluginLoadReport {
	report := PluginLoadReport{}
	seenConnectors := make(map[string]string)
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			report.Failures = append(report.Failures, PluginPackageFailure{Root: root, Err: err})
			break
		}
		plugin, err := loader.LoadDirectory(ctx, root)
		if err != nil {
			report.Failures = append(report.Failures, PluginPackageFailure{Root: root, Err: err})
			continue
		}
		accepted := plugin.Definitions[:0]
		for _, definition := range plugin.Definitions {
			if priorPlugin, exists := seenConnectors[definition.ID]; exists {
				plugin.Failures = append(plugin.Failures, PluginComponentFailure{
					PluginID: plugin.PluginID, ConnectorIndex: -1, Code: "duplicate_connector_id",
					Err: fmt.Errorf("%w: connector_id already admitted by %s", ErrDuplicateConnector, priorPlugin),
				})
				continue
			}
			seenConnectors[definition.ID] = plugin.PluginID
			accepted = append(accepted, definition)
			report.Definitions = append(report.Definitions, cloneDefinition(definition))
		}
		plugin.Definitions = accepted
		report.Plugins = append(report.Plugins, cloneLoadedPlugin(plugin))
	}
	return report
}

// LoadAndAdmitDirectories persists definition metadata one plugin at a time.
// The only injected capability is DefinitionAdmitter, making it impossible for
// plugin installation to create a Connection or handle a user credential.
func (loader *PluginLoader) LoadAndAdmitDirectories(
	ctx context.Context,
	roots []string,
	admitter DefinitionAdmitter,
) PluginAdmissionReport {
	loadReport := loader.LoadDirectories(ctx, roots)
	report := PluginAdmissionReport{LoadReport: loadReport}
	if admitter == nil {
		report.Failures = append(report.Failures, PluginPackageFailure{Err: ErrPluginAdmissionUnavailable})
		return report
	}
	for _, plugin := range loadReport.Plugins {
		if len(plugin.Definitions) == 0 {
			continue
		}
		definitions := make([]Definition, len(plugin.Definitions))
		for index, definition := range plugin.Definitions {
			definitions[index] = cloneDefinition(definition)
		}
		if err := admitter.Admit(ctx, definitions); err != nil {
			report.Failures = append(report.Failures, PluginPackageFailure{Err: err})
			continue
		}
		report.AdmittedDefinitions = append(report.AdmittedDefinitions, definitions...)
	}
	return report
}

type agentPluginManifest struct {
	Schema      string                     `json:"$schema"`
	Name        string                     `json:"name"`
	Version     string                     `json:"version,omitempty"`
	Description string                     `json:"description,omitempty"`
	Extensions  map[string]json.RawMessage `json:"extensions,omitempty"`
}

var portableManifestFields = map[string]struct{}{
	"$schema": {}, "name": {}, "version": {}, "description": {}, "author": {},
	"homepage": {}, "repository": {}, "license": {}, "keywords": {}, "extensions": {},
}

func (loader *PluginLoader) parseManifest(
	manifestBytes []byte,
) (agentPluginManifest, json.RawMessage, []PluginLoadWarning, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(manifestBytes, &raw); err != nil || raw == nil {
		return agentPluginManifest{}, nil, nil, fmt.Errorf("%w: document", ErrPluginManifestInvalid)
	}
	warnings := make([]PluginLoadWarning, 0)
	unknown := make([]string, 0)
	for field := range raw {
		if _, supported := portableManifestFields[field]; !supported {
			unknown = append(unknown, field)
			delete(raw, field)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		warnings = append(warnings, PluginLoadWarning{Code: "unknown_top_level_field", Field: field})
	}

	if extensions, exists := raw["extensions"]; exists && !rawJSONObject(extensions) {
		warnings = append(warnings, PluginLoadWarning{Code: "non_object_extensions", Field: "extensions"})
		delete(raw, "extensions")
	}
	var extension json.RawMessage
	if extensions, exists := raw["extensions"]; exists {
		var namespaces map[string]json.RawMessage
		if err := json.Unmarshal(extensions, &namespaces); err == nil {
			extension = append(json.RawMessage(nil), namespaces[ZGIConnectorExtensionNamespace]...)
			// Agent Plugins requires clients to ignore unimplemented namespaces
			// without validating their values. The supported namespace is also
			// validated independently so a bad Connector component does not
			// invalidate portable Skills or MCP components.
			raw["extensions"] = json.RawMessage(`{}`)
		}
	}
	if schemaRaw, exists := raw["$schema"]; exists {
		var schemaID string
		if json.Unmarshal(schemaRaw, &schemaID) == nil && schemaID != AgentPluginsV1ManifestSchemaID {
			return agentPluginManifest{}, nil, nil, ErrUnsupportedPluginSchema
		}
	}
	sanitized, err := json.Marshal(raw)
	if err != nil {
		return agentPluginManifest{}, nil, nil, fmt.Errorf("%w: document", ErrPluginManifestInvalid)
	}
	var document interface{}
	if err := json.Unmarshal(sanitized, &document); err != nil {
		return agentPluginManifest{}, nil, nil, fmt.Errorf("%w: document", ErrPluginManifestInvalid)
	}
	if err := loader.manifestSchema.Validate(document); err != nil {
		return agentPluginManifest{}, nil, nil, fmt.Errorf("%w: schema", ErrPluginManifestInvalid)
	}
	var manifest agentPluginManifest
	if err := json.Unmarshal(sanitized, &manifest); err != nil {
		return agentPluginManifest{}, nil, nil, fmt.Errorf("%w: document", ErrPluginManifestInvalid)
	}
	if !validAgentPluginName(manifest.Name) {
		return agentPluginManifest{}, nil, nil, fmt.Errorf("%w: name", ErrPluginManifestInvalid)
	}
	if manifest.Schema != AgentPluginsV1ManifestSchemaID {
		return agentPluginManifest{}, nil, nil, ErrUnsupportedPluginSchema
	}
	return manifest, extension, warnings, nil
}

type zgiConnectorExtension struct {
	SchemaVersion string            `json:"schema_version,omitempty"`
	Connectors    []json.RawMessage `json:"connectors"`
}

type zgiConnectorMetadata struct {
	ID               string            `json:"id"`
	ProviderID       string            `json:"provider_id"`
	DriverID         string            `json:"driver_id"`
	AuthMethodID     string            `json:"auth_method_id"`
	Name             string            `json:"name"`
	NameI18n         LocalizedText     `json:"name_i18n,omitempty"`
	Description      string            `json:"description"`
	DescriptionI18n  LocalizedText     `json:"description_i18n,omitempty"`
	Icon             string            `json:"icon,omitempty"`
	Classification   Classification    `json:"classification"`
	CapabilityTypes  []CapabilityType  `json:"capability_types"`
	SetupMethod      SetupMethod       `json:"setup_method"`
	Ownership        Ownership         `json:"ownership"`
	Surfaces         []Surface         `json:"surfaces"`
	KeyFields        []KeyField        `json:"key_fields"`
	Operations       []Operation       `json:"operations"`
	CredentialFields []CredentialField `json:"credential_fields,omitempty"`
	Setup            *SetupDescriptor  `json:"setup,omitempty"`
}

func parseZGIConnectorExtension(
	manifest agentPluginManifest,
	pluginDigest string,
	raw json.RawMessage,
) ([]Definition, []PluginComponentFailure) {
	if !rawJSONObject(raw) {
		return nil, []PluginComponentFailure{{
			PluginID: manifest.Name, ConnectorIndex: -1, Code: "invalid_connector_extension",
			Err: ErrConnectorMetadataInvalid,
		}}
	}
	var extension zgiConnectorExtension
	if err := decodeStrictJSON(raw, &extension); err != nil {
		return nil, []PluginComponentFailure{{
			PluginID: manifest.Name, ConnectorIndex: -1, Code: "invalid_connector_extension",
			Err: ErrConnectorMetadataInvalid,
		}}
	}
	version := strings.TrimSpace(extension.SchemaVersion)
	if version == "" {
		version = ZGIMetadataSchemaVersion1
	}
	if version != ZGIMetadataSchemaVersion1 && version != ZGIMetadataSchemaVersion11 && version != ZGIMetadataSchemaVersion12 && version != ZGIMetadataSchemaVersion14 && version != ZGIMetadataSchemaVersion15 {
		return nil, []PluginComponentFailure{{
			PluginID: manifest.Name, ConnectorIndex: -1, Code: "unsupported_connector_extension_schema",
			Err: ErrConnectorMetadataInvalid,
		}}
	}
	pluginVersion := strings.TrimSpace(manifest.Version)
	if pluginVersion == "" && len(extension.Connectors) > 0 {
		return nil, []PluginComponentFailure{{
			PluginID: manifest.Name, ConnectorIndex: -1, Code: "plugin_version_required",
			Err: ErrConnectorMetadataInvalid,
		}}
	}

	definitions := make([]Definition, 0, len(extension.Connectors))
	failures := make([]PluginComponentFailure, 0)
	seen := make(map[string]struct{}, len(extension.Connectors))
	for index, rawConnector := range extension.Connectors {
		var metadata zgiConnectorMetadata
		if err := decodeStrictJSON(rawConnector, &metadata); err != nil || (version == ZGIMetadataSchemaVersion1 && hasNewCredentialMetadata(rawConnector)) {
			failures = append(failures, PluginComponentFailure{
				PluginID: manifest.Name, ConnectorIndex: index, Code: "invalid_connector_metadata",
				Err: ErrConnectorMetadataInvalid,
			})
			continue
		}
		definition := Definition{
			ID: metadata.ID, PluginID: manifest.Name, PluginVersion: pluginVersion, PluginDigest: pluginDigest,
			SourceDialect: SourceDialectAgentPluginsV1, SourceSchemaVersion: SourceSchemaVersion1,
			ProviderID: metadata.ProviderID, DriverID: metadata.DriverID, AuthMethodID: metadata.AuthMethodID,
			Name: metadata.Name, Description: metadata.Description, Icon: metadata.Icon,
			NameI18n: metadata.NameI18n, DescriptionI18n: metadata.DescriptionI18n,
			Classification: metadata.Classification, CapabilityTypes: metadata.CapabilityTypes,
			SetupMethod: metadata.SetupMethod, Ownership: metadata.Ownership,
			Surfaces: metadata.Surfaces, KeyFields: metadata.KeyFields, Operations: metadata.Operations,
			CredentialFields: metadata.CredentialFields, Setup: metadata.Setup,
		}
		if version == ZGIMetadataSchemaVersion11 {
			definition.ExtensionSchemaVersion = version
			definition.ContractVersion = 2
		}
		if version == ZGIMetadataSchemaVersion12 {
			definition.ExtensionSchemaVersion = version
			definition.ContractVersion = 3
		}
		if version == ZGIMetadataSchemaVersion14 {
			definition.ExtensionSchemaVersion = version
			definition.ContractVersion = 5
		}
		if version == ZGIMetadataSchemaVersion15 {
			definition.ExtensionSchemaVersion = version
			definition.ContractVersion = 6
		}
		if err := ValidatePluginDefinition(definition); err != nil {
			failures = append(failures, PluginComponentFailure{
				PluginID: manifest.Name, ConnectorIndex: index, Code: "connector_definition_rejected",
				Err: ErrConnectorMetadataInvalid,
			})
			continue
		}
		if _, duplicate := seen[definition.ID]; duplicate {
			failures = append(failures, PluginComponentFailure{
				PluginID: manifest.Name, ConnectorIndex: index, Code: "duplicate_connector_id",
				Err: ErrDuplicateConnector,
			})
			continue
		}
		seen[definition.ID] = struct{}{}
		definitions = append(definitions, cloneDefinition(definition))
	}
	return definitions, failures
}

func decodeStrictJSON(raw []byte, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func rawJSONObject(raw json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func validAgentPluginName(value string) bool {
	return len(value) >= 1 && len(value) <= 64 && agentPluginNamePattern.MatchString(value) &&
		!strings.Contains(value, "--") && !strings.Contains(value, "..")
}

func readPluginManifest(root string) (string, []byte, error) {
	absoluteRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", nil, fmt.Errorf("resolve Agent Plugin root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(absoluteRoot)
	if err != nil {
		return "", nil, fmt.Errorf("resolve Agent Plugin root: %w", err)
	}
	rootInfo, err := os.Stat(resolvedRoot)
	if err != nil || !rootInfo.IsDir() {
		return "", nil, fmt.Errorf("%w: root", ErrPluginManifestInvalid)
	}
	manifestPath, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, "plugin.json"))
	if err != nil {
		return "", nil, fmt.Errorf("%w: plugin.json", ErrPluginManifestInvalid)
	}
	if !pathWithinRoot(resolvedRoot, manifestPath) {
		return "", nil, ErrPluginPackagePathEscape
	}
	info, err := os.Stat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxPluginManifestBytes {
		return "", nil, fmt.Errorf("%w: plugin.json", ErrPluginManifestInvalid)
	}
	contents, err := os.ReadFile(manifestPath)
	if err != nil || len(contents) > maxPluginManifestBytes {
		return "", nil, fmt.Errorf("%w: plugin.json", ErrPluginManifestInvalid)
	}
	return resolvedRoot, contents, nil
}

func pathWithinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func digestPluginPackage(ctx context.Context, resolvedRoot string) (string, error) {
	hasher := sha256.New()
	err := filepath.WalkDir(resolvedRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(resolvedRoot, path)
		if err != nil || filepath.IsAbs(relative) {
			return ErrPluginPackagePathEscape
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || !pathWithinRoot(resolvedRoot, resolved) {
				return ErrPluginPackagePathEscape
			}
			target, err := filepath.Rel(resolvedRoot, resolved)
			if err != nil || filepath.IsAbs(target) {
				return ErrPluginPackagePathEscape
			}
			hashPackageChunk(hasher, "symlink")
			hashPackageChunk(hasher, relative)
			hashPackageChunk(hasher, filepath.ToSlash(target))
		case info.IsDir():
			hashPackageChunk(hasher, "directory")
			hashPackageChunk(hasher, relative)
		case info.Mode().IsRegular():
			hashPackageChunk(hasher, "file")
			hashPackageChunk(hasher, relative)
			hashPackageChunk(hasher, fmt.Sprintf("%o", info.Mode().Perm()))
			hashPackageChunk(hasher, fmt.Sprintf("%d", info.Size()))
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(hasher, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return fmt.Errorf("%w: unsupported package file", ErrPluginManifestInvalid)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("digest Agent Plugin package: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func hashPackageChunk(hasher interface{ Write([]byte) (int, error) }, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hasher.Write(length[:])
	_, _ = hasher.Write([]byte(value))
}

func cloneLoadedPlugin(plugin LoadedPlugin) LoadedPlugin {
	cloned := plugin
	cloned.Warnings = append([]PluginLoadWarning(nil), plugin.Warnings...)
	cloned.Failures = append([]PluginComponentFailure(nil), plugin.Failures...)
	cloned.Definitions = make([]Definition, len(plugin.Definitions))
	for index, definition := range plugin.Definitions {
		cloned.Definitions[index] = cloneDefinition(definition)
	}
	return cloned
}

const agentPluginsV1ManifestSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
  "type": "object",
  "properties": {
    "$schema": {"const": "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"},
    "name": {"type": "string", "minLength": 1, "maxLength": 64, "pattern": "^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$"},
    "version": {"type": "string"},
    "description": {"type": "string"},
    "author": {
      "type": "object",
      "properties": {"name": {"type": "string"}, "email": {"type": "string"}, "url": {"type": "string"}},
      "additionalProperties": false
    },
    "homepage": {"type": "string"},
    "repository": {"type": "string"},
    "license": {"type": "string"},
    "keywords": {"type": "array", "items": {"type": "string"}},
    "extensions": {"type": "object", "additionalProperties": {"type": "object"}}
  },
  "required": ["$schema", "name"],
  "additionalProperties": false
}`
