package catalog

import (
	"errors"
	"strings"
	"testing"
)

func TestCompatibleDefinitionUpdatePreservesAcceptedContracts(t *testing.T) {
	t.Parallel()
	current := pluginContractDefinitionFixture()
	candidate := cloneDefinition(current)
	candidate.PluginVersion = "1.1.0"
	candidate.PluginDigest = strings.Repeat("b", 64)
	candidate.Name = "Example Cloud"
	candidate.Description = "Updated member-facing copy"
	candidate.Icon = "updated-icon"
	candidate.Operations[0].Name = "List current items"
	candidate.Operations[0].Description = "Updated operation copy"
	candidate.CapabilityTypes = append(candidate.CapabilityTypes, CapabilityTypeData)
	candidate.Operations = append(candidate.Operations, additiveOperationFixture())

	if err := CheckCompatibleUpdate(current, candidate); err != nil {
		t.Fatalf("CheckCompatibleUpdate() error = %v", err)
	}
}

func TestCompatibleDefinitionUpdateAdmitsAdditiveDigitalHumanSurface(t *testing.T) {
	t.Parallel()
	current := pluginContractDefinitionFixture()
	candidate, err := WithDigitalHumanSurface(current, "1.1.0-digitalhuman.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckCompatibleUpdate(current, candidate); err != nil {
		t.Fatalf("additive digital human surface rejected: %v", err)
	}

	// The host surface is additive and one-way: an admitted digital human
	// contract cannot be reverted to the plain Connector surface set.
	if err := CheckCompatibleUpdate(candidate, current); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("digital human surface removal = %v, want incompatible update", err)
	}
}

func TestCompatibleDefinitionUpdatePreservesCredentialDialectForDigitalHumanSurface(t *testing.T) {
	t.Parallel()
	current, err := FeishuDefinition()
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := WithDigitalHumanSurface(current, current.PluginVersion+"-digitalhuman.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckCompatibleUpdate(current, candidate); err != nil {
		t.Fatalf("credential digital human projection rejected: %v", err)
	}

	mismatched := cloneDefinition(candidate)
	mismatched.CredentialFields[0].MaxLength++
	if err := CheckCompatibleUpdate(current, mismatched); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("credential field change = %v, want incompatible update", err)
	}
}

func TestDefinitionUpdateBlocksIdentitySchemaEffectExposureAndRiskChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{name: "Connector ID", mutate: func(value *Definition) { value.ID = "different" }},
		{name: "plugin ID", mutate: func(value *Definition) { value.PluginID = "different.plugin" }},
		{name: "provider ID", mutate: func(value *Definition) { value.ProviderID = "different" }},
		{name: "driver ID", mutate: func(value *Definition) { value.DriverID = "different-rest" }},
		{name: "auth method ID", mutate: func(value *Definition) { value.AuthMethodID = "different_key" }},
		{name: "classification", mutate: func(value *Definition) { value.Classification = ClassificationDataSource }},
		{name: "key schema", mutate: func(value *Definition) { value.KeyFields[0].Name = "token" }},
		{name: "operation removal", mutate: func(value *Definition) { value.Operations = nil }},
		{name: "tool identity", mutate: func(value *Definition) { value.Operations[0].ToolName = "changed_tool" }},
		{name: "Skill identity", mutate: func(value *Definition) { value.Operations[0].SkillID = "changed-skill" }},
		{name: "input schema", mutate: func(value *Definition) {
			value.Operations[0].InputSchema["required"] = []interface{}{"query"}
		}},
		{name: "output schema", mutate: func(value *Definition) {
			value.Operations[0].OutputSchema["required"] = []interface{}{"items"}
		}},
		{name: "external effect", mutate: func(value *Definition) { value.Operations[0].Effect = OperationEffectDelete }},
		{name: "data exposure", mutate: func(value *Definition) {
			value.Operations[0].DataEgress = true
			value.Operations[0].ExternalDestination = "api.example.com"
		}},
		{name: "sensitive data exposure", mutate: func(value *Definition) {
			value.Operations[0].DataEgress = true
			value.Operations[0].ExternalDestination = "api.example.com"
			value.Operations[0].SensitiveDataAllowed = true
		}},
		{name: "risk hint", mutate: func(value *Definition) { value.Operations[0].RiskHint = OperationRiskHintHigh }},
		{name: "idempotency", mutate: func(value *Definition) { value.Operations[0].Idempotent = false }},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			current := pluginContractDefinitionFixture()
			candidate := cloneDefinition(current)
			candidate.PluginVersion = "2.0.0"
			candidate.PluginDigest = strings.Repeat("b", 64)
			testCase.mutate(&candidate)
			if err := CheckCompatibleUpdate(current, candidate); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
				t.Fatalf("CheckCompatibleUpdate() error = %v, want incompatible update", err)
			}
		})
	}
}

func TestDefinitionUpdateBlocksSameVersionContentReplacement(t *testing.T) {
	t.Parallel()
	current := pluginContractDefinitionFixture()
	candidate := cloneDefinition(current)
	candidate.PluginDigest = strings.Repeat("b", 64)
	if err := CheckCompatibleUpdate(current, candidate); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("CheckCompatibleUpdate() error = %v, want immutable version failure", err)
	}
}

func TestDefinitionUpdateDoesNotTrustPluginRiskDowngrade(t *testing.T) {
	t.Parallel()
	current := pluginContractDefinitionFixture()
	current.Operations[0].RiskHint = OperationRiskHintHigh
	candidate := cloneDefinition(current)
	candidate.PluginVersion = "1.1.0"
	candidate.PluginDigest = strings.Repeat("b", 64)
	candidate.Operations[0].RiskHint = OperationRiskHintLow
	if err := CheckCompatibleUpdate(current, candidate); !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("CheckCompatibleUpdate() error = %v, want risk downgrade blocked", err)
	}
}

func TestDefinitionContractDigestIsCanonicalAndExcludesDisplayCopy(t *testing.T) {
	t.Parallel()
	first := pluginContractDefinitionFixture()
	first.CapabilityTypes = []CapabilityType{CapabilityTypeAction, CapabilityTypeData}
	first.KeyFields = append(first.KeyFields, KeyField{Name: "endpoint", Label: "Endpoint", Required: false, Secret: true})
	first.Operations = append(first.Operations, additiveOperationFixture())
	second := cloneDefinition(first)
	second.Name = "Renamed"
	second.NameI18n = LocalizedText{"en-US": "Renamed", "zh-Hans": "已重命名"}
	second.Description = "New description"
	second.DescriptionI18n = LocalizedText{"en-US": "New description", "zh-Hans": "新描述"}
	second.Icon = "new-icon"
	second.CapabilityTypes[0], second.CapabilityTypes[1] = second.CapabilityTypes[1], second.CapabilityTypes[0]
	second.Surfaces[0], second.Surfaces[2] = second.Surfaces[2], second.Surfaces[0]
	second.KeyFields[0], second.KeyFields[1] = second.KeyFields[1], second.KeyFields[0]
	second.Operations[0], second.Operations[1] = second.Operations[1], second.Operations[0]
	for index := range second.Operations {
		second.Operations[index].Name = "Updated display name"
		second.Operations[index].NameI18n = LocalizedText{"en-US": "Updated display name", "zh-Hans": "更新后的显示名"}
		second.Operations[index].Description = "Updated operation description"
		second.Operations[index].DescriptionI18n = LocalizedText{"en-US": "Updated operation description", "zh-Hans": "更新后的操作描述"}
	}

	left, err := DefinitionContractDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := DefinitionContractDigest(second)
	if err != nil {
		t.Fatal(err)
	}
	if left != right || !sha256HexPattern.MatchString(left) {
		t.Fatalf("canonical digests = %q / %q", left, right)
	}
}

func pluginContractDefinitionFixture() Definition {
	definition := validDefinition()
	definition.PluginDigest = strings.Repeat("a", 64)
	definition.Operations[0].SkillID = "external-apps"
	definition.Operations[0].InputSchema = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{"type": "string"},
		},
		"additionalProperties": false,
	}
	definition.Operations[0].OutputSchema = map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"items": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
		},
		"additionalProperties": false,
	}
	definition.Operations[0].Effect = OperationEffectRead
	definition.Operations[0].RiskHint = OperationRiskHintLow
	definition.Operations[0].Idempotent = true
	return definition
}

func additiveOperationFixture() Operation {
	return Operation{
		ID: "example.item.search", ToolName: "search_example_items", SkillID: "external-apps",
		Name: "Search items", InputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{}, "additionalProperties": false,
		},
		OutputSchema: map[string]interface{}{
			"type": "object", "properties": map[string]interface{}{}, "additionalProperties": true,
		},
		Effect: OperationEffectRead, RiskHint: OperationRiskHintLow, Idempotent: true,
	}
}
