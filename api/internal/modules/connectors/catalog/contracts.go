package catalog

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	ErrIncompatibleDefinitionUpdate = errors.New("incompatible Connector definition update")
	sha256HexPattern                = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// OperationEffect is an untrusted plugin description of the external effect.
// It is frozen as contract evidence, but runtime safety is resolved only from
// platform-owned policy.
type OperationEffect string

const (
	OperationEffectRead         OperationEffect = "read"
	OperationEffectCreate       OperationEffect = "create"
	OperationEffectUpdate       OperationEffect = "update"
	OperationEffectDelete       OperationEffect = "delete"
	OperationEffectPublish      OperationEffect = "publish"
	OperationEffectInvoke       OperationEffect = "invoke"
	OperationEffectSchedule     OperationEffect = "schedule"
	OperationEffectExternalSend OperationEffect = "external_send"
)

func (effect OperationEffect) valid() bool {
	switch effect {
	case OperationEffectRead, OperationEffectCreate, OperationEffectUpdate,
		OperationEffectDelete, OperationEffectPublish, OperationEffectInvoke,
		OperationEffectSchedule, OperationEffectExternalSend:
		return true
	default:
		return false
	}
}

// OperationRiskHint is package metadata, not a trusted classification. A new
// operation remains unclassified until platform policy explicitly recognizes
// its exact Connector, operation, and tool contract.
type OperationRiskHint string

const (
	OperationRiskHintUnclassified OperationRiskHint = "unclassified"
	OperationRiskHintLow          OperationRiskHint = "low"
	OperationRiskHintMedium       OperationRiskHint = "medium"
	OperationRiskHintHigh         OperationRiskHint = "high"
	OperationRiskHintCritical     OperationRiskHint = "critical"
)

func (hint OperationRiskHint) valid() bool {
	switch hint {
	case OperationRiskHintUnclassified, OperationRiskHintLow, OperationRiskHintMedium,
		OperationRiskHintHigh, OperationRiskHintCritical:
		return true
	default:
		return false
	}
}

// ValidatePluginDefinition applies the stricter admission floor for a dynamic
// Agent Plugin. Built-in compatibility projections may omit the newer
// operation contract fields while they are being migrated, but a newly loaded
// plugin may not.
func ValidatePluginDefinition(definition Definition) error {
	if err := Validate(definition); err != nil {
		return err
	}
	if !sha256HexPattern.MatchString(definition.PluginDigest) {
		return invalid("plugin_digest")
	}
	for _, operation := range definition.Operations {
		if !operationContractPresent(operation) {
			return invalid("operation_contract")
		}
		if err := validateCompleteOperationContract(operation); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalOperationContract(operation Operation) error {
	if !operationContractPresent(operation) {
		return nil
	}
	return validateCompleteOperationContract(operation)
}

func operationContractPresent(operation Operation) bool {
	return operation.InputSchema != nil || operation.OutputSchema != nil || operation.Effect != "" ||
		operation.ExternalDestination != "" || operation.RiskHint != ""
}

func validateCompleteOperationContract(operation Operation) error {
	if operation.InputSchema == nil || operation.OutputSchema == nil || !operation.Effect.valid() || !operation.RiskHint.valid() {
		return invalid("operation_contract")
	}
	if operation.ExternalDestination != strings.TrimSpace(operation.ExternalDestination) {
		return invalid("operation_contract")
	}
	if operation.DataEgress {
		if operation.ExternalDestination == "" {
			return invalid("operation_contract")
		}
	} else if operation.ExternalDestination != "" || operation.SensitiveDataAllowed {
		return invalid("operation_contract")
	}
	if err := compileSchemaDocument(operation.InputSchema); err != nil {
		return invalid("operation_input_schema")
	}
	if err := compileSchemaDocument(operation.OutputSchema); err != nil {
		return invalid("operation_output_schema")
	}
	return nil
}

func compileSchemaDocument(document map[string]interface{}) error {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	resource := "urn:zgi:connector-operation-schema"
	if err := compiler.AddResource(resource, cloneJSONObject(document)); err != nil {
		return err
	}
	_, err := compiler.Compile(resource)
	return err
}

type frozenDefinitionContractV1 struct {
	ConnectorID    string                      `json:"connector_id"`
	PluginID       string                      `json:"plugin_id"`
	ProviderID     string                      `json:"provider_id"`
	DriverID       string                      `json:"driver_id"`
	AuthMethodID   string                      `json:"auth_method_id"`
	Classification Classification              `json:"classification"`
	Capability     []CapabilityType            `json:"capability_types"`
	SetupMethod    SetupMethod                 `json:"setup_method"`
	Ownership      Ownership                   `json:"ownership"`
	Surfaces       []Surface                   `json:"surfaces"`
	KeyFields      []frozenKeyFieldV1          `json:"key_fields"`
	Operations     []frozenOperationContractV1 `json:"operations"`
}

type frozenKeyFieldV1 struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Secret   bool   `json:"secret"`
}

type frozenOperationContractV1 struct {
	ID                   string                 `json:"id"`
	ToolName             string                 `json:"tool_name"`
	SkillID              string                 `json:"skill_id,omitempty"`
	InputSchema          map[string]interface{} `json:"input_schema,omitempty"`
	OutputSchema         map[string]interface{} `json:"output_schema,omitempty"`
	Effect               OperationEffect        `json:"effect,omitempty"`
	DataEgress           bool                   `json:"data_egress"`
	ExternalDestination  string                 `json:"external_destination,omitempty"`
	SensitiveDataAllowed bool                   `json:"sensitive_data_allowed"`
	RiskHint             OperationRiskHint      `json:"risk_hint,omitempty"`
	Idempotent           bool                   `json:"idempotent"`
}

// DefinitionContractDigest freezes stable identities, setup requirements, and
// operation behavior while intentionally excluding member-facing copy.
func DefinitionContractDigest(definition Definition) (string, error) {
	if err := Validate(definition); err != nil {
		return "", err
	}
	if definition.IsDigitalHumanContract() {
		return digestJSON(struct {
			Version  int `json:"version"`
			Contract any `json:"contract"`
		}{4, canonicalDefinitionContractV2(definition)})
	}
	if definition.IsAppCredentials() {
		return digestJSON(canonicalDefinitionContractV2(definition))
	}
	if definition.IsMailCredentials() {
		return digestJSON(struct {
			Version  int `json:"version"`
			Contract any `json:"contract"`
		}{definition.ContractVersion, canonicalDefinitionContractV2(definition)})
	}
	if definition.IsMessagingCredentials() {
		return digestJSON(struct {
			Version  int `json:"version"`
			Contract any `json:"contract"`
		}{3, canonicalDefinitionContractV2(definition)})
	}
	return digestJSON(canonicalDefinitionContract(definition))
}

func canonicalDefinitionContract(definition Definition) frozenDefinitionContractV1 {
	capabilities := append([]CapabilityType(nil), definition.CapabilityTypes...)
	sort.Slice(capabilities, func(left, right int) bool { return capabilities[left] < capabilities[right] })
	surfaces := append([]Surface(nil), definition.Surfaces...)
	sort.Slice(surfaces, func(left, right int) bool { return surfaces[left] < surfaces[right] })

	keyFields := make([]frozenKeyFieldV1, 0, len(definition.KeyFields))
	for _, field := range definition.KeyFields {
		keyFields = append(keyFields, frozenKeyFieldV1{Name: field.Name, Required: field.Required, Secret: field.Secret})
	}
	sort.Slice(keyFields, func(left, right int) bool { return keyFields[left].Name < keyFields[right].Name })

	operations := make([]frozenOperationContractV1, 0, len(definition.Operations))
	for _, operation := range definition.Operations {
		operations = append(operations, frozenOperationContractV1{
			ID: operation.ID, ToolName: operation.ToolName, SkillID: operation.SkillID,
			InputSchema: cloneJSONObject(operation.InputSchema), OutputSchema: cloneJSONObject(operation.OutputSchema),
			Effect: operation.Effect, DataEgress: operation.DataEgress,
			ExternalDestination: operation.ExternalDestination, SensitiveDataAllowed: operation.SensitiveDataAllowed,
			RiskHint: operation.RiskHint, Idempotent: operation.Idempotent,
		})
	}
	sort.Slice(operations, func(left, right int) bool { return operations[left].ID < operations[right].ID })

	return frozenDefinitionContractV1{
		ConnectorID: definition.ID, PluginID: definition.PluginID, ProviderID: definition.ProviderID,
		DriverID: definition.DriverID, AuthMethodID: definition.AuthMethodID,
		Classification: definition.Classification, Capability: capabilities,
		SetupMethod: definition.SetupMethod, Ownership: definition.Ownership,
		Surfaces: surfaces, KeyFields: keyFields, Operations: operations,
	}
}

// CheckCompatibleUpdate permits display changes and additive operations, but
// every previously accepted identity and operation contract must remain exact.
// The sole reviewed classification correction is bound to exact release and
// contract identities; it is not a general classification-change permission.
// The same declared plugin version is immutable and cannot point at new
// manifest content.
func CheckCompatibleUpdate(current, candidate Definition) error {
	if err := Validate(current); err != nil {
		return err
	}
	if err := Validate(candidate); err != nil {
		return fmt.Errorf("%w: invalid candidate definition: %v", ErrIncompatibleDefinitionUpdate, err)
	}
	if current.ID != candidate.ID || current.PluginID != candidate.PluginID ||
		current.ProviderID != candidate.ProviderID || current.DriverID != candidate.DriverID ||
		current.AuthMethodID != candidate.AuthMethodID || current.SourceDialect != candidate.SourceDialect ||
		current.SourceSchemaVersion != candidate.SourceSchemaVersion ||
		current.SetupMethod != candidate.SetupMethod || current.Ownership != candidate.Ownership {
		return incompatible("identity", "")
	}
	if current.Classification != candidate.Classification && !reviewedClassificationCorrection(current, candidate) {
		return incompatible("classification", "")
	}
	if current.UsesCredentials() || candidate.UsesCredentials() {
		left, leftErr := digestJSON(canonicalAuthentication(credentialBase(current)))
		right, rightErr := digestJSON(canonicalAuthentication(credentialBase(candidate)))
		if leftErr != nil || rightErr != nil || (left != right &&
			!reviewedFeishuTemplateReduction(current, candidate) && !reviewedFeishuTemplateExpansion(current, candidate) &&
			!reviewedServerOpsSSHPrivilegeExpansion(current, candidate)) {
			return incompatible("authentication_contract", "")
		}
	}
	if current.PluginVersion == candidate.PluginVersion && current.PluginDigest != "" &&
		candidate.PluginDigest != "" && current.PluginDigest != candidate.PluginDigest {
		return incompatible("plugin_version_content", "")
	}
	if !isSubsetCapabilities(current.CapabilityTypes, candidate.CapabilityTypes) ||
		(!sameSurfaces(current.Surfaces, candidate.Surfaces) && !additiveDigitalHumanSurface(current, candidate)) ||
		!sameKeyFieldContract(current.KeyFields, candidate.KeyFields) {
		return incompatible("connector_contract", "")
	}

	candidateOperations := make(map[string]Operation, len(candidate.Operations))
	for _, operation := range candidate.Operations {
		candidateOperations[operation.ID] = operation
	}
	for _, operation := range current.Operations {
		replacement, ok := candidateOperations[operation.ID]
		if !ok {
			return incompatible("operation_removed", operation.ID)
		}
		left, err := digestJSON(canonicalOperationContract(operation))
		if err != nil {
			return err
		}
		right, err := digestJSON(canonicalOperationContract(replacement))
		if err != nil {
			return err
		}
		if left != right && !reviewedServerOpsSSHSkillRelabel(current, operation, replacement) {
			return incompatible("operation_contract", operation.ID)
		}
		if current.UsesCredentials() {
			left, leftErr := digestJSON(canonicalPermissions(operation))
			right, rightErr := digestJSON(canonicalPermissions(replacement))
			if leftErr != nil || rightErr != nil || left != right {
				return incompatible("operation_permissions", operation.ID)
			}
		}
	}
	return nil
}

func canonicalOperationContract(operation Operation) frozenOperationContractV1 {
	return frozenOperationContractV1{
		ID: operation.ID, ToolName: operation.ToolName, SkillID: operation.SkillID,
		InputSchema: cloneJSONObject(operation.InputSchema), OutputSchema: cloneJSONObject(operation.OutputSchema),
		Effect: operation.Effect, DataEgress: operation.DataEgress,
		ExternalDestination: operation.ExternalDestination, SensitiveDataAllowed: operation.SensitiveDataAllowed,
		RiskHint: operation.RiskHint, Idempotent: operation.Idempotent,
	}
}

func isSubsetCapabilities(current, candidate []CapabilityType) bool {
	available := make(map[CapabilityType]struct{}, len(candidate))
	for _, value := range candidate {
		available[value] = struct{}{}
	}
	for _, value := range current {
		if _, ok := available[value]; !ok {
			return false
		}
	}
	return true
}

func sameSurfaces(left, right []Surface) bool {
	if len(left) != len(right) {
		return false
	}
	available := make(map[Surface]struct{}, len(right))
	for _, value := range right {
		available[value] = struct{}{}
	}
	for _, value := range left {
		if _, ok := available[value]; !ok {
			return false
		}
	}
	return true
}

func sameKeyFieldContract(left, right []KeyField) bool {
	if len(left) != len(right) {
		return false
	}
	fields := make(map[string]frozenKeyFieldV1, len(right))
	for _, value := range right {
		fields[value.Name] = frozenKeyFieldV1{Name: value.Name, Required: value.Required, Secret: value.Secret}
	}
	for _, value := range left {
		if fields[value.Name] != (frozenKeyFieldV1{Name: value.Name, Required: value.Required, Secret: value.Secret}) {
			return false
		}
	}
	return true
}

type incompatibleUpdateError struct {
	field       string
	operationID string
}

func (err incompatibleUpdateError) Error() string {
	if err.operationID == "" {
		return fmt.Sprintf("%s: %s", ErrIncompatibleDefinitionUpdate, err.field)
	}
	return fmt.Sprintf("%s: %s %s", ErrIncompatibleDefinitionUpdate, err.field, err.operationID)
}

func (err incompatibleUpdateError) Unwrap() error { return ErrIncompatibleDefinitionUpdate }

func incompatible(field, operationID string) error {
	return incompatibleUpdateError{field: field, operationID: operationID}
}
