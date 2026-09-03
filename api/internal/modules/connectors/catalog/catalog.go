package catalog

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	// SourceDialectAgentPluginsV1 identifies definitions distributed through
	// the published Agent Plugins Specification family.
	SourceDialectAgentPluginsV1 = "agentplugins-v1"
	// SourceSchemaVersion1 is the first published specification baseline. Draft
	// schema versions are deliberately not admitted by the first release.
	SourceSchemaVersion1 = "1.0.0"
)

var (
	ErrInvalidDefinition  = errors.New("invalid connector definition")
	ErrDuplicateConnector = errors.New("duplicate connector identity")
	stableIDPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
)

// Classification is the one product section in which a Connector definition
// is presented. A Data Source still uses the same Connector lifecycle.
type Classification string

const (
	ClassificationConnector  Classification = "connector"
	ClassificationDataSource Classification = "data_source"
)

// SetupMethod describes the only member connection journey offered by a
// definition. The first release intentionally admits Key setup only.
type SetupMethod string

const SetupMethodKey SetupMethod = "key"

// Ownership describes who owns an authorization created from the definition.
// Organization and shared ownership are outside the first release.
type Ownership string

const OwnershipPersonal Ownership = "personal"

// Surface is a product context that may bind a ready Personal Connection.
type Surface string

const (
	SurfaceChat     Surface = "chat"
	SurfaceAgent    Surface = "agent"
	SurfaceWorkflow Surface = "workflow"
)

// CapabilityType is a connector-level summary. Classification controls where
// the item appears; capability types retain its secondary behavior.
type CapabilityType string

const (
	CapabilityTypeAction CapabilityType = "action"
	CapabilityTypeData   CapabilityType = "data"
)

// KeyField is a write-only input schema. It describes a value but can never
// hold a submitted value.
type KeyField struct {
	Name            string        `json:"name"`
	Label           string        `json:"label"`
	LabelI18n       LocalizedText `json:"label_i18n,omitempty"`
	Placeholder     string        `json:"placeholder,omitempty"`
	PlaceholderI18n LocalizedText `json:"placeholder_i18n,omitempty"`
	HelpURL         string        `json:"help_url,omitempty"`
	Required        bool          `json:"required"`
	Secret          bool          `json:"secret"`
}

// Operation preserves the stable provider action and tool identities used by
// saved agents, workflows, chats, Skills, and runtime tool calls.
type Operation struct {
	ID                   string                 `json:"id"`
	ToolName             string                 `json:"tool_name"`
	SkillID              string                 `json:"skill_id,omitempty"`
	Name                 string                 `json:"name"`
	NameI18n             LocalizedText          `json:"name_i18n,omitempty"`
	Description          string                 `json:"description,omitempty"`
	DescriptionI18n      LocalizedText          `json:"description_i18n,omitempty"`
	InputSchema          map[string]interface{} `json:"input_schema,omitempty"`
	OutputSchema         map[string]interface{} `json:"output_schema,omitempty"`
	Effect               OperationEffect        `json:"effect,omitempty"`
	DataEgress           bool                   `json:"data_egress"`
	ExternalDestination  string                 `json:"external_destination,omitempty"`
	SensitiveDataAllowed bool                   `json:"sensitive_data_allowed"`
	RiskHint             OperationRiskHint      `json:"risk_hint,omitempty"`
	Idempotent           bool                   `json:"idempotent"`
	CredentialSubject    string                 `json:"credential_subject,omitempty"`
	RequiredScopes       [][]string             `json:"required_scopes,omitempty"`
}

// Definition is the immutable, secret-free contract projected into the member
// Connector catalog. Plugin admission and user authorization are separate:
// this type has no Connection, credential value, grant, policy, or scope field.
type Definition struct {
	ID            string `json:"id"`
	PluginID      string `json:"plugin_id"`
	PluginVersion string `json:"plugin_version"`
	// PluginDigest pins the locally admitted plugin manifest content. It is
	// internal lifecycle evidence and is never part of the member catalog DTO.
	PluginDigest           string            `json:"-"`
	SourceDialect          string            `json:"source_dialect"`
	SourceSchemaVersion    string            `json:"source_schema_version"`
	ProviderID             string            `json:"provider_id"`
	DriverID               string            `json:"driver_id"`
	AuthMethodID           string            `json:"auth_method_id"`
	Name                   string            `json:"name"`
	NameI18n               LocalizedText     `json:"name_i18n,omitempty"`
	Description            string            `json:"description"`
	DescriptionI18n        LocalizedText     `json:"description_i18n,omitempty"`
	Icon                   string            `json:"icon,omitempty"`
	Classification         Classification    `json:"classification"`
	CapabilityTypes        []CapabilityType  `json:"capability_types"`
	SetupMethod            SetupMethod       `json:"setup_method"`
	Ownership              Ownership         `json:"ownership"`
	Surfaces               []Surface         `json:"surfaces"`
	KeyFields              []KeyField        `json:"key_fields"`
	Operations             []Operation       `json:"operations"`
	ExtensionSchemaVersion string            `json:"extension_schema_version,omitempty"`
	ContractVersion        int               `json:"contract_version,omitempty"`
	CredentialFields       []CredentialField `json:"credential_fields,omitempty"`
	Setup                  *SetupDescriptor  `json:"setup,omitempty"`
}

// Supports reports whether the definition is eligible in a product surface.
func (definition Definition) Supports(surface Surface) bool {
	for _, candidate := range definition.Surfaces {
		if candidate == surface {
			return true
		}
	}
	return false
}

// FirstReleaseSurfaces returns a fresh slice containing every supported
// binding surface. Callers may mutate it without affecting another caller.
func FirstReleaseSurfaces() []Surface {
	return []Surface{SurfaceChat, SurfaceAgent, SurfaceWorkflow}
}

// Filter selects catalog definitions without introducing owner or Connection
// state into the definition catalog.
type Filter struct {
	Classification Classification
	Query          string
}

// Catalog is the concurrency-safe active view of admitted definitions. Each
// returned Definition is still an immutable defensive snapshot, so a plugin
// activation cannot switch the contract held by an operation already in
// progress.
type Catalog struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

// New validates and freezes trusted definitions. Dynamic plugin loading can
// call Validate per package so one rejected plugin does not hide valid ones.
func New(definitions ...Definition) (*Catalog, error) {
	result := &Catalog{definitions: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		if err := Validate(definition); err != nil {
			return nil, err
		}
		if _, exists := result.definitions[definition.ID]; exists {
			return nil, fmt.Errorf("%w: connector_id", ErrDuplicateConnector)
		}
		result.definitions[definition.ID] = cloneDefinition(definition)
	}
	return result, nil
}

// Validate enforces the published-spec, Key-only, Personal-only, all-surface
// contract of the first Connector release.
func Validate(definition Definition) error {
	if !stableIdentifier(definition.ID) {
		return invalid("connector_id")
	}
	if !stableIdentifier(definition.PluginID) {
		return invalid("plugin_id")
	}
	if strings.TrimSpace(definition.PluginVersion) == "" {
		return invalid("plugin_version")
	}
	if definition.SourceDialect != SourceDialectAgentPluginsV1 || definition.SourceSchemaVersion != SourceSchemaVersion1 {
		return invalid("plugin_source")
	}
	if !stableIdentifier(definition.ProviderID) {
		return invalid("provider_id")
	}
	if !stableIdentifier(definition.DriverID) {
		return invalid("driver_id")
	}
	if !stableIdentifier(definition.AuthMethodID) {
		return invalid("auth_method_id")
	}
	if strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.Description) == "" {
		return invalid("display")
	}
	if !validLocalizedText(definition.NameI18n) || !validLocalizedText(definition.DescriptionI18n) {
		return invalid("display_i18n")
	}
	if definition.Classification != ClassificationConnector && definition.Classification != ClassificationDataSource {
		return invalid("classification")
	}
	if err := validateCapabilityTypes(definition.CapabilityTypes); err != nil {
		return err
	}
	if definition.Ownership != OwnershipPersonal {
		return invalid("ownership")
	}
	if !hasAllFirstReleaseSurfaces(definition.Surfaces) {
		return invalid("surfaces")
	}
	if err := validateCredentialContract(definition); err != nil {
		return err
	}
	if err := validateOperations(definition.Operations); err != nil {
		return err
	}
	return nil
}

// Get returns a defensive copy of an admitted definition.
func (catalog *Catalog) Get(connectorID string) (Definition, bool) {
	if catalog == nil {
		return Definition{}, false
	}
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	definition, ok := catalog.definitions[connectorID]
	if !ok {
		return Definition{}, false
	}
	return cloneDefinition(definition), true
}

// List returns deterministic defensive copies matching the presentation
// classification and optional member search query.
func (catalog *Catalog) List(filter Filter) []Definition {
	if catalog == nil {
		return []Definition{}
	}
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	result := make([]Definition, 0, len(catalog.definitions))
	for _, definition := range catalog.definitions {
		if filter.Classification != "" && definition.Classification != filter.Classification {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(definition.ID + "\n" + definition.Name + "\n" + definition.Description)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		result = append(result, cloneDefinition(definition))
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := strings.ToLower(result[i].Name), strings.ToLower(result[j].Name)
		if left == right {
			return result[i].ID < result[j].ID
		}
		return left < right
	})
	return result
}

// ReplaceActive atomically installs a complete active catalog snapshot. It is
// used only when rebuilding process state from the authoritative accepted
// version store during startup.
func (catalog *Catalog) ReplaceActive(definitions []Definition) error {
	if catalog == nil {
		return ErrInvalidDefinition
	}
	next, err := validatedDefinitionMap(definitions)
	if err != nil {
		return err
	}
	catalog.mu.Lock()
	catalog.definitions = next
	catalog.mu.Unlock()
	return nil
}

// ActivatePackage atomically activates one compatible plugin package in the
// active view. Every previously active Connector from that plugin must remain
// present; adding new Connector identities and operations is permitted only
// when the frozen contracts accepted by CheckCompatibleUpdate are preserved.
func (catalog *Catalog) ActivatePackage(definitions []Definition) error {
	if catalog == nil {
		return ErrInvalidDefinition
	}
	candidates, pluginID, err := validatePluginPackageDefinitions(definitions)
	if err != nil {
		return err
	}
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	if err := validateActivePackageReplacement(catalog.definitions, candidates, pluginID); err != nil {
		return err
	}
	next := cloneDefinitionMap(catalog.definitions)
	for connectorID, definition := range candidates {
		next[connectorID] = cloneDefinition(definition)
	}
	catalog.definitions = next
	return nil
}

// DeactivatePlugin removes only the active presentation/runtime lookup. The
// authoritative definition, accepted versions, Connections, bindings and Keys
// remain durable for impact reporting and an explicit future recovery path.
func (catalog *Catalog) DeactivatePlugin(pluginID string) {
	if catalog == nil || !stableIdentifier(pluginID) {
		return
	}
	catalog.mu.Lock()
	defer catalog.mu.Unlock()
	next := cloneDefinitionMap(catalog.definitions)
	for connectorID, definition := range next {
		if definition.PluginID == pluginID {
			delete(next, connectorID)
		}
	}
	catalog.definitions = next
}

func validatedDefinitionMap(definitions []Definition) (map[string]Definition, error) {
	result := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		if err := Validate(definition); err != nil {
			return nil, err
		}
		if _, exists := result[definition.ID]; exists {
			return nil, fmt.Errorf("%w: connector_id", ErrDuplicateConnector)
		}
		result[definition.ID] = cloneDefinition(definition)
	}
	return result, nil
}

func validatePluginPackageDefinitions(definitions []Definition) (map[string]Definition, string, error) {
	if len(definitions) == 0 {
		return nil, "", ErrInvalidDefinition
	}
	candidates, err := validatedDefinitionMap(definitions)
	if err != nil {
		return nil, "", err
	}
	pluginID := definitions[0].PluginID
	pluginVersion := definitions[0].PluginVersion
	pluginDigest := definitions[0].PluginDigest
	for _, definition := range definitions {
		if definition.PluginID != pluginID || definition.PluginVersion != pluginVersion ||
			definition.PluginDigest != pluginDigest || pluginDigest == "" {
			return nil, "", invalid("plugin_package_identity")
		}
	}
	return candidates, pluginID, nil
}

func validateActivePackageReplacement(current, candidates map[string]Definition, pluginID string) error {
	for connectorID, existing := range current {
		if existing.PluginID != pluginID {
			continue
		}
		candidate, exists := candidates[connectorID]
		if !exists {
			return incompatible("connector_removed", connectorID)
		}
		if err := CheckCompatibleUpdate(existing, candidate); err != nil {
			return err
		}
	}
	for connectorID, candidate := range candidates {
		if existing, exists := current[connectorID]; exists && existing.PluginID != candidate.PluginID {
			return fmt.Errorf("%w: connector_id", ErrDuplicateConnector)
		}
	}
	return nil
}

func cloneDefinitionMap(source map[string]Definition) map[string]Definition {
	result := make(map[string]Definition, len(source))
	for connectorID, definition := range source {
		result[connectorID] = cloneDefinition(definition)
	}
	return result
}

func validateCapabilityTypes(values []CapabilityType) error {
	if len(values) == 0 {
		return invalid("capability_types")
	}
	seen := make(map[CapabilityType]struct{}, len(values))
	for _, value := range values {
		if value != CapabilityTypeAction && value != CapabilityTypeData {
			return invalid("capability_types")
		}
		if _, exists := seen[value]; exists {
			return invalid("capability_types")
		}
		seen[value] = struct{}{}
	}
	return nil
}

func hasAllFirstReleaseSurfaces(values []Surface) bool {
	if len(values) != len(FirstReleaseSurfaces()) {
		return false
	}
	seen := make(map[Surface]struct{}, len(values))
	for _, value := range values {
		if value != SurfaceChat && value != SurfaceAgent && value != SurfaceWorkflow {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return len(seen) == len(FirstReleaseSurfaces())
}

func validateKeyFields(fields []KeyField) error {
	if len(fields) == 0 {
		return invalid("key_fields")
	}
	required := false
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if !stableIdentifier(field.Name) || strings.TrimSpace(field.Label) == "" || !field.Secret {
			return invalid("key_fields")
		}
		if !validLocalizedText(field.LabelI18n) || !validLocalizedText(field.PlaceholderI18n) {
			return invalid("key_fields_i18n")
		}
		if _, exists := seen[field.Name]; exists {
			return invalid("key_fields")
		}
		seen[field.Name] = struct{}{}
		required = required || field.Required
	}
	if !required {
		return invalid("key_fields")
	}
	return nil
}

func validateOperations(operations []Operation) error {
	if len(operations) == 0 {
		return invalid("operations")
	}
	actionIDs := make(map[string]struct{}, len(operations))
	toolNames := make(map[string]struct{}, len(operations))
	for _, operation := range operations {
		if !stableIdentifier(operation.ID) || !stableIdentifier(operation.ToolName) || strings.TrimSpace(operation.Name) == "" {
			return invalid("operations")
		}
		if !validLocalizedText(operation.NameI18n) || !validLocalizedText(operation.DescriptionI18n) {
			return invalid("operations_i18n")
		}
		if operation.SkillID != "" && !stableIdentifier(operation.SkillID) {
			return invalid("operations")
		}
		if err := validateOptionalOperationContract(operation); err != nil {
			return err
		}
		if _, exists := actionIDs[operation.ID]; exists {
			return invalid("operations")
		}
		if _, exists := toolNames[operation.ToolName]; exists {
			return invalid("operations")
		}
		actionIDs[operation.ID] = struct{}{}
		toolNames[operation.ToolName] = struct{}{}
	}
	return nil
}

func stableIdentifier(value string) bool {
	return value == strings.TrimSpace(value) && stableIDPattern.MatchString(value)
}

func invalid(field string) error {
	return fmt.Errorf("%w: %s", ErrInvalidDefinition, field)
}

func cloneDefinition(definition Definition) Definition {
	cloned := definition
	cloned.CredentialFields = cloneCredentialFields(definition.CredentialFields)
	cloned.Setup = cloneSetup(definition.Setup)
	cloned.NameI18n = definition.NameI18n.Clone()
	cloned.DescriptionI18n = definition.DescriptionI18n.Clone()
	cloned.CapabilityTypes = append([]CapabilityType(nil), definition.CapabilityTypes...)
	cloned.Surfaces = append([]Surface(nil), definition.Surfaces...)
	cloned.KeyFields = append([]KeyField(nil), definition.KeyFields...)
	for index, field := range definition.KeyFields {
		cloned.KeyFields[index].LabelI18n = field.LabelI18n.Clone()
		cloned.KeyFields[index].PlaceholderI18n = field.PlaceholderI18n.Clone()
	}
	cloned.Operations = make([]Operation, len(definition.Operations))
	for index, operation := range definition.Operations {
		cloned.Operations[index] = operation
		cloned.Operations[index].RequiredScopes = cloneScopes(operation.RequiredScopes)
		cloned.Operations[index].NameI18n = operation.NameI18n.Clone()
		cloned.Operations[index].DescriptionI18n = operation.DescriptionI18n.Clone()
		cloned.Operations[index].InputSchema = cloneJSONObject(operation.InputSchema)
		cloned.Operations[index].OutputSchema = cloneJSONObject(operation.OutputSchema)
	}
	return cloned
}

func cloneJSONObject(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(value))
	for key, item := range value {
		cloned[key] = cloneJSONValue(item)
	}
	return cloned
}

func cloneJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneJSONObject(typed)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = cloneJSONValue(item)
		}
		return cloned
	default:
		return typed
	}
}
